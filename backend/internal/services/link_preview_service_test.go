package services

import (
	"net/url"
	"strings"
	"testing"
)

func TestApplyPlatformSupportsBilibiliAndDouyinOnly(t *testing.T) {
	tests := []struct {
		name      string
		rawURL    string
		platform  string
		supported bool
		playable  bool
		embedPart string
	}{
		{name: "bilibili bv", rawURL: "https://www.bilibili.com/video/BV1abCDefGh1", platform: "bilibili", supported: true, playable: true, embedPart: "bvid=BV1abCDefGh1"},
		{name: "bilibili short link", rawURL: "https://b23.tv/abc123", platform: "bilibili", supported: true, playable: true},
		{name: "douyin", rawURL: "https://v.douyin.com/abc123/", platform: "douyin", supported: true, playable: true},
		{name: "lookalike host", rawURL: "https://notbilibili.com/video/BV1abCDefGh1", platform: "unsupported", supported: false, playable: false},
		{name: "other platform", rawURL: "https://www.youtube.com/watch?v=abcdefghijk", platform: "unsupported", supported: false, playable: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.rawURL)
			if err != nil {
				t.Fatal(err)
			}
			preview := quickLinkPreview(u)
			if preview.Platform != tt.platform || preview.Supported != tt.supported || preview.Playable != tt.playable {
				t.Fatalf("preview = %+v", preview)
			}
			if tt.embedPart != "" && !strings.Contains(preview.EmbedURL, tt.embedPart) {
				t.Fatalf("embed URL %q does not contain %q", preview.EmbedURL, tt.embedPart)
			}
		})
	}
}
