package services

import (
	"errors"
	"fmt"
	"ninimenu/internal/config"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"ninimenu/internal/testutil"
	"os"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	cleanup, err := testutil.InitDB()
	if err != nil {
		fmt.Println("init db:", err)
		os.Exit(1)
	}
	code := m.Run()
	cleanup()
	os.Exit(code)
}

// seedCode 模拟“已发送”的验证码（真实发送走 SMTP，这里直接落库）。
func seedCode(t *testing.T, email, code string) {
	t.Helper()
	rec := models.EmailCode{Email: email, Purpose: "login", CodeHash: hashCode(email, "login", code), IP: "127.0.0.1", ExpiresAt: time.Now().Add(10 * time.Minute)}
	if err := database.DB.Create(&rec).Error; err != nil {
		t.Fatal(err)
	}
}

func TestEmailCodeLoginFlow(t *testing.T) {
	email := "newbie@qq.com"
	seedCode(t, email, "123456")

	if _, _, err := LoginWithEmailCode(email, "000000", ""); !errors.Is(err, ErrCodeInvalid) {
		t.Fatalf("wrong code err = %v", err)
	}
	u, created, err := LoginWithEmailCode(" NewBie@QQ.com ", "123456", "")
	if err != nil || !created || u.Email == nil || *u.Email != email || u.Role != models.RoleUser {
		t.Fatalf("first login: user=%+v created=%v err=%v", u, created, err)
	}
	// 验证码一次性
	if _, _, err := LoginWithEmailCode(email, "123456", ""); err == nil {
		t.Fatal("code must be single-use")
	}
	// 再次登录不重复建号
	seedCode(t, email, "654321")
	u2, created, err := LoginWithEmailCode(email, "654321", "")
	if err != nil || created || u2.ID != u.ID {
		t.Fatalf("second login: id=%d created=%v err=%v", u2.ID, created, err)
	}
}

func TestEmailCodeAttemptLimit(t *testing.T) {
	email := "brute@qq.com"
	seedCode(t, email, "111111")
	var err error
	for i := 0; i < codeMaxAttempts; i++ {
		_, _, err = LoginWithEmailCode(email, "999999", "")
	}
	if !errors.Is(err, ErrCodeTooMany) {
		t.Fatalf("after %d wrong attempts err = %v", codeMaxAttempts, err)
	}
	if _, _, err := LoginWithEmailCode(email, "111111", ""); err == nil {
		t.Fatal("correct code must be rejected after too many attempts")
	}
}

func TestAdminEmailGetsAdminRole(t *testing.T) {
	old := config.C.AdminEmail
	config.C.AdminEmail = "boss@qq.com"
	defer func() { config.C.AdminEmail = old }()
	seedCode(t, "boss@qq.com", "222222")
	u, _, err := LoginWithEmailCode("boss@qq.com", "222222", "")
	if err != nil || u.Role != models.RoleAdmin {
		t.Fatalf("admin email login: role=%v err=%v", u, err)
	}
}

func TestSendEmailCodeCooldownAndValidation(t *testing.T) {
	if _, err := SendEmailCode("not-an-email", "login", "1.1.1.1"); !errors.Is(err, ErrInvalidEmail) {
		t.Fatalf("invalid email err = %v", err)
	}
	// 测试环境未配置 SMTP：开发回显模式下直接成功
	config.C.EmailCodeDevEcho = true
	if _, err := SendEmailCode("cool@qq.com", "login", "1.1.1.2"); err != nil {
		t.Fatalf("first send: %v", err)
	}
	if wait, err := SendEmailCode("cool@qq.com", "login", "1.1.1.2"); err == nil || wait <= 0 {
		t.Fatalf("second send should hit cooldown, wait=%d err=%v", wait, err)
	}
}

func TestEmailDomainWhitelist(t *testing.T) {
	old := config.C.EmailDomains
	config.C.EmailDomains = []string{"qq.com", "foxmail.com"}
	defer func() { config.C.EmailDomains = old }()
	if _, err := NormalizeEmail("a@gmail.com"); !errors.Is(err, ErrEmailDomain) {
		t.Fatalf("gmail should be rejected: %v", err)
	}
	if e, err := NormalizeEmail("A@QQ.COM"); err != nil || e != "a@qq.com" {
		t.Fatalf("qq email: %q %v", e, err)
	}
}
