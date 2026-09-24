// Package testutil 为集成测试准备隔离的临时数据库与配置。
package testutil

import (
	"ninimenu/internal/auth"
	"ninimenu/internal/config"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"os"
	"path/filepath"
)

// InitDB 在临时目录里初始化一套全新的数据库（含种子菜谱与初始管理员），返回清理函数。
func InitDB() (cleanup func(), err error) {
	dir, err := os.MkdirTemp("", "ninimenu-test-*")
	if err != nil {
		return nil, err
	}
	for _, k := range []string{"SMTP_USER", "SMTP_PASSWORD", "S3_ACCESS_KEY", "S3_SECRET_KEY", "LLM_API_KEY", "DEEPSEEK_API_KEY", "WECHAT_APPID", "ADMIN_EMAIL"} {
		os.Unsetenv(k)
	}
	os.Setenv("APP_ENV", "test")
	os.Setenv("DB_PATH", filepath.Join(dir, "test.db"))
	os.Setenv("UPLOAD_DIR", filepath.Join(dir, "uploads"))
	os.Setenv("BACKUP_DIR", filepath.Join(dir, "backup"))
	os.Setenv("JWT_SECRET", "test-secret-test-secret-test-secret")
	os.Setenv("AGENT_RATE_LIMIT", "10000")
	config.Load()
	database.PasswordHasher = auth.HashPassword
	if err := database.Init(); err != nil {
		return nil, err
	}
	return func() {
		if sqlDB, err := database.DB.DB(); err == nil {
			sqlDB.Close()
		}
		os.RemoveAll(dir)
	}, nil
}

// NewUser 直接建一个普通用户并签发登录令牌。
func NewUser(email string) (models.User, string, error) {
	u := models.User{Email: &email, Nickname: email, Role: models.RoleUser, TokenVersion: 1}
	if err := database.DB.Create(&u).Error; err != nil {
		return u, "", err
	}
	token, _, err := auth.IssueUserToken(&u)
	return u, token, err
}
