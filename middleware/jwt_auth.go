package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"lan-im-go/pkg"
	"lan-im-go/repository"
)

// JWTAuth 核心拦截器
func JWTAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. 提取 Authorization Header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "缺失 Authorization 请求头"})
			c.Abort() // 拔枪击毙，严禁代码继续向后执行
			return
		}

		// 2. 严苛校验 Bearer 格式
		// 标准格式为 "Bearer eyJhbGciOiJIUzI1NiIs..."
		parts := strings.SplitN(authHeader, " ", 2)
		if !(len(parts) == 2 && parts[0] == "Bearer") {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Token 格式错误，必须为 Bearer 类型"})
			c.Abort()
			return
		}

		// 3. 解析底层 Token
		claims, err := pkg.ParseToken(parts[1])
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Token 无效或已过期"})
			c.Abort()
			return
		}

		// 4. B端管控的致命一击：防范幽灵 Token
		// 假如一个用户被超管软删除了，但他手里的 Token 还有 10 小时才过期怎么办？
		// 必须在这里利用之前写好的 Repository 查一次库，确认该账号是否还合法存活！
		user, err := repository.User.GetByID(claims.UserID)
		if err != nil || user == nil {
			c.JSON(http.StatusForbidden, gin.H{"error": "该账号已被冻结或物理注销"})
			c.Abort()
			return
		}

		// 5. 身份注入上下文
		// 把清洗干净、绝对可信的身份信息塞进 Gin Context 中
		// 后续所有的业务 Handler (包括上传接口)，都只能从这里拿 userID，绝对信任！
		c.Set("user_id", claims.UserID)
		c.Set("role", claims.Role)
		c.Next() // 放行，进入业务逻辑
	}
}

// SuperAdminOnly B端超管专属拦截器 (必须接在 JWTAuth 之后使用)
func SuperAdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 由于它接在 JWTAuth 后面，这里一定能拿到 role
		role, exists := c.Get("role")
		if !exists || role.(int8) != 1 {
			c.JSON(http.StatusForbidden, gin.H{"error": "越权访问：仅超级管理员可执行此操作"})
			c.Abort()
			return
		}
		c.Next()
	}
}
