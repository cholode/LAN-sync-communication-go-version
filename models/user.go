package models

import (
	"time"

	"gorm.io/gorm"
)

// User 用户表
type User struct {
	ID        int64  `gorm:"primaryKey;autoIncrement"` // 内部流转使用的 UID
	Username  string `gorm:"type:varchar(64);uniqueIndex;not null;comment:'登录用户名'"`
	Password  string `gorm:"type:varchar(255);not null;comment:'Bcrypt哈希密码'"`
	Avatar    string `gorm:"type:varchar(255);default:'';comment:'用户头像URL'"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"` // 软删除标记
}
