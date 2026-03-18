package pkg

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// 严苛要求：在真实生产环境中，这个 Secret 绝对不能硬编码在代码里！
// 必须通过环境变量 (os.Getenv) 或 KMS 密钥管理系统注入。
// 这里仅作演示。
var jwtSecret = []byte("YOUR_SUPER_SECRET_KEY_DONT_LEAK")

// CustomClaims 自定义载荷：除了标准的过期时间，我们把核心身份数据也打包进去
type CustomClaims struct {
	UserID int64 `json:"user_id"`
	Role   int8  `json:"role"` // 0:普通用户 1:超管，提前装入 Token 减少查库
	jwt.RegisteredClaims
}

// GenerateToken 登录成功后调用，颁发通行证
func GenerateToken(userID int64, role int8) (string, error) {
	claims := CustomClaims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			// 严苛的安全策略：Token 必须有极短的有效期（如 24 小时）
			// 拒绝签发长久有效的 Token，防止泄露后被无限期利用
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "lan-im-server",
		},
	}
	// 使用 HMAC SHA256 算法进行签名
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

// ParseToken 中间件拦截时调用，验明正身
func ParseToken(tokenString string) (*CustomClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &CustomClaims{}, func(token *jwt.Token) (interface{}, error) {
		// 严苛防御：强校验算法类型，防止黑客将算法篡改为 "None" 绕过签名机制
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("非法的签名算法")
		}
		return jwtSecret, nil
	})
	if err != nil {
		return nil, err
	}
	// 提取出我们自定义的载荷
	if claims, ok := token.Claims.(*CustomClaims); ok && token.Valid {
		return claims, nil
	}
	return nil, errors.New("Token 解析失败或已失效")
}
