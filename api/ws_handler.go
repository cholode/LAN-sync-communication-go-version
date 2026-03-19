package api

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"lan-im-go/core"
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

		// ====================================================================
		// 5. 启动读写分离双泵 (CSP 并发模型起飞)
		// ====================================================================
		// 释放 Gin 的 HTTP 主协程，将 TCP 句柄彻底交接给这两个常驻子协程
		go client.WritePump()
		go client.ReadPump()
	}
}
