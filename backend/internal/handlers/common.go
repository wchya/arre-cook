package handlers

import (
	"ninimenu/internal/auth"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func pageParams(c *gin.Context, def int) (int, int) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", c.DefaultQuery("page_size", strconv.Itoa(def))))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = def
	}
	return page, pageSize
}

func uid(c *gin.Context) uint { return auth.UID(c) }

func isAdmin(c *gin.Context) bool { return auth.CurrentUser(c).IsAdmin() }

func queryBool(v string) bool {
	v = strings.ToLower(strings.TrimSpace(v))
	return v == "1" || v == "true" || v == "yes"
}

func splitParam(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '，' || r == '、' || r == '|' })
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func todayStr() string { return time.Now().Format("2006-01-02") }
