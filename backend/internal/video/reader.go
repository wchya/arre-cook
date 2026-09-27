package video

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const MaxDuration = 600
const MaxTranscriptRunes = 8000
const MaxMediaBytes = 20 << 20

type Metadata struct {
	URL      string `json:"url"`
	Platform string `json:"platform"`
	Title    string `json:"title"`
	Cover    string `json:"cover"`
	Author   string `json:"author"`
	Duration int    `json:"duration"`
	AID      int64  `json:"-"`
	CID      int64  `json:"-"`
	BVID     string `json:"-"`
	Subtitle string `json:"-"`
	Media    string `json:"-"`
}

type Source struct {
	URL      string `json:"url"`
	Platform string `json:"platform"`
	Method   string `json:"method"` // subtitle / audio / manual
	Text     string `json:"text"`
}

type Reader struct {
	client      *Client
	transcripts *cache
}

func NewReader(client *Client) *Reader {
	transcripts := newCache(64, 2<<20)
	transcripts.cacheErrors = false // Includes per-request AI-quota failures.
	return &Reader{client: client, transcripts: transcripts}
}

func (r *Reader) resolve(ctx context.Context, in Input) (Input, []byte, error) {
	if !in.Short {
		return in, nil, nil
	}
	res, err := r.client.get(ctx, in.Platform, "page", in.URL, 2<<20)
	if err != nil {
		return Input{}, nil, err
	}
	resolved, err := Parse(res.URL)
	if err != nil || resolved.Short || resolved.Platform != in.Platform {
		return Input{}, nil, problem("unresolved_link", "短链接未能定位到视频，请粘贴视频的完整地址")
	}
	return resolved, res.Body, nil
}

func (r *Reader) Preview(ctx context.Context, raw string) (Metadata, error) {
	in, err := Parse(raw)
	if err != nil {
		return Metadata{}, err
	}
	in, page, err := r.resolve(ctx, in)
	if err != nil {
		return Metadata{}, err
	}
	return r.metadata(ctx, in, page)
}

func (r *Reader) metadata(ctx context.Context, in Input, page []byte) (Metadata, error) {
	if in.Platform == "bilibili" {
		return r.biliMetadata(ctx, in)
	}
	if len(page) == 0 {
		// Public share pages contain the target video's JSON on supported versions.
		res, err := r.client.get(ctx, "douyin", "page", "https://www.iesdouyin.com/share/video/"+in.ID+"/", 2<<20)
		if err != nil {
			return Metadata{}, err
		}
		resolved, err := Parse(res.URL)
		if err != nil || resolved.ID != in.ID {
			return Metadata{}, problem("video_mismatch", "平台未返回目标视频，请使用完整链接或粘贴字幕")
		}
		page = res.Body
	}
	return parseDouyin(page, in)
}

