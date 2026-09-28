package video

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// Only visitor cookies from a fresh, dedicated browser profile are accepted.
// Account/session cookies are deliberately absent, so authenticated private
// content cannot enter the shared public-video cache through this integration.
var douyinGuestCookieNames = map[string]bool{
	"ttwid": true, "s_v_web_id": true, "passport_csrf_token": true,
	"passport_csrf_token_default": true, "__ac_nonce": true,
	"__ac_signature": true, "__ac_referer": true, "msToken": true,
	"odin_tt": true, "UIFID": true, "UIFID_TEMP": true,
	"web_sign_token": true, "x-web-secsdk-uid": true,
	"fpk1": true, "fpk2": true, "biz_trace_id": true,
	"IsDouyinActive": true, "enter_pc_once": true,
	"device_web_cpu_core": true, "device_web_memory_size": true,
	"dy_sheight": true, "dy_swidth": true, "hevc_supported": true,
	"home_can_add_dy_2_desktop": true, "is_support_rtm_web_ts": true,
	"strategyABtestKey": true, "stream_recommend_feed_params": true,
}

type douyinCredential struct {
	header string
	key    string
}

func (r *Reader) douyinCredential(in Input) (douyinCredential, error) {
	if in.Platform != "douyin" || r.douyinCookieFile == nil {
		return douyinCredential{}, nil
	}
	path := strings.TrimSpace(r.douyinCookieFile())
	if path == "" {
		return douyinCredential{}, nil
	}
	bad := problem("platform_credentials_unavailable", "抖音游客凭据不可用，请联系管理员更新；也可粘贴文稿提炼")
	if !filepath.IsAbs(path) {
		return douyinCredential{}, bad
	}
	f, err := os.Open(path)
	if err != nil {
		return douyinCredential{}, bad
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 32<<10 || (runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0) {
		return douyinCredential{}, bad
	}
	body, err := io.ReadAll(io.LimitReader(f, (32<<10)+1))
	if err != nil || len(body) > 32<<10 {
		return douyinCredential{}, bad
	}
	return parseDouyinCredential(body, time.Now())
}

func parseDouyinCredential(body []byte, now time.Time) (douyinCredential, error) {
	bad := problem("platform_credentials_unavailable", "抖音游客凭据格式无效或已过期，请联系管理员更新；也可粘贴文稿提炼")
	var data struct {
		Version int `json:"version"`
		Cookies []struct {
			Name    string  `json:"name"`
			Value   string  `json:"value"`
			Domain  string  `json:"domain"`
			Path    string  `json:"path"`
			Expires float64 `json:"expires"`
		} `json:"cookies"`
	}
	if len(body) > 32<<10 || json.Unmarshal(body, &data) != nil || data.Version != 1 || len(data.Cookies) == 0 || len(data.Cookies) > 64 {
		return douyinCredential{}, bad
	}
	values := make(map[string]string)
	for _, item := range data.Cookies {
		domain := strings.TrimPrefix(item.Domain, ".")
		cookie := http.Cookie{Name: item.Name, Value: item.Value}
		if !douyinGuestCookieNames[item.Name] || (domain != "douyin.com" && domain != "www.douyin.com") || item.Path != "/" || cookie.Valid() != nil || len(item.Value) > 4096 {
			return douyinCredential{}, bad
		}
		if item.Value == "" || (item.Expires > 0 && item.Expires <= float64(now.Unix())) {
			continue
		}
		if previous, ok := values[item.Name]; ok && previous != item.Value {
			return douyinCredential{}, bad
		}
		values[item.Name] = item.Value
	}
	if values["ttwid"] == "" || values["s_v_web_id"] == "" {
		return douyinCredential{}, bad
	}
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, (&http.Cookie{Name: name, Value: values[name]}).String())
	}
	header := strings.Join(parts, "; ")
	if len(header) > 16<<10 {
		return douyinCredential{}, bad
	}
	digest := sha256.Sum256([]byte(header))
	return douyinCredential{header: header, key: hex.EncodeToString(digest[:])}, nil
}
