package video

import (
	"bytes"
	"encoding/json"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// Hydration JSON is parsed as data, never evaluated as JavaScript. Only a record
// with the requested aweme_id is accepted; recommended videos are ignored.
func parseDouyin(page []byte, in Input) (Metadata, error) {
	var documents []any
	z := html.NewTokenizer(bytes.NewReader(page))
	for {
		tokenType := z.Next()
		if tokenType == html.ErrorToken {
			break
		}
		if tokenType != html.StartTagToken {
			continue
		}
		token := z.Token()
		if token.Data != "script" {
			continue
		}
		id := ""
		for _, attr := range token.Attr {
			if attr.Key == "id" {
				id = attr.Val
			}
		}
		if z.Next() != html.TextToken {
			continue
		}
		raw := string(z.Text())
		if id == "RENDER_DATA" {
			decoded, err := url.QueryUnescape(raw)
			if err != nil {
				continue
			}
			raw = decoded
		} else {
			found := false
			for _, marker := range []string{"window._ROUTER_DATA", "window.__INIT_PROPS__"} {
				if pos := strings.Index(raw, marker); pos >= 0 {
					rest := strings.TrimSpace(raw[pos+len(marker):])
					if strings.HasPrefix(rest, "=") {
						raw, found = strings.TrimSpace(rest[1:]), true
						break
					}
				}
			}
			if !found {
				continue
			}
		}
		decoder := json.NewDecoder(strings.NewReader(raw))
		decoder.UseNumber()
		var document any
		if decoder.Decode(&document) == nil {
			documents = append(documents, document)
		}
	}
	for _, document := range documents {
		visited := 0
		item := findVideo(document, in.ID, 0, &visited)
		if item == nil {
			continue
		}
		return douyinMetadata(item, in), nil
	}
	return Metadata{}, problem("transcript_required", "抖音暂未开放此视频内容，可能需要登录或验证；可粘贴字幕继续提炼")
}

func findVideo(value any, id string, depth int, visited *int) map[string]any {
	*visited++
	if depth > 32 || *visited > 50000 {
		return nil
	}
	switch node := value.(type) {
	case map[string]any:
		for _, key := range []string{"aweme_id", "awemeId"} {
			if stringValue(node[key]) == id {
				if _, ok := node["video"].(map[string]any); ok {
					return node
				}
			}
		}
		for _, child := range node {
			if found := findVideo(child, id, depth+1, visited); found != nil {
				return found
			}
		}
	case []any:
		for _, child := range node {
			if found := findVideo(child, id, depth+1, visited); found != nil {
				return found
			}
		}
	}
	return nil
}

func stringValue(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	}
	return ""
}
func number(value any) int {
	if n, ok := value.(json.Number); ok {
		result, _ := n.Int64()
		if result > 0 && result < 86400000 {
			return int(result)
		}
	}
	return 0
}
func firstURL(value any) string {
	if direct, ok := value.(string); ok {
		return secureResource(direct)
	}
	if node, ok := value.(map[string]any); ok {
		for _, key := range []string{"url_list", "urlList"} {
			if list, ok := node[key].([]any); ok {
				for _, item := range list {
					if raw, ok := item.(string); ok && raw != "" {
						return secureResource(raw)
					}
				}
			}
		}
	}
	return ""
}
