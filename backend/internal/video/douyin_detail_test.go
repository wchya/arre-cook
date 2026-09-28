package video

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const douyinTestURL = "https://www.douyin.com/video/7673525775051948287"

func writeGuestCredential(t *testing.T, path, value string) {
	t.Helper()
	body, err := json.Marshal(map[string]any{"version": 1, "cookies": []map[string]any{
		{"name": "ttwid", "value": value, "domain": ".douyin.com", "path": "/", "expires": -1},
		{"name": "s_v_web_id", "value": "visitor", "domain": ".douyin.com", "path": "/", "expires": -1},
	}})
	if err != nil || os.WriteFile(path, body, 0600) != nil {
		t.Fatal("cannot write test guest credentials")
	}
}

func guestReader(t *testing.T, client *Client) (*Reader, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "guest.json")
	writeGuestCredential(t, path, "first-guest")
	return NewReader(client, ReaderOptions{DouyinCookieFile: func() string { return path }}), path
}

func douyinEnvelope(subtitle bool) string {
	// Real sample's six media sizes, sanitized CDN URLs. No music address is a
	// valid source, and 540p is not necessarily the smallest representation.
	video := map[string]any{
		"duration":  51000,
		"play_addr": map[string]any{"data_size": 18074758, "url_list": []string{"https://v.example.douyinvod.com/original.mp4"}},
		"bit_rate": []map[string]any{
			{"play_addr": map[string]any{"data_size": 11492764, "url_list": []string{"https://v.example.douyinvod.com/h264.mp4"}}},
			{"play_addr": map[string]any{"data_size": 8177200, "url_list": []string{"https://v.example.douyinvod.com/large.mp4"}}},
			{"play_addr": map[string]any{"data_size": 6364166, "url_list": []string{"https://v.example.douyinvod.com/mid.mp4"}}},
			{"play_addr": map[string]any{"data_size": 6028987, "url_list": []string{"https://v.example.douyinvod.com/720.mp4"}}},
			{"play_addr": map[string]any{"data_size": 4550389, "url_list": []string{"https://api-play.amemv.com/aweme/v1/play/?video_id=test", "https://v.example.365yg.com/small.mp4"}}},
		},
	}
	if subtitle {
		video["subtitle_infos"] = []map[string]any{{"url": "https://example.douyinstatic.com/sub.json"}}
	}
	body, _ := json.Marshal(map[string]any{"status_code": 0, "aweme_detail": map[string]any{
		"aweme_id": "7673525775051948287", "desc": "花椒烤鸡腿", "author": map[string]any{"nickname": "作者"}, "video": video,
		"music": map[string]any{"play_url": map[string]any{"data_size": 10, "url_list": []string{"https://v.example.douyinvod.com/music.mp3"}}},
	}})
	return string(body)
}

func TestDouyinGuestDetailCacheRotationAndCredentialScope(t *testing.T) {
	apiCalls, subtitleCalls, otherCalls := 0, 0, 0
	client, _ := testClient(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == douyinDetailPath {
			apiCalls++
			expected := "first-guest"
			if apiCalls == 2 {
				expected = "second-guest"
			}
			if req.URL.Host != "www.douyin.com" || req.URL.Query().Get("aweme_id") != "7673525775051948287" || !strings.Contains(req.Header.Get("Cookie"), "ttwid="+expected) {
				t.Fatal("wrong credentialed endpoint or guest snapshot")
			}
			return httpResult(req, 200, douyinEnvelope(true)), nil
		}
		if req.Header.Get("Cookie") != "" || req.Header.Get("Authorization") != "" {
			t.Fatal("guest credentials escaped the detail endpoint")
		}
		if req.URL.Path == "/sub.json" {
			subtitleCalls++
			return httpResult(req, 200, `{"body":[{"content":"`+transcript+`"}]}`), nil
		}
		otherCalls++
		return httpResult(req, 200, "metadata"), nil
	})
	reader, path := guestReader(t, client)
	meta, err := reader.Preview(context.Background(), douyinTestURL)
	if err != nil || meta.Duration != 51 || meta.MediaBytes != 4550389 || meta.Media != "https://v.example.365yg.com/small.mp4" {
		t.Fatalf("incorrect target/media selection: %+v %v", meta, err)
	}
	for i := 0; i < 2; i++ {
		source, err := reader.Transcript(context.Background(), douyinTestURL, "", ASRSettings{}, func(string) {}, func() error { t.Fatal("subtitle read consumed AI"); return nil })
		if err != nil || source.Text != transcript || source.Method != "subtitle" {
			t.Fatalf("subtitle read: %+v %v", source, err)
		}
	}
	if apiCalls != 1 || subtitleCalls != 1 {
		t.Fatal("preview and transcript did not share bounded caches")
	}
	writeGuestCredential(t, path, "second-guest")
	if _, err := reader.Transcript(context.Background(), douyinTestURL, "", ASRSettings{}, func(string) {}, func() error { return nil }); err != nil || apiCalls != 2 {
		t.Fatal("rotated credentials reused an old transcript/detail cache")
	}
	for _, resource := range []struct{ platform, kind, url string }{
		{"douyin", "media", meta.Media},
		{"douyin", "page", douyinTestURL},
		{"bilibili", "api", "https://api.bilibili.com/x/test"},
	} {
		if _, err := client.get(context.Background(), resource.platform, resource.kind, resource.url, 100); err != nil {
			t.Fatal(err)
		}
	}
	if otherCalls != 3 {
		t.Fatal("scope checks did not execute")
	}
}

