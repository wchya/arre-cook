package config

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Config struct {
	Port           string
	Env            string // development / production
	AdminUsername  string
	AdminPassword  string
	JWTSecret      string
	JWTExpire      time.Duration
	AllowRegister  bool
	CORSOrigins    []string
	RepeatDays     int
	DBDriver       string // mysql or sqlite
	DBPath         string
	DBDSN          string
	DBMaxOpenConns int
	DBMaxIdleConns int
	DBConnMaxLife  time.Duration
	UploadDir      string
	BackupDir      string
	MaxUploadSize  int64
	CompressMaxDim int
	JpegQuality    int

	// 微信小程序登录（code2session）
	WechatAppID  string
	WechatSecret string

	// 邮箱验证码登录（默认 QQ 邮箱 SMTP，465 端口 SSL）
	AdminEmail       string
	SMTPHost         string
	SMTPPort         int
	SMTPUser         string
	SMTPPassword     string // QQ 邮箱为“授权码”，不是 QQ 密码
	SMTPFromName     string
	EmailCodeTTL     time.Duration
	EmailDomains     []string // 允许登录的邮箱域名白名单，空 = 不限制
	EmailCodeDevEcho bool     // 开发环境未配置 SMTP 时把验证码打印到日志

	// 图片对象存储（S3 兼容，默认对接博客站的 Garage）。未配置密钥时退回本地 UPLOAD_DIR。
	S3Endpoint  string
	S3Region    string
	S3Bucket    string
	S3AccessKey string
	S3SecretKey string
	S3PathStyle bool
	S3PublicURL string // 写进图片地址的前缀：/uploads/ = 由本服务回源读取；也可填 https://arrebyte.top/uploads/
	S3Prefix    string // 本应用的对象前缀，便于识别和按用户管理

	// 站内 AI 助手：OpenAI 兼容接口（默认 DeepSeek），可被管理后台设置覆盖
	LLMBaseURL string
	LLMAPIKey  string
	LLMModel   string
	// CPA 配置文件：生产环境读取线上 DSH 的 provider override。
	CPAConfigPath string
	CPABaseURL    string
	CPAModel      string

	// 嵌入式智能体会话令牌有效期（父页通过 postMessage 交给 iframe 内的智能体）
	AgentSessionTTL time.Duration
	// 每个智能体令牌每分钟最多请求数
	AgentRateLimit int
	// 公网访问地址，用于生成 MCP / OpenAPI 文档里的完整 URL（可空，空则按请求 Host 推断）
	PublicURL string
}

var C Config

const defaultJWTSecret = "ninimenu-secret-key"

