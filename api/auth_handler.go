package api

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"lan-im-go/models"
	"lan-im-go/pkg"
	"lan-im-go/repository"
	"log"
	"net/http"
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
		// 防爆破：账号不存在和密码错误，统一返回一模一样的提示
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
		log.Printf("%s/n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "系统签发凭证失败"})
		return
	}

	// 4. 闭环返回
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

// RegisterRequest 注册请求载荷
type RegisterRequest struct {
	// 防线 1：利用 Gin 的 binding 标签进行第一层物理隔离
	// 拒绝超长恶意字符串，防止撑爆内存或引发 SQL 注入风险
	Username string `json:"username" binding:"required,min=3,max=32"`
	Password string `json:"password" binding:"required,min=6,max=32"`
}

func RegisterHandler(c *gin.Context) {

	var req RegisterRequest
	fmt.Printf(("进来了"))
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数格式不合法：账号或密码长度不符合规范"})
		return
	}

	// 防线 2：业务层查重 (防过度查库)
	// 虽然数据库有唯一索引，但在业务层先拦截可以减轻 DB 压力，并给用户明确的提示
	existingUser, _ := repository.User.GetByUsername(req.Username)
	if existingUser != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "该用户名已被注册，请更换"})
		return
	}

	// 防线 3：密码哈希单向加密 (Bcrypt)
	// 不允许将明文密码直接写入数据库！
	// Bcrypt.DefaultCost (通常是 10) 意味着每次加密大概需要 50-100 毫秒
	// 这个刻意设计的延迟，能让黑客的彩虹表爆破成本呈指数级上升
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "系统加密引擎异常，请稍后再试"})
		return
	}

	// 组装要落库的用户实体
	user := &models.User{
		Username: req.Username,
		Password: string(hashedPassword),
		Role:     0,  // 0 代表普通用户，新注册用户绝不能被赋予超管权限
		Avatar:   "", // 真实项目中可以在这里赋一个默认头像的 URL
	}

	// 防线 4：落库与并发冲突防御
	// 极端并发场景下，如果两个请求在同一毫秒内通过了前面的查重校验，并尝试注册同一个名字
	// 这里的 CreateUser 依然会因为我们在 models.User 中定义的 `uniqueIndex` 而报错拦截
	if err := repository.User.CreateUser(user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "注册落库失败，可能是遭遇了并发冲突"})
		return
	}

	// 注册成功后，仅仅返回成功提示，坚决不直接返回 JWT Token。
	// 强制要求前端跳转到登录页面重新走一遍登录流程，让全系统的身份签发 100% 收口在 LoginHandler。
	c.JSON(http.StatusOK, gin.H{
		"msg": "注册成功，请前往登录",
	})
}
