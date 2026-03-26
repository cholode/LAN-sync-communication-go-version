package models

import (
	//"gorm.io/gorm"
	"gorm.io/plugin/soft_delete"
	"time"
)

// Message 统一消息表 (系统的绝对核心)
type Message struct {
	ID        int64                 `gorm:"primaryKey;autoIncrement:false;index:idx_room_id_id,priority:2;comment:'雪花算法MsgID'"` // 雪花算法 ID 包含时间戳属性，天生有序
	RoomID    int64                 `gorm:"type:bigint;not null;index:idx_room_id_id,priority:1;comment:'所属房间'"`
	SenderID  int64                 `gorm:"type:bigint;not null;index:idx_sender_id;comment:'发送者ID'"`
	Type      int8                  `gorm:"type:tinyint;not null;default:1;comment:'1:文本 2:文件/图片 3:系统通知'"`
	Content   string                `gorm:"type:text;not null;comment:'消息内容或文件JSON载荷'"`
	CreatedAt time.Time             `gorm:"comment:'创建时间'"`
	DeletedAt soft_delete.DeletedAt `gorm:"type:bigint unsigned;index:idx_msg_deleted;softDelete:milli;comment:'用于实现消息撤回的软删除'"`
}
