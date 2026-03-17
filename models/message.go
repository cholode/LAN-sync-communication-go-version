package models

import (
	"time"

	"gorm.io/gorm"
)

// Message 统一消息表 (系统的绝对核心)
type Message struct {
	// 采用我们讨论过的单机雪花算法，彻底放弃自增，防止分表困难和主键暴露
	ID int64 `gorm:"primaryKey;autoIncrement:false;comment:'雪花算法MsgID'"`
	// 核心索引设计：组合索引 idx_room_id
	// 为什么把 RoomID 放在前面？因为拉取历史记录的 SQL 永远是 WHERE room_id = ? ORDER BY id DESC
	RoomID    int64          `gorm:"type:bigint;not null;index:idx_room_id,priority:1;comment:'所属房间'"`
	SenderID  int64          `gorm:"type:bigint;not null;comment:'发送者ID'"`
	Type      int8           `gorm:"type:tinyint;not null;default:1;comment:'1:文本 2:文件/图片 3:系统通知'"`
	Content   string         `gorm:"type:text;not null;comment:'消息内容或文件JSON载荷'"`
	CreatedAt time.Time      `gorm:"index;comment:'创建时间，辅助兜底排序'"`
	DeletedAt gorm.DeletedAt `gorm:"index;comment:'用于实现消息撤回的软删除'"`
}
