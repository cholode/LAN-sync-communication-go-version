package repository

import (
	"gorm.io/gorm"
	"lan-im-go/models"
)

type roomMemberRepoImpl struct {
	db *gorm.DB
}

func NewRoomMemberRepoImpl(db *gorm.DB) RoomMemberRepository {
	return &roomMemberRepoImpl{db: db}
}

func (r *roomMemberRepoImpl) AddMember(roomID, userID int64, role int8) error {
	member := &models.RoomMember{
		RoomID: roomID,
		UserID: userID,
		Role:   role,
	}
	// 依赖模型中定义的唯一索引 idx_room_user 避免重复添加成员
	return r.db.Create(member).Error
}

func (r *roomMemberRepoImpl) RemoveMember(roomID, userID int64) error {
	// 物理删除群成员关联关系，无需软删除
	result := r.db.Where("room_id = ? AND user_id = ?", roomID, userID).Delete(&models.RoomMember{})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}

	return nil
}

// GetUserRoomIDs 查询用户加入的所有群聊ID，用于WebSocket连接初始化
func (r *roomMemberRepoImpl) GetUserRoomIDs(userID int64) ([]int64, error) {
	var roomIDs []int64
	// 使用 Pluck 仅查询 room_id 字段，减少内存占用，提升查询效率
	err := r.db.Model(&models.RoomMember{}).Where("user_id = ?", userID).Pluck("room_id", &roomIDs).Error
	return roomIDs, err
}

func (r *roomMemberRepoImpl) CheckIsMember(roomID, userID int64) (bool, error) {
	var count int64
	// 轻量计数查询，校验用户是否为群成员，用于权限验证
	err := r.db.Model(&models.RoomMember{}).Where("room_id = ? AND user_id = ?", roomID, userID).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetRoomMembers 查询指定群聊的所有成员信息
func (r *roomMemberRepoImpl) GetRoomMembers(roomID int64) ([]*models.User, error) {
	var users []*models.User
	// 内连接查询，避免N+1查询问题
	err := r.db.Table("users").
		Select("users.*").
		Joins("INNER JOIN room_members ON users.id = room_members.user_id").
		Where("room_members.room_id = ?", roomID).
		Find(&users).Error
	return users, err
}
