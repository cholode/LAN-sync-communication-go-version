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
	// 严苛细节：密码必须在传入前就已经被 bcrypt 哈希过，绝对不能在这里存明文
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
	// B端超管的核武器：精确软删。
	// 这里会将被删除用户的 deleted_at 设为当前时间。由于 GORM 默认查询会加上 deleted_at IS NULL，
	// 这意味着该用户瞬间被整个系统“物理蒸发”，无法登录，查无此人，但他的聊天记录依然安全。
	return r.db.Model(&models.User{}).Where("id = ?", id).Update("deleted_at", gorm.Expr("NOW()")).Error
}
