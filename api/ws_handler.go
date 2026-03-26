package api

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"lan-im-go/core"
	"lan-im-go/models"
	"lan-im-go/repository"
	"log"
	"net/http"
	"time"
)

// WebSocket协议升级器
var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	// 跨域校验，生产环境需配置合法域名，防止CSRF攻击
	CheckOrigin: func(r *http.Request) bool {
		// 生产环境替换为正式域名校验
		// return strings.Contains(r.Header.Get("Origin"), "yourdomain.com")
		return true // 开发环境放行跨域
	},
}

// WsEndpoint WebSocket连接入口
// 路由：authorized.GET("/ws", api.WsEndpoint(hub))
// 前端连接地址：ws://ip:port/api/v1/ws?token=JWT令牌
func WsEndpoint(hub *core.Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. 身份验证：从Gin上下文获取用户ID（由JWT中间件校验通过）
		userID, exists := c.Get("user_id")
		if !exists {
			log.Printf("用户身份信息不存在\n")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "身份验证失败，连接拒绝"})
			return
		}
		realUserID := userID.(int64)

		// 2. 协议升级：将HTTP协议升级为WebSocket全双工协议
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			log.Printf("[连接失败] WebSocket协议升级异常 UID:%d, Err:%v", realUserID, err)
			return
		}
		log.Printf("WebSocket连接建立成功\n")

		// 3. 初始化群聊订阅：查询用户已加入的群聊列表
		roomIDs, err := repository.RoomMember.GetUserRoomIDs(realUserID)
		if err != nil {
			log.Printf("[连接警告] 获取用户%d群聊列表失败，使用空列表初始化", realUserID)
			roomIDs = []int64{}
		}

		// 4. 创建客户端实例，初始化消息发送通道
		client := &core.Client{
			Hub:    hub,
			UserID: realUserID,
			Conn:   conn,
			Send:   make(chan []byte, 256), // 缓冲通道，防止高并发阻塞
		}

		// 构建订阅信息，注册客户端到Hub
		subscription := &core.Subscription{
			Client:  client,
			RoomIDs: roomIDs,
		}
		hub.Subscribe <- subscription

		// 延迟执行：连接断开时注销客户端并关闭连接，防止资源泄漏
		defer func() {
			hub.Unsubscribe <- subscription
			conn.Close()
			log.Printf("[WebSocket] 用户%d连接已释放", realUserID)
		}()

		// 启动消息读写协程
		go client.ReadPump()
		go client.WritePump()

		// 消息读取循环：监听前端发送的消息
		for {
			_, rawMsg, err := conn.ReadMessage()
			if err != nil {
				// 连接异常断开，退出循环
				break
			}

			// 解析消息数据
			var payload struct {
				RoomID  int64  `json:"room_id"`
				Content string `json:"content"`
			}
			if err := json.Unmarshal(rawMsg, &payload); err != nil {
				log.Printf("[WebSocket] 消息格式解析失败: %v", err)
				continue
			}

			// 构建消息实体，用户ID从上下文获取，禁止前端传入
			msg := &models.Message{
				RoomID:    payload.RoomID,
				SenderID:  realUserID,
				Content:   payload.Content,
				CreatedAt: time.Now(),
				Type:      1,
			}

			// 将消息发送至Hub，由Hub统一广播至群内所有在线用户
			hub.Broadcast <- msg
			log.Printf("[消息中心] 用户%d向群聊%d发送消息，已完成广播", realUserID, payload.RoomID)
		}
	}
}
