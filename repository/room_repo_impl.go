package repository

import (
	"gorm.io/gorm"
	"lan-im-go/models"
)

type roomRepoImpl struct {
	db *gorm.DB
}

func NewRoomRepoImpl(db *gorm.DB) RoomRepository {
	return &roomRepoImpl{db: db}
}

func (r *roomRepoImpl) CreateRoom(room *models.Room) error {
	return r.db.Create(room).Error
}

func (r *roomRepoImpl) GetRoomByID(roomID int64) (*models.Room, error) {
	var room models.Room
	err := r.db.First(&room, roomID).Error
	if err != nil {
		return nil, err
	}
	return &room, nil
}

func (r *roomRepoImpl) SoftDeleteRoom(roomID int64) error {
	// 超管解散群聊：仅做软删除，保留历史证据
	return r.db.Model(&models.Room{}).Where("id = ?", roomID).Update("deleted_at", gorm.Expr("NOW()")).Error
}

// GetJoinedRooms 架构师的骄傲：彻底解决 N+1 风暴的联表查询
func (r *roomRepoImpl) GetJoinedRooms(userID int64) ([]*models.Room, error) {
	var rooms []*models.Room
	// 严苛细节：利用 INNER JOIN 一次性把用户所在的房间全捞出来
	// GORM 这里的 Table("rooms") 会自动处理软删除 (deleted_at IS NULL)
	err := r.db.Table("rooms").
		Select("rooms.*").
		Joins("INNER JOIN room_members ON rooms.id = room_members.room_id").
		Where("room_members.user_id = ?", userID).
		Find(&rooms).Error
	return rooms, err
}

// GetRoomByExactName 防爬虫搜索机制
func (r *roomRepoImpl) GetRoomByExactName(exactName string) (*models.Room, error) {
	var room models.Room
	// 严苛要求：这里坚决不用 LIKE，强迫走普通索引或唯一索引进行精确等值查询
	err := r.db.Where("name = ?", exactName).First(&room).Error
	if err != nil {
		return nil, err
	}
	return &room, nil
}