func TestDouyinGuestShortLinkSkipsHTMLAndNeverSendsCookie(t *testing.T) {
	client, gate := testClient(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "v.douyin.com" {
			if req.Header.Get("Cookie") != "" {
				t.Fatal("cookie sent to short-link service")
			}
			res := httpResult(req, 302, "")
			res.Header.Set("Location", "https://www.iesdouyin.com/share/video/7673525775051948287/?region=CN")
			return res, nil
		}
		if req.URL.Path != douyinDetailPath {
			t.Fatal("fetched HTML instead of the known detail API")
		}
		return httpResult(req, 200, douyinEnvelope(false)), nil
	})
	reader, _ := guestReader(t, client)
	meta, err := reader.Preview(context.Background(), "分享 https://v.douyin.com/Z163A0D1xSs/")
	if err != nil || meta.URL != douyinTestURL || gate.calls.Load() != 2 {
		t.Fatalf("short link: %+v %v", meta, err)
	}
}

func TestDouyinCredentialedRedirectsAndChallengesStop(t *testing.T) {
	for _, destination := range []string{"https://www.douyin.com" + douyinDetailPath + "?aweme_id=333", "https://v.example.douyinvod.com/file.mp4", "https://evil.invalid/collect"} {
		t.Run(destination, func(t *testing.T) {
			client, gate := testClient(func(req *http.Request) (*http.Response, error) {
				res := httpResult(req, 302, "")
				res.Header.Set("Location", destination)
				return res, nil
			})
			reader, _ := guestReader(t, client)
			_, err := reader.Preview(context.Background(), douyinTestURL)
			if PublicError(err).Code != "platform_challenge" || gate.calls.Load() != 1 || gate.cooldown.Load() < int64(15*time.Minute) {
				t.Fatalf("redirect did not stop: %v", err)
			}
		})
	}
	for _, body := range []string{"", "<html><script>var __ac_nonce='x',__ac_signature='x'</script></html>"} {
		client, gate := testClient(func(req *http.Request) (*http.Response, error) { return httpResult(req, 200, body), nil })
		reader, _ := guestReader(t, client)
		_, err := reader.Transcript(context.Background(), douyinTestURL, "", ASRSettings{}, func(string) {}, func() error { t.Fatal("challenge charged AI"); return nil })
		if PublicError(err).Code != "platform_challenge" || gate.calls.Load() != 1 || gate.cooldown.Load() == 0 {
			t.Fatalf("challenge was not cooled down: %v", err)
		}
	}
}