func Load() {
	loadDotEnv()

	adminPassword := getEnv("ADMIN_PASSWORD", "nini123")
	C = Config{
		Port:            getEnv("PORT", "8080"),
		Env:             strings.ToLower(getEnv("APP_ENV", getEnv("GIN_MODE", "development"))),
		AdminUsername:   getEnv("ADMIN_USERNAME", "admin"),
		AdminPassword:   adminPassword,
		JWTSecret:       getEnv("JWT_SECRET", defaultJWTSecret),
		JWTExpire:       getEnvDuration("JWT_EXPIRE", 30*24*time.Hour),
		AllowRegister:   getEnvBool("ALLOW_REGISTER", true),
		CORSOrigins:     splitList(getEnv("CORS_ORIGINS", "*")),
		RepeatDays:      3,
		DBDriver:        strings.ToLower(getEnv("DB_DRIVER", "sqlite")),
		DBPath:          getEnv("DB_PATH", "data/ninimenu.db"),
		DBDSN:           getEnv("MYSQL_DSN", ""),
		DBMaxOpenConns:  getEnvInt("DB_MAX_OPEN_CONNS", 20),
		DBMaxIdleConns:  getEnvInt("DB_MAX_IDLE_CONNS", 5),
		DBConnMaxLife:   getEnvDuration("DB_CONN_MAX_LIFETIME", 30*time.Minute),
		UploadDir:       getEnv("UPLOAD_DIR", "uploads"),
		BackupDir:       getEnv("BACKUP_DIR", "uploads_backup"),
		MaxUploadSize:   int64(getEnvInt("MAX_UPLOAD_SIZE_MB", 5)) * 1024 * 1024,
		CompressMaxDim:  getEnvInt("COMPRESS_MAX_DIM", 1200),
		JpegQuality:     getEnvInt("JPEG_QUALITY", 85),
		WechatAppID:     getEnv("WECHAT_APPID", ""),
		WechatSecret:    getEnv("WECHAT_SECRET", ""),
		AdminEmail:      strings.ToLower(getEnv("ADMIN_EMAIL", "")),
		SMTPHost:        getEnv("SMTP_HOST", "smtp.qq.com"),
		SMTPPort:        getEnvInt("SMTP_PORT", 465),
		SMTPUser:        getEnv("SMTP_USER", getEnv("SMTP_USERNAME", "")),
		SMTPPassword:    getEnv("SMTP_PASSWORD", getEnv("SMTP_PASS", "")),
		SMTPFromName:    getEnv("SMTP_FROM_NAME", "NiniMenu"),
		EmailCodeTTL:    getEnvDuration("EMAIL_CODE_TTL", 10*time.Minute),
		EmailDomains:    splitList(strings.ToLower(getEnv("EMAIL_DOMAINS", ""))),
		S3Endpoint:      strings.TrimRight(getEnv("S3_ENDPOINT", "http://127.0.0.1:3900"), "/"),
		S3Region:        getEnv("S3_REGION", "garage"),
		S3Bucket:        getEnv("S3_BUCKET", "cook-uploads"),
		S3AccessKey:     getEnv("S3_ACCESS_KEY", getEnv("COOK_S3_ACCESS_KEY", "")),
		S3SecretKey:     getEnv("S3_SECRET_KEY", getEnv("COOK_S3_SECRET_KEY", "")),
		S3PathStyle:     getEnvBool("S3_PATH_STYLE", true),
		S3PublicURL:     ensureSlash(getEnv("S3_PUBLIC_URL", "/uploads/")),
		S3Prefix:        strings.Trim(getEnv("S3_PREFIX", "cook"), "/"),
		LLMBaseURL:      strings.TrimRight(getEnv("LLM_BASE_URL", "https://api.deepseek.com"), "/"),
		LLMAPIKey:       getEnv("LLM_API_KEY", getEnv("DEEPSEEK_API_KEY", "")),
		LLMModel:        getEnv("LLM_MODEL", "deepseek-chat"),
		CPAConfigPath:   getEnv("LLM_CPA_CONFIG_PATH", getEnv("CPA_CONFIG_PATH", "")),
		CPABaseURL:      strings.TrimRight(getEnv("LLM_CPA_BASE_URL", getEnv("CPA_BASE_URL", "")), "/"),
		CPAModel:        getEnv("LLM_CPA_MODEL", getEnv("CPA_MODEL", "")),
		AgentSessionTTL: getEnvDuration("AGENT_SESSION_TTL", 2*time.Hour),
		AgentRateLimit:  getEnvInt("AGENT_RATE_LIMIT", 120),
		PublicURL:       strings.TrimRight(getEnv("PUBLIC_URL", ""), "/"),
	}
	if C.DBDriver == "mysql" && C.DBDSN == "" {
		C.DBDSN = mysqlDSNFromEnv()
	}
	if C.DBDriver != "mysql" {
		C.DBDriver = "sqlite"
	}
	if d := getEnvInt("REPEAT_DAYS", 0); d > 0 {
		C.RepeatDays = d
	}
	if C.JWTExpire < time.Hour {
		C.JWTExpire = 30 * 24 * time.Hour
	}
	C.EmailCodeDevEcho = !C.IsProduction() && !C.SMTPEnabled()
	if C.IsProduction() && !C.SMTPEnabled() {
		log.Println("[配置警告] 未配置 SMTP_USER / SMTP_PASSWORD，邮箱验证码无法发送，用户将无法登录")
	}

	if C.IsProduction() {
		if C.JWTSecret == defaultJWTSecret || len(C.JWTSecret) < 16 {
			log.Println("[安全警告] JWT_SECRET 使用了默认值或过短，生产环境请设置 32 位以上随机字符串")
		}
		if C.AdminPassword == "nini123" {
			log.Println("[安全警告] ADMIN_PASSWORD 仍是默认值 nini123，请尽快修改（仅在首次创建管理员时生效，之后请在 App 内改密）")
		}
	}
}

func mysqlDSNFromEnv() string {
	host := getEnv("MYSQL_HOST", "127.0.0.1")
	port := getEnv("MYSQL_PORT", "3306")
	user := getEnv("MYSQL_USER", "ninimenu")
	password := getEnv("MYSQL_PASSWORD", "")
	database := getEnv("MYSQL_DATABASE", "ninimenu")
	// parseTime and utf8mb4 keep time fields and Chinese content consistent across drivers.
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Asia%%2FShanghai", user, password, host, port, database)
}

func (c Config) IsProduction() bool {
	return c.Env == "production" || c.Env == "release"
}

func (c Config) WechatEnabled() bool {
	return c.WechatAppID != "" && c.WechatSecret != ""
}

func (c Config) SMTPEnabled() bool {
	return c.SMTPUser != "" && c.SMTPPassword != ""
}

func (c Config) S3Enabled() bool {
	return c.S3Endpoint != "" && c.S3Bucket != "" && c.S3AccessKey != "" && c.S3SecretKey != ""
}

func ensureSlash(s string) string {
	if s == "" || strings.HasSuffix(s, "/") {
		return s
	}
	return s + "/"
}

func loadDotEnv() {
	candidates := []string{".env"}
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		candidates = append(candidates, filepath.Join(exeDir, ".env"), filepath.Join(exeDir, "env.bak"))
	}
	for _, p := range candidates {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
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
			v = strings.Trim(strings.TrimSpace(v), `"'`)
			if os.Getenv(k) == "" {
				os.Setenv(k, v)
			}
		}
		f.Close()
		return
	}
}

func getEnv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
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

func getEnvBool(key string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	if d, err := time.ParseDuration(v); err == nil && d > 0 {
		return d
	}
	// 兼容 "30d" 写法
	if strings.HasSuffix(v, "d") {
		var n int
		if _, err := fmt.Sscanf(strings.TrimSuffix(v, "d"), "%d", &n); err == nil && n > 0 {
			return time.Duration(n) * 24 * time.Hour
		}
	}
	return fallback
}

func splitList(raw string) []string {
	var out []string
	for _, p := range strings.Split(raw, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, strings.TrimRight(p, "/"))
		}
	}
	return out
}
