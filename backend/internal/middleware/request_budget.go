package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"ninimenu/internal/auth"
	"ninimenu/internal/database"
	"ninimenu/internal/utils"
)

var apiSlots = make(chan struct{}, 16)

// SSE answers hold a request for up to 90 seconds. They are already bounded by
// the shared assistant lease (four site-wide), so keep them out of the pool
// that every page load needs.
var streamSlots = make(chan struct{}, 6)
var passwordSlots = make(chan struct{}, 2)
var largeReadSlots = make(chan struct{}, 2)
var apiLimiter = newWindowLimiter(time.Minute)
var passwordLimiter = newWindowLimiter(time.Minute)

// busyRejections counts admission refusals so /healthz can expose saturation.
var busyRejections atomic.Int64

// BusyRejections 自进程启动以来因并发预算已满而拒绝的请求数。
func BusyRejections() int64 { return busyRejections.Load() }

// admissionWait absorbs short bursts (a page firing several requests at once)
// instead of failing them the instant every slot is taken.
const admissionWait = 300 * time.Millisecond

func acquireSlot(ctx context.Context, slots chan struct{}) bool {
	select {
	case slots <- struct{}{}:
		return true
	default:
	}
	timer := time.NewTimer(admissionWait)
	defer timer.Stop()
	select {
	case slots <- struct{}{}:
		return true
	case <-ctx.Done():
		return false
	case <-timer.C:
		return false
	}
}

func isStreamPath(path string) bool {
	return path == "/api/assistant/chat" || path == "/api/assistant/video-recipe"
}

// Admission precedes authentication and body allocation. Long-running AI calls
// retain their own shorter sub-operation deadlines and a 90 second total budget.
func RequestBudget() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if !strings.HasPrefix(path, "/api/") && path != "/mcp" {
			c.Next()
			return
		}
		if len(c.GetHeader("Authorization")) > 4096 || len(c.GetHeader("X-Agent-Token")) > 4096 {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		if !apiLimiter.Allow(clientKey(c), 360) {
			rejectBusy(c)
			return
		}
		slots := apiSlots
		if isStreamPath(path) {
			slots = streamSlots
		}
		if !acquireSlot(c.Request.Context(), slots) {
			rejectSaturated(c)
			return
		}
		defer func() { <-slots }()
		if c.Request.Method == http.MethodGet && (strings.HasSuffix(path, "/export") || path == "/api/dishes" || path == "/api/agent/dishes") {
			if !acquireSlot(c.Request.Context(), largeReadSlots) {
				rejectSaturated(c)
				return
			}
			defer func() { <-largeReadSlots }()
		}
		timeout := 15 * time.Second
		if strings.HasPrefix(path, "/api/assistant/") || strings.HasPrefix(path, "/api/agent/") || path == "/mcp" {
			timeout = 90 * time.Second
		}
		if path == "/api/upload/image" || path == "/api/auth/email/code" {
			timeout = 30 * time.Second
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		// Uploads stream through a separate multipart byte limit after admission.
		if path != "/api/upload/image" && c.Request.Body != nil {
			limit := int64(64 << 10)
			if path == "/api/dishes" || strings.HasPrefix(path, "/api/dishes/") || path == "/mcp" || strings.HasPrefix(path, "/api/agent/") {
				limit = 256 << 10
			}
			reader := http.MaxBytesReader(c.Writer, c.Request.Body, limit)
			body, err := io.ReadAll(reader)
			_ = reader.Close()
			if err != nil {
				utils.Error(c, 413, 41300, "请求内容过大或读取失败")
				c.Abort()
				return
			}
			if len(body) > 0 && strings.Contains(c.GetHeader("Content-Type"), "application/json") {
				decoder := json.NewDecoder(bytes.NewReader(body))
				decoder.UseNumber()
				var value any
				if decoder.Decode(&value) == nil {
					nodes := 0
					if !boundedJSON(value, 0, &nodes) {
						utils.Error(c, 413, 41300, "请求条目或层级过多")
						c.Abort()
						return
					}
				}
			}
			c.Request.Body = io.NopCloser(bytes.NewReader(body))
		}
		c.Next()
	}
}

func rejectBusy(c *gin.Context) {
	c.Header("Retry-After", "60")
	utils.Error(c, 429, 42900, "请求过于频繁，请稍后重试")
	c.Abort()
}

// rejectSaturated is server capacity, not client behaviour: say so, and retry soon.
func rejectSaturated(c *gin.Context) {
	busyRejections.Add(1)
	c.Header("Retry-After", "3")
	utils.Error(c, 429, 42900, "服务繁忙，请稍后重试")
	c.Abort()
}

func PasswordBudget() gin.HandlerFunc {
	return func(c *gin.Context) {
		key := "ip:" + clientKey(c)
		if auth.UID(c) != 0 {
			key = "user:" + itoa(auth.UID(c))
		}
		if !passwordLimiter.Allow(key, 10) || !database.ReserveWindow(database.DB.WithContext(c.Request.Context()), "password:"+key, time.Minute, 10, 1) {
			rejectBusy(c)
			return
		}
		select {
		case passwordSlots <- struct{}{}:
			defer func() { <-passwordSlots }()
		default:
			rejectBusy(c)
			return
		}
		c.Next()
	}
}

func boundedJSON(value any, depth int, nodes *int) bool {
	*nodes++
	if depth > 16 || *nodes > 5000 {
		return false
	}
	switch v := value.(type) {
	case []any:
		if len(v) > 500 {
			return false
		}
		for _, item := range v {
			if !boundedJSON(item, depth+1, nodes) {
				return false
			}
		}
	case map[string]any:
		if len(v) > 100 {
			return false
		}
		for key, item := range v {
			if len(key) > 128 || !boundedJSON(item, depth+1, nodes) {
				return false
			}
		}
	}
	return true
}
