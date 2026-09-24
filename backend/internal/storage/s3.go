// Package storage 图片存储：配置了 S3 密钥时写入 S3 兼容对象存储（默认是博客站的 Garage），
// 否则写本地 UPLOAD_DIR。S3 客户端只用标准库实现 SigV4 签名（PUT/GET/DELETE），不引入 AWS SDK。
package storage

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"ninimenu/internal/config"
	"sort"
	"strings"
	"time"
)

const emptySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// S3 最小化的 S3 客户端。
type S3 struct {
	Endpoint  string // http(s)://host[:port]
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
	PathStyle bool
	HTTP      *http.Client
	now       func() time.Time
}

// FromConfig 按全局配置构造客户端；未配置返回 nil。
func FromConfig() *S3 {
	c := config.C
	if !c.S3Enabled() {
		return nil
	}
	return &S3{
		Endpoint: c.S3Endpoint, Region: c.S3Region, Bucket: c.S3Bucket,
		AccessKey: c.S3AccessKey, SecretKey: c.S3SecretKey, PathStyle: c.S3PathStyle,
		HTTP: &http.Client{Timeout: 30 * time.Second},
	}
}

func (s *S3) objectURL(key string) (*url.URL, error) {
	u, err := url.Parse(s.Endpoint)
	if err != nil {
		return nil, err
	}
	key = strings.TrimLeft(key, "/")
	if s.PathStyle {
		u.Path = "/" + s.Bucket + "/" + key
	} else {
		u.Host = s.Bucket + "." + u.Host
		u.Path = "/" + key
	}
	return u, nil
}

// Put 上传对象。
func (s *S3) Put(ctx context.Context, key string, body []byte, contentType string) error {
	headers := http.Header{}
	if contentType != "" {
		headers.Set("Content-Type", contentType)
	}
	headers.Set("Cache-Control", "public, max-age=31536000, immutable")
	resp, err := s.do(ctx, http.MethodPut, key, body, headers)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return s3Error(resp)
	}
	return nil
}

// Delete 删除对象（不存在也视为成功）。
func (s *S3) Delete(ctx context.Context, key string) error {
	resp, err := s.do(ctx, http.MethodDelete, key, nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 && resp.StatusCode != http.StatusNotFound {
		return s3Error(resp)
	}
	return nil
}

// Get 读取对象；调用方负责关闭 Body。extra 里的条件请求头（If-None-Match 等）原样透传。
func (s *S3) Get(ctx context.Context, key string, extra http.Header) (*http.Response, error) {
	return s.do(ctx, http.MethodGet, key, nil, extra)
}

func (s *S3) do(ctx context.Context, method, key string, body []byte, extra http.Header) (*http.Response, error) {
	u, err := s.objectURL(key)
	if err != nil {
		return nil, err
	}
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), reader)
	if err != nil {
		return nil, err
	}
	for k, vs := range extra {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	if body != nil {
		req.ContentLength = int64(len(body))
	}
	s.sign(req, body)
	client := s.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	return client.Do(req)
}

func s3Error(resp *http.Response) error {
	msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	return fmt.Errorf("对象存储返回 %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
}

// sign 按 AWS Signature Version 4 为请求签名（签 host、x-amz-date、x-amz-content-sha256 及 content-type）。
func (s *S3) sign(req *http.Request, body []byte) {
	now := time.Now
	if s.now != nil {
		now = s.now
	}
	t := now().UTC()
	amzDate := t.Format("20060102T150405Z")
	date := t.Format("20060102")

	payloadHash := emptySHA256
	if len(body) > 0 {
		sum := sha256.Sum256(body)
		payloadHash = hex.EncodeToString(sum[:])
	}
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)

	signed := map[string]string{
		"host":                 req.URL.Host,
		"x-amz-date":           amzDate,
		"x-amz-content-sha256": payloadHash,
	}
	for _, h := range []string{"Content-Type", "Range"} {
		if v := req.Header.Get(h); v != "" {
			signed[strings.ToLower(h)] = strings.TrimSpace(v)
		}
	}
	names := make([]string, 0, len(signed))
	for k := range signed {
		names = append(names, k)
	}
	sort.Strings(names)
	var canonHeaders strings.Builder
	for _, k := range names {
		canonHeaders.WriteString(k + ":" + signed[k] + "\n")
	}
	signedHeaders := strings.Join(names, ";")

	canonical := strings.Join([]string{
		req.Method,
		uriEncode(req.URL.Path, false),
		canonicalQuery(req.URL.Query()),
		canonHeaders.String(),
		signedHeaders,
		payloadHash,
	}, "\n")

	scope := date + "/" + s.Region + "/s3/aws4_request"
	hashed := sha256.Sum256([]byte(canonical))
	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hex.EncodeToString(hashed[:])

	kDate := hmacSHA256([]byte("AWS4"+s.SecretKey), date)
	kRegion := hmacSHA256(kDate, s.Region)
	kService := hmacSHA256(kRegion, "s3")
	kSigning := hmacSHA256(kService, "aws4_request")
	signature := hex.EncodeToString(hmacSHA256(kSigning, stringToSign))

	req.Header.Set("Authorization", fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		s.AccessKey, scope, signedHeaders, signature))
}

func hmacSHA256(key []byte, data string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(data))
	return m.Sum(nil)
}

func canonicalQuery(q url.Values) string {
	if len(q) == 0 {
		return ""
	}
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		vals := append([]string{}, q[k]...)
		sort.Strings(vals)
		for _, v := range vals {
			parts = append(parts, uriEncode(k, true)+"="+uriEncode(v, true))
		}
	}
	return strings.Join(parts, "&")
}

// uriEncode S3 规范的 URI 编码：只保留 A-Za-z0-9-_.~，路径中的 / 不编码。
func uriEncode(s string, encodeSlash bool) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' || c == '~':
			b.WriteByte(c)
		case c == '/' && !encodeSlash:
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}
