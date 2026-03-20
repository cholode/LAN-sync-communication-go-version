package repository

import (
	"gorm.io/gorm"
	"lan-im-go/models"
)

type roomRepository interface {
	// 底层封装事务，保证操作原子性
	CreateRoomWithCreator(room *models.Room, creatorID int64) error
	GetRoomByID(roomID int64) (*models.Room, error)
	SoftDeleteRoom(roomID int64) error
	GetJoinedRooms(userID int64) ([]*models.Room, error)
	GetRoomByExactName(exactName string) (*models.Room, error)
}

type roomRepoImpl struct {
	db *gorm.DB
}

func NewRoomRepoImpl(db *gorm.DB) roomRepository {
	return &roomRepoImpl{db: db}
}

// CreateRoomWithCreator 创建群聊并添加创建者，通过事务保证原子性
func (r *roomRepoImpl) CreateRoomWithCreator(room *models.Room, creatorID int64) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// 1. 创建群聊数据
		if err := tx.Create(room).Error; err != nil {
			return err
		}
		// 2. 添加创建者为群成员并设置管理员权限 (Role: 2)
		member := &models.RoomMember{
			RoomID: room.ID,
			UserID: creatorID,
			Role:   2,
		}
		if err := tx.Create(member).Error; err != nil {
			return err // 事务异常自动回滚
		}
		return nil // 事务执行成功自动提交
	})
}

func (r *roomRepoImpl) GetRoomByID(roomID int64) (*models.Room, error) {
	var room models.Room
	err := r.db.First(&room, roomID).Error
	return &room, err
}

func (r *roomRepoImpl) SoftDeleteRoom(roomID int64) error {
	// 基于gorm软删除特性，自动转换为更新deleted_at字段
	return r.db.Delete(&models.Room{}, roomID).Error
}

// GetJoinedRooms 联表查询用户加入的群聊，避免N+1查询问题
func (r *roomRepoImpl) GetJoinedRooms(userID int64) ([]*models.Room, error) {
	var rooms []*models.Room
	// 内连接查询，继承软删除规则，一次性获取用户所有群聊
	err := r.db.Model(&models.Room{}).
		Select("rooms.*").
		Joins("INNER JOIN room_members ON rooms.id = room_members.room_id").
		Where("room_members.user_id = ?", userID).
		Find(&rooms).Error
	return rooms, err
}

// GetRoomByExactName 根据群聊名称精确查询群聊
func (r *roomRepoImpl) GetRoomByExactName(exactName string) (*models.Room, error) {
	var room models.Room
	// 采用等值查询，使用索引提升查询效率，不使用模糊查询
	err := r.db.Where("name = ?", exactName).First(&room).Error
	return &room, err
}
