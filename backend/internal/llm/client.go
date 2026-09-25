// Package llm 是 OpenAI 兼容的 Chat Completions 客户端（流式 + 工具调用）。
// 默认对接 DeepSeek（https://api.deepseek.com，模型 deepseek-chat）；
// 通义千问、Kimi、智谱、OpenAI 等兼容接口只需改 base_url / model / api_key。
package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"ninimenu/internal/config"
	"ninimenu/internal/database"
	"os"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
)

type Settings struct {
	BaseURL string
	APIKey  string
	Model   string
}

func (s Settings) Enabled() bool { return s.APIKey != "" && s.BaseURL != "" && s.Model != "" }

// Resolve 优先读取 CPA 配置；未挂载 CPA 配置时再使用管理后台或环境变量。
func Resolve() Settings {
	if settings, ok := resolveCPA(); ok {
		return settings
	}
	return Settings{
		BaseURL: strings.TrimRight(database.GetSetting("llm_base_url", config.C.LLMBaseURL), "/"),
		APIKey:  database.GetSetting("llm_api_key", config.C.LLMAPIKey),
		Model:   database.GetSetting("llm_model", config.C.LLMModel),
	}
}

type cpaModel struct {
	Name  string `yaml:"name"`
	Alias string `yaml:"alias"`
}

type cpaCredential struct {
	Models []cpaModel `yaml:"models"`
}

type cpaConfig struct {
	APIKeys      []string        `yaml:"api-keys"`
	CodexAPIKey  []cpaCredential `yaml:"codex-api-key"`
	ClaudeAPIKey []cpaCredential `yaml:"claude-api-key"`
}

type cpaOverride struct {
	BaseURL         string `json:"baseUrl"`
	APIKey          string `json:"apiKey"`
	Model           string `json:"model"`
	CompletionsPath string `json:"completionsPath"`
}

// resolveCPA 读取 CPA 的原始配置，让 CPA 成为唯一的模型凭据来源。
// 配置文件只读挂载到应用容器；未挂载时继续使用原有 LLM_* 配置。
func resolveCPA() (Settings, bool) {
	path := strings.TrimSpace(config.C.CPAConfigPath)
	baseURL := strings.TrimRight(strings.TrimSpace(config.C.CPABaseURL), "/")
	if path == "" {
		return Settings{}, false
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return Settings{}, false
	}
	if settings, ok := parseCPAOverride(b, baseURL); ok {
		return settings, true
	}
	var raw cpaConfig
	if err := yaml.Unmarshal(b, &raw); err != nil {
		return Settings{}, false
	}
	apiKey := ""
	for _, key := range raw.APIKeys {
		if strings.TrimSpace(key) != "" {
			apiKey = strings.TrimSpace(key)
			break
		}
	}
	model := strings.TrimSpace(config.C.CPAModel)
	if model == "" {
		model = preferredCPAModel(raw.CodexAPIKey, raw.ClaudeAPIKey)
	}
	if apiKey == "" || model == "" {
		return Settings{}, false
	}
	return Settings{BaseURL: baseURL, APIKey: apiKey, Model: model}, true
}

func parseCPAOverride(data []byte, fallbackBaseURL string) (Settings, bool) {
	var overrides map[string]cpaOverride
	if err := json.Unmarshal(data, &overrides); err != nil {
		return Settings{}, false
	}
	for _, override := range overrides {
		apiKey := strings.TrimSpace(override.APIKey)
		model := strings.TrimSpace(override.Model)
		baseURL := strings.TrimSpace(fallbackBaseURL)
		if baseURL == "" {
			baseURL = strings.TrimSpace(override.BaseURL)
		}
		baseURL = cpaChatBaseURL(baseURL, override.CompletionsPath)
		if apiKey != "" && model != "" && baseURL != "" {
			return Settings{BaseURL: baseURL, APIKey: apiKey, Model: model}, true
		}
	}
	return Settings{}, false
}

