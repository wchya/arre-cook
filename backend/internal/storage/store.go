package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"ninimenu/internal/config"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	once   sync.Once
	client *S3
)

func s3Client() *S3 {
	once.Do(func() { client = FromConfig() })
	return client
}

// UsingS3 是否写入对象存储。
func UsingS3() bool { return s3Client() != nil }

// Backend 存储后端名称（展示用）。
func Backend() string {
	if UsingS3() {
		return "s3"
	}
	return "local"
}

// UserKey 生成用户上传对象的键：<prefix>/u/<uid>/<yyyy>/<mm>/<dd>/<name>。
// 相对于公共前缀，删除时据 u/<uid>/ 校验归属。
func UserKey(uid uint, name string, t time.Time) string {
	rel := fmt.Sprintf("u/%d/%s/%s", uid, t.Format("2006/01/02"), name)
	if UsingS3() && config.C.S3Prefix != "" {
		return config.C.S3Prefix + "/" + rel
	}
	return rel
}

// Save 保存文件，返回可直接放进 <img src> 的地址。
func Save(ctx context.Context, key string, data []byte, contentType string) (string, error) {
	key = strings.TrimLeft(path.Clean("/"+key), "/")
	if c := s3Client(); c != nil {
		if err := c.Put(ctx, key, data, contentType); err != nil {
			return "", err
		}
		return config.C.S3PublicURL + key, nil
	}
	dst := filepath.Join(config.C.UploadDir, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return "", err
	}
	if err := os.WriteFile(dst, data, 0644); err != nil {
		return "", err
	}
	return "/uploads/" + key, nil
}

// KeyFromURL 从图片地址反推存储键；不是本系统上传的地址返回 false。
func KeyFromURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if u, err := urlPath(raw); err == nil {
		raw = u
	}
	for _, prefix := range uploadPrefixes() {
		if strings.HasPrefix(raw, prefix) {
			key := strings.TrimPrefix(raw, prefix)
			clean := strings.TrimLeft(path.Clean("/"+key), "/")
			if clean == "" || clean != key {
				return "", false
			}
			return clean, true
		}
	}
	return "", false
}

// IsUploadURL 地址是否属于本系统的上传文件（本地 /uploads/ 或对象存储公共前缀）。
func IsUploadURL(raw string) bool {
	_, ok := KeyFromURL(raw)
	return ok
}

func uploadPrefixes() []string {
	prefixes := []string{"/uploads/"}
	if p := config.C.S3PublicURL; p != "" && p != "/uploads/" {
		prefixes = append([]string{p}, prefixes...)
	}
	return prefixes
}

// urlPath 同源绝对地址（http(s)://本站/uploads/...）归一成路径，外站地址原样返回。
func urlPath(raw string) (string, error) {
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		return raw, nil
	}
	if p := config.C.S3PublicURL; p != "" && strings.HasPrefix(raw, p) {
		return raw, nil
	}
	if pub := config.C.PublicURL; pub != "" && strings.HasPrefix(raw, pub+"/uploads/") {
		return strings.TrimPrefix(raw, pub), nil
	}
	return raw, errors.New("external")
}

// OwnedBy 键是否位于该用户的上传目录下。
func OwnedBy(key string, uid uint) bool {
	rel := key
	if p := config.C.S3Prefix; p != "" {
		rel = strings.TrimPrefix(rel, p+"/")
	}
	return strings.HasPrefix(rel, fmt.Sprintf("u/%d/", uid))
}

// Delete 删除上传文件：本地存在就删本地，否则删对象存储。
func Delete(ctx context.Context, key string) error {
	local := filepath.Join(config.C.UploadDir, filepath.FromSlash(key))
	if root, err := filepath.Abs(config.C.UploadDir); err == nil {
		if abs, err := filepath.Abs(local); err == nil {
			if rel, err := filepath.Rel(root, abs); err != nil || strings.HasPrefix(rel, "..") {
				return errors.New("路径无效")
			}
		}
	}
	if _, err := os.Stat(local); err == nil {
		return os.Remove(local)
	}
	if c := s3Client(); c != nil {
		return c.Delete(ctx, key)
	}
	return nil
}

// ServeUploads GET /uploads/*：先找本地文件（种子图片、历史上传），找不到再回源对象存储。
func ServeUploads(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/uploads/")
	clean := strings.TrimLeft(path.Clean("/"+key), "/")
	if clean == "" || clean != key {
		http.NotFound(w, r)
		return
	}
	root, err := filepath.Abs(config.C.UploadDir)
	if err == nil {
		local := filepath.Join(root, filepath.FromSlash(clean))
		if info, err := os.Stat(local); err == nil && !info.IsDir() {
			w.Header().Set("Cache-Control", "public, max-age=86400")
			http.ServeFile(w, r, local)
			return
		}
	}
	c := s3Client()
	if c == nil {
		http.NotFound(w, r)
		return
	}
	extra := http.Header{}
	for _, h := range []string{"If-None-Match", "If-Modified-Since"} {
		if v := r.Header.Get(h); v != "" {
			extra.Set(h, v)
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	resp, err := c.Get(ctx, clean, extra)
	if err != nil {
		log.Printf("[storage] 回源 %s 失败: %v", clean, err)
		http.Error(w, "image unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotModified:
		w.WriteHeader(http.StatusNotModified)
		return
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusForbidden:
		http.NotFound(w, r)
		return
	case resp.StatusCode/100 != 2:
		http.Error(w, "image unavailable", http.StatusBadGateway)
		return
	}
	for _, h := range []string{"Content-Type", "Content-Length", "ETag", "Last-Modified"} {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	// 键里带时间戳 + 随机串，内容永不覆盖，可长期强缓存
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = io.Copy(w, resp.Body)
	}
}
