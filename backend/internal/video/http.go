package video

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Gate interface {
	Acquire(context.Context, string) error
	CoolDown(context.Context, string, time.Duration) error
}

type Client struct {
	http  *http.Client
	gate  Gate
	cache *cache
}

type response struct {
	Body []byte
	URL  string
}

var blockedNetworks = func() []netip.Prefix {
	// Excludes private, special-use, documentation, transition and reserved ranges.
	ranges := []string{"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12", "192.0.0.0/24", "192.0.2.0/24", "192.88.99.0/24", "192.168.0.0/16", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4", "2001::/23", "2001:db8::/32", "2002::/16", "3fff::/20"}
	out := make([]netip.Prefix, 0, len(ranges))
	for _, raw := range ranges {
		out = append(out, netip.MustParsePrefix(raw))
	}
	return out
}()

func IsPublicIP(ip netip.Addr) bool {
	if !ip.IsValid() || ip.Zone() != "" {
		return false
	}
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	// Allow only allocated global IPv6, excluding IPv4 translation/tunneling ranges.
	if ip.Is6() && !netip.MustParsePrefix("2000::/3").Contains(ip) {
		return false
	}
	for _, prefix := range blockedNetworks {
		if prefix.Contains(ip) {
			return false
		}
	}
	return true
}

// GuardPublicAddr runs after DNS resolution, immediately before every connection.
func GuardPublicAddr(network, address string, _ syscall.RawConn) error {
	if network != "tcp4" && network != "tcp6" && network != "tcp" {
		return errors.New("unsupported network")
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return errors.New("invalid address")
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || !IsPublicIP(ip) {
		return errors.New("non-public address")
	}
	return nil
}

func ValidatePublicURL(u *url.URL) error {
	if u == nil || u.User != nil || u.Opaque != "" || u.Host == "" || len(u.String()) > 4096 || strings.Contains(u.Host, "%") {
		return errors.New("invalid URL")
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return errors.New("invalid protocol")
	}
	if p := u.Port(); p != "" && !(u.Scheme == "https" && p == "443") && !(u.Scheme == "http" && p == "80") {
		return errors.New("invalid port")
	}
	if ip, err := netip.ParseAddr(u.Hostname()); err == nil && !IsPublicIP(ip) {
		return errors.New("non-public address")
	}
	return nil
}

// No environment proxy, cookie jar, automatic redirects or insecure TLS settings.
// The Douyin reader may explicitly attach scoped guest credentials to its detail API.
func PublicHTTPClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{Timeout: 4 * time.Second, Control: GuardPublicAddr}
	return &http.Client{
		Timeout:       timeout,
		Transport:     &http.Transport{DialContext: dialer.DialContext, TLSHandshakeTimeout: 4 * time.Second, ResponseHeaderTimeout: 8 * time.Second, MaxResponseHeaderBytes: 32 << 10, DisableKeepAlives: true},
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func NewClient(gate Gate) *Client {
	return &Client{http: PublicHTTPClient(25 * time.Second), gate: gate, cache: newCache(128, 20<<20)}
}

func domain(host string, domains ...string) bool {
	for _, d := range domains {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}

func allowedURL(platform, kind string, u *url.URL) bool {
	if ValidatePublicURL(u) != nil || u.Scheme != "https" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if kind == "page" || (platform == "douyin" && kind == "resolve") {
		in, err := Parse(u.String())
		return err == nil && in.Platform == platform
	}
	if platform == "bilibili" {
		switch kind {
		case "api":
			return host == "api.bilibili.com"
		case "subtitle":
			return domain(host, "hdslb.com")
		case "media":
			return domain(host, "bilivideo.com", "bilivideo.cn", "bilivideo.net")
		}
	}
	if platform == "douyin" {
		if kind == "api" {
			q, err := url.ParseQuery(u.RawQuery)
			return err == nil && host == "www.douyin.com" && u.Path == douyinDetailPath && u.RawPath == "" && u.Fragment == "" && len(q) == 1 && len(q["aweme_id"]) == 1 && videoID.MatchString(q.Get("aweme_id"))
		}
		if kind == "subtitle" || kind == "media" {
			return domain(host, "douyinvod.com", "bytevod.com", "ibytedtos.com", "bytetos.com", "douyinstatic.com") || (kind == "media" && (domain(host, "365yg.com") || ((host == "aweme.snssdk.com" || host == "api-play.amemv.com" || host == "api.amemv.com") && u.Path == "/aweme/v1/play/" && u.RawPath == "")))
		}
	}
	return false
}

func retryDelay(value string, now time.Time) time.Duration {
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
		return time.Duration(min(86400, seconds)) * time.Second
	}
	if until, err := http.ParseTime(value); err == nil && until.After(now) {
		return min(24*time.Hour, until.Sub(now))
	}
	return 15 * time.Minute
}

func (c *Client) blocked(ctx context.Context, platform, retryAfter string) error {
	delay := max(15*time.Minute, retryDelay(retryAfter, time.Now()))
	if err := c.gate.CoolDown(ctx, platform, delay); err != nil {
		return problem("unavailable", "视频服务暂时不可用，请稍后重试")
	}
	return &Error{Code: "platform_limited", Message: "平台暂时限制读取，已暂停请求；可稍后重试或粘贴字幕", RetryAfter: int(delay.Seconds())}
}

func (c *Client) get(ctx context.Context, platform, kind, raw string, limit int64) (response, error) {
	load := func() (response, error) { return c.fetch(ctx, platform, kind, raw, limit) }
	if kind == "media" {
		return load()
	}
	return c.cache.get(ctx, platform+":"+kind+":"+raw, load)
}

func (c *Client) fetch(ctx context.Context, platform, kind, raw string, limit int64) (response, error) {
	return c.fetchWithDouyinCredential(ctx, platform, kind, raw, limit, "")
}

// The credential is scoped to one fixed API request. It is never attached to
// share pages, CDN requests, ASR requests, or redirects (including same-host ones).
func (c *Client) fetchWithDouyinCredential(ctx context.Context, platform, kind, raw string, limit int64, credential string) (response, error) {
	if c.gate == nil {
		return response{}, errors.New("missing platform budget")
	}
	for redirects := 0; redirects <= 3; redirects++ {
		u, err := url.Parse(raw)
		if err != nil || !allowedURL(platform, kind, u) {
			return response{}, problem("unsafe_url", "视频资源地址不受支持，请使用公开视频链接")
		}
		if err := c.gate.Acquire(ctx, platform); err != nil {
			return response{}, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return response{}, err
		}
		req.Header.Set("User-Agent", "ArreCook/1.0 (+public-recipe-import)")
		req.Header.Set("Accept", "*/*")
		if platform == "bilibili" {
			req.Header.Set("Referer", "https://www.bilibili.com/")
		}
		if credential != "" {
			if platform != "douyin" || kind != "api" || !allowedURL("douyin", "api", u) {
				return response{}, problem("unsafe_url", "抖音凭据不能用于此资源地址")
			}
			req.Header.Set("Cookie", credential)
		}
		res, err := c.http.Do(req)
		if err != nil {
			return response{}, err
		}
		if res.StatusCode == 403 || res.StatusCode == 412 || res.StatusCode == 429 {
			res.Body.Close()
			return response{}, c.blocked(ctx, platform, res.Header.Get("Retry-After"))
		}
		if res.StatusCode >= 300 && res.StatusCode < 400 {
			res.Body.Close()
			if credential != "" {
				return response{}, c.douyinChallenge(ctx)
			}
			next, err := res.Location()
			if err != nil {
				return response{}, err
			}
			if platform == "douyin" && kind == "resolve" {
				if !allowedURL(platform, kind, next) {
					return response{}, problem("unsafe_url", "视频资源地址不受支持，请使用公开视频链接")
				}
				if target, err := Parse(next.String()); err == nil && !target.Short {
					// A validated video ID is sufficient for the credentialed API.
					// Do not fetch another HTML challenge page just to rediscover it.
					return response{URL: target.URL}, nil
				}
			}
			raw = next.String()
			continue
		}
		if res.StatusCode != http.StatusOK {
			res.Body.Close()
			return response{}, problem("video_unavailable", "视频暂时不可读取，请确认视频公开可见，或粘贴字幕提炼")
		}
		if res.ContentLength > limit {
			res.Body.Close()
			return response{}, problem("too_large", "视频资源超过大小限制，请选择较短的视频或粘贴字幕")
		}
		body, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
		res.Body.Close()
		if err != nil {
			return response{}, err
		}
		if int64(len(body)) > limit {
			return response{}, problem("too_large", "视频资源超过大小限制，请选择较短的视频或粘贴字幕")
		}
		return response{Body: body, URL: raw}, nil
	}
	return response{}, problem("redirects", "视频链接跳转过多，请粘贴视频的完整地址")
}
