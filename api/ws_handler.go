package api

import (
	"log"
	"net/http"

	"encoding/json"
	"github.com/bwmarrin/snowflake"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"lan-im-go/core"
	"lan-im-go/models"
	"lan-im-go/repository"
)

// 全局 WebSocket 协议升级器
var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096, // 扩容到 4KB，与 client.go 中的 maxMessageSize 保持一致
	WriteBufferSize: 4096,
	// 跨域校验函数 (架构师红线：生产环境必须校验 Origin，防止 CSRF 劫持 WebSocket 握手)
	CheckOrigin: func(r *http.Request) bool {
		// return strings.Contains(r.Header.Get("Origin"), "yourdomain.com")
		return true // 本地开发暂且放行
	},
}

// WsEndpoint 长连接接入大门
// 路由挂载建议: authorized.GET("/ws", api.WsEndpoint(hub))
// 注意：前端连接时 url 必须是 ws://ip:port/api/v1/ws?token=你的JWT
func WsEndpoint(hub *core.Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		// ====================================================================
		// 1. 终极身份核验 (复用 Gin Context)
		// ====================================================================
		// 由于这个接口被 middleware.JWTAuth() 保护，走到这里时，
		// 用户的真实身份绝对已经被安全地注入到了 Context 中！

		userID, exists := c.Get("user_id")
		if !exists {
			log.Printf("用户不存在\n")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "系统上下文身份丢失，拒绝握手"})
			return
		}
		realUserID := userID.(int64)

		// ====================================================================
		// 2. 协议升级 (HTTP -> TCP WebSocket)
		// ====================================================================
		// 这一步之后，不再受 HTTP 短连接的约束，进入全双工长连接状态
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			log.Printf("[握手失败] 协议升级异常 UID:%d, Err:%v", realUserID, err)
			return // Upgrade 失败时底层会自动写入 HTTP 错误响应，不要再 c.JSON 了
		}
		log.Printf("握手成功\n")
		// ====================================================================
		// 3. 构建初始内存路由表 (核心状态同步)
		// ====================================================================
		// 架构师绝杀：在连接刚建立的这一刻，去数据库查出他所有加入的群聊！
		// 这样 Hub 在初始化他的内存节点时，就能一次性把他挂载到正确的路由树上。
		roomIDs, err := repository.RoomMember.GetUserRoomIDs(realUserID)
		if err != nil {
			log.Printf("[握手警告] 无法拉取用户 %d 的群聊列表，暂以空列表初始化", realUserID)
			roomIDs = []int64{}
		}

		// ====================================================================
		// 4. 组装物理连接与订阅载体
		// ====================================================================
		client := &core.Client{
			Hub:    hub,
			UserID: realUserID,
			Conn:   conn,
			// 工业级缓冲：防止高并发时 Send 管道瞬间被打满导致假死
			Send: make(chan []byte, 256),
		}

		// 构建融合了上下线与动态路由的订阅动作载体
		subscription := &core.Subscription{
			Client:  client,
			RoomIDs: roomIDs,
		}

		// 将其推入 Hub 的单线程大循环，进行无锁化挂载
		hub.Subscribe <- subscription

		///--------------------------------------------------------------------
		// ==========================================================
		// 前面的代码：升级协议 conn, err := upgrader.Upgrade(...)
		// 注册进 Hub：hub.Subscribe <- sub
		// ==========================================================
		// 架构师的防御编程：当死循环被打破（客户端断开）时，必须清理内存，防止 Goroutine 泄露！
		go client.WritePump()
		go client.ReadPump()

		defer func() {
			// 注销该用户的路由节点 (假设你的 hub 有注销通道)
			hub.Unsubscribe <- subscription
			conn.Close()
			log.Printf("[WebSocket] 用户 %d 的物理连接已彻底释放", realUserID)
		}()

		// 开启读泵 (Read Pump)：全双工持续监听前端发来的二进制流
		for {
			_, rawMsg, err := conn.ReadMessage()
			if err != nil {
				// 正常断开或网络异常，跳出死循环，触发 defer 清理
				break
			}

			// 1. 解析前端传来的载荷 (对应我们前端测试台发来的 JSON)
			var payload struct {
				RoomID  int64  `json:"room_id"`
				Content string `json:"content"`
			}
			if err := json.Unmarshal(rawMsg, &payload); err != nil {
				log.Printf("[WebSocket 警告] 收到畸形消息: %v", err)
				continue
			}

			node, _ := snowflake.NewNode(1)
			msgID := node.Generate().Int64()
			// 2. 组装标准消息结构体
			// ⚡ 绝命红线 1 (零信任原则)：
			// 发送者 ID 绝对、绝对、绝对不能从前端的 JSON 里取！
			// 必须用我们刚刚在 JWT 中间件里解出来的 realUserID，彻底杜绝越权伪造身份！
			msg := &models.Message{
				ID:       msgID,
				RoomID:   payload.RoomID,
				SenderID: realUserID,
				Content:  payload.Content,
				// Type 等其他字段视你的表结构而定
			}

			// ⚡ 绝命红线 2 (异步持久化)：
			// 绝对不能在这里同步调用 db.Create(&msg)！
			// 如果数据库 I/O 发生哪怕 50 毫秒的抖动，整个 WebSocket 的读循环就会卡死，导致消息积压。
			// 必须开辟新的协程异步落盘（大厂甚至会在这里把消息丢进 Kafka）
			go func(m *models.Message) {
				// 直接接收接口返回的 error！绝不能带 .Error！
				if err := repository.Message.SaveMessage(m); err != nil {
					log.Printf("[持久化致命错误] 消息落盘失败, RoomID: %d, SenderID: %d, Err: %v", m.RoomID, m.SenderID, err)
					// 注意：这里是异步协程，千万不要在这里 c.JSON() 或 return，没有意义
				} else {
					log.Printf("[持久化成功] 消息已安全落入物理硬盘 (MsgID: %d)", m.ID)
				}
			}(msg)

			// ⚡ 绝命红线 3 (内存级广播转发)：
			// 将消息丢进 Hub 的全局广播通道。
			// Hub 的大循环一旦监听到这个通道有数据，就会瞬间遍历该 RoomID 下所有的活跃连接，并推送到他们的机器上！
			// 如果你的 Hub 设计了 Broadcast 通道，就在这里调用：
			hub.Broadcast <- msg
			log.Printf("[Hub 引擎] 收到来自用户 %d 往房间 %d 发送的载荷，已交由引擎广播", realUserID, payload.RoomID)
		}

		// ====================================================================
		// 5. 启动读写分离双泵 (CSP 并发模型起飞)
		// ====================================================================
		// 释放 Gin 的 HTTP 主协程，将 TCP 句柄彻底交接给这两个常驻子协程
	}
}
