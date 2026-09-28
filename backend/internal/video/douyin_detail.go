package video

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/url"
	"strconv"
	"strings"
)

// Same metadata-only endpoint used by yt-dlp's DouyinIE (2026.08.19).
// Keep transport in Go so every request retains the app's network and budget guards.
const douyinDetailPath = "/aweme/v1/web/aweme/detail/"

func (r *Reader) douyinDetail(ctx context.Context, in Input, credential douyinCredential) (Metadata, error) {
	endpoint := "https://www.douyin.com" + douyinDetailPath + "?aweme_id=" + in.ID
	res, err := r.client.cache.get(ctx, "douyin:detail:"+in.ID+":"+credential.key, func() (response, error) {
		return r.client.fetchWithDouyinCredential(ctx, "douyin", "api", endpoint, 1<<20, credential.header)
	})
	if err != nil {
		return Metadata{}, err
	}
	if len(bytes.TrimSpace(res.Body)) == 0 || isDouyinChallenge(res.Body) {
		return Metadata{}, r.client.douyinChallenge(ctx)
	}
	var envelope struct {
		Code   *int           `json:"status_code"`
		Detail map[string]any `json:"aweme_detail"`
	}
	decoder := json.NewDecoder(bytes.NewReader(res.Body))
	decoder.UseNumber()
	if decoder.Decode(&envelope) != nil || decoder.Decode(new(any)) != io.EOF || envelope.Code == nil {
		return Metadata{}, problem("platform_format_changed", "抖音返回的数据格式暂不支持，可粘贴文稿提炼")
	}
	if *envelope.Code != 0 || envelope.Detail == nil {
		return Metadata{}, problem("video_unavailable", "抖音未返回目标视频，可能已失效或需要登录；可粘贴文稿提炼")
	}
	item := envelope.Detail
	if stringValue(item["aweme_id"]) != in.ID && stringValue(item["awemeId"]) != in.ID {
		return Metadata{}, problem("video_mismatch", "抖音未返回目标视频，请检查链接")
	}
	status, _ := item["status"].(map[string]any)
	if status["is_private"] == true || positiveInteger(status["is_private"]) > 0 || status["is_delete"] == true || positiveInteger(status["is_delete"]) > 0 || positiveInteger(status["private_status"]) > 0 {
		return Metadata{}, problem("video_unavailable", "只支持公开可见的抖音视频，可粘贴文稿提炼")
	}
	if _, ok := item["video"].(map[string]any); !ok {
		return Metadata{}, problem("video_unavailable", "这条抖音内容没有可读取的视频，可粘贴文稿提炼")
	}
	return douyinMetadata(item, in), nil
}

func isDouyinChallenge(body []byte) bool {
	// Match an HTML challenge, not words inside a valid JSON description.
	trimmed := bytes.TrimSpace(body)
	return bytes.HasPrefix(trimmed, []byte("<")) && bytes.Contains(body, []byte("<script")) && bytes.Contains(body, []byte("__ac_nonce")) && bytes.Contains(body, []byte("__ac_signature"))
}

func (c *Client) douyinChallenge(ctx context.Context) error {
	err := c.blocked(ctx, "douyin", "")
	if known, ok := err.(*Error); ok && known.Code == "platform_limited" {
		known.Code = "platform_challenge"
		known.Message = "抖音要求验证或游客凭据已失效，已暂停读取；请联系管理员更新凭据，或粘贴文稿提炼"
	}
	return err
}

func douyinMetadata(item map[string]any, in Input) Metadata {
	video, _ := item["video"].(map[string]any)
	author, _ := item["author"].(map[string]any)
	duration := number(video["duration"])
	if duration == 0 {
		duration = number(item["duration"])
	}
	meta := Metadata{URL: in.URL, Platform: in.Platform, Title: stringValue(item["desc"]), Author: stringValue(author["nickname"]), Duration: (duration + 999) / 1000, Cover: firstURL(video["cover"])}
	meta.Media, meta.MediaBytes = selectDouyinMedia(video)
	// Background music and descriptions are never transcript sources.
	for _, parent := range []map[string]any{video, item} {
		for _, key := range []string{"subtitle_infos", "caption_info", "caption_infos", "subtitleInfos", "captionInfos"} {
			entries, _ := parent[key].([]any)
			for _, entry := range entries {
				m, _ := entry.(map[string]any)
				for _, field := range []string{"url", "Url", "subtitle_url"} {
					if raw := secureResource(stringValue(m[field])); raw != "" && meta.Subtitle == "" {
						if u, err := url.Parse(raw); err == nil && allowedURL("douyin", "subtitle", u) {
							meta.Subtitle = raw
						}
					}
				}
			}
		}
	}
	return meta
}

type douyinMedia struct {
	url   string
	size  int64
	audio bool
}

func positiveInteger(value any) int64 {
	n, err := strconv.ParseInt(stringValue(value), 10, 64)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

func selectDouyinMedia(video map[string]any) (string, int64) {
	var candidates []douyinMedia
	add := func(value any) {
		addr, _ := value.(map[string]any)
		size := positiveInteger(addr["data_size"])
		if size == 0 {
			size = positiveInteger(addr["dataSize"])
		}
		var urls []string
		if raw, ok := value.(string); ok {
			urls = append(urls, raw)
		}
		for _, key := range []string{"url_list", "urlList"} {
			list, _ := addr[key].([]any)
			for _, raw := range list {
				urls = append(urls, stringValue(raw))
			}
		}
		// Prefer a direct CDN URL over another API redirect to reduce requests.
		selected, direct := "", false
		for _, raw := range urls {
			u, err := url.Parse(secureResource(raw))
			if err != nil || !allowedURL("douyin", "media", u) {
				continue
			}
			isDirect := u.Hostname() != "api-play.amemv.com" && u.Hostname() != "api.amemv.com" && u.Hostname() != "aweme.snssdk.com"
			if selected == "" || (!direct && isDirect) {
				selected, direct = u.String(), isDirect
			}
		}
		if selected != "" {
			u, _ := url.Parse(selected)
			path := strings.ToLower(u.Path)
			audio := false
			for _, ext := range []string{".mp3", ".m4a", ".wav", ".flac", ".ogg"} {
				audio = audio || strings.HasSuffix(path, ext)
			}
			candidates = append(candidates, douyinMedia{selected, size, audio})
		}
	}
	for _, key := range []string{"play_addr", "playAddr", "play_addr_h264", "play_addr_bytevc1", "download_addr"} {
		add(video[key])
	}
	for _, key := range []string{"bit_rate", "bitRate"} {
		bitrates, _ := video[key].([]any)
		for _, value := range bitrates {
			rate, _ := value.(map[string]any)
			add(rate["play_addr"])
			add(rate["playAddr"])
		}
	}
	var best douyinMedia
	for _, item := range candidates {
		if best.url == "" || betterDouyinMedia(item, best) {
			best = item
		}
	}
	return best.url, best.size
}

func betterDouyinMedia(a, b douyinMedia) bool {
	if (a.size > MaxMediaBytes) != (b.size > MaxMediaBytes) {
		return a.size <= MaxMediaBytes
	}
	if a.audio != b.audio {
		return a.audio
	}
	if (a.size > 0) != (b.size > 0) {
		return a.size > 0
	}
	return a.size > 0 && a.size < b.size
}
