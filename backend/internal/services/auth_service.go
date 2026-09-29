package services

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"net/mail"
	"net/url"
	"ninimenu/internal/auth"
	"ninimenu/internal/config"
	"ninimenu/internal/database"
	"ninimenu/internal/mailer"
	"ninimenu/internal/models"
	"strings"
	"time"

	"gorm.io/gorm"
)

var (
	ErrInvalidEmail   = errors.New("邮箱格式不正确")
	ErrEmailDomain    = errors.New("暂不支持该邮箱，请使用 QQ 邮箱")
	ErrCodeInvalid    = errors.New("验证码错误或已过期")
	ErrCodeTooMany    = errors.New("验证码错误次数过多，请重新获取")
	ErrAccountLocked  = errors.New("账号已被停用，请联系管理员")
	ErrBadCredentials = errors.New("账号或密码错误")
)

const (
	codeCooldown       = 60 * time.Second
	codeMaxAttempts    = 5
	codePerEmailHourly = 6
	codePerIPHourly    = 20
)

// NormalizeEmail 校验并规范化邮箱（小写、去空格），按白名单限制域名。
func NormalizeEmail(raw string) (string, error) {
	raw = strings.ToLower(strings.TrimSpace(raw))
	addr, err := mail.ParseAddress(raw)
	if err != nil || addr.Address != raw || len(raw) > 128 || !strings.Contains(raw, ".") {
		return "", ErrInvalidEmail
	}
	if len(config.C.EmailDomains) > 0 {
		_, domain, _ := strings.Cut(raw, "@")
		ok := false
		for _, d := range config.C.EmailDomains {
			if domain == d {
				ok = true
				break
			}
		}
		if !ok {
			return "", ErrEmailDomain
		}
	}
	return raw, nil
}

func hashCode(email, purpose, code string) string {
	sum := sha256.Sum256([]byte(email + "|" + purpose + "|" + code + "|" + config.C.JWTSecret))
	return hex.EncodeToString(sum[:])
}

func randomDigits(n int) (string, error) {
	var b strings.Builder
	for i := 0; i < n; i++ {
		d, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", err
		}
		b.WriteByte(byte('0' + d.Int64()))
	}
	return b.String(), nil
}

// SendEmailCode 发送邮箱验证码：60 秒冷却、每邮箱每小时 6 次、每 IP 每小时 20 次。
// 返回距离可再次发送的秒数（用于前端倒计时）。
func SendEmailCode(rawEmail, purpose, ip string, dbs ...*gorm.DB) (int, error) {
	requestDB := database.Handle(dbs...)

	email, err := NormalizeEmail(rawEmail)
	if err != nil {
		return 0, err
	}
	if purpose == "" {
		purpose = "login"
	}
	now := time.Now()
	code, err := randomDigits(6)
	if err != nil {
		return 0, err
	}
	rec := models.EmailCode{Email: email, Purpose: purpose, CodeHash: hashCode(email, purpose, code), IP: ip, ExpiresAt: now.Add(config.C.EmailCodeTTL)}
	waitSeconds := 0
	err = requestDB.Transaction(func(tx *gorm.DB) error {
		// Lock only while reserving the rolling quotas, never during SMTP I/O.
		if err := database.LockKey(tx, "email-code-reservation"); err != nil {
			return err
		}
		var last models.EmailCode
		e := tx.Where("email = ? AND purpose = ?", email, purpose).Order("created_at DESC").First(&last).Error
		if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if e == nil {
			if wait := codeCooldown - now.Sub(last.CreatedAt); wait > 0 {
				waitSeconds = int(wait.Seconds()) + 1
				return errors.New("发送太频繁，请稍后再试")
			}
		}
		var n int64
		if err := tx.Model(&models.EmailCode{}).Where("email = ? AND created_at > ?", email, now.Add(-time.Hour)).Count(&n).Error; err != nil {
			return err
		}
		if n >= codePerEmailHourly {
			return errors.New("该邮箱获取验证码次数过多，请稍后再试")
		}
		if err := tx.Model(&models.EmailCode{}).Where("ip = ? AND created_at > ?", ip, now.Add(-time.Hour)).Count(&n).Error; err != nil {
			return err
		}
		if n >= codePerIPHourly {
			return errors.New("请求过于频繁，请稍后再试")
		}
		return tx.Create(&rec).Error
	})
	if err != nil {
		return waitSeconds, err
	}

	appName := database.GetSetting("app_name", "arre食谱推荐小助手", requestDB)
	subject := fmt.Sprintf("【%s】登录验证码 %s", appName, code)
	html := mailer.CodeEmailHTML(appName, code, int(config.C.EmailCodeTTL.Minutes()))
	if err := mailer.Send(email, subject, html, requestDB.Statement.Context); err != nil {
		if errors.Is(err, mailer.ErrNotConfigured) && config.C.EmailCodeDevEcho {
			log.Printf("[开发模式] 未配置 SMTP，%s 的验证码：%s", email, code)
			return int(codeCooldown.Seconds()), nil
		}
		requestDB.Model(&rec).Update("expires_at", time.Now())
		log.Printf("[mail] 发送验证码到 %s 失败: %v", email, err)
		return 0, errors.New("验证码发送失败，请稍后再试")
	}
	return int(codeCooldown.Seconds()), nil
}

