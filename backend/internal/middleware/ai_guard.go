package middleware

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/url"
	"ninimenu/internal/auth"
	"ninimenu/internal/config"
	"ninimenu/internal/utils"
	"strings"
	"time"
)

import "github.com/gin-gonic/gin"

var aiIngressLimiter = newWindowLimiter(time.Minute)
var videoLimiter = newWindowLimiter(time.Minute)

// AIIngress applies before authentication so random tokens cannot cause unbounded
// database lookups. Origin checks protect browser embedding; credentials, account
// budgets and scopes remain mandatory for clients that can forge HTTP headers.
func AIIngress() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		protected := path == "/mcp" || path == "/api/link-preview" || strings.HasPrefix(path, "/api/agent/") || strings.HasPrefix(path, "/api/assistant/") || strings.HasPrefix(path, "/api/me/agent-") || strings.HasPrefix(path, "/api/auth/")
		if !protected {
			c.Next()
			return
		}
		if !aiIngressLimiter.Allow("ai-ingress:"+c.ClientIP(), 240) {
			c.Header("Retry-After", "60")
			utils.Error(c, http.StatusTooManyRequests, 42900, "请求过于频繁，请稍后再试")
			c.Abort()
			return
		}
		if origin := c.GetHeader("Origin"); origin != "" && !allowedAIOrigin(origin, c.Request.Host) {
			utils.Forbidden(c, "请从食谱站或小程序使用此功能")
			c.Abort()
			return
		}
		if len(c.GetHeader("Authorization")) > 4096 || len(c.GetHeader("X-Agent-Token")) > 4096 {
			utils.Unauthorized(c, "登录凭证无效")
			c.Abort()
			return
		}
		limit := int64(256 * 1024)
		if path == "/api/assistant/chat" || strings.HasPrefix(path, "/api/auth/") {
			limit = 16 * 1024
		}
		if path == "/api/assistant/video-recipe" {
			limit = 40 * 1024
		}
		if c.Request.Body != nil && c.Request.Method != http.MethodGet && c.Request.Method != http.MethodOptions {
			reader := http.MaxBytesReader(c.Writer, c.Request.Body, limit)
			body, err := io.ReadAll(reader)
			_ = reader.Close()
			if err != nil {
				utils.Error(c, http.StatusRequestEntityTooLarge, 41300, "请求内容过大或读取失败，请精简后重试")
				c.Abort()
				return
			}
			c.Request.Body = io.NopCloser(bytes.NewReader(body))
		}
		c.Next()
	}
}

func VideoRateLimit(limit int) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !videoLimiter.Allow(c.FullPath()+":"+itoa(auth.UID(c)), limit) {
			c.Header("Retry-After", "60")
			utils.Error(c, http.StatusTooManyRequests, 42900, "视频请求过于频繁，请稍后重试")
			c.Abort()
			return
		}
		c.Next()
	}
}

func allowedAIOrigin(origin, requestHost string) bool {
	parsed, err := url.Parse(origin)
	if err != nil || parsed.User != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return false
	}
	normalized := strings.TrimRight(origin, "/")
	if config.C.PublicURL != "" && normalized == strings.TrimRight(config.C.PublicURL, "/") {
		return true
	}
	for _, configured := range config.C.CORSOrigins {
		if configured != "*" && normalized == strings.TrimRight(configured, "/") {
			return true
		}
	}
	// The local Vite proxy and API use different loopback ports/aliases.
	// Never derive production trust from Host or accept remote development origins implicitly.
	if !config.C.IsProduction() && config.C.PublicURL == "" {
		loopback := func(host string) bool {
			return host == "localhost" || net.ParseIP(host).IsLoopback()
		}
		apiURL := url.URL{Host: requestHost}
		return parsed.Host == requestHost || (loopback(parsed.Hostname()) && loopback(apiURL.Hostname()))
	}
	return false
}

// ReserveAgentRequests counts REST, MCP batches, PATs and short-lived sessions
// against the same user. Rotating or minting another token cannot add capacity.
func ReserveAgentRequests(p *auth.Principal, units int) bool {
	limit := config.C.AgentRateLimit
	if limit <= 0 {
		limit = 120
	}
	limit = min(limit, 600)
	return agentLimiter.AllowN(agentLimitKey(p), limit, units)
}
