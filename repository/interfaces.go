package repository

import (
	"gorm.io/gorm"
	"lan-im-go/models"
)

// ============================================================================
// 【领域驱动设计 (DDD) 核心持久层契约】
// 严苛约束：业务逻辑层 (Service/Handler) 绝对不允许直接引入 gorm.DB，
// 必须且只能调用以下接口中定义的方法！
// ============================================================================

// UserRepository 用户与超管管控领域
type UserRepository interface {
	// 基础身份
	CreateUser(user *models.User) error
	GetByUsername(username string) (*models.User, error)
	GetByID(id int64) (*models.User, error)
	// 管控：只支持指定精确 ID 删除，拒绝模糊匹配
	SoftDeleteUser(id int64) error
}

// RoomRepository 房间元数据领域
type RoomRepository interface {
	// 基础操作
	CreateRoom(room *models.Room) error
	GetRoomByID(roomID int64) (*models.Room, error)
	// 管控：指定精确 ID 解散群聊
	SoftDeleteRoom(roomID int64) error
	// 业务：联表查询该用户已加入的所有群聊列表 (解决 N+1 性能风暴)
	GetJoinedRooms(userID int64) ([]*models.Room, error)

	GetRoomByExactName(exactName string) (*models.Room, error)
}

// RoomMemberRepository 万物皆群聊的核心流转枢纽
type RoomMemberRepository interface {
	// 关系绑定与解绑
	AddMember(roomID, userID int64, role int8) error
	RemoveMember(roomID, userID int64) error
	// 核心引擎依赖：建立 WebSocket 时拉取订阅清单
	GetUserRoomIDs(userID int64) ([]int64, error)
	// 越权防御：发消息前的身份校验
	CheckIsMember(roomID, userID int64) (bool, error)
	// 业务：联表查询获取指定群聊内的所有成员详细信息 (头像、昵称、Role)
	GetRoomMembers(roomID int64) ([]*models.User, error)
}

// MessageRepository 核心消息吞吐领域
type MessageRepository interface {
	// 核心写入：异步高并发落盘
	SaveMessage(msg *models.Message) error
	// 核心查询：触顶无感加载，强依赖 MsgID 游标，严禁使用 Offset
	GetHistoryByCursor(roomID int64, cursorMsgID int64, limit int) ([]*models.Message, error)
	// 管控：精确抹除指定用户在指定群聊内的全部历史痕迹 (命中组合索引的批量 UPDATE)
	SoftDeleteUserMessagesInRoom(roomID int64, userID int64) error
}

// ============================================================================
// 【全局单例注册中心 (Registry)】
// 作用：统一收口所有的接口实例，避免在业务代码中到处 new 对象
// ============================================================================

var (
	User       UserRepository
	Room       RoomRepository
	RoomMember RoomMemberRepository
	Message    MessageRepository
)

// InitRepositories 依赖注入引擎
// 必须在 main.go 中基础设施 (MySQL 连接池) 启动后，立刻调用此方法
func InitRepositories(db *gorm.DB) {
	// 提醒：这里的 NewXXXImpl 方法我们需要在各自的物理文件中实现
	User = NewUserRepoImpl(db)
	Room = NewRoomRepoImpl(db)
	RoomMember = NewRoomMemberRepoImpl(db)
	Message = NewMessageRepoImpl(db)
}