// VerifyEmailCode 校验并消费验证码（一次性）。
func VerifyEmailCode(rawEmail, purpose, code string, dbs ...*gorm.DB) (string, error) {
	requestDB := database.Handle(dbs...)

	email, err := NormalizeEmail(rawEmail)
	if err != nil {
		return "", err
	}
	if purpose == "" {
		purpose = "login"
	}
	code = strings.TrimSpace(code)
	var rec models.EmailCode
	err = requestDB.Where("email = ? AND purpose = ? AND consumed_at IS NULL", email, purpose).
		Order("created_at DESC").First(&rec).Error
	if err != nil || time.Now().After(rec.ExpiresAt) {
		return "", ErrCodeInvalid
	}
	if rec.Attempts >= codeMaxAttempts {
		return "", ErrCodeTooMany
	}
	// Reserve the attempt before comparing: concurrent guesses must each consume
	// one of the five tries instead of all passing a stale attempts check.
	reserved := requestDB.Model(&models.EmailCode{}).
		Where("id = ? AND consumed_at IS NULL AND attempts < ?", rec.ID, codeMaxAttempts).
		UpdateColumn("attempts", gorm.Expr("attempts + 1"))
	if reserved.Error != nil {
		return "", reserved.Error
	}
	if reserved.RowsAffected == 0 {
		return "", ErrCodeTooMany
	}
	if rec.CodeHash != hashCode(email, purpose, code) {
		if rec.Attempts+1 >= codeMaxAttempts {
			return "", ErrCodeTooMany
		}
		return "", ErrCodeInvalid
	}
	now := time.Now()
	res := requestDB.Model(&models.EmailCode{}).Where("id = ? AND consumed_at IS NULL", rec.ID).Update("consumed_at", now)
	if res.RowsAffected == 0 {
		return "", ErrCodeInvalid
	}
	return email, nil
}

// LoginWithEmailCode 邮箱验证码登录：已注册直接登录，未注册自动创建账号。
// wechatCode 非空时（小程序里用邮箱登录）顺带把微信 openid 绑定到该账号。
func LoginWithEmailCode(rawEmail, code, wechatCode string, dbs ...*gorm.DB) (*models.User, bool, error) {
	requestDB := database.Handle(dbs...)

	email, err := VerifyEmailCode(rawEmail, "login", code, requestDB)
	if err != nil {
		return nil, false, err
	}
	var u models.User
	created := false
	err = requestDB.Where("email = ?", email).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if !RegistrationOpen(requestDB) && email != config.C.AdminEmail {
			return nil, false, errors.New("暂未开放注册")
		}
		local, _, _ := strings.Cut(email, "@")
		u = models.User{Email: &email, Nickname: truncateRunes(local, 20), Role: models.RoleUser, TokenVersion: 1}
		if config.C.AdminEmail != "" && email == config.C.AdminEmail {
			u.Role = models.RoleAdmin
		}
		if err := requestDB.Create(&u).Error; err != nil {
			return nil, false, err
		}
		appName := database.GetSetting("app_name", "arre食谱推荐小助手", requestDB)
		_, _ = CreateNotification(u.ID, "system_update", "欢迎来到 "+appName, "你可以记录每天吃的菜、生成健康饮食报告，并在 AI 连接中为 Hermes、DSH 等 Agent 创建独立凭证。", "/", requestDB)
		created = true
	} else if err != nil {
		return nil, false, err
	}
	if u.Disabled {
		return nil, false, ErrAccountLocked
	}
	if wechatCode != "" {
		if openid, unionid, err := code2Session(wechatCode); err == nil {
			bindWechat(&u, openid, unionid, requestDB)
		}
	}
	touchLogin(&u, requestDB)
	return &u, created, nil
}

