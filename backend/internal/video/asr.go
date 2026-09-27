package video

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"time"
)

// ASR uses separate, server-only credentials. A text-model/CPA key is never
// implicitly reused or sent to an endpoint supplied by the user or platform.
type ASRSettings struct{ URL, APIKey, Model string }

func (s ASRSettings) Enabled() bool {
	u, err := url.Parse(s.URL)
	return err == nil && u.Scheme == "https" && ValidatePublicURL(u) == nil && u.RawQuery == "" && u.Fragment == "" && s.APIKey != "" && s.Model != ""
}

func (s ASRSettings) cacheKey() string {
	if !s.Enabled() {
		return "subtitle-only"
	}
	digest := sha256.Sum256([]byte(s.URL + "\n" + s.Model))
	return hex.EncodeToString(digest[:])
}

func mediaFormat(data []byte) (string, string, error) {
	if len(data) < 12 || len(data) > MaxMediaBytes {
		return "", "", problem("unsupported_audio", "视频音频格式暂不支持，可粘贴字幕提炼")
	}
	switch {
	case string(data[4:8]) == "ftyp":
		return "video.mp4", "video/mp4", nil
	case string(data[:3]) == "ID3" || data[0] == 0xff && data[1]&0xe0 == 0xe0 && data[1]&0x06 != 0:
		return "video.mp3", "audio/mpeg", nil
	case string(data[:4]) == "RIFF" && string(data[8:12]) == "WAVE":
		return "video.wav", "audio/wav", nil
	case string(data[:4]) == "fLaC":
		return "video.flac", "audio/flac", nil
	case string(data[:4]) == "OggS":
		return "video.ogg", "audio/ogg", nil
	case bytes.Equal(data[:4], []byte{0x1a, 0x45, 0xdf, 0xa3}):
		return "video.webm", "audio/webm", nil
	default:
		return "", "", problem("unsupported_audio", "视频音频格式暂不支持，可粘贴字幕提炼")
	}
}

func transcribe(ctx context.Context, settings ASRSettings, data []byte, gate Gate) (string, error) {
	return transcribeWithClient(ctx, settings, data, gate, PublicHTTPClient(40*time.Second))
}

func transcribeWithClient(ctx context.Context, settings ASRSettings, data []byte, gate Gate, client *http.Client) (string, error) {
	if !settings.Enabled() {
		return "", problem("transcript_required", "语音转写尚未配置，可粘贴字幕提炼")
	}
	filename, mime, err := mediaFormat(data)
	if err != nil {
		return "", err
	}
	if err := gate.Acquire(ctx, "asr"); err != nil {
		return "", err
	}
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
	header.Set("Content-Type", mime)
	part, err := w.CreatePart(header)
	if err != nil {
		return "", err
	}
	if _, err := part.Write(data); err != nil {
		return "", err
	}
	if err := w.WriteField("model", settings.Model); err != nil {
		return "", err
	}
	if err := w.WriteField("response_format", "json"); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	asrCtx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(asrCtx, http.MethodPost, settings.URL, &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+settings.APIKey)
	resp, err := client.Do(req)
	if err != nil {
		return "", problem("asr_unavailable", "语音转写暂时不可用，可稍后重试或粘贴字幕")
	}
	defer resp.Body.Close()
	if resp.StatusCode == 429 || resp.StatusCode == 403 {
		delay := max(15*time.Minute, retryDelay(resp.Header.Get("Retry-After"), time.Now()))
		if err := gate.CoolDown(ctx, "asr", delay); err != nil {
			return "", err
		}
		return "", &Error{Code: "asr_limited", Message: "语音转写暂时繁忙，可稍后重试或粘贴字幕", RetryAfter: int(delay.Seconds())}
	}
	if resp.StatusCode != http.StatusOK {
		return "", problem("asr_unavailable", "语音转写暂时不可用，可稍后重试或粘贴字幕")
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, (128<<10)+1))
	if err != nil || len(raw) > 128<<10 {
		return "", problem("asr_unavailable", "语音转写返回内容过长，请选择较短的视频")
	}
	var result struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &result) != nil || strings.TrimSpace(result.Text) == "" {
		return "", problem("transcript_required", "视频中未识别到有效语音，请粘贴字幕或做法文稿")
	}
	return ValidateTranscript(result.Text)
}