func (r *Reader) Transcript(ctx context.Context, raw, manual string, asr ASRSettings, status func(string), beforeAI func() error) (Source, error) {
	in, err := Parse(raw)
	if err != nil {
		return Source{}, err
	}
	if strings.TrimSpace(manual) != "" {
		text, err := ValidateTranscript(manual)
		return Source{URL: in.URL, Platform: in.Platform, Method: "manual", Text: text}, err
	}
	status("正在读取公开视频内容…")
	in, page, err := r.resolve(ctx, in)
	if err != nil {
		return Source{}, err
	}
	// No manual text enters this shared public-content cache.
	key := in.URL + ":" + asr.cacheKey()
	res, err := r.transcripts.get(ctx, key, func() (response, error) {
		meta, err := r.metadata(ctx, in, page)
		if err != nil {
			return response{}, err
		}
		if meta.Duration > MaxDuration {
			return response{}, problem("too_long", "自动读取支持 10 分钟以内的视频，长视频可粘贴相关字幕提炼")
		}
		subtitle := meta.Subtitle
		if in.Platform == "bilibili" {
			subtitle, err = r.biliSubtitle(ctx, meta)
			if err != nil {
				return response{}, err
			}
		}
		source := Source{URL: in.URL, Platform: in.Platform, Method: "subtitle"}
		if subtitle != "" {
			status("正在读取视频字幕…")
			res, err := r.client.get(ctx, in.Platform, "subtitle", secureResource(subtitle), 512<<10)
			if err != nil {
				return response{}, err
			}
			source.Text, err = parseSubtitle(res.Body)
			if err != nil {
				return response{}, err
			}
		} else {
			if !asr.Enabled() {
				return response{}, problem("transcript_required", "平台未返回可读取的字幕文件；画面上的字幕无法直接当作字幕文件读取。本站尚未启用语音转写，可先粘贴字幕或文稿提炼")
			}
			if meta.Duration <= 0 {
				return response{}, problem("unknown_duration", "无法确认视频时长，请粘贴字幕提炼")
			}
			media := meta.Media
			if in.Platform == "bilibili" {
				media, err = r.biliMedia(ctx, meta)
				if err != nil {
					return response{}, err
				}
			}
			if media == "" {
				return response{}, problem("transcript_required", "平台未提供可读取的字幕或音频，可粘贴字幕继续提炼")
			}
			status("正在读取视频音频…")
			res, err := r.client.get(ctx, in.Platform, "media", secureResource(media), MaxMediaBytes)
			if err != nil {
				return response{}, err
			}
			if _, _, err := mediaFormat(res.Body); err != nil {
				return response{}, err
			}
			status("正在将视频语音转成文字…")
			source.Method = "audio"
			source.Text, err = transcribe(ctx, asr, res.Body, r.client.gate, beforeAI)
			if err != nil {
				return response{}, err
			}
		}
		body, err := json.Marshal(source)
		return response{Body: body}, err
	})
	if err != nil {
		return Source{}, err
	}
	var source Source
	if err := json.Unmarshal(res.Body, &source); err != nil {
		return Source{}, err
	}
	return source, nil
}

func secureResource(raw string) string {
	if strings.HasPrefix(raw, "//") {
		return "https:" + raw
	}
	if strings.HasPrefix(raw, "http://") {
		return "https://" + strings.TrimPrefix(raw, "http://")
	}
	return raw
}

func (r *Reader) biliJSON(ctx context.Context, endpoint string, target any) error {
	res, err := r.client.get(ctx, "bilibili", "api", "https://api.bilibili.com"+endpoint, 1<<20)
	if err != nil {
		return err
	}
	var envelope struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal(res.Body, &envelope) != nil {
		return problem("video_unavailable", "平台未提供公开视频内容，可粘贴字幕提炼")
	}
	if envelope.Code == -412 || envelope.Code == -403 || envelope.Code == -352 || envelope.Code == -429 {
		return r.client.blocked(ctx, "bilibili", "")
	}
	if envelope.Code != 0 {
		return problem("video_unavailable", "视频需要登录、已失效或暂时不可读取，可粘贴字幕提炼")
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return problem("video_unavailable", "平台未提供公开视频内容，可粘贴字幕提炼")
	}
	if err := json.Unmarshal(envelope.Data, target); err != nil {
		return problem("video_unavailable", "视频内容格式暂不支持，可粘贴字幕提炼")
	}
	return nil
}

