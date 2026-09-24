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
	"strings"
	"time"
)

type Settings struct {
	BaseURL string
	APIKey  string
	Model   string
}

func (s Settings) Enabled() bool { return s.APIKey != "" && s.BaseURL != "" && s.Model != "" }

// Resolve 管理后台设置优先，其次环境变量。
func Resolve() Settings {
	return Settings{
		BaseURL: strings.TrimRight(database.GetSetting("llm_base_url", config.C.LLMBaseURL), "/"),
		APIKey:  database.GetSetting("llm_api_key", config.C.LLMAPIKey),
		Model:   database.GetSetting("llm_model", config.C.LLMModel),
	}
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
