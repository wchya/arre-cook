package middleware

import (
	"encoding/json"
	"log"
	"net/http"
	"ninimenu/internal/auth"
	"ninimenu/internal/config"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"ninimenu/internal/utils"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

func bearerToken(c *gin.Context) string {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		return ""
	}
	scheme, token, ok := strings.Cut(authHeader, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}

func loadActiveUser(id uint, version int) (*models.User, bool) {
	var u models.User
	if err := database.DB.First(&u, id).Error; err != nil {
		return nil, false
	}
	if u.Disabled || u.TokenVersion != version {
		return nil, false
	}
	return &u, true
}

// resolvePrincipal 从请求中解析调用者。allowAgent=false 时只接受用户登录态。
func resolvePrincipal(c *gin.Context, allowAgent bool) (*auth.Principal, string) {
	raw := bearerToken(c)
	if allowAgent && raw == "" {
		raw = strings.TrimSpace(c.GetHeader("X-Agent-Token"))
	}
	if raw == "" {
		return nil, "请先登录"
	}

	if strings.HasPrefix(raw, auth.PATPrefix) {
		if !allowAgent {
			return nil, "智能体令牌不能用于此接口"
		}
		return resolvePAT(raw)
	}

	claims, err := auth.ParseToken(raw)
	if err != nil {
		return nil, "登录已过期，请重新登录"
	}
	u, ok := loadActiveUser(claims.UserID(), claims.Version)
	if !ok {
		return nil, "登录已失效，请重新登录"
	}
	switch claims.Kind {
	case auth.KindUser:
		return &auth.Principal{User: u, Kind: auth.KindUser, Scopes: auth.NewScopeSet(auth.AllScopes)}, ""
	case auth.KindAgentSession:
		if !allowAgent {
			return nil, "智能体会话令牌不能用于此接口"
		}
		actor := claims.Actor
		if actor == "" {
			actor = "embed"
		}
		return &auth.Principal{User: u, Kind: auth.KindAgentSession, Scopes: auth.NewScopeSet(claims.Scopes), Actor: actor}, ""
	}
	return nil, "token 类型无效"
}

func resolvePAT(raw string) (*auth.Principal, string) {
	var tok models.AgentToken
	if err := database.DB.Where("token_hash = ?", auth.HashPAT(raw)).First(&tok).Error; err != nil {
		return nil, "智能体令牌无效"
	}
	now := time.Now()
	if tok.RevokedAt != nil {
		return nil, "智能体令牌已被撤销"
	}
	if tok.ExpiresAt != nil && now.After(*tok.ExpiresAt) {
		return nil, "智能体令牌已过期"
	}
	var u models.User
	if err := database.DB.First(&u, tok.UserID).Error; err != nil || u.Disabled {
		return nil, "令牌所属账号不可用"
	}
	// 最近使用时间按分钟级节流写入，避免每个请求都写库
	if tok.LastUsedAt == nil || now.Sub(*tok.LastUsedAt) > time.Minute {
		database.DB.Model(&models.AgentToken{}).Where("id = ?", tok.ID).Update("last_used_at", now)
	}
	var scopes []string
	_ = json.Unmarshal([]byte(tok.Scopes), &scopes)
	return &auth.Principal{User: &u, Kind: auth.KindPAT, Scopes: auth.NewScopeSet(scopes), TokenID: tok.ID, Actor: tok.Name}, ""
}

// OptionalAuth 有登录态就解析，没有也放行（用于 app-info 等公开接口按需返回个性化内容）。
func OptionalAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if p, _ := resolvePrincipal(c, false); p != nil {
			auth.SetPrincipal(c, p)
		}
		c.Next()
	}
}

// UserAuth 用户登录态（App / 小程序 / 管理后台）。
func UserAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		p, msg := resolvePrincipal(c, false)
		if p == nil {
			utils.Unauthorized(c, msg)
			c.Abort()
			return
		}
		auth.SetPrincipal(c, p)
		c.Next()
	}
}

// AdminOnly 需在 UserAuth 之后使用。
func AdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !auth.CurrentUser(c).IsAdmin() {
			utils.Forbidden(c, "需要管理员权限")
			c.Abort()
			return
		}
		c.Next()
	}
}

