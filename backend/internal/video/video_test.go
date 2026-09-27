package video

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeGate struct {
	calls    atomic.Int32
	cooldown atomic.Int64
}

func (g *fakeGate) Acquire(context.Context, string) error { g.calls.Add(1); return nil }
func (g *fakeGate) CoolDown(_ context.Context, _ string, delay time.Duration) error {
	g.cooldown.Store(int64(delay))
	return nil
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func httpResult(r *http.Request, status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}
}
func testClient(fn roundTrip) (*Client, *fakeGate) {
	gate := &fakeGate{}
	client := NewClient(gate)
	client.http.Transport = fn
	return client, gate
}

const transcript = "今天做番茄炒蛋，番茄两个切块，鸡蛋三个打散。锅里加油，倒入鸡蛋炒熟盛出，再炒番茄，最后加盐半勺炒匀。"

func TestParseVideoLinksAndUnsafeInputs(t *testing.T) {
	for _, item := range []struct{ raw, want string }{
		{"来做菜 https://www.bilibili.com/video/BV1abCDefGh1/?p=2&share_source=copy 一起看", "https://www.bilibili.com/video/BV1abCDefGh1/?p=2"},
		{"http://b23.tv/abc123", "https://b23.tv/abc123"},
		{"抖音分享 https://v.douyin.com/Abc123/ 复制打开", "https://v.douyin.com/Abc123/"},
		{"https://www.iesdouyin.com/share/video/123456789/", "https://www.douyin.com/video/123456789"},
		{"https://www.douyin.com/?modal_id=123456789", "https://www.douyin.com/video/123456789"},
	} {
		in, err := Parse(item.raw)
		if err != nil || in.URL != item.want {
			t.Fatalf("parse %q: %+v %v", item.raw, in, err)
		}
	}
	for _, raw := range []string{
		"file:///etc/passwd", "https://127.0.0.1/video/123", "https://www.bilibili.com.evil.test/video/BV1abCDefGh1",
		"https://www.bilibili.com@127.0.0.1/video/BV1abCDefGh1", "https://www.bilibili.com:8443/video/BV1abCDefGh1",
		"https://www.bilibili.com./video/BV1abCDefGh1", "https://www.bilibili.com/video/%42V1abCDefGh1", "https://www.bilibili.com/video/BV1abCDefGh1?p=0",
		"https://www.bilibili.com/video/BV1abCDefGh1?p=2&p=3", "https://www.bilibili.com/video/BV1abCDefGh1?p=101", "https://www.douyin.com/user/123",
		"https://b23.tv/a https://b23.tv/b", "https://b23.tv/../a", "https://b23.tv/a%2fb",
	} {
		if _, err := Parse(raw); err == nil {
			t.Errorf("unsafe link accepted: %s", raw)
		}
	}
}

func TestPublicNetworkGuardCoversSpecialAndReboundAddresses(t *testing.T) {
	for _, raw := range []string{"0.0.0.0", "127.0.0.1", "10.0.0.1", "100.64.0.1", "169.254.169.254", "172.16.0.1", "192.168.0.1", "192.0.2.1", "198.18.0.1", "198.51.100.1", "203.0.113.1", "224.0.0.1", "255.255.255.255", "::1", "::ffff:127.0.0.1", "64:ff9b::a00:1", "2001:db8::1", "2002:7f00:1::", "3fff::1", "fc00::1", "fe80::1", "ff02::1"} {
		if IsPublicIP(netip.MustParseAddr(raw)) {
			t.Errorf("allowed %s", raw)
		}
	}
	for _, raw := range []string{"1.1.1.1", "223.5.5.5", "2606:4700:4700::1111"} {
		if !IsPublicIP(netip.MustParseAddr(raw)) {
			t.Errorf("blocked public IP %s", raw)
		}
	}
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer server.Close()
	// Even when URL validation is accidentally skipped by a caller, dialing a
	// loopback result (including localhost DNS) cannot reach the server.
	client := PublicHTTPClient(time.Second)
	for _, raw := range []string{server.URL, strings.Replace(server.URL, "127.0.0.1", "localhost", 1)} {
		if response, err := client.Get(raw); err == nil {
			response.Body.Close()
			t.Fatal("private server reached")
		}
	}
	if hits.Load() != 0 {
		t.Fatal("SSRF request escaped guard")
	}
}

