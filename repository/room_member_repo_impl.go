package repository

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"lan-im-go/models"
)

type roomMemberRepoImpl struct {
	db *gorm.DB
}

func NewRoomMemberRepoImpl(db *gorm.DB) RoomMemberRepository {
	return &roomMemberRepoImpl{db: db}
}

// AddMember 添加群成员 ✅【终极修复】冲突自动忽略，永远不报错
func (r *roomMemberRepoImpl) AddMember(roomID, userID int64, role int8) error {
	member := &models.RoomMember{
		RoomID: roomID,
		UserID: userID,
		Role:   role,
	}

	// 核心修复：插入时遇到唯一索引冲突，直接忽略，不抛出任何错误
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "room_id"}, {Name: "user_id"}},
		DoNothing: true, // 冲突啥也不做
	}).Create(member).Error
}

func (r *roomMemberRepoImpl) RemoveMember(roomID, userID int64) error {
	result := r.db.Where("room_id = ? AND user_id = ?", roomID, userID).Delete(&models.RoomMember{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *roomMemberRepoImpl) GetUserRoomIDs(userID int64) ([]int64, error) {
	var roomIDs []int64
	err := r.db.Model(&models.RoomMember{}).Where("user_id = ?", userID).Pluck("room_id", &roomIDs).Error
	return roomIDs, err
}

func (r *roomMemberRepoImpl) CheckIsMember(roomID, userID64 int64) (bool, error) {
	var count int64
	err := r.db.Model(&models.RoomMember{}).
		Where("room_id = ? AND user_id = ?", roomID, userID64).
		Limit(1).
		Count(&count).Error
	return count > 0, err
}

func (r *roomMemberRepoImpl) GetRoomMembers(roomID int64) ([]*models.User, error) {
	var users []*models.User
	err := r.db.Table("users").
		Select("users.*").
		Joins("INNER JOIN room_members ON users.id = room_members.user_id").
		Where("room_members.room_id = ? AND users.deleted_at IS NULL", roomID).
		Find(&users).Error
	return users, err
}
