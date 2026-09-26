package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// LinkPreview 视频 / 网页链接的预览信息：用于添加菜谱时预展示，以及详情页嵌入播放。
type LinkPreview struct {
	URL          string `json:"url"`
	Platform     string `json:"platform"`
	PlatformName string `json:"platform_name"`
	Title        string `json:"title"`
	Cover        string `json:"cover"`
	Author       string `json:"author"`
	Duration     string `json:"duration"`
	EmbedURL     string `json:"embed_url"`
	Playable     bool   `json:"playable"`
}

const linkPreviewMaxBody = 512 << 10 // 抓取网页时最多读取 512KB

var (
	reBiliBV    = regexp.MustCompile(`(?i)(BV[0-9A-Za-z]{10})`)
	reYouTubeID = regexp.MustCompile(`(?i)(?:v=|/embed/|youtu\.be/|/shorts/)([0-9A-Za-z_-]{11})`)
	reMetaTag   = regexp.MustCompile(`(?is)<meta\s+[^>]*?>`)
	reTitleTag  = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	reAttrKV    = regexp.MustCompile(`(?is)([a-zA-Z:_-]+)\s*=\s*(?:"([^"]*)"|'([^']*)')`)
)

// FetchLinkPreview 解析链接并尽力抓取标题/封面；抓取失败时仍返回按 URL 识别出的平台与嵌入地址。
func FetchLinkPreview(raw string) (*LinkPreview, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("请提供链接")
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("链接无效，请以 http(s):// 开头")
	}
	preview := quickLinkPreview(u)
	if metas, title, finalURL, err := fetchHTML(raw); err == nil {
		if finalURL != nil && finalURL.Host != u.Host {
			applyPlatform(preview, finalURL) // 短链跳转后按最终地址识别平台
		}
		enrichFromHTML(preview, metas, title)
	}
	if preview.Title == "" {
		preview.Title = preview.PlatformName
	}
	return preview, nil
}

// BuildVideoMeta 供保存菜谱时填充 video_meta（JSON）；空链接返回空串。
func BuildVideoMeta(rawURL string) string {
	if strings.TrimSpace(rawURL) == "" {
		return ""
	}
	preview, err := FetchLinkPreview(rawURL)
	if err != nil || preview == nil {
		return ""
	}
	b, err := json.Marshal(preview)
	if err != nil {
		return ""
	}
	return string(b)
}

func quickLinkPreview(u *url.URL) *LinkPreview {
	p := &LinkPreview{URL: u.String(), Platform: "web", PlatformName: "网页链接"}
	applyPlatform(p, u)
	return p
}

// applyPlatform 按域名识别平台，并尽量生成可嵌入播放的 embed_url。
func applyPlatform(p *LinkPreview, u *url.URL) {
	host := strings.ToLower(u.Host)
	switch {
	case strings.Contains(host, "bilibili.com") || strings.Contains(host, "b23.tv"):
		p.Platform, p.PlatformName = "bilibili", "哔哩哔哩"
		if m := reBiliBV.FindStringSubmatch(u.String()); len(m) > 1 {
			p.EmbedURL = "https://player.bilibili.com/player.html?bvid=" + m[1] + "&autoplay=0&high_quality=1"
			p.Playable = true
		}
	case strings.Contains(host, "youtube.com") || strings.Contains(host, "youtu.be"):
		p.Platform, p.PlatformName = "youtube", "YouTube"
		if m := reYouTubeID.FindStringSubmatch(u.String()); len(m) > 1 {
			p.EmbedURL = "https://www.youtube.com/embed/" + m[1]
			p.Playable = true
		}
	case strings.Contains(host, "douyin.com") || strings.Contains(host, "iesdouyin.com"):
		p.Platform, p.PlatformName = "douyin", "抖音"
	case strings.Contains(host, "xiaohongshu.com") || strings.Contains(host, "xhslink.com"):
		p.Platform, p.PlatformName = "xiaohongshu", "小红书"
	case strings.Contains(host, "weixin.qq.com"): // 必须在 qq.com 之前判断
		p.Platform, p.PlatformName = "weixin", "微信"
	case strings.Contains(host, "v.qq.com") || strings.Contains(host, "qq.com"):
		p.Platform, p.PlatformName = "tencent", "腾讯视频"
	default:
		if p.Platform == "" {
			p.Platform, p.PlatformName = "web", "网页链接"
		}
	}
}

