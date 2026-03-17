package api

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"lan-im-go/core"
)

// 全局WebSocket协议升级器
// 必须显式配置Buffer大小
// 跨域校验（生产环境严禁直接return true）
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024, // 读缓冲
	WriteBufferSize: 1024, // 写缓冲
	// 跨域校验函数
	CheckOrigin: func(r *http.Request) bool {
		// 本地开发：放行所有
		return true
		// 生产环境：严格校验Origin（必须写！）
		// return strings.Contains(r.Header.Get("Origin"), "你的域名")
	},
}

// WsEndpoint Gin 路由处理函数
// 参数：hub 全局连接管理器 / c Gin上下文
func WsEndpoint(hub *core.Hub, c *gin.Context) {
	// ====================== 1. 身份鉴权（必填！）======================
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
		return
	}
	// 生产环境：解析Token、校验用户ID、绑定用户身份
	// userID := parseToken(token)

	// ====================== 2. HTTP 升级为 WebSocket 协议 ======================
	// 将 Gin 的响应流、请求 升级为长连接
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("协议升级失败: %v", err)
		return
		// ⚠️ 重点：Upgrade失败内部已返回HTTP错误，禁止再调用 c.JSON
	}

	// ====================== 3. 包装客户端对象 ======================
	client := &core.Client{

		Hub:  hub,
		Conn: conn,
		// 带缓冲通道：防止高并发阻塞（你之前学的channel最佳实践）
		//UserId: userID;
		Send: make(chan []byte, 256),
	}

	// 将客户端注册到全局Hub（管理所有在线连接）
	client.Hub.Register <- client

	// ====================== 4. 启动双协程 处理读写 ======================
	// 读写分离，两个独立goroutine
	// 释放Gin主协程，长连接交由子协程管理
	go client.WritePump() // 写协程：向客户端推送消息
	go client.ReadPump()  // 读协程：接收客户端消息
}