func TestRedirectBodyLimitsCredentialsAndCooldown(t *testing.T) {
	for _, destination := range []string{"http://api.bilibili.com/x/test", "https://api.bilibili.com:8443/x/test", "https://127.0.0.1/x", "https://api.bilibili.com.evil.test/x", "https://user:pass@api.bilibili.com/x"} {
		client, gate := testClient(func(req *http.Request) (*http.Response, error) {
			if req.Header.Get("Cookie") != "" || req.Header.Get("Authorization") != "" {
				t.Fatal("credentials forwarded")
			}
			res := httpResult(req, 302, "")
			res.Header.Set("Location", destination)
			return res, nil
		})
		if _, err := client.get(context.Background(), "bilibili", "api", "https://api.bilibili.com/x/start", 50); err == nil {
			t.Fatalf("followed %s", destination)
		}
		if gate.calls.Load() != 1 {
			t.Fatal("unsafe redirect used a network slot")
		}
	}
	client, _ := testClient(func(req *http.Request) (*http.Response, error) {
		return httpResult(req, 200, strings.Repeat("x", 51)), nil
	})
	if _, err := client.get(context.Background(), "bilibili", "api", "https://api.bilibili.com/x/test", 50); err == nil {
		t.Fatal("truncated body accepted")
	}
	for _, code := range []int{403, 412, 429} {
		client, gate := testClient(func(req *http.Request) (*http.Response, error) {
			res := httpResult(req, code, "secret-upstream-body")
			res.Header.Set("Retry-After", "3600")
			return res, nil
		})
		_, err := client.get(context.Background(), "bilibili", "api", "https://api.bilibili.com/x/test", 50)
		if PublicError(err).Code != "platform_limited" || gate.cooldown.Load() != int64(time.Hour) || strings.Contains(err.Error(), "secret") {
			t.Fatalf("cooldown failed: %v", err)
		}
	}
}

func TestCacheCoalescesAndCancellationDoesNotPoisonNextRequest(t *testing.T) {
	cache := newCache(3, 32)
	var calls atomic.Int32
	start, release := make(chan struct{}), make(chan struct{})
	load := func() (response, error) {
		if calls.Add(1) == 1 {
			close(start)
		}
		<-release
		return response{Body: []byte("content")}, nil
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); _, _ = cache.get(context.Background(), "same", load) }()
	<-start
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := cache.get(ctx, "same", load); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled waiter still running")
	}
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = cache.get(context.Background(), "same", load) }()
	}
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("fetches=%d", calls.Load())
	}
	ctx, cancel = context.WithCancel(context.Background())
	_, _ = cache.get(ctx, "cancelled-owner", func() (response, error) { cancel(); return response{}, ctx.Err() })
	_, err := cache.get(context.Background(), "cancelled-owner", func() (response, error) { return response{Body: []byte("retry")}, nil })
	if err != nil {
		t.Fatal("cancellation was cached")
	}
	for _, key := range []string{"a", "b", "c", "d"} {
		_, _ = cache.get(context.Background(), key, func() (response, error) { return response{Body: []byte(strings.Repeat("x", 20))}, nil })
	}
	if len(cache.entries) > 1 {
		t.Fatal("byte capacity exceeded")
	}
}

func TestSubtitleContentAndCorrectBilibiliPart(t *testing.T) {
	for _, raw := range []string{`{"body":[{"content":"` + transcript + `"}]}`, "WEBVTT\n\n00:00.000 --> 00:10.000\n" + transcript + "\n", "1\n00:00:00,000 --> 00:00:10,000\n" + transcript + "\n"} {
		got, err := parseSubtitle([]byte(raw))
		if err != nil || got != transcript {
			t.Fatalf("subtitle %q %v", got, err)
		}
	}
	if _, err := parseSubtitle([]byte(`<html><title>` + transcript + `</title></html>`)); err == nil {
		t.Fatal("title masqueraded as transcript")
	}
	var cidOK bool
	client, gate := testClient(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/x/web-interface/view":
			return httpResult(req, 200, `{"code":0,"data":{"aid":123,"bvid":"BV1abCDefGh1","title":"标题不能当做法","pages":[{"cid":111,"page":1,"duration":10},{"cid":222,"page":2,"duration":60}]}}`), nil
		case "/x/v2/dm/view":
			cidOK = req.URL.Query().Get("oid") == "222" && req.URL.Query().Get("aid") == "123" && req.URL.Query().Get("type") == "1"
			return httpResult(req, 200, `{"code":0,"data":{"subtitle":{"subtitles":[{"lan":"zh-CN","subtitle_url":"//aisubtitle.hdslb.com/sub.json"}]}}}`), nil
		case "/sub.json":
			return httpResult(req, 200, `{"body":[{"content":"`+transcript+`"}]}`), nil
		default:
			t.Fatalf("unexpected network request %s", req.URL.Path)
			return nil, errors.New("unexpected request")
		}
	})
	reader := NewReader(client)
	var aiCalls int
	source, err := reader.Transcript(context.Background(), "https://www.bilibili.com/video/BV1abCDefGh1?p=2", "", ASRSettings{}, func(string) {}, func() error { aiCalls++; return nil })
	if err != nil || !cidOK || source.Text != transcript || source.Method != "subtitle" || aiCalls != 0 {
		t.Fatalf("source=%+v cid=%v err=%v", source, cidOK, err)
	}
	_, err = reader.Preview(context.Background(), source.URL)
	if err != nil || gate.calls.Load() != 3 {
		t.Fatal("preview did not share import's metadata cache")
	}
	_, err = reader.Transcript(context.Background(), source.URL, "", ASRSettings{}, func(string) {}, func() error { aiCalls++; return nil })
	if err != nil || gate.calls.Load() != 3 {
		t.Fatal("transcript cache did not prevent refetching")
	}
}