func TestDouyinRejectsWrongVideoPrivateContentMalformedAndOversize(t *testing.T) {
	for _, row := range []struct{ body, code string }{
		{strings.Replace(douyinEnvelope(false), `"aweme_id":"7673525775051948287"`, `"aweme_id":"333"`, 1), "video_mismatch"},
		{strings.Replace(douyinEnvelope(false), `"desc":`, `"status":{"private_status":1},"desc":`, 1), "video_unavailable"},
		{strings.Replace(douyinEnvelope(false), `"desc":`, `"status":{"is_private":true},"desc":`, 1), "video_unavailable"},
		{strings.Replace(douyinEnvelope(false), `"desc":`, `"status":{"is_delete":1},"desc":`, 1), "video_unavailable"},
		{`{"status_code":0,"aweme_detail":null}`, "video_unavailable"},
		{`{"status_code":9,"aweme_detail":null}`, "video_unavailable"},
		{`{"unexpected":"upstream-private-body"}`, "platform_format_changed"},
		{douyinEnvelope(false) + `{}`, "platform_format_changed"},
		{strings.Repeat("x", (1<<20)+1), "too_large"},
	} {
		client, gate := testClient(func(req *http.Request) (*http.Response, error) { return httpResult(req, 200, row.body), nil })
		reader, _ := guestReader(t, client)
		_, err := reader.Preview(context.Background(), douyinTestURL)
		if err == nil || PublicError(err).Code != row.code || strings.Contains(err.Error(), "upstream-private-body") || gate.calls.Load() != 1 {
			t.Fatalf("expected %s, got %v", row.code, err)
		}
	}
	client, gate := testClient(func(req *http.Request) (*http.Response, error) {
		return httpResult(req, 200, `{"status_code":0,"aweme_detail":{"aweme_id":"7673525775051948287","video":{"duration":51000,"play_addr":{"data_size":30000000,"url_list":["https://v.example.douyinvod.com/big.mp4"]}}}}`), nil
	})
	reader, _ := guestReader(t, client)
	asr := ASRSettings{URL: "https://asr.example.com/transcriptions", APIKey: "test-only", Model: "test"}
	_, err := reader.Transcript(context.Background(), douyinTestURL, "", asr, func(string) {}, func() error { t.Fatal("oversize media charged AI"); return nil })
	if PublicError(err).Code != "too_large" || gate.calls.Load() != 1 {
		t.Fatal("oversize media was downloaded or sent to ASR")
	}
}

func TestDouyinAnonymousChallengeAndCredentialedHTTPBlocks(t *testing.T) {
	client, gate := testClient(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Cookie") != "" {
			t.Fatal("anonymous reader sent a cookie")
		}
		return httpResult(req, 200, `<html><script>var __ac_nonce='x',__ac_signature='x'</script></html>`), nil
	})
	_, err := NewReader(client).Preview(context.Background(), douyinTestURL)
	if PublicError(err).Code != "platform_challenge" || gate.calls.Load() != 1 || gate.cooldown.Load() < int64(15*time.Minute) {
		t.Fatalf("anonymous verification page did not stop and cool down: %v", err)
	}
	for _, code := range []int{403, 412, 429} {
		client, gate := testClient(func(req *http.Request) (*http.Response, error) {
			res := httpResult(req, code, "")
			res.Header.Set("Retry-After", "1200")
			return res, nil
		})
		reader, _ := guestReader(t, client)
		_, err := reader.Transcript(context.Background(), douyinTestURL, "", ASRSettings{}, func(string) {}, func() error { t.Fatal("platform block charged AI"); return nil })
		if PublicError(err).Code != "platform_limited" || gate.calls.Load() != 1 || gate.cooldown.Load() != int64(20*time.Minute) {
			t.Fatalf("HTTP %d retried or did not cool down: %v", code, err)
		}
	}
}

func TestDouyinMediaUsesSmallestSafeSourceAndNeverBackgroundMusic(t *testing.T) {
	for _, row := range []struct {
		video, suffix string
		size          int64
	}{
		{`{"play_addr":{"data_size":100,"url_list":["https://evil.invalid/small.mp4","https://v.example.douyinvod.com/valid.mp4"]}}`, "/valid.mp4", 100},
		{`{"play_addr":{"data_size":100,"url_list":["https://v.example.douyinvod.com/video.mp4"]},"bit_rate":[{"play_addr":{"data_size":150,"url_list":["https://v.example.douyinvod.com/speech.m4a"]}}]}`, "/speech.m4a", 150},
		{`{"play_addr":{"data_size":30000000,"url_list":["https://v.example.douyinvod.com/audio.m4a"]},"bit_rate":[{"play_addr":{"data_size":100,"url_list":["https://v.example.douyinvod.com/video.mp4"]}}]}`, "/video.mp4", 100},
		{`{"play_addr":{"url_list":["https://v.example.douyinvod.com/unknown.mp4"]},"bit_rate":[{"play_addr":{"data_size":100,"url_list":["https://v.example.douyinvod.com/known.mp4"]}}]}`, "/known.mp4", 100},
		{`{"music":{"play_url":{"url_list":["https://v.example.douyinvod.com/background.mp3"]}}}`, "", 0},
	} {
		decoder := json.NewDecoder(strings.NewReader(row.video))
		decoder.UseNumber()
		var video map[string]any
		if err := decoder.Decode(&video); err != nil {
			t.Fatal(err)
		}
		media, size := selectDouyinMedia(video)
		if !strings.HasSuffix(media, row.suffix) || (row.suffix == "" && media != "") || size != row.size {
			t.Fatalf("selected %s (%d), want %s (%d)", media, size, row.suffix, row.size)
		}
	}
}

