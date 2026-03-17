package main

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"

	"lan-im-go/api"
	"lan-im-go/core"
	"lan-im-go/infrastructure"
)

func main() {
	// 1. 获取环境变量中的 DSN (还记得我们在 docker-compose.yml 里配的吗？)
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		// 本地 fallback (仅供脱离 docker 调试时使用)
		dsn = "root:123456@tcp(127.0.0.1:3306)/lan_im?charset=utf8mb4&parseTime=True&loc=Local"
	}

	// 2. 触发建表和连接池初始化！
	infrastructure.InitDatabase(dsn)

	// 3. 启动你的 Hub 引擎
	hub := core.NewHub()
	// 这里可以把 infrastructure.DB 注入给 hub 里面的 repository...
	go hub.Run()

	// 4. 启动 Gin 路由
	r := gin.Default()
	r.GET("/ws", func(c *gin.Context) {
		api.WsEndpoint(hub, c)
	})

	log.Println("LAN-IM 服务端启动于 :8080")
	r.Run(":8080")
}
