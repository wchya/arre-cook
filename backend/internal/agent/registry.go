// Package agent 定义对智能体开放的工具集（单一事实来源）。
//
// 同一套工具通过三种通道暴露，全部绑定到“当前调用者所属的用户”，天然只能读写本人数据：
//   - REST 函数调用：GET /api/agent/tools（OpenAI/DeepSeek function calling 格式）+ POST /api/agent/tools/:name
//   - MCP：POST /mcp（Streamable HTTP，JSON-RPC 2.0），供 Hermes Agent、Claude、Cursor 等 MCP 客户端直连
//   - 站内 AI 助手：服务端用 OpenAI 兼容接口（默认 DeepSeek）驱动工具调用循环
//
// 每次调用都做 scope 校验并写审计日志，用户可在「我的 → AI 连接」查看与撤销。
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"ninimenu/internal/auth"
	"ninimenu/internal/services"
	"sort"
	"strings"
	"time"
)

// Ctx 工具调用上下文。Principal 决定数据归属与权限。
type Ctx struct {
	Context   context.Context
	Principal *auth.Principal
	Channel   string // rest / mcp / chat
}

func (c *Ctx) UID() uint { return c.Principal.UserID() }

// Source 行为事件来源：站内助手记为 assistant，外部智能体记为 agent:<名称>。
func (c *Ctx) Source() string {
	if c.Channel == "chat" {
		return "assistant"
	}
	return c.Principal.Source()
}

type Handler func(ctx *Ctx, args json.RawMessage) (any, error)

type Tool struct {
	Name        string
	Title       string
	Description string
	Scope       string
	Scopes      []string
	Write       bool
	Schema      map[string]any
	Handler     Handler
}

func (t *Tool) requiredScopes() []string {
	if len(t.Scopes) > 0 {
		return t.Scopes
	}
	if t.Scope == "" {
		return nil
	}
	return []string{t.Scope}
}

func (t *Tool) ScopeDescription() string {
	return strings.Join(t.requiredScopes(), " + ")
}

func (t *Tool) allowed(p *auth.Principal) bool {
	for _, scope := range t.requiredScopes() {
		if !p.Can(scope) {
			return false
		}
	}
	return true
}

var (
	registry = map[string]*Tool{}
	order    []string
)

func register(t *Tool) {
	if t.Schema == nil {
		t.Schema = object(nil)
	}
	registry[t.Name] = t
	order = append(order, t.Name)
}

// ErrForbidden 缺少权限。
type ErrForbidden struct{ Scope string }

func (e ErrForbidden) Error() string { return "令牌缺少权限：" + e.Scope }

var ErrUnknownTool = errors.New("未知工具")

// Get 按名称取工具。
func Get(name string) (*Tool, bool) {
	t, ok := registry[name]
	return t, ok
}

// List 调用者有权使用的工具（按注册顺序）。
func List(p *auth.Principal) []*Tool {
	out := make([]*Tool, 0, len(order))
	for _, name := range order {
		t := registry[name]
		if t.allowed(p) {
			out = append(out, t)
		}
	}
	return out
}

// Invoke 执行工具：权限校验 → 调用 → 审计。
func Invoke(ctx *Ctx, name string, args json.RawMessage) (any, error) {
	t, ok := registry[name]
	start := time.Now()
	entry := services.AuditEntry{
		UserID: ctx.UID(), TokenID: ctx.Principal.TokenID, Actor: actorName(ctx), Channel: ctx.Channel,
		Tool: name, Args: compactArgs(args),
	}
	if !ok {
		return nil, ErrUnknownTool
	}
	if !t.allowed(ctx.Principal) {
		missing := make([]string, 0)
		for _, scope := range t.requiredScopes() {
			if !ctx.Principal.Can(scope) {
				missing = append(missing, scope)
			}
		}
		entry.Status, entry.Error = "denied", "missing scope "+strings.Join(missing, ", ")
		services.WriteAudit(entry)
		return nil, ErrForbidden{Scope: strings.Join(missing, ", ")}
	}
	if len(args) == 0 || string(args) == "null" {
		args = json.RawMessage("{}")
	}
	result, err := safeCall(t.Handler, ctx, args)
	entry.DurationMs = time.Since(start).Milliseconds()
	if err != nil {
		entry.Status, entry.Error = "error", err.Error()
	} else {
		entry.Status = "ok"
	}
	// 只读调用量大，只审计外部智能体；站内助手与所有写操作都记录
	if t.Write || ctx.Principal.IsAgent() || err != nil {
		services.WriteAudit(entry)
	}
	return result, err
}

func safeCall(h Handler, ctx *Ctx, args json.RawMessage) (res any, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("工具执行异常: %v", r)
		}
	}()
	return h(ctx, args)
}

func actorName(ctx *Ctx) string {
	if ctx.Channel == "chat" {
		return "assistant"
	}
	if ctx.Principal.Actor != "" {
		return ctx.Principal.Actor
	}
	return ctx.Principal.Kind
}

func compactArgs(args json.RawMessage) string {
	s := strings.TrimSpace(string(args))
	if len(s) > 2000 {
		s = s[:2000]
	}
	return s
}

func decode[T any](args json.RawMessage) (T, error) {
	var v T
	if err := json.Unmarshal(args, &v); err != nil {
		return v, fmt.Errorf("参数格式错误: %v", err)
	}
	return v, nil
}

// ---------------- JSON Schema 小工具 ----------------

func object(props map[string]any, required ...string) map[string]any {
	if props == nil {
		props = map[string]any{}
	}
	s := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func str(desc string, enum ...string) map[string]any {
	s := map[string]any{"type": "string", "description": desc}
	if len(enum) > 0 {
		s["enum"] = enum
	}
	return s
}

func integer(desc string, min, max int) map[string]any {
	return map[string]any{"type": "integer", "description": desc, "minimum": min, "maximum": max}
}

func boolean(desc string) map[string]any {
	return map[string]any{"type": "boolean", "description": desc}
}

func strArray(desc string) map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": desc}
}

func intArray(desc string) map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": "integer"}, "description": desc}
}

// ---------------- 对外描述 ----------------

// OpenAITools OpenAI / DeepSeek function calling 格式。
func OpenAITools(p *auth.Principal) []map[string]any {
	tools := List(p)
	out := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  t.Schema,
			},
		})
	}
	return out
}

// MCPTools MCP tools/list 格式。
func MCPTools(p *auth.Principal) []map[string]any {
	tools := List(p)
	out := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		out = append(out, map[string]any{
			"name":        t.Name,
			"title":       t.Title,
			"description": t.Description,
			"inputSchema": t.Schema,
			"annotations": map[string]any{
				"readOnlyHint":    !t.Write,
				"destructiveHint": t.Name == "delete_meal_record" || t.Name == "delete_food_journal" || t.Name == "delete_private_recipe",
				"openWorldHint":   false,
			},
		})
	}
	return out
}

// Catalog 工具目录（名称、权限、是否写操作），用于能力清单与前端展示。
func Catalog(p *auth.Principal) []map[string]any {
	names := append([]string{}, order...)
	sort.SliceStable(names, func(i, j int) bool { return !registry[names[i]].Write && registry[names[j]].Write })
	out := make([]map[string]any, 0, len(names))
	for _, n := range names {
		t := registry[n]
		if !t.allowed(p) {
			continue
		}
		scopes := t.requiredScopes()
		out = append(out, map[string]any{"name": t.Name, "title": t.Title, "scope": t.ScopeDescription(), "scopes": scopes, "write": t.Write, "description": t.Description})
	}
	return out
}
