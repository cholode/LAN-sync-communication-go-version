package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"lan-im-go/pkg"
	"lan-im-go/repository"
)

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func LoginHandler(c *gin.Context) {
	var req LoginRequest
	// Gin 提供的极致数据绑定与参数校验
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数不合法"})
		return
	}

	// 1. 调用持久层防腐层查出用户
	user, err := repository.User.GetByUsername(req.Username)
	if err != nil {
		// 架构师的防爆破细节：账号不存在和密码错误，统一返回一模一样的提示
		// 绝不告诉黑客到底是账号错了还是密码错了，防止爆破扫描
		c.JSON(http.StatusUnauthorized, gin.H{"error": "账号或密码错误"})
		return
	}

	// 2. 核验密码
	// 数据库里存的是 Bcrypt Hash，绝对不能明文比对！
	err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "账号或密码错误"})
		return
	}

	// 3. 密码正确，签发 JWT 通行证
	token, err := pkg.GenerateToken(user.ID, user.Role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "系统签发凭证失败"})
		return
	}

	// 4. 完美闭环返回
	c.JSON(http.StatusOK, gin.H{
		"msg":   "登录成功",
		"token": token,
		"user": gin.H{
			"id":       user.ID,
			"username": user.Username,
			"role":     user.Role,
			"avatar":   user.Avatar,
		},
	})
}
