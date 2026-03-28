package repository

import (
	"gorm.io/gorm"
	"lan-im-go/models"
)

// ============================================================================
// 数据访问层接口定义
// 规范：业务层禁止直接操作gorm.DB，仅允许调用以下接口方法
// ============================================================================

// UserRepository 用户数据访问接口
type UserRepository interface {
	// 基础用户操作
	CreateUser(user *models.User) error
	GetByUsername(username string) (*models.User, error)
	GetByID(id int64) (*models.User, error)
	// 按ID软删除用户
	SoftDeleteUser(id int64) error
}

// RoomRepository 群组数据访问接口
type RoomRepository interface {
	// 创建群组并添加创建者，基于数据库事务保证一致性
	CreateRoomWithCreator(room *models.Room, creatorID int64) error
	GetRoomByID(roomID int64) (*models.Room, error)
	// 按ID软删除群组
	SoftDeleteRoom(roomID int64) error
	// 查询用户加入的所有群组，优化查询性能避免N+1问题
	GetJoinedRooms(userID int64) ([]*models.Room, error)
	// 根据名称精确查询群组
	GetRoomByExactName(exactName string) (*models.Room, error)
}

// RoomMemberRepository 群成员数据访问接口
type RoomMemberRepository interface {
	// 群成员管理
	AddMember(roomID, userID int64, role int8) error
	RemoveMember(roomID, userID int64) error
	// 查询用户加入的所有群组ID，用于WebSocket初始化
	GetUserRoomIDs(userID int64) ([]int64, error)
	// 校验用户是否为群成员，用于权限验证
	CheckIsMember(roomID, userID int64) (bool, error)
	// GetMemberRole 查询当前用户在群内的角色；ok=false 表示非成员或记录不存在
	GetMemberRole(roomID, userID int64) (role int8, ok bool, err error)
	// 查询群成员详细信息
	GetRoomMembers(roomID int64) ([]*models.User, error)
}

// MessageRepository 消息数据访问接口
type MessageRepository interface {
	// 异步保存消息
	SaveMessage(msg *models.Message) error

	SaveMessageBatch(msgs []*models.Message) error
	// 基于游标分页查询历史消息，避免深分页性能问题
	GetHistoryByCursor(roomID int64, cursorMsgID int64, limit int) ([]*models.Message, error)
	// 批量软删除指定用户在群组内的消息
	SoftDeleteUserMessagesInRoom(roomID int64, userID int64) error
}

// ============================================================================
// 全局数据访问接口实例
// 统一管理所有接口实现，避免业务层重复创建实例
// ============================================================================

var (
	User       UserRepository
	Room       RoomRepository
	RoomMember RoomMemberRepository
	Message    MessageRepository
)

// InitRepositories 初始化数据访问层
// 需在数据库连接初始化完成后调用，完成依赖注入
func InitRepositories(db *gorm.DB) {
	User = NewUserRepoImpl(db)
	Room = NewRoomRepoImpl(db)
	RoomMember = NewRoomMemberRepoImpl(db)
	Message = NewMessageRepoImpl(db)
}
