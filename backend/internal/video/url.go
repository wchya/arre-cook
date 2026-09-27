// Package video reads public cooking-video content. It never uses login cookies,
// executes page scripts, or bypasses platform challenges.
package video

import (
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

type Input struct {
	URL      string
	Platform string
	ID       string
	Page     int
	Short    bool
}

type Error struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	RetryAfter int    `json:"retry_after,omitempty"`
}

func (e *Error) Error() string { return e.Message }

func problem(code, message string) error { return &Error{Code: code, Message: message} }

var (
	linkPattern = regexp.MustCompile(`https?://[^\s<>"'，。；！）】]+`)
	biliPath    = regexp.MustCompile(`^/video/(BV[0-9A-Za-z]{10}|av[0-9]{1,20})/?$`)
	douyinPath  = regexp.MustCompile(`^/(?:share/)?video/([0-9]{1,20})/?$`)
	shortPath   = regexp.MustCompile(`^/[a-zA-Z0-9_-]{1,80}/?$`)
	videoID     = regexp.MustCompile(`^[0-9]{1,20}$`)
)

// Parse removes tracking parameters and accepts exactly one URL in share text.
// Paths, hosts and ports are validated before any network operation.
func Parse(raw string) (Input, error) {
	bad := problem("invalid_url", "请粘贴一个有效的 B 站或抖音视频链接")
	if len(raw) > 6144 {
		return Input{}, bad
	}
	links := linkPattern.FindAllString(strings.TrimSpace(raw), -1)
	if len(links) != 1 {
		return Input{}, bad
	}
	u, err := url.Parse(strings.TrimRight(links[0], ".,;!)]}"))
	if err != nil || u.User != nil || u.Host == "" || u.Opaque != "" || u.RawPath != "" || strings.Contains(u.Path, "\\") {
		return Input{}, bad
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return Input{}, bad
	}
	if port := u.Port(); port != "" && !(u.Scheme == "http" && port == "80") && !(u.Scheme == "https" && port == "443") {
		return Input{}, bad
	}
	host := strings.ToLower(u.Hostname())
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return Input{}, bad
	}
	in := Input{Page: 1}
	switch host {
	case "b23.tv":
		if !shortPath.MatchString(u.Path) {
			return Input{}, bad
		}
		in.Platform, in.Short = "bilibili", true
		in.URL = "https://b23.tv" + u.Path
	case "bilibili.com", "www.bilibili.com", "m.bilibili.com":
		match := biliPath.FindStringSubmatch(u.Path)
		if match == nil {
			return Input{}, bad
		}
		in.Platform, in.ID = "bilibili", match[1]
		if values, ok := q["p"]; ok {
			if len(values) != 1 {
				return Input{}, bad
			}
			in.Page, err = strconv.Atoi(values[0])
			if err != nil || in.Page < 1 || in.Page > 100 {
				return Input{}, bad
			}
		}
		in.URL = "https://www.bilibili.com/video/" + in.ID + "/"
		if in.Page > 1 {
			in.URL += "?p=" + strconv.Itoa(in.Page)
		}
	case "v.douyin.com":
		if !shortPath.MatchString(u.Path) {
			return Input{}, bad
		}
		in.Platform, in.Short = "douyin", true
		in.URL = "https://v.douyin.com" + u.Path
	case "douyin.com", "www.douyin.com", "iesdouyin.com", "www.iesdouyin.com":
		match := douyinPath.FindStringSubmatch(u.Path)
		if match != nil {
			in.ID = match[1]
		} else if (u.Path == "" || u.Path == "/") && len(q["modal_id"]) == 1 && videoID.MatchString(q.Get("modal_id")) {
			in.ID = q.Get("modal_id")
		} else {
			return Input{}, bad
		}
		in.Platform = "douyin"
		in.URL = "https://www.douyin.com/video/" + in.ID
	default:
		return Input{}, bad
	}
	return in, nil
}

// PublicError deliberately excludes upstream bodies, credentials and signed URLs.
func PublicError(err error) *Error {
	var known *Error
	if errors.As(err, &known) {
		return known
	}
	return &Error{Code: "unavailable", Message: "视频内容暂时无法读取，可稍后重试或粘贴字幕提炼"}
}
