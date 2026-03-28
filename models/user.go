package models

import (
	"time"

	"gorm.io/plugin/soft_delete"
)

// User 用户表
type User struct {
	ID        int64                 `gorm:"primaryKey;autoIncrement;comment:'内部流转使用的UID'"` // 修复拼写错误，统一下划线命名
	Username  string                `gorm:"type:varchar(64);uniqueIndex:idx_username_deleted,priority:1;not null;comment:'登录用户名'"`
	Password  string                `gorm:"type:varchar(255);not null;comment:'Bcrypt哈希密码'"`
	Avatar    string                `gorm:"type:varchar(255);default:'';comment:'用户头像URL'"`
	Role      int8                  `gorm:"type:tinyint;not null;default:0;comment:'0:普通用户 1:超管'"`
	CreatedAt time.Time             `gorm:"comment:'创建时间'"`
	UpdatedAt time.Time             `gorm:"comment:'更新时间'"`
	DeletedAt soft_delete.DeletedAt `gorm:"type:bigint unsigned;uniqueIndex:idx_username_deleted,priority:2;softDelete:milli;comment:'毫秒级软删除标记'"` // 软删除：强转为毫秒级 BIGINT，完美解决高并发同名注销复用问题
}
