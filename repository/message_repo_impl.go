package repository

import (
	"gorm.io/gorm"
	"lan-im-go/models"
)

type messageRepoImpl struct {
	db *gorm.DB
}

func NewMessageRepoImpl(db *gorm.DB) MessageRepository {
	return &messageRepoImpl{db: db}
}

func (r *messageRepoImpl) SaveMessage(msg *models.Message) error {
	// 消息持久化方法，提供高性能写入能力
	return r.db.Create(msg).Error
}

// GetHistoryByCursor 基于游标分页查询历史消息
func (r *messageRepoImpl) GetHistoryByCursor(roomID int64, cursorMsgID int64, limit int) ([]*models.Message, error) {
	var messages []*models.Message
	// 1. 基础查询条件，匹配群聊ID索引
	query := r.db.Where("room_id = ?", roomID)
	// 2. 游标条件：游标大于0时，查询更早的历史消息
	// 游标为0时，查询最新消息
	if cursorMsgID > 0 {
		query = query.Where("id < ?", cursorMsgID)
	}
	// 3. 按消息ID倒序查询，匹配数据库索引，提升查询效率
	err := query.Order("id DESC").Limit(limit).Find(&messages).Error
	return messages, err
}

// SoftDeleteUserMessagesInRoom 软删除指定用户在群聊内的所有消息
func (r *messageRepoImpl) SoftDeleteUserMessagesInRoom(roomID int64, userID int64) error {
	// 采用软删除而非物理删除：
	// 1. 保留数据记录，满足数据追溯需求
	// 2. 避免物理删除导致的数据库索引结构变动，保证高并发场景下的数据库性能稳定
	return r.db.Model(&models.Message{}).
		Where("room_id = ? AND sender_id = ?", roomID, userID).
		Update("deleted_at", gorm.Expr("NOW()")).Error
}
