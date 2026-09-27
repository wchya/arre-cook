package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"ninimenu/internal/video"
	"regexp"
	"strings"
	"time"
)

// LinkPreview 视频 / 网页链接的预览信息：用于添加菜谱时预展示，以及详情页嵌入播放。
type LinkPreview struct {
	URL          string `json:"url"`
	Platform     string `json:"platform"`
	PlatformName string `json:"platform_name"`
	Supported    bool   `json:"supported"`
	Title        string `json:"title"`
	Cover        string `json:"cover"`
	Author       string `json:"author"`
	Duration     string `json:"duration"`
	EmbedURL     string `json:"embed_url"`
	Playable     bool   `json:"playable"`
}

var reBiliBV = regexp.MustCompile(`(BV[0-9A-Za-z]{10})`)

// FetchLinkPreview shares the public-video reader with extraction and saving.
// Unsupported links are identified locally and are never fetched by the server.
func FetchLinkPreview(raw string) (*LinkPreview, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	return FetchLinkPreviewContext(ctx, raw)
}

func FetchLinkPreviewContext(ctx context.Context, raw string) (*LinkPreview, error) {
	in, err := video.Parse(raw)
	if err != nil {
		u, parseErr := url.Parse(strings.TrimSpace(raw))
		if parseErr != nil || video.ValidatePublicURL(u) != nil {
			return nil, errors.New("请提供有效的视频链接")
		}
		p := quickLinkPreview(u)
		p.Title = p.PlatformName
		return p, nil
	}
	u, _ := url.Parse(in.URL)
	preview := quickLinkPreview(u)
	preview.Title = preview.PlatformName
	readCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	meta, err := publicVideoReader.Preview(readCtx, in.URL)
	if err == nil {
		finalURL, _ := url.Parse(meta.URL)
		preview.URL = meta.URL
		applyPlatform(preview, finalURL)
		preview.Title, preview.Author = clip(meta.Title, 200), clip(meta.Author, 80)
		preview.Duration = formatDuration(meta.Duration)
		if cover, err := url.Parse(meta.Cover); err == nil && video.ValidatePublicURL(cover) == nil && cover.Scheme == "https" {
			preview.Cover = clip(meta.Cover, 600)
		}
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
	p := &LinkPreview{URL: u.String(), Platform: "unsupported", PlatformName: "暂不支持的平台"}
	applyPlatform(p, u)
	return p
}

// applyPlatform 只识别当前支持的外部播放平台。视频链接在客户端通过原始地址打开，
// B 站同时提供播放器地址供 Web 端后续扩展使用；抖音不提供稳定的公开 iframe 地址。
func applyPlatform(p *LinkPreview, u *url.URL) {
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	switch {
	case hostMatches(host, "bilibili.com", "b23.tv"):
		p.Platform, p.PlatformName = "bilibili", "哔哩哔哩"
		p.Supported, p.Playable = true, true
		if m := reBiliBV.FindStringSubmatch(u.String()); len(m) > 1 {
			p.EmbedURL = "https://player.bilibili.com/player.html?bvid=" + m[1] + "&autoplay=0&high_quality=1"
		}
	case hostMatches(host, "douyin.com", "iesdouyin.com"):
		p.Platform, p.PlatformName = "douyin", "抖音"
		p.Supported, p.Playable = true, true
	}
}

func hostMatches(host string, domains ...string) bool {
	for _, domain := range domains {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
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
