package repository

import (
	"gorm.io/gorm"
	"lan-im-go/models"
)

type userRepoImpl struct {
	db *gorm.DB
}

func NewUserRepoImpl(db *gorm.DB) UserRepository {
	return &userRepoImpl{
		db: db,
	}
}

func (r *userRepoImpl) CreateUser(user *models.User) error {
	// 规范：用户密码需在调用前完成bcrypt加密处理，禁止存储明文密码
	return r.db.Create(user).Error
}

func (r *userRepoImpl) GetByUsername(username string) (*models.User, error) {
	var user models.User
	err := r.db.Where("username = ?", username).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *userRepoImpl) GetByID(id int64) (*models.User, error) {
	var user models.User
	err := r.db.First(&user, id).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *userRepoImpl) SoftDeleteUser(id int64) error {
	// 软删除用户：更新删除时间标记
	// GORM软删除机制会自动过滤已删除数据，使用户无法登录，同时保留历史数据
	return r.db.Model(&models.User{}).Where("id = ?", id).Update("deleted_at", gorm.Expr("NOW()")).Error
}
