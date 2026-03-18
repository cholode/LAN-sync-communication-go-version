package main

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"

	"lan-im-go/api"
	"lan-im-go/core"
	"lan-im-go/infrastructure"
	"lan-im-go/repository"
)

func main() {
	// ========================================================================
	// Phase 1: 环境与基础设施初始化 (Infrastructure & Config)
	// ========================================================================
	// 从环境变量读取 DSN，如果为空则使用本地兜底配置 (应对脱离 Docker 的本地调试)
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		dsn = "root:123456@tcp(127.0.0.1:3306)/lan_im?charset=utf8mb4&parseTime=True&loc=Local"
		log.Println("[警告] 未检测到 DB_DSN 环境变量，正在使用本地默认配置连接 MySQL")
	}

	// 初始化数据库连接池并执行 AutoMigrate 建表
	// 严苛要求：如果这里连不上数据库，程序必须直接 panic/fatal 退出，绝对不能带病启动！
	infrastructure.InitDatabase(dsn)

	// ========================================================================
	// Phase 2: 领域层与持久层装配 (Repository Injection)
	// ========================================================================
	// 将底层的 MySQL 物理连接池注入给 Repository 防腐层
	// 从此刻起，所有的业务逻辑只能通过 repository.User / repository.Room 等单例来查库
	repository.InitRepositories(infrastructure.DB)
	log.Println("[就绪] Repository 领域数据层装载完毕")

	// ========================================================================
	// Phase 3: 核心通信引擎启动 (Core Engine)
	// ========================================================================
	// 实例化全局唯一的心脏枢纽
	hub := core.NewHub()
	// 启动单线程大循环，开始监听 Register, Unregister, Broadcast 等通道
	go hub.Run()
	log.Println("[就绪] WebSocket Hub 核心路由引擎已启动")

	// ========================================================================
	// Phase 4: API 路由接入层 (HTTP/WS Router)
	// ========================================================================
	// 生产环境中应切换为 gin.ReleaseMode
	r := gin.Default()

	// 1. 公共接口 (无需 Token)
	public := r.Group("/api/v1")
	{
		// 严苛预留：稍后我们将在这里实现真正的登录与注册
		// public.POST("/login", api.LoginHandler)
		// public.POST("/register", api.RegisterHandler)
	}

	// 2. C端用户鉴权接口 (需要 JWT Token)
	// 注意：这里的 middleware.JWTAuth() 我们稍后实现
	authorized := r.Group("/api/v1")
	// authorized.Use(middleware.JWTAuth())
	{
		// WebSocket 握手升级大门 (注入了全局唯一 hub)
		authorized.GET("/ws", func(c *gin.Context) {
			api.WsEndpoint(hub, c)
		})
		// 文件上传/下载入口
		authorized.POST("/upload", api.UploadFile(hub))
		authorized.GET("/download/:filename", api.Downloadfile())
	}

	// 3. B端超管隔离区 (需要双重鉴权：JWT + Role==1)
	admin := r.Group("/api/v1/admin")
	// admin.Use(middleware.JWTAuth(), middleware.SuperAdminOnly())
	{
		// 严苛预留：超管的管控接口
		// admin.DELETE("/users/:id", api.AdminDeleteUser)
		// admin.DELETE("/rooms/:id", api.AdminDeleteRoom)
	}

	// ========================================================================
	// Phase 5: 点火起飞 (Start Server)
	// ========================================================================
	port := os.Getenv("SERVER_PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("🚀 LAN-IM 服务端启动成功，正在监听端口 :%s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("[致命错误] Gin HTTP 引擎启动失败: %v", err)
	}
}