// fetchHTML 受控抓取网页：SSRF 防护（禁私网/环回）、限时、限跳转、限大小；返回 meta 表、<title> 与最终 URL。
func fetchHTML(raw string) (map[string]string, string, *url.URL, error) {
	dialer := &net.Dialer{Timeout: 4 * time.Second, Control: guardPrivateAddr}
	client := &http.Client{
		Timeout: 6 * time.Second,
		Transport: &http.Transport{
			DialContext:           dialer.DialContext,
			TLSHandshakeTimeout:   4 * time.Second,
			ResponseHeaderTimeout: 4 * time.Second,
			DisableKeepAlives:     true,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return errors.New("重定向次数过多")
			}
			return nil
		},
	}
	req, err := http.NewRequest(http.MethodGet, raw, nil)
	if err != nil {
		return nil, "", nil, err
	}
	req.Header.Set("User-Agent", "NiniMenuBot/1.0 (+link-preview)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", resp.Request.URL, fmt.Errorf("抓取失败：%d", resp.StatusCode)
	}
	if ct := strings.ToLower(resp.Header.Get("Content-Type")); ct != "" &&
		!strings.Contains(ct, "html") && !strings.Contains(ct, "xml") {
		return nil, "", resp.Request.URL, errors.New("非网页内容")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, linkPreviewMaxBody))
	if err != nil {
		return nil, "", resp.Request.URL, err
	}
	metas, title := parseHTMLMeta(string(body))
	return metas, title, resp.Request.URL, nil
}

// guardPrivateAddr 在 DNS 解析后、真正拨号前拦截指向私网/环回/链路本地地址的连接，防御 SSRF 与 DNS 重绑定。
func guardPrivateAddr(network, address string, _ syscall.RawConn) error {
	if network != "tcp4" && network != "tcp6" && network != "tcp" {
		return fmt.Errorf("不允许的网络类型：%s", network)
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("无法解析地址：%s", host)
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return errors.New("拒绝访问内网地址")
	}
	return nil
}

// parseHTMLMeta 从 HTML 中提取 <title> 与所有 <meta>（按 name/property 建表，先到先得）。
func parseHTMLMeta(body string) (map[string]string, string) {
	metas := make(map[string]string)
	for _, tag := range reMetaTag.FindAllString(body, -1) {
		var key, content string
		for _, kv := range reAttrKV.FindAllStringSubmatch(tag, -1) {
			name := strings.ToLower(strings.TrimSpace(kv[1]))
			val := kv[2]
			if val == "" {
				val = kv[3]
			}
			switch name {
			case "property", "name", "itemprop":
				if key == "" {
					key = strings.ToLower(strings.TrimSpace(val))
				}
			case "content":
				content = val
			}
		}
		if key != "" && content != "" {
			if _, exists := metas[key]; !exists {
				metas[key] = html.UnescapeString(content)
			}
		}
	}
	title := ""
	if m := reTitleTag.FindStringSubmatch(body); len(m) > 1 {
		title = strings.TrimSpace(html.UnescapeString(m[1]))
	}
	return metas, title
}

// enrichFromHTML 用抓取到的 meta/title 补全预览：标题、封面、作者、时长；og 优先，已识别出的 embed 不覆盖。
func enrichFromHTML(p *LinkPreview, metas map[string]string, title string) {
	p.Title = firstNonEmpty(p.Title, metas["og:title"], metas["twitter:title"], title)
	p.Cover = firstNonEmpty(p.Cover, metas["og:image"], metas["og:image:url"], metas["twitter:image"], metas["twitter:image:src"])
	p.Author = firstNonEmpty(p.Author, metas["author"], metas["og:site_name"])
	if dur := firstNonEmpty(metas["og:video:duration"], metas["video:duration"]); dur != "" {
		if secs, err := strconv.Atoi(strings.TrimSpace(dur)); err == nil && secs > 0 {
			p.Duration = formatDuration(secs)
		}
	}
	if p.EmbedURL == "" {
		if embed := firstNonEmpty(metas["og:video:secure_url"], metas["og:video:url"], metas["og:video"]); strings.HasPrefix(embed, "https://") {
			p.EmbedURL = embed
			p.Playable = true
		}
	}
	p.Title = clip(p.Title, 200)
	p.Author = clip(p.Author, 80)
	p.Cover = clip(p.Cover, 600)
}

func clip(s string, max int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

func formatDuration(seconds int) string {
	if seconds <= 0 {
		return ""
	}
	m, s := seconds/60, seconds%60
	if m >= 60 {
		return fmt.Sprintf("%d:%02d:%02d", m/60, m%60, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}
