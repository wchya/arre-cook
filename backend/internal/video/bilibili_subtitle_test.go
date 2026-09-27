package video

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestBilibiliSubtitlesPreferHumanChineseAndSkipLocked(t *testing.T) {
	var data biliSubtitles
	err := json.Unmarshal([]byte(`{"subtitle":{"subtitles":[
		{"lan":"en","subtitle_url":"https://aisubtitle.hdslb.com/en"},
		{"lan":"ai-zh","subtitle_url":"https://aisubtitle.hdslb.com/ai"},
		{"lan":"zh-CN","subtitle_url":"https://aisubtitle.hdslb.com/locked","is_lock":true},
		{"lan":"zh-Hans","subtitle_url":"https://aisubtitle.hdslb.com/human"}
	]}}`), &data)
	if err != nil || data.choose() != "https://aisubtitle.hdslb.com/human" {
		t.Fatalf("selected %q: %v", data.choose(), err)
	}
}

func TestBilibiliPublicAISubtitlesDoNotRequireASROrPlayerEndpoint(t *testing.T) {
	client, gate := testClient(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/x/v2/dm/view" || req.Header.Get("Cookie") != "" {
			t.Fatalf("unexpected request: %s", req.URL.Path)
		}
		return httpResult(req, 200, `{"code":0,"data":{"subtitle":{"subtitles":[{"lan":"ai-en","subtitle_url":"http://aisubtitle.hdslb.com/en"},{"lan":"ai-zh","subtitle_url":"http://aisubtitle.hdslb.com/zh"}]}}}`), nil
	})
	got, err := NewReader(client).biliSubtitle(context.Background(), Metadata{AID: 123, CID: 222, BVID: "BV1abCDefGh1"})
	if err != nil || secureResource(got) != "https://aisubtitle.hdslb.com/zh" || gate.calls.Load() != 1 {
		t.Fatalf("subtitle %q, calls %d: %v", got, gate.calls.Load(), err)
	}
}

func TestBilibiliEmptySubtitlesFallBackButPlatformBlocksStop(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		wantCalls  int32
	}{
		{"empty", `{"code":0,"data":{"subtitle":{"subtitles":[]}}}`, 200, 2},
		{"locked", `{"code":0,"data":{"subtitle":{"subtitles":[{"lan":"zh","subtitle_url":"http://aisubtitle.hdslb.com/locked","is_lock":true}]}}}`, 200, 2},
		{"platform-code", `{"code":-412}`, 200, 1},
		{"rate-limit", `{}`, 429, 1},
		{"broken-json", `not json`, 200, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, gate := testClient(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/x/v2/dm/view" {
					return httpResult(req, tc.status, tc.body), nil
				}
				if req.URL.Path != "/x/player/v2" || req.URL.Query().Get("cid") != "222" {
					t.Fatalf("wrong fallback: %s", req.URL)
				}
				return httpResult(req, 200, `{"code":0,"data":{"subtitle":{"subtitles":[{"lan":"zh","subtitle_url":"https://aisubtitle.hdslb.com/player"}]}}}`), nil
			})
			got, err := NewReader(client).biliSubtitle(context.Background(), Metadata{AID: 123, CID: 222, BVID: "BV1abCDefGh1"})
			if gate.calls.Load() != tc.wantCalls || (err == nil) != (tc.wantCalls == 2) {
				t.Fatalf("calls=%d got=%q err=%v", gate.calls.Load(), got, err)
			}
			if (tc.name == "rate-limit" || tc.name == "platform-code") && gate.cooldown.Load() < int64(15*time.Minute) {
				t.Fatal("missing cooldown")
			}
		})
	}
}
