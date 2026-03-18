package core

import (
	"encoding/json"
	"log"
	"time"

	"github.com/gorilla/websocket"
	"lan-im-go/models"
)

const (
	// 工业级 WebSocket 调优参数
	writeWait      = 10 * time.Second    // 写入超时时间
	pongWait       = 60 * time.Second    // 期待前端心跳回应的超时时间
	pingPeriod     = (pongWait * 9) / 10 // 服务端发送心跳包的频率 (必须小于 pongWait)
	maxMessageSize = 4096                // 严防死守：限制单条消息最大体积(4KB)，防止恶意构造超大包打爆内存
)

// Client 物理连接包装
type Client struct {
	Hub    *Hub
	UserID int64
	Conn   *websocket.Conn
	// 发送缓冲通道：类型为 []byte 极大节省 CPU
	Send chan []byte
}

// Subscription 动作载体
type Subscription struct {
	Client  *Client
	RoomIDs []int64 // 该操作涉及的房间集合
}

// ReadPump 读泵：将前端发来的二进制/文本流，反序列化并灌入 Hub 引擎
// 严苛要求：每个连接有且仅有一个 goroutine 运行 ReadPump
func (c *Client) ReadPump() {
	// 物理崩溃/下线时的最终兜底方案
	defer func() {
		c.Hub.Unsubscribe <- &Subscription{Client: c, RoomIDs: nil} // 交给 Hub 去清理路由和关闭 Conn
		c.Conn.Close()
	}()

	// 配置底层的读取限制与心跳感知
	c.Conn.SetReadLimit(maxMessageSize)
	c.Conn.SetReadDeadline(time.Now().Add(pongWait))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[ReadPump 异常] 用户 %d 连接异常断开: %v", c.UserID, err)
			}
			break // 退出循环，触发 defer 销毁
		}

		// 解析前端发来的业务 JSON
		var msg models.Message
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("[防爆破] 用户 %d 发送了非法的 JSON 载荷", c.UserID)
			continue
		}

		// 零信任安全：绝对不能相信前端传过来的 SenderID！
		// 必须在后端用 JWT 鉴权通过后的 Client.UserID 强行覆盖，防止冒名顶替！
		msg.SenderID = c.UserID

		// 压入 Hub 进行并发路由
		c.Hub.Broadcast <- &msg
	}
}

// WritePump 写泵：从 Hub 引擎接收 []byte，并发往网卡缓冲区
// 严苛要求：WebSocket 的 Write 操作不支持并发，必须被严格收敛在 WritePump 这一个协程里！
func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// Hub 主动关闭了 Send 通道 (比如被踢下线)
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// 直接将拿到的字节流写入 TCP
			w, err := c.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// 缓冲合并写入 (如果 Send 管道里积压了多条消息，一次性合并写入网卡，极大降低系统调用开销)
			n := len(c.Send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'}) // 可以用换行符分割
				w.Write(<-c.Send)
			}

			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			// 定时发送 Ping 心跳包保活，防止被 Nginx 或运营商中间件切断 TCP 状态机
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