func TestDouyinOnlyUsesTheRequestedVideoAndNeverMusicOrDescription(t *testing.T) {
	input, _ := Parse("https://www.douyin.com/video/222")
	data := `{"loaderData":{"items":[{"aweme_id":"111","desc":"推荐视频","video":{"duration":1000}},{"aweme_id":"222","desc":"目标视频","author":{"nickname":"作者"},"music":{"play_url":{"url_list":["https://evil.invalid/music"]}},"video":{"duration":59000,"play_addr":{"url_list":["https://v.example.douyinvod.com/target.mp4"]}}}]}}`
	for _, html := range []string{`<script id="RENDER_DATA">` + url.QueryEscape(data) + `</script>`, `<script>window._ROUTER_DATA = ` + data + `;</script>`} {
		meta, err := parseDouyin([]byte(html), input)
		if err != nil || meta.Title != "目标视频" || meta.Duration != 59 || meta.Subtitle != "" || !strings.HasSuffix(meta.Media, "/target.mp4") {
			t.Fatalf("metadata=%+v err=%v", meta, err)
		}
	}
	input.ID = "333"
	if _, err := parseDouyin([]byte(`<script>window._ROUTER_DATA = `+data+`;</script>`), input); err == nil {
		t.Fatal("recommended video selected instead")
	}
}

func TestASRUsesServerConfigurationAndBoundedMultipart(t *testing.T) {
	gate := &fakeGate{}
	settings := ASRSettings{URL: "https://asr.example.com/v1/audio/transcriptions", APIKey: "server-only-test-key", Model: "test-transcribe"}
	media := append([]byte{0, 0, 0, 20}, []byte("ftypisom00000000")...)
	client := PublicHTTPClient(time.Second)
	client.Transport = roundTrip(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != settings.URL || req.Header.Get("Authorization") != "Bearer server-only-test-key" || req.Header.Get("Cookie") != "" {
			t.Fatal("wrong ASR destination or credentials")
		}
		reader, err := req.MultipartReader()
		if err != nil {
			t.Fatal(err)
		}
		fields := map[string]string{}
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(part)
			fields[part.FormName()] = string(body)
			if part.FormName() == "file" && (part.FileName() != "video.mp4" || part.Header.Get("Content-Type") != "video/mp4") {
				t.Fatal("format metadata missing")
			}
		}
		if fields["model"] != settings.Model || fields["response_format"] != "json" || fields["file"] != string(media) {
			t.Fatal("bad multipart")
		}
		body, _ := json.Marshal(map[string]string{"text": transcript})
		return httpResult(req, 200, string(body)), nil
	})
	got, err := transcribeWithClient(context.Background(), settings, media, gate, client)
	if err != nil || got != transcript || gate.calls.Load() != 1 {
		t.Fatalf("ASR %q %v", got, err)
	}
	if _, err := transcribeWithClient(context.Background(), settings, []byte("<html>not media</html>"), gate, client); err == nil || gate.calls.Load() != 1 {
		t.Fatal("invalid media sent to ASR")
	}
	client.Transport = roundTrip(func(req *http.Request) (*http.Response, error) {
		res := httpResult(req, 302, "private upstream text")
		res.Header.Set("Location", "https://evil.invalid/steal")
		return res, nil
	})
	if _, err := transcribeWithClient(context.Background(), settings, media, gate, client); err == nil || strings.Contains(err.Error(), "upstream") {
		t.Fatal("ASR redirect/response leaked")
	}
}
