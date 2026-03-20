package core

import (
	"encoding/json"
	"log"
	"time"

	"github.com/gorilla/websocket"
	"lan-im-go/models"
)

const (
	// WebSocket 配置参数
	writeWait      = 10 * time.Second    // 写入超时时间
	pongWait       = 60 * time.Second    // 客户端心跳响应超时时间
	pingPeriod     = (pongWait * 9) / 10 // 服务端心跳发送频率
	maxMessageSize = 4096                // 限制单条消息最大长度，防止超大消息占用过多内存
)

// Client 客户端连接实体
type Client struct {
	Hub    *Hub
	UserID int64
	Conn   *websocket.Conn
	// 消息发送缓冲通道，使用字节数组提升性能
	Send chan []byte
}

// Subscription 订阅信息
type Subscription struct {
	Client  *Client
	RoomIDs []int64 // 操作关联的群聊集合
}

// ReadPump 读取消息：接收客户端消息，解析后发送至消息中心
// 每个客户端连接仅启动一个协程执行该方法
func (c *Client) ReadPump() {
	// 连接关闭时释放资源
	defer func() {
		c.Hub.Unsubscribe <- &Subscription{Client: c, RoomIDs: nil} // 由Hub注销客户端并清理连接
		c.Conn.Close()
	}()

	// 设置消息读取限制和心跳处理
	c.Conn.SetReadLimit(maxMessageSize)
	c.Conn.SetReadDeadline(time.Now().Add(pongWait))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.Conn.ReadMessage()
		log.Printf("收到客户端消息：%s\n", message)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[消息读取异常] 用户 %d 连接异常断开: %v", c.UserID, err)
			}
			break
		}

		// 解析客户端消息
		var payload struct {
			RoomID  int64  `json:"room_id"`
			Content string `json:"content"`
		}
		var msg models.Message
		if err := json.Unmarshal(message, &payload); err != nil {
			log.Printf("[消息解析失败] 用户 %d 发送了非法的 JSON 格式消息", c.UserID)
			continue
		}
		// 安全校验：用户ID从服务端获取，禁止客户端伪造身份
		msg.SenderID = c.UserID
		msg.Content = payload.Content
		msg.CreatedAt = time.Now()
		msg.Type = 1
		msg.RoomID = payload.RoomID

		// 发送至消息中心进行广播
		c.Hub.Broadcast <- &msg
	}
}

// WritePump 发送消息：从消息中心接收数据并发送给客户端
// WebSocket写入操作非并发安全，仅允许单个协程执行
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
				// 消息通道已关闭，断开客户端连接
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// 写入消息数据
			w, err := c.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// 批量写入优化：合并积压消息，减少系统IO调用
			n := len(c.Send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.Send)
			}

			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			// 定时发送心跳包，维持连接存活
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
