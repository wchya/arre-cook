package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Port           string
	AppPassword    string
	AdminPassword  string
	AgentToken     string
	JWTSecret      string
	JWTExpire      string
	RepeatDays     int
	DBPath         string
	UploadDir      string
	BackupDir      string
	MaxUploadSize  int64
	CompressMaxDim int
	JpegQuality    int
}

var C Config

func Load() {
	loadDotEnv()

	C = Config{
		Port:          getEnv("PORT", "8080"),
		AppPassword:   getEnv("APP_PASSWORD", getEnv("ADMIN_PASSWORD", "nini123")),
		AdminPassword: getEnv("ADMIN_PASSWORD", "nini123"),
		// 智能体访问令牌：/api/agent/* 全量能力接口的凭证，未设置时沿用管理端密码
		AgentToken:     getEnv("AGENT_TOKEN", getEnv("ADMIN_PASSWORD", "nini123")),
		JWTSecret:      getEnv("JWT_SECRET", "ninimenu-secret-key"),
		JWTExpire:      getEnv("JWT_EXPIRE", "24h"),
		RepeatDays:     3,
		DBPath:         getEnv("DB_PATH", "data/ninimenu.db"),
		UploadDir:      getEnv("UPLOAD_DIR", "uploads"),
		BackupDir:      getEnv("BACKUP_DIR", "uploads_backup"),
		MaxUploadSize:  int64(getEnvInt("MAX_UPLOAD_SIZE_MB", 5)) * 1024 * 1024,
		CompressMaxDim: getEnvInt("COMPRESS_MAX_DIM", 1200),
		JpegQuality:    getEnvInt("JPEG_QUALITY", 85),
	}
	if v := os.Getenv("REPEAT_DAYS"); v != "" {
		var d int
		if _, err := fmt.Sscanf(v, "%d", &d); err == nil && d > 0 {
			C.RepeatDays = d
		}
	}
}

func loadDotEnv() {
	exePath, err := os.Executable()
	if err != nil {
		return
	}
	exeDir := filepath.Dir(exePath)
	for _, name := range []string{".env", "env.bak"} {
		p := filepath.Join(exeDir, name)
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			k, v, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			k = strings.TrimSpace(k)
			v = strings.TrimSpace(v)
			if os.Getenv(k) == "" {
				os.Setenv(k, v)
			}
		}
		break
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			return n
		}
	}
	return fallback
}