func (r *Reader) biliMetadata(ctx context.Context, in Input) (Metadata, error) {
	query := "bvid=" + in.ID
	if strings.HasPrefix(in.ID, "av") {
		query = "aid=" + strings.TrimPrefix(in.ID, "av")
	}
	var data struct {
		AID   json.Number `json:"aid"`
		BVID  string      `json:"bvid"`
		Title string      `json:"title"`
		Pic   string      `json:"pic"`
		Owner struct {
			Name string `json:"name"`
		} `json:"owner"`
		Pages []struct {
			CID      int64  `json:"cid"`
			Page     int    `json:"page"`
			Duration int    `json:"duration"`
			Part     string `json:"part"`
		} `json:"pages"`
	}
	if err := r.biliJSON(ctx, "/x/web-interface/view?"+query, &data); err != nil {
		return Metadata{}, err
	}
	if !biliPath.MatchString("/video/"+data.BVID) || strings.HasPrefix(in.ID, "BV") && data.BVID != in.ID {
		return Metadata{}, problem("video_mismatch", "未能确认目标视频，请检查链接")
	}
	if strings.HasPrefix(in.ID, "av") && data.AID.String() != strings.TrimLeft(strings.TrimPrefix(in.ID, "av"), "0") {
		return Metadata{}, problem("video_mismatch", "未能确认目标视频，请检查链接")
	}
	aid, err := data.AID.Int64()
	if err != nil || aid <= 0 {
		return Metadata{}, problem("video_mismatch", "未能确认目标视频，请检查链接")
	}
	for _, page := range data.Pages {
		if page.Page != in.Page || page.CID <= 0 {
			continue
		}
		title := data.Title
		if len(data.Pages) > 1 {
			title += " · " + page.Part
		}
		return Metadata{URL: in.URL, Platform: in.Platform, Title: title, Cover: secureResource(data.Pic), Author: data.Owner.Name, Duration: page.Duration, AID: aid, CID: page.CID, BVID: data.BVID}, nil
	}
	return Metadata{}, problem("invalid_part", "未找到视频的这一分 P，请检查链接")
}

func (r *Reader) biliSubtitle(ctx context.Context, meta Metadata) (string, error) {
	// The player endpoint can omit public AI subtitles. The DM metadata endpoint
	// exposes them for some videos without login; both use the selected part's CID.
	endpoints := []string{
		fmt.Sprintf("/x/v2/dm/view?aid=%d&oid=%d&type=1", meta.AID, meta.CID),
		"/x/player/v2?bvid=" + url.QueryEscape(meta.BVID) + "&cid=" + strconv.FormatInt(meta.CID, 10),
	}
	for _, endpoint := range endpoints {
		var data biliSubtitles
		if err := r.biliJSON(ctx, endpoint, &data); err != nil {
			// Do not try alternate endpoints after transport errors or platform blocks.
			return "", err
		}
		if selected := data.choose(); selected != "" {
			return selected, nil
		}
	}
	return "", nil
}

type biliSubtitles struct {
	Subtitle struct {
		Subtitles []struct {
			URL    string `json:"subtitle_url"`
			Lang   string `json:"lan"`
			Locked bool   `json:"is_lock"`
		} `json:"subtitles"`
	} `json:"subtitle"`
}

func (data biliSubtitles) choose() string {
	selected, best := "", 5
	for _, sub := range data.Subtitle.Subtitles {
		if sub.Locked || sub.URL == "" {
			continue
		}
		rank := 2
		switch {
		case strings.HasPrefix(sub.Lang, "zh"):
			rank = 0
		case strings.HasPrefix(sub.Lang, "ai-zh"):
			rank = 1
		case strings.HasPrefix(sub.Lang, "ai-"):
			rank = 3
		}
		if rank < best {
			selected, best = sub.URL, rank
		}
	}
	return selected
}

func (r *Reader) biliMedia(ctx context.Context, meta Metadata) (string, error) {
	var data struct {
		Dash struct {
			Audio []struct {
				BaseURL   string `json:"baseUrl"`
				AltURL    string `json:"base_url"`
				Bandwidth int    `json:"bandwidth"`
			} `json:"audio"`
		} `json:"dash"`
		DURL []struct {
			URL string `json:"url"`
		} `json:"durl"`
	}
	endpoint := fmt.Sprintf("/x/player/playurl?bvid=%s&cid=%d&qn=16&fnval=16&fnver=0&fourk=0", url.QueryEscape(meta.BVID), meta.CID)
	if err := r.biliJSON(ctx, endpoint, &data); err != nil {
		return "", err
	}
	selected, bitrate := "", 0
	for _, audio := range data.Dash.Audio {
		raw := audio.BaseURL
		if raw == "" {
			raw = audio.AltURL
		}
		if raw != "" && (selected == "" || audio.Bandwidth < bitrate) {
			selected, bitrate = raw, audio.Bandwidth
		}
	}
	if selected == "" && len(data.DURL) == 1 {
		selected = data.DURL[0].URL
	}
	return selected, nil
}