func TestDouyinCredentialValidationAndManualIndependence(t *testing.T) {
	reader, path := guestReader(t, NewClient(&fakeGate{}))
	body, _ := os.ReadFile(path)
	for _, modified := range []string{
		strings.Replace(string(body), `"ttwid"`, `"sessionid"`, 1),
		strings.ReplaceAll(string(body), `".douyin.com"`, `".evil.invalid"`),
		strings.Replace(string(body), `"first-guest"`, `"value\r\nX-Evil: yes"`, 1),
		strings.ReplaceAll(string(body), `"expires":-1`, `"expires":1`),
		strings.Replace(string(body), `"version":1`, `"version":2`, 1),
	} {
		if _, err := parseDouyinCredential([]byte(modified), time.Now()); err == nil {
			t.Fatal("invalid/login/expired credentials accepted")
		}
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	source, err := reader.Transcript(context.Background(), douyinTestURL, transcript, ASRSettings{}, func(string) {}, func() error { return nil })
	if err != nil || source.Method != "manual" || source.Text != transcript {
		t.Fatal("missing credentials broke manual transcript input")
	}
	if _, err := reader.Preview(context.Background(), douyinTestURL); PublicError(err).Code != "platform_credentials_unavailable" {
		t.Fatal("missing configured credentials were silently ignored")
	}
}

func TestDouyinCredentialFilePermissionsAndSize(t *testing.T) {
	reader, path := guestReader(t, NewClient(&fakeGate{}))
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := reader.Preview(context.Background(), douyinTestURL); PublicError(err).Code != "platform_credentials_unavailable" {
			t.Fatal("world-readable credentials were accepted")
		}
		if err := os.Chmod(path, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("x", (32<<10)+1)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.Preview(context.Background(), douyinTestURL); PublicError(err).Code != "platform_credentials_unavailable" {
		t.Fatal("oversized credentials were accepted")
	}
}

func TestDouyinAPIAndCDNAllowlist(t *testing.T) {
	for _, raw := range []string{
		"https://www.douyin.com" + douyinDetailPath + "?aweme_id=1&url=https://evil.invalid",
		"https://www.douyin.com" + douyinDetailPath + "?aweme_id=1&aweme_id=2",
		"https://www.douyin.com" + douyinDetailPath + "?aweme_id=1&bad=%zz",
		"https://www.douyin.com/aweme/v1/web/user/profile/other/?aweme_id=1",
		"https://www.douyin.com.evil.invalid" + douyinDetailPath + "?aweme_id=1",
	} {
		u, _ := url.Parse(raw)
		if allowedURL("douyin", "api", u) {
			t.Fatal("unsafe API URL accepted")
		}
	}
	for _, raw := range []string{"https://api.amemv.com/unrelated", "https://v.example.365yg.com.evil.invalid/file.mp4", "https://127.0.0.1/file.mp4"} {
		u, _ := url.Parse(raw)
		if allowedURL("douyin", "media", u) {
			t.Fatal("unsafe CDN URL accepted")
		}
	}
}

// Explicit opt-in; reads metadata only, without downloading media or using ASR.
func TestDouyinLiveMetadata(t *testing.T) {
	path := os.Getenv("ARRE_DOUYIN_TEST_COOKIE_FILE")
	if path == "" {
		t.Skip("set ARRE_DOUYIN_TEST_COOKIE_FILE to opt in to a live public-metadata check")
	}
	reader := NewReader(NewClient(&fakeGate{}), ReaderOptions{DouyinCookieFile: func() string { return path }})
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	meta, err := reader.Preview(ctx, "https://v.douyin.com/Z163A0D1xSs/")
	if err != nil {
		t.Fatal(err)
	}
	if meta.URL != douyinTestURL || !strings.Contains(meta.Title, "花椒烤鸡腿") || meta.Duration <= 0 || meta.Media == "" || meta.MediaBytes <= 0 {
		t.Fatal("incomplete video metadata")
	}
	u, _ := url.Parse(meta.Media)
	t.Logf("target=%s duration=%ds selected_bytes=%d media_host=%s", meta.URL, meta.Duration, meta.MediaBytes, u.Hostname())
}