func cpaChatBaseURL(baseURL, completionsPath string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" || strings.HasSuffix(baseURL, "/v1") {
		return baseURL
	}
	path := strings.Trim(strings.TrimSpace(completionsPath), "/")
	if marker := strings.LastIndex(path, "/chat/completions"); marker >= 0 {
		if prefix := strings.Trim(path[:marker], "/"); prefix != "" {
			return baseURL + "/" + prefix
		}
	}
	return baseURL
}

func preferredCPAModel(groups ...[]cpaCredential) string {
	preferred := []string{"glm-5.3", "glm-5.3-flash", "gpt-5.5", "gpt-6-sol", "gpt-5.6", "gpt-5.6-sol", "gpt-6-astra"}
	available := make(map[string]bool)
	for _, group := range groups {
		for _, credential := range group {
			for _, model := range credential.Models {
				name := strings.TrimSpace(model.Name)
				if name != "" {
					available[name] = true
				}
				alias := strings.TrimSpace(model.Alias)
				if alias != "" {
					available[alias] = true
				}
			}
		}
	}
	for _, name := range preferred {
		if available[name] {
			return name
		}
	}
	for name := range available {
		return name
	}
	return ""
}

type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

type Request struct {
	Model       string           `json:"model"`
	Messages    []Message        `json:"messages"`
	Tools       []map[string]any `json:"tools,omitempty"`
	ToolChoice  string           `json:"tool_choice,omitempty"`
	Stream      bool             `json:"stream"`
	Temperature float64          `json:"temperature"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
}

// Result 一轮流式输出聚合后的结果。
type Result struct {
	Content      string
	ToolCalls    []ToolCall
	FinishReason string
}

type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

var httpClient = &http.Client{Timeout: 120 * time.Second}

// Stream 发起一轮流式对话；onDelta 收到正文增量时回调。
func Stream(ctx context.Context, s Settings, req Request, onDelta func(string)) (*Result, error) {
	if !s.Enabled() {
		return nil, errors.New("AI 模型未配置")
	}
	req.Model = s.Model
	req.Stream = true
	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, s.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("Authorization", "Bearer "+s.APIKey)

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("连接 AI 服务失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("AI 服务返回 %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}

	res := &Result{}
	calls := map[int]*ToolCall{}
	maxIndex := -1
	var content strings.Builder

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if chunk.Error != nil {
			return nil, errors.New(chunk.Error.Message)
		}
		for _, ch := range chunk.Choices {
			if ch.Delta.Content != "" {
				content.WriteString(ch.Delta.Content)
				if onDelta != nil {
					onDelta(ch.Delta.Content)
				}
			}
			for _, tc := range ch.Delta.ToolCalls {
				c := calls[tc.Index]
				if c == nil {
					c = &ToolCall{Type: "function"}
					calls[tc.Index] = c
				}
				if tc.Index > maxIndex {
					maxIndex = tc.Index
				}
				if tc.ID != "" {
					c.ID = tc.ID
				}
				if tc.Function.Name != "" {
					c.Function.Name += tc.Function.Name
				}
				c.Function.Arguments += tc.Function.Arguments
			}
			if ch.FinishReason != "" {
				res.FinishReason = ch.FinishReason
			}
		}
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) {
		return nil, fmt.Errorf("读取 AI 响应失败: %w", err)
	}
	res.Content = content.String()
	for i := 0; i <= maxIndex; i++ {
		if c := calls[i]; c != nil && c.Function.Name != "" {
			if c.ID == "" {
				c.ID = fmt.Sprintf("call_%d", i)
			}
			res.ToolCalls = append(res.ToolCalls, *c)
		}
	}
	return res, nil
}

// Ping 用一条极短请求验证配置是否可用（管理后台“测试连接”）。
func Ping(ctx context.Context, s Settings) error {
	_, err := Stream(ctx, s, Request{
		Messages:  []Message{{Role: "user", Content: "回复 ok"}},
		MaxTokens: 5,
	}, nil)
	return err
}
