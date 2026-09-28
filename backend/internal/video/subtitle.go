package video

import (
	"bytes"
	"encoding/json"
	"html"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var cueTime = regexp.MustCompile(`^\s*(?:\d{1,2}:)?\d{2}:\d{2}[.,]\d{3}\s+-->\s+(?:\d{1,2}:)?\d{2}:\d{2}[.,]\d{3}`)
var cueTags = regexp.MustCompile(`<[^>]{0,160}>`)

func ValidateTranscript(text string) (string, error) {
	if !utf8.ValidString(text) || len(text) > 32<<10 {
		return "", problem("invalid_transcript", "字幕请填写 20–8000 个字")
	}
	text = strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
	if n := utf8.RuneCountInString(text); n < 20 || n > MaxTranscriptRunes {
		return "", problem("invalid_transcript", "字幕请填写 20–8000 个字，长视频请选取相关片段")
	}
	for _, r := range text {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return "", problem("invalid_transcript", "字幕包含无法处理的字符，请粘贴纯文本")
		}
	}
	return text, nil
}

func parseSubtitle(body []byte) (string, error) {
	body = bytes.TrimPrefix(body, []byte{0xef, 0xbb, 0xbf})
	var data struct {
		Body []struct {
			Content string `json:"content"`
		} `json:"body"`
		Utterances []struct {
			Text string `json:"text"`
		} `json:"utterances"`
		Subtitles []struct {
			Text string `json:"text"`
		} `json:"subtitles"`
		Sentences []struct {
			Text string `json:"text"`
		} `json:"sentences"`
	}
	var lines []string
	if json.Unmarshal(body, &data) == nil {
		for _, cue := range data.Body {
			lines = append(lines, cue.Content)
		}
		if len(lines) == 0 {
			for _, cue := range data.Utterances {
				lines = append(lines, cue.Text)
			}
		}
		if len(lines) == 0 {
			for _, cue := range data.Subtitles {
				lines = append(lines, cue.Text)
			}
		}
		if len(lines) == 0 {
			for _, cue := range data.Sentences {
				lines = append(lines, cue.Text)
			}
		}
	} else {
		// Only actual timed VTT/SRT cues are accepted, never HTML titles/descriptions.
		inCue := false
		for _, line := range strings.Split(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				inCue = false
				continue
			}
			if cueTime.MatchString(line) {
				inCue = true
				continue
			}
			// Cue identifiers are outside the timed body. Numeric lines inside
			// it can be ingredient quantities or cooking times and must survive.
			if inCue {
				lines = append(lines, html.UnescapeString(cueTags.ReplaceAllString(line, "")))
			}
		}
	}
	if len(lines) == 0 {
		return "", problem("transcript_required", "未读取到有效视频字幕，可粘贴字幕或文稿继续提炼")
	}
	return ValidateTranscript(strings.Join(lines, "\n"))
}
