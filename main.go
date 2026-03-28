package main

import (
	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	//"io"
	"lan-im-go/api"
	"lan-im-go/core"
	"lan-im-go/infrastructure"
	"lan-im-go/middleware"
	"lan-im-go/repository"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {

	//  单独启动一个 goroutine 监听 6060 端口（不影响主业务）
	go func() {
		// 地址：0.0.0.0:6060 允许外部/宿主机访问
		err := http.ListenAndServe("0.0.0.0:6060", nil)
		if err != nil {
			panic("pprof start failed: " + err.Error())
		}
	}()
	//log.SetOutput(io.Discard)
	// ========================================================================
	// 阶段1：环境与基础设施初始化
	// ========================================================================
	// 从环境变量读取数据库配置，为空时使用本地默认配置（适配本地调试）
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		dsn = "root:123456@tcp(127.0.0.1:3306)/lan_im?charset=utf8mb4&parseTime=True&loc=Local"
		log.Println("[警告] 未检测到DB_DSN环境变量，使用本地默认配置连接MySQL")
	}

	// 初始化数据库连接池并自动同步表结构
	// 数据库连接失败时程序直接终止，保证服务启动完整性
	infrastructure.InitDatabase(dsn)
	api.InitFileDirs()
	// ========================================================================
	// 阶段2：数据访问层初始化
	// ========================================================================
	// 注入数据库连接实例到数据访问层
	// 业务逻辑统一通过数据访问层接口操作数据库
	repository.InitRepositories(infrastructure.DB)
	log.Println("[就绪] 数据访问层初始化完成")

	// ========================================================================
	// 阶段3：WebSocket核心引擎启动
	// ========================================================================
	// 创建全局WebSocket路由中心
	hub := core.NewHub()
	// 启动引擎监听调度通道
	go hub.Run()
	log.Println("[就绪] WebSocket核心引擎启动完成")

	// ========================================================================
	// 阶段4：HTTP服务与路由配置
	// ========================================================================
	// 开发环境使用默认模式，生产环境建议切换为发布模式
	//r := gin.Default()
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	pprof.Register(r)
	// ========================================================================
	// 跨域配置（需在路由注册前配置）
	// ========================================================================
	r.Use(cors.New(cors.Config{
		AllowAllOrigins:  true, // 开发环境允许所有域名，生产环境需配置指定前端域名
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour, // 预检请求缓存时长，减少重复请求
	}))

	// ========================================================================
	// 路由分组配置
	// ========================================================================

	// 公共路由组：无需身份验证
	public := r.Group("/api/v1")
	{
		// 开放接口：用户注册、登录
		public.POST("/register", api.RegisterHandler)
		public.POST("/login", api.LoginHandler)
		// 文件下载：路径中含整文件 SHA-256 前缀，视为能力链接；群成员无需在 URL 中带各自 JWT 即可在浏览器中打开下载
		public.GET("/download/*filepath", api.DownloadFile)
	}

	// 鉴权路由组：需JWT身份验证
	authorized := r.Group("/api/v1")
	// 注册JWT身份验证中间件
	authorized.Use(middleware.JWTAuth())
	{
		// WebSocket连接入口
		log.Printf("进入WebSocket连接配置\n")
		authorized.GET("/ws", func(c *gin.Context) {
			api.WsEndpoint(hub)(c)
		})

		// 文件上传相关接口
		authorized.GET("/upload/status", api.CheckUploadStatus)
		authorized.POST("/upload/chunk", api.UploadChunk)
		authorized.POST("/upload/merge", api.MergeChunks)
		// 群聊相关接口
		authorized.GET("/rooms/:id/messages", api.GetChatHistory())
		authorized.POST("/rooms/:id/join", api.JoinRoom(hub))
		authorized.GET("/rooms/:id/members", api.GetRoomMembers())
		authorized.DELETE("/rooms/:id/members/:user_id", api.RemoveRoomMember(hub))
		authorized.DELETE("/upload/cancel", api.CancelUpload)
		// 群聊管理接口
		authorized.POST("/rooms", api.CreateRoom(hub))
		authorized.GET("/my_rooms", api.GetMyRooms())
	}

	// 管理员路由组：需管理员权限
	admin := r.Group("/api/v1/admin")
	// 中间件执行顺序：先身份验证，再权限校验
	admin.Use(middleware.JWTAuth(), middleware.SuperAdminOnly())
	{
		// 管理员操作接口
		admin.DELETE("/users/:id", api.AdminDeleteUser(hub))
		admin.DELETE("/rooms/:id", api.AdminDeleteRoom(hub))
	}

	// ========================================================================
	// 阶段5：启动HTTP服务
	// ========================================================================
	port := os.Getenv("SERVER_PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("LAN-IM服务端启动成功，监听端口 :%s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("[错误] HTTP服务启动失败: %v", err)
	}
}
