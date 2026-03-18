package main

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"lan-im-go/api"
	"lan-im-go/core"
	"lan-im-go/infrastructure"
	"lan-im-go/middleware"
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
	// 生产环境中应切换为 gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	// 【防线 1：公共开放区】 (零信任边缘，任何人可访问)
	public := r.Group("/api/v1")
	{
		// 严苛规范：注册和登录是全系统唯二暴露在公网的无鉴权接口
		public.POST("/register", api.RegisterHandler)
		public.POST("/login", api.LoginHandler)
	}

	// 【防线 2：C 端用户鉴权区】 (核心业务区)
	authorized := r.Group("/api/v1")
	// 架构师的铁壁：只要进了这个 Group，必须带有合法的 JWT Token
	authorized.Use(middleware.JWTAuth())
	{
		// WebSocket 握手升级大门 (依赖注入了全局唯一 hub)
		authorized.GET("/ws", func(c *gin.Context) {
			api.WsEndpoint(hub, c)
		})

		// 断点续传与文件流转体系
		authorized.GET("/upload/status", api.CheckUploadStatus) // 战损探针
		authorized.POST("/upload/chunk", api.UploadChunk)       // 分片上传
		authorized.POST("/upload/merge", api.MergeChunks)       // 物理合并

		// 极速零拷贝下载 (注意：实际业务中下载可能不需要鉴权以方便分享，视产品需求而定。严苛起见，我们这里放在鉴权区)
		authorized.GET("/download/:filename", api.DownloadFile)
	}

	// 【防线 3：B 端超管绝对隔离区】 (最高权限区)
	admin := r.Group("/api/v1/admin")
	// 防雷：中间件的挂载顺序绝对不能反！
	// 必须先经过 JWTAuth 解析出身份并塞入 Context，接着才能让 SuperAdminOnly 去 Context 里查 Role！
	admin.Use(middleware.JWTAuth(), middleware.SuperAdminOnly())
	{
		// 依赖注入 (Dependency Injection)：
		// 将 hub 指针传入闭包，让 HTTP 协程能够跨界操控底层的 WebSocket 引擎进行“物理击杀”和“系统广播”
		admin.DELETE("/users/:id", api.AdminDeleteUser(hub))
		admin.DELETE("/rooms/:id", api.AdminDeleteRoom(hub))

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
