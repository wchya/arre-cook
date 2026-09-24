// Package mailer 通过 SMTP 发送邮件。默认按 QQ 邮箱配置：smtp.qq.com:465（SSL 直连）；
// 587 端口走 STARTTLS。QQ 邮箱的 SMTP_PASSWORD 是在「设置 → 账户」里开启 SMTP 后生成的授权码。
package mailer

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"ninimenu/internal/config"
	"strings"
	"time"
)

var ErrNotConfigured = errors.New("邮件服务未配置")

// Send 发送一封 HTML 邮件。
func Send(to, subject, html string) error {
	c := config.C
	if !c.SMTPEnabled() {
		return ErrNotConfigured
	}
	addr := net.JoinHostPort(c.SMTPHost, fmt.Sprint(c.SMTPPort))
	msg := buildMessage(c.SMTPFromName, c.SMTPUser, to, subject, html)
	auth := smtp.PlainAuth("", c.SMTPUser, c.SMTPPassword, c.SMTPHost)
	tlsCfg := &tls.Config{ServerName: c.SMTPHost, MinVersion: tls.VersionTLS12}

	var client *smtp.Client
	if c.SMTPPort == 465 {
		conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 10 * time.Second}, "tcp", addr, tlsCfg)
		if err != nil {
			return fmt.Errorf("连接邮件服务器失败: %w", err)
		}
		client, err = smtp.NewClient(conn, c.SMTPHost)
		if err != nil {
			conn.Close()
			return err
		}
	} else {
		conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
		if err != nil {
			return fmt.Errorf("连接邮件服务器失败: %w", err)
		}
		client, err = smtp.NewClient(conn, c.SMTPHost)
		if err != nil {
			conn.Close()
			return err
		}
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(tlsCfg); err != nil {
				client.Close()
				return err
			}
		}
	}
	defer client.Close()

	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("邮箱认证失败（请检查 SMTP 授权码）: %w", err)
	}
	if err := client.Mail(c.SMTPUser); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("收件地址被拒绝: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func buildMessage(fromName, from, to, subject, html string) []byte {
	var b bytes.Buffer
	header := map[string]string{
		"From":                      fmt.Sprintf("%s <%s>", mime.BEncoding.Encode("UTF-8", fromName), from),
		"To":                        to,
		"Subject":                   mime.BEncoding.Encode("UTF-8", subject),
		"MIME-Version":              "1.0",
		"Content-Type":              "text/html; charset=UTF-8",
		"Content-Transfer-Encoding": "base64",
		"Date":                      time.Now().Format(time.RFC1123Z),
	}
	for _, k := range []string{"From", "To", "Subject", "Date", "MIME-Version", "Content-Type", "Content-Transfer-Encoding"} {
		b.WriteString(k + ": " + header[k] + "\r\n")
	}
	b.WriteString("\r\n")
	enc := base64.StdEncoding.EncodeToString([]byte(html))
	for len(enc) > 76 {
		b.WriteString(enc[:76] + "\r\n")
		enc = enc[76:]
	}
	b.WriteString(enc + "\r\n")
	return b.Bytes()
}

// CodeEmailHTML 验证码邮件模板。
func CodeEmailHTML(appName, code string, ttlMinutes int) string {
	appName = htmlEscape(appName)
	return strings.NewReplacer("{{app}}", appName, "{{code}}", code, "{{ttl}}", fmt.Sprint(ttlMinutes)).Replace(`<!doctype html>
<html><body style="margin:0;padding:0;background:#faf7f2;font-family:-apple-system,BlinkMacSystemFont,'PingFang SC','Microsoft YaHei',sans-serif;">
<table width="100%" cellpadding="0" cellspacing="0" style="padding:32px 12px;"><tr><td align="center">
<table width="100%" cellpadding="0" cellspacing="0" style="max-width:440px;background:#ffffff;border-radius:20px;padding:32px 28px;box-shadow:0 8px 30px rgba(0,0,0,.06);">
<tr><td style="font-size:20px;font-weight:800;color:#1a1a2e;">🍳 {{app}}</td></tr>
<tr><td style="padding-top:18px;font-size:14px;color:#6b7280;line-height:1.7;">你正在登录 {{app}}，本次验证码为：</td></tr>
<tr><td style="padding:18px 0;"><div style="font-size:34px;font-weight:800;letter-spacing:10px;color:#e8734a;background:#fff0eb;border-radius:14px;text-align:center;padding:16px 0;">{{code}}</div></td></tr>
<tr><td style="font-size:13px;color:#9ca3af;line-height:1.7;">验证码 {{ttl}} 分钟内有效，请勿泄露给他人。<br/>如果不是你本人操作，忽略这封邮件即可。</td></tr>
</table>
<div style="font-size:12px;color:#b0b0b0;padding-top:16px;">此邮件由系统自动发送，请勿回复</div>
</td></tr></table></body></html>`)
}

func htmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}
