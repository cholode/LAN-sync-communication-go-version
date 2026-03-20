package pkg

import (
	"errors"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// 注意：生产环境中，密钥禁止硬编码
// 需通过环境变量或密钥管理系统配置，此处为演示代码
var jwtSecret = func() []byte {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		return []byte("fK29gArL2zTtX7V8ErD9bF2wR4qJ6pM3")
	}
	return []byte(secret)
}

// CustomClaims 自定义JWT载荷
// 包含用户身份信息，集成JWT标准声明
type CustomClaims struct {
	UserID int64 `json:"user_id"`
	Role   int8  `json:"role"` // 0:普通用户 1:超级管理员
	jwt.RegisteredClaims
}

// GenerateToken 生成JWT身份令牌
// 登录验证通过后调用，生成用户令牌
func GenerateToken(userID int64, role int8) (string, error) {
	claims := CustomClaims{
		UserID: userID,
		Role:   role,

		RegisteredClaims: jwt.RegisteredClaims{
			// 设置令牌有效期为24小时，降低令牌泄露风险
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "lan-im-server",
		},
	}

	// 采用HMAC SHA256算法签名生成令牌
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret())
}

// ParseToken 解析并验证JWT令牌
// 用于身份验证中间件，校验令牌有效性
func ParseToken(tokenString string) (*CustomClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &CustomClaims{}, func(token *jwt.Token) (interface{}, error) {
		// 校验签名算法类型，防止算法篡改攻击
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("非法的签名算法")
		}
		return jwtSecret(), nil
	})
	if err != nil {
		return nil, err
	}
	// 验证令牌并提取自定义载荷
	if claims, ok := token.Claims.(*CustomClaims); ok && token.Valid {
		return claims, nil
	}
	return nil, errors.New("Token 解析失败或已失效")
}
