package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"ninimenu/internal/config"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAIIngressOriginAndPayloadBounds(t *testing.T) {
	previous := config.C
	t.Cleanup(func() { config.C = previous })
	config.C.Env = "production"
	config.C.PublicURL = "https://cook.example.test"
	config.C.CORSOrigins = []string{"*"}
	router := gin.New()
	router.Use(AIIngress())
	router.POST("/api/assistant/chat", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	for i, row := range []struct {
		origin, body, authorization string
		want                        int
	}{
		{"https://cook.example.test", `{}`, "", http.StatusNoContent},
		{"", `{}`, "", http.StatusNoContent}, // Native/CLI clients still need downstream authentication.
		{"https://external.example.test", `{}`, "", http.StatusForbidden},
		{"https://cook.example.test.evil.test", `{}`, "", http.StatusForbidden},
		{"null", `{}`, "", http.StatusForbidden},
		{"https://cook.example.test/path", `{}`, "", http.StatusForbidden},
		{"https://cook.example.test", strings.Repeat("x", 16385), "", http.StatusRequestEntityTooLarge},
		{"", `{}`, strings.Repeat("x", 4097), http.StatusUnauthorized},
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/assistant/chat", strings.NewReader(row.body))
		req.RemoteAddr = fmt.Sprintf("198.51.100.%d:1000", i+1)
		req.Header.Set("Origin", row.origin)
		req.Header.Set("Authorization", row.authorization)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != row.want {
			t.Fatalf("case %d: got %d, want %d", i, w.Code, row.want)
		}
	}
}

func TestAIIngressLimitsRandomTokensBeforeAuthentication(t *testing.T) {
	router := gin.New()
	_ = router.SetTrustedProxies(nil)
	lookups := 0
	router.Use(AIIngress())
	router.GET("/api/agent/me", func(c *gin.Context) { lookups++; c.Status(http.StatusUnauthorized) })
	for i := 0; i <= 240; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/agent/me", nil)
		req.RemoteAddr = "203.0.113.202:1000"
		req.Header.Set("Authorization", fmt.Sprintf("Bearer nm_random%d", i))
		// Untrusted callers cannot rotate the ingress key with forwarded headers.
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("192.0.2.%d", i))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		want := http.StatusUnauthorized
		if i == 240 {
			want = http.StatusTooManyRequests
		}
		if w.Code != want {
			t.Fatalf("request %d: got %d, want %d", i, w.Code, want)
		}
	}
	if lookups != 240 {
		t.Fatalf("authentication reached %d times", lookups)
	}
}

func TestAILocalDevelopmentOriginsDoNotLoosenProduction(t *testing.T) {
	previous := config.C
	t.Cleanup(func() { config.C = previous })
	config.C.PublicURL = ""
	config.C.CORSOrigins = []string{"*"}
	config.C.Env = "development"
	if !allowedAIOrigin("http://127.0.0.1:5173", "localhost:8080") {
		t.Fatal("local Vite proxy rejected")
	}
	if allowedAIOrigin("https://external.example.test", "localhost:8080") {
		t.Fatal("external origin accepted in development")
	}
	config.C.Env = "production"
	if allowedAIOrigin("http://127.0.0.1:5173", "localhost:8080") {
		t.Fatal("production trusts a local Host without explicit origin")
	}
}
