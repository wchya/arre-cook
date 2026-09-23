package middleware

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"ninimenu/internal/config"
	"ninimenu/internal/utils"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func parseAdminToken(rawToken string) bool {
	token, err := jwt.Parse(rawToken, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("意外的签名算法: %v", token.Header["alg"])
		}
		return []byte(config.C.JWTSecret), nil
	}, jwt.WithValidMethods([]string{"HS256"}))
	return err == nil && token.Valid
}

func bearerToken(c *gin.Context) string {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		return ""
	}
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		return ""
	}
	return parts[1]
}

func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		rawToken := bearerToken(c)
		if rawToken == "" {
			utils.Unauthorized(c, "未提供认证Token")
			c.Abort()
			return
		}

		if !parseAdminToken(rawToken) {
			utils.Unauthorized(c, "Token无效或已过期")
			c.Abort()
			return
		}

		c.Next()
	}
}

func AppAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if rawToken := bearerToken(c); rawToken != "" && parseAdminToken(rawToken) {
			c.Next()
			return
		}

		appToken := c.GetHeader("X-App-Token")
		if appToken == "" {
			utils.Unauthorized(c, "请输入应用密码")
			c.Abort()
			return
		}

		if subtle.ConstantTimeCompare([]byte(appToken), []byte(config.C.AppPassword)) != 1 {
			utils.Unauthorized(c, "应用密码错误")
			c.Abort()
			return
		}

		c.Next()
	}
}

// AgentAuthMiddleware 保护 /api/agent/* 全量能力接口。
// 接受三种凭证：X-Agent-Token 头（= AGENT_TOKEN）、Bearer AGENT_TOKEN、或管理端 JWT。
func AgentAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		expected := []byte(config.C.AgentToken)
		if token := c.GetHeader("X-Agent-Token"); token != "" && subtle.ConstantTimeCompare([]byte(token), expected) == 1 {
			c.Next()
			return
		}
		if rawToken := bearerToken(c); rawToken != "" {
			if subtle.ConstantTimeCompare([]byte(rawToken), expected) == 1 || parseAdminToken(rawToken) {
				c.Next()
				return
			}
		}
		utils.Unauthorized(c, "需要智能体访问令牌（X-Agent-Token）")
		c.Abort()
	}
}

func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-App-Token, X-Agent-Token")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

func LoggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		duration := time.Since(start)
		_ = duration
	}
}

func GenerateToken() (string, error) {
	now := time.Now()
	duration, err := time.ParseDuration(config.C.JWTExpire)
	if err != nil {
		duration = 24 * time.Hour
	}

	claims := jwt.MapClaims{
		"role": "admin",
		"exp":  now.Add(duration).Unix(),
		"iat":  now.Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(config.C.JWTSecret))
}

func CacheControlMiddleware(maxAge int) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", fmt.Sprintf("public, max-age=%d, immutable", maxAge))
		c.Next()
	}
}
