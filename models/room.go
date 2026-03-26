package models

import (
	"time"

	"gorm.io/plugin/soft_delete"
)

// Room 统一房间表
type Room struct {
	ID        int64  `gorm:"primaryKey;autoIncrement;comment:'房间号QID'"`
	Type      int8   `gorm:"type:tinyint;not null;comment:'1:双人私聊 2:多人普通群'"`
	Name      string `gorm:"type:varchar(128);default:'';comment:'群名称，私聊可为空'"`
	CreatorID int64  `gorm:"type:bigint unsigned;not null;index:idx_creator_id;comment:'创建者ID'"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt soft_delete.DeletedAt `gorm:"type:bigint unsigned;index:idx_deleted_at;softDelete:milli"`
}

// RoomMember 房间成员关系表 (替代传统的好友关系表和群成员表)
type RoomMember struct {
	ID        int64                 `gorm:"primaryKey;autoIncrement;comment:'关系主键ID'"` // 关系表直接用数据库自增即可，没必要上雪花算法，修复了幽灵注释
	RoomID    int64                 `gorm:"type:bigint;not null;uniqueIndex:uk_room_user_deleted,priority:1;comment:'房间ID'"`
	UserID    int64                 `gorm:"type:bigint;not null;uniqueIndex:uk_room_user_deleted,priority:2;index:idx_user_id;comment:'用户ID'"`
	Role      int8                  `gorm:"type:tinyint;default:1;comment:'1:普通成员 2:管理员 3:群主'"`
	JoinedAt  time.Time             `gorm:"autoCreateTime;comment:'加入时间'"`
	DeletedAt soft_delete.DeletedAt `gorm:"type:bigint unsigned;uniqueIndex:uk_room_user_deleted,priority:3;softDelete:milli;comment:'毫秒级软删除标记'"` // 【核心修复】：必须加入 DeletedAt 并纳入联合唯一索引，否则退群后永远无法再进群！
}
