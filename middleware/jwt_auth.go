package middleware

import (
	"lan-im-go/pkg"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	// "lan-im-go/pkg" // JWT 解析工具包
)

func JWTAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		var tokenString string
		// 1. 尝试从标准 HTTP 的 Authorization Header 中获取
		log.Printf("[JWT 防线] 收到请求: %s %s", c.Request.Method, c.Request.URL.String())
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			log.Printf("")
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && parts[0] == "Bearer" {

				tokenString = parts[1]
				log.Printf("token提取成功")
			} else {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Header 中的 Token 格式错误"})
				c.Abort()
				return
			}
		} else {

			// 2. 尝试从 URL Query 参数中获取 (专为 WebSocket 握手开启的绿色通道！)
			tokenString = c.Query("token")
			log.Printf("成功从query提取token")
		}

		// 3. 终极判决：如果两个地方都没有，直接击毙
		if tokenString == "" {
			log.Printf("token为空")

			c.JSON(http.StatusUnauthorized, gin.H{"error": "拒绝访问：缺失 Token 凭证"})
			c.Abort()
			return
		}

		claims, err := pkg.ParseToken(tokenString)
		if err != nil {
			log.Printf("Token解析错误\n")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Token解析错误"})
		}

		c.Set("user_id", claims.UserID)

		c.Next()
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