// LoginWithPassword 密码登录（账号可以是邮箱或用户名）。主要给管理员在邮件服务不可用时兜底。
func LoginWithPassword(account, password string, dbs ...*gorm.DB) (*models.User, error) {
	requestDB := database.Handle(dbs...)

	account = strings.TrimSpace(account)
	var u models.User
	q := requestDB
	if strings.Contains(account, "@") {
		q = q.Where("email = ?", strings.ToLower(account))
	} else {
		q = q.Where("username = ?", account)
	}
	if err := q.First(&u).Error; err != nil || u.PasswordHash == "" || !auth.VerifyPassword(u.PasswordHash, password) {
		return nil, ErrBadCredentials
	}
	if u.Disabled {
		return nil, ErrAccountLocked
	}
	touchLogin(&u, requestDB)
	return &u, nil
}

// RegistrationOpen 是否允许新邮箱自动注册（环境变量与后台开关同时为开）。
func RegistrationOpen(dbs ...*gorm.DB) bool {
	requestDB := database.Handle(dbs...)

	return config.C.AllowRegister && database.GetSetting("allow_register", "1", requestDB) != "0"
}

func touchLogin(u *models.User, dbs ...*gorm.DB) {
	requestDB := database.Handle(dbs...)

	now := time.Now()
	u.LastLoginAt = &now
	requestDB.Model(&models.User{}).Where("id = ?", u.ID).Update("last_login_at", now)
}

// ---------------- 微信小程序 ----------------

type code2SessionResp struct {
	OpenID     string `json:"openid"`
	UnionID    string `json:"unionid"`
	SessionKey string `json:"session_key"`
	ErrCode    int    `json:"errcode"`
	ErrMsg     string `json:"errmsg"`
}

var wechatHTTP = &http.Client{Timeout: 8 * time.Second}

func code2Session(code string) (openid, unionid string, err error) {
	if !config.C.WechatEnabled() {
		return "", "", errors.New("未配置小程序 AppID / Secret")
	}
	q := url.Values{}
	q.Set("appid", config.C.WechatAppID)
	q.Set("secret", config.C.WechatSecret)
	q.Set("js_code", code)
	q.Set("grant_type", "authorization_code")
	resp, err := wechatHTTP.Get("https://api.weixin.qq.com/sns/jscode2session?" + q.Encode())
	if err != nil {
		return "", "", fmt.Errorf("请求微信登录接口失败: %w", err)
	}
	defer resp.Body.Close()
	var r code2SessionResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", "", err
	}
	if r.ErrCode != 0 || r.OpenID == "" {
		return "", "", fmt.Errorf("微信登录失败(%d): %s", r.ErrCode, r.ErrMsg)
	}
	return r.OpenID, r.UnionID, nil
}

func bindWechat(u *models.User, openid, unionid string, dbs ...*gorm.DB) {
	requestDB := database.Handle(dbs...)

	if u.WechatOpenID != nil && *u.WechatOpenID == openid {
		return
	}
	var other int64
	requestDB.Model(&models.User{}).Where("wechat_open_id = ? AND id <> ?", openid, u.ID).Count(&other)
	if other > 0 {
		return // 该微信已绑定其他账号，不抢绑
	}
	u.WechatOpenID = &openid
	u.WechatUnionID = unionid
	requestDB.Model(&models.User{}).Where("id = ?", u.ID).Updates(map[string]any{"wechat_open_id": openid, "wechat_union_id": unionid})
}

// LoginWithWechat 小程序 wx.login 的 code 换取 openid，已绑定则登录；未绑定返回 needBind=true，
// 由小程序引导用户用邮箱验证码登录并顺带绑定（保证一个人只有一个账号、数据不分裂）。
func LoginWithWechat(code string, dbs ...*gorm.DB) (u *models.User, needBind bool, err error) {
	requestDB := database.Handle(dbs...)

	openid, _, err := code2Session(code)
	if err != nil {
		return nil, false, err
	}
	var user models.User
	if err := requestDB.Where("wechat_open_id = ?", openid).First(&user).Error; err != nil {
		return nil, true, nil
	}
	if user.Disabled {
		return nil, false, ErrAccountLocked
	}
	touchLogin(&user, requestDB)
	return &user, false, nil
}

// PurgeExpiredCodes 清理一天前的验证码记录。
func PurgeExpiredCodes(dbs ...*gorm.DB) {
	requestDB := database.Handle(dbs...)

	requestDB.Where("created_at < ?", time.Now().Add(-24*time.Hour)).Delete(&models.EmailCode{})
}
