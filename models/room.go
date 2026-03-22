package models

import (
	"time"

	"gorm.io/gorm"
)

// Room 统一房间表 (单聊、群聊全部在这里)
type Room struct {
	ID        int64  `gorm:"primaryKey;autoIncrement"` // 房间号 QID
	Type      int8   `gorm:"type:tinyint;not null;comment:'1:双人私聊 2:多人普通群'"`
	Name      string `gorm:"type:varchar(128);default:'';comment:'群名称，私聊可为空'"`
	CreatorID int64
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

// RoomMember 房间成员关系表 (替代传统的好友关系表和群成员表)
type RoomMember struct {
	ID       int64     `gorm:"primaryKey;autoIncrement:false;index:idx_room_id_id,priority:2;comment:'雪花算法MsgID'"`
	RoomID   int64     `gorm:"type:bigint;not null;uniqueIndex:idx_room_user,priority:1;comment:'房间ID'"`
	UserID   int64     `gorm:"type:bigint;not null;uniqueIndex:idx_room_user,priority:2;inx_user_id;comment:'用户ID'"`
	Role     int8      `gorm:"type:tinyint;default:1;comment:'1:普通成员 2:管理员 3:群主'"`
	JoinedAt time.Time `gorm:"autoCreateTime;comment:'加入时间'"`
	// 绝对不要在这里写 GORM 的 foreignKey 约束标签！
	// 工业级高并发系统全靠逻辑外键，物理外键造成的死锁和级联扫描是性能灾难。
}
