package repository

import (
	"gorm.io/gorm"
	"lan-im-go/models"
)

type roomRepository interface {
	// 将事务封装在底层，保证原子性
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

// CreateRoomWithCreator 架构师红线：建群与加冕群主必须是 ACID 强事务！
func (r *roomRepoImpl) CreateRoomWithCreator(room *models.Room, creatorID int64) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// 1. 物理落盘建群
		if err := tx.Create(room).Error; err != nil {
			return err
		}
		// 2. 将创世者写入映射表，赋予最高权限 (Role: 2)
		member := &models.RoomMember{
			RoomID: room.ID,
			UserID: creatorID,
			Role:   2,
		}
		if err := tx.Create(member).Error; err != nil {
			return err // 报错自动 Rollback，群也会被撤销
		}
		return nil // 完美执行，自动 Commit
	})
}

func (r *roomRepoImpl) GetRoomByID(roomID int64) (*models.Room, error) {
	var room models.Room
	err := r.db.First(&room, roomID).Error
	return &room, err
}

func (r *roomRepoImpl) SoftDeleteRoom(roomID int64) error {
	// 极简魔法：因为有 gorm.DeletedAt，这里的 Delete 会被自动转换为 UPDATE deleted_at
	return r.db.Delete(&models.Room{}, roomID).Error
}

// GetJoinedRooms 架构师的骄傲：彻底解决 N+1 风暴的联表查询
func (r *roomRepoImpl) GetJoinedRooms(userID int64) ([]*models.Room, error) {
	var rooms []*models.Room
	// 严苛细节：使用 Model 继承软删属性，利用 INNER JOIN 一次性把用户所在的房间全捞出来
	err := r.db.Model(&models.Room{}).
		Select("rooms.*").
		Joins("INNER JOIN room_members ON rooms.id = room_members.room_id").
		Where("room_members.user_id = ?", userID).
		Find(&rooms).Error
	return rooms, err
}

// GetRoomByExactName 防爬虫精确搜索机制
func (r *roomRepoImpl) GetRoomByExactName(exactName string) (*models.Room, error) {
	var room models.Room
	// 严苛要求：坚决不用 LIKE，强迫走普通索引或唯一索引进行精确等值查询
	err := r.db.Where("name = ?", exactName).First(&room).Error
	return &room, err
}
