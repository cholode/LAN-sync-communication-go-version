package models

import (
	"time"

	"gorm.io/gorm"
)

// User 用户表
type User struct {
	ID        int64  `gorm:"primaryKey;autoIncrement"` // 内部流转使用的 UID
	Username  string `gorm:"type:varchar(64);uniqueIndex:idx_username_dleted,priority:1;not null;comment:'登录用户名'"`
	Password  string `gorm:"type:varchar(255);not null;comment:'Bcrypt哈希密码'"`
	Avatar    string `gorm:"type:varchar(255);default:'';comment:'用户头像URL'"`
	Role      int8   `gorm:"type:tinyint;not null;default:0;comment:'0:普通用户 1:超管'"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index;uniqueIndex:idx_username_deleted,priority:2;comment:'软删除标记'"` // 软删除标记
}
