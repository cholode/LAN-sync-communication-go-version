package infrastructure

import (
	"log"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"lan-im-go/models" // 导入你的图纸
)

// DB 全局数据库实例指针 (在整个应用生命周期内复用)
var DB *gorm.DB

// InitDatabase 负责初始化数据库引擎并完成建表
func InitDatabase(dsn string) {
	var err error
	// 1. 建立物理连接
	DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
		// 细节：在生产环境中通常会关闭 GORM 的默认控制台 SQL 打印，或者重定向到日志库
		// Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		log.Fatalf("[致命错误] MySQL 连接失败，请检查 DSN 配置: %v", err)
	}

	// 2. 考点：配置底层 SQL 连接池
	// 很多新手不写这段代码，导致高并发时连接数打满，数据库直接拒绝服务
	sqlDB, err := DB.DB()
	if err != nil {
		log.Fatalf("[致命错误] 获取底层 sql.DB 失败: %v", err)
	}
	sqlDB.SetMaxIdleConns(10)           // 空闲连接池中连接的最大数量
	sqlDB.SetMaxOpenConns(100)          // 打开数据库连接的最大数量
	sqlDB.SetConnMaxLifetime(time.Hour) // 连接可复用的最大时间

	// 3. 真正建表的地方！将图纸翻译成 MySQL 物理表
	log.Println("正在同步数据库表结构...")
	err = DB.AutoMigrate(
		&models.User{},
		&models.Room{},
		&models.RoomMember{},
		&models.Message{},
	)
	if err != nil {
		log.Fatalf("[致命错误] 数据库表结构同步 (AutoMigrate) 失败: %v", err)
	}

	log.Println("MySQL 连接成功，表结构同步完毕，连接池已配置！")
}
