package auth

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// 密码使用 PBKDF2-HMAC-SHA512（OWASP 推荐 21 万次迭代）存储，只依赖标准库。
// 格式：pbkdf2-sha512$<iter>$<salt-b64>$<hash-b64>，便于将来调参或换算法时识别旧格式。
const (
	pbkdf2Iter    = 210_000
	pbkdf2KeyLen  = 32
	pbkdf2SaltLen = 16
	hashPrefix    = "pbkdf2-sha512"
)

var ErrWeakPassword = errors.New("密码至少 6 位")

func ValidatePassword(pw string) error {
	if utf8.RuneCountInString(pw) < 6 {
		return ErrWeakPassword
	}
	if len(pw) > 128 {
		return errors.New("密码过长")
	}
	return nil
}

func HashPassword(pw string) (string, error) {
	salt := make([]byte, pbkdf2SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha512.New, pw, salt, pbkdf2Iter, pbkdf2KeyLen)
	if err != nil {
		return "", err
	}
	enc := base64.RawStdEncoding
	return fmt.Sprintf("%s$%d$%s$%s", hashPrefix, pbkdf2Iter, enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}

func VerifyPassword(encoded, pw string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != hashPrefix {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter < 1 {
		return false
	}
	enc := base64.RawStdEncoding
	salt, err1 := enc.DecodeString(parts[2])
	want, err2 := enc.DecodeString(parts[3])
	if err1 != nil || err2 != nil {
		return false
	}
	got, err := pbkdf2.Key(sha512.New, pw, salt, iter, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}
