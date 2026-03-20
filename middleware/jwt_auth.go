package middleware

import (
	"lan-im-go/pkg"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// JWTAuth JWT身份验证中间件
func JWTAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		var tokenString string
		// 1. 从标准HTTP请求头中获取Token
		log.Printf("[JWT认证] 收到请求: %s %s", c.Request.Method, c.Request.URL.String())
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && parts[0] == "Bearer" {
				tokenString = parts[1]
				log.Printf("从请求头提取Token成功")
			} else {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "请求头Token格式非法"})
				c.Abort()
				return
			}
		} else {
			// 2. 从URL参数中获取Token（兼容WebSocket握手）
			tokenString = c.Query("token")
			log.Printf("从URL参数提取Token成功")
		}

		// 3. 校验Token是否存在
		if tokenString == "" {
			log.Printf("Token凭证为空")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未提供Token凭证，访问被拒绝"})
			c.Abort()
			return
		}

		claims, err := pkg.ParseToken(tokenString)
		if err != nil {
			log.Printf("Token解析失败\n")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Token解析失败"})
			c.Abort()
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("user_role", claims.Role)
		c.Next()
	}
}

// SuperAdminOnly 超级管理员权限校验中间件
// 依赖JWTAuth中间件，需在其后使用
func SuperAdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从上下文获取用户权限
		role, exists := c.Get("user_role")
		if !exists || role.(int8) != 1 {
			c.JSON(http.StatusForbidden, gin.H{"error": "权限不足，仅超级管理员可访问"})
			c.Abort()
			return
		}
		c.Next()
	}
}
