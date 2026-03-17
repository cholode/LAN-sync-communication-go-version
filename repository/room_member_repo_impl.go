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
	// 利用我们在 models 里建的 uniqueIndex:idx_room_user
	// 如果重复加入，GORM 会返回 Duplicate Entry 错误，天然防脏数据
	return r.db.Create(member).Error
}

func (r *roomMemberRepoImpl) RemoveMember(roomID, userID int64) error {
	// 物理删除关系表中的记录 (关系解绑不需要软删)
	return r.db.Where("room_id = ? AND user_id = ?", roomID, userID).Delete(&models.RoomMember{}).Error
}

// GetUserRoomIDs Hub 引擎的生命线，要求极致的响应速度
func (r *roomMemberRepoImpl) GetUserRoomIDs(userID int64) ([]int64, error) {
	var roomIDs []int64
	// 架构师绝杀：使用 Pluck 方法！
	// 不查询整个结构体，仅仅从 B+ 树上剥离 room_id 这一个字段，内存占用极小，速度极快
	err := r.db.Model(&models.RoomMember{}).Where("user_id = ?", userID).Pluck("room_id", &roomIDs).Error
	return roomIDs, err
}

func (r *roomMemberRepoImpl) CheckIsMember(roomID, userID int64) (bool, error) {
	var count int64
	// 越权防线：发消息前只做极其轻量的 Count，不要 Select 实体
	err := r.db.Model(&models.RoomMember{}).Where("room_id = ? AND user_id = ?", roomID, userID).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetRoomMembers 业务需求：获取群内所有人的头像和基本信息
func (r *roomMemberRepoImpl) GetRoomMembers(roomID int64) ([]*models.User, error) {
	var users []*models.User
	// 同样是规避 N+1 的完美 INNER JOIN
	err := r.db.Table("users").
		Select("users.*").
		Joins("INNER JOIN room_members ON users.id = room_members.user_id").
		Where("room_members.room_id = ?", roomID).
		Find(&users).Error
	return users, err
}
