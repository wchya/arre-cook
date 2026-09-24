package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"ninimenu/internal/config"
	"ninimenu/internal/models"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// 令牌类型：
//   - user：App/小程序登录态，拥有本人全部权限；
//   - agent：嵌入式智能体会话令牌（短期，带 scopes），由用户在 App 内签发并通过 postMessage 交给 iframe；
//   - 个人访问令牌（PAT，nm_ 前缀）不是 JWT，见 GeneratePAT。
const (
	KindUser         = "user"
	KindAgentSession = "agent"
	KindPAT          = "pat"

	PATPrefix = "nm_"
	issuer    = "ninimenu"
)

type Claims struct {
	Kind    string   `json:"typ"`
	Role    string   `json:"role,omitempty"`
	Version int      `json:"ver"`
	Scopes  []string `json:"scp,omitempty"`
	Actor   string   `json:"act,omitempty"`
	jwt.RegisteredClaims
}

func (c *Claims) UserID() uint {
	id, _ := strconv.ParseUint(c.Subject, 10, 64)
	return uint(id)
}

func sign(claims Claims) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(config.C.JWTSecret))
}

// IssueUserToken 登录态令牌。
func IssueUserToken(u *models.User) (string, time.Time, error) {
	now := time.Now()
	exp := now.Add(config.C.JWTExpire)
	token, err := sign(Claims{
		Kind:    KindUser,
		Role:    u.Role,
		Version: u.TokenVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Subject:   strconv.FormatUint(uint64(u.ID), 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	})
	return token, exp, err
}

// IssueAgentSession 给嵌入式智能体的短期会话令牌：只能访问本人数据、受 scopes 限制。
func IssueAgentSession(u *models.User, scopes []string, actor string, ttl time.Duration) (string, time.Time, error) {
	if ttl <= 0 {
		ttl = config.C.AgentSessionTTL
	}
	now := time.Now()
	exp := now.Add(ttl)
	token, err := sign(Claims{
		Kind:    KindAgentSession,
		Version: u.TokenVersion,
		Scopes:  NormalizeScopes(scopes),
		Actor:   actor,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Subject:   strconv.FormatUint(uint64(u.ID), 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	})
	return token, exp, err
}

func ParseToken(raw string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("意外的签名算法: %v", t.Header["alg"])
		}
		return []byte(config.C.JWTSecret), nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer(issuer))
	if err != nil || !token.Valid {
		return nil, errors.New("token 无效或已过期")
	}
	if claims.UserID() == 0 {
		return nil, errors.New("token 无效")
	}
	return claims, nil
}

// GeneratePAT 生成个人访问令牌：明文只在创建时返回一次，库里只存 SHA-256。
func GeneratePAT() (plain, hash, prefix string, err error) {
	buf := make([]byte, 24)
	if _, err = rand.Read(buf); err != nil {
		return
	}
	plain = PATPrefix + hex.EncodeToString(buf)
	hash = HashPAT(plain)
	prefix = plain[:len(PATPrefix)+6]
	return
}

func HashPAT(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}
