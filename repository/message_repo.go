package repository

import (
	"gorm.io/gorm"

	"lan-im-go/models"
)

// MessageRepo 封装了所有和消息表相关的数据库操作
type MessageRepo struct {
	db *gorm.DB
}

func NewMessageRepo(db *gorm.DB) *MessageRepo {
	return &MessageRepo{db: db}
}

// SaveMessage [怎么调用]：运行时插入一条新消息
func (repo *MessageRepo) SaveMessage(msgID, roomID, senderID int64, content string, msgType int8) error {
	// 实例化 models 中定义的结构体
	newMsg := &models.Message{
		ID:       msgID,
		RoomID:   roomID,
		SenderID: senderID,
		Type:     msgType,
		Content:  content,
	}
	// 调取 GORM 执行 Insert
	return repo.db.Create(newMsg).Error
}