// AgentAuth 智能体开放接口：只接受个人访问令牌（nm_…）或短期嵌入会话令牌。
// 用户登录 JWT 只用于站内 App，避免把长期登录凭证交给 Hermes、DSH 等第三方。
func AgentAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		p, msg := resolvePrincipal(c, true)
		if p == nil {
			c.Header("WWW-Authenticate", `Bearer realm="ninimenu-agent"`)
			utils.Unauthorized(c, msg)
			c.Abort()
			return
		}
		if p.Kind == auth.KindUser {
			c.Header("WWW-Authenticate", `Bearer realm="ninimenu-agent"`)
			utils.Unauthorized(c, "第三方接口必须使用个人 Agent 令牌或短期 Agent 会话令牌")
			c.Abort()
			return
		}
		if p.IsAgent() && !agentLimiter.Allow(agentLimitKey(p), config.C.AgentRateLimit) {
			c.Header("Retry-After", "60")
			utils.Error(c, http.StatusTooManyRequests, 42900, "请求过于频繁，请稍后再试")
			c.Abort()
			return
		}
		auth.SetPrincipal(c, p)
		c.Next()
	}
}

// RequireScope 智能体权限校验；用户本人登录态天然拥有全部权限。
func RequireScope(scope string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !auth.GetPrincipal(c).Can(scope) {
			utils.Forbidden(c, "令牌缺少权限："+scope)
			c.Abort()
			return
		}
		c.Next()
	}
}

func agentLimitKey(p *auth.Principal) string {
	if p.TokenID > 0 {
		return "pat:" + itoa(p.TokenID)
	}
	return "session:" + itoa(p.UserID())
}

func itoa(n uint) string {
	return strconv.FormatUint(uint64(n), 10)
}

// ---------- 限流：固定窗口计数，进程内即可满足单实例部署 ----------

type windowLimiter struct {
	mu      sync.Mutex
	window  time.Duration
	buckets map[string]*bucket
}

type bucket struct {
	start time.Time
	count int
}

func newWindowLimiter(window time.Duration) *windowLimiter {
	l := &windowLimiter{window: window, buckets: map[string]*bucket{}}
	go func() {
		for range time.Tick(5 * time.Minute) {
			l.mu.Lock()
			now := time.Now()
			for k, b := range l.buckets {
				if now.Sub(b.start) > l.window {
					delete(l.buckets, k)
				}
			}
			l.mu.Unlock()
		}
	}()
	return l
}

func (l *windowLimiter) Allow(key string, limit int) bool {
	if limit <= 0 {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	b := l.buckets[key]
	if b == nil || now.Sub(b.start) > l.window {
		l.buckets[key] = &bucket{start: now, count: 1}
		return true
	}
	if b.count >= limit {
		return false
	}
	b.count++
	return true
}

var (
	agentLimiter = newWindowLimiter(time.Minute)
	authLimiter  = newWindowLimiter(10 * time.Minute)
	chatLimiter  = newWindowLimiter(time.Minute)
)

// AuthRateLimit 登录/注册按 IP 限流，防暴力破解。
func AuthRateLimit(limit int) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !authLimiter.Allow("auth:"+c.ClientIP(), limit) {
			c.Header("Retry-After", "600")
			utils.Error(c, http.StatusTooManyRequests, 42900, "尝试次数过多，请 10 分钟后再试")
			c.Abort()
			return
		}
		c.Next()
	}
}

// ChatRateLimit AI 对话按用户限流，控制模型调用成本。
func ChatRateLimit(limit int) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !chatLimiter.Allow("chat:"+itoa(auth.UID(c)), limit) {
			utils.Error(c, http.StatusTooManyRequests, 42900, "说得太快啦，歇一会儿再聊")
			c.Abort()
			return
		}
		c.Next()
	}
}

func CORSMiddleware() gin.HandlerFunc {
	allowAll := false
	allowed := map[string]bool{}
	for _, o := range config.C.CORSOrigins {
		if o == "*" {
			allowAll = true
		}
		allowed[o] = true
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		switch {
		case allowAll:
			c.Header("Access-Control-Allow-Origin", "*")
		case origin != "" && allowed[strings.TrimRight(origin, "/")]:
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
		}
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Agent-Token, Mcp-Session-Id, Mcp-Protocol-Version")
		c.Header("Access-Control-Expose-Headers", "Mcp-Session-Id")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// SecurityHeaders 基础安全响应头。不设置 X-Frame-Options，以便站点作为小程序 web-view 页面加载。
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Next()
	}
}

// LoggerMiddleware 结构化访问日志：只记录 API 请求，跳过静态资源，避免刷屏。
func LoggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		path := c.Request.URL.Path
		if !strings.HasPrefix(path, "/api") && path != "/mcp" {
			return
		}
		status := c.Writer.Status()
		if status < 400 && !gin.IsDebugging() && time.Since(start) < 500*time.Millisecond {
			return
		}
		log.Printf("[api] %s %s %d %s uid=%d ip=%s", c.Request.Method, path, status, time.Since(start).Round(time.Millisecond), auth.UID(c), c.ClientIP())
	}
}

func CacheControlMiddleware(maxAge int) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "public, max-age="+itoa(uint(maxAge))+", immutable")
		c.Next()
	}
}
