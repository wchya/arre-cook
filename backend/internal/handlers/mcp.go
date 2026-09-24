package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"ninimenu/internal/agent"
	"ninimenu/internal/auth"
	"ninimenu/internal/database"
	"strings"

	"github.com/gin-gonic/gin"
)

// MCP（Model Context Protocol）Streamable HTTP 传输，无状态实现：
//   - POST /mcp 接收 JSON-RPC 2.0 消息（单条或批量），以 application/json 返回；
//   - GET /mcp 返回 405（不提供服务端主动推送流）；
//   - 认证：Authorization: Bearer <个人访问令牌>，工具只作用于令牌所属用户。
// Hermes Agent、Claude、Cursor、Cherry Studio 等 MCP 客户端配置 URL + 请求头即可接入。

var mcpSupportedVersions = []string{"2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"}

const mcpLatestVersion = "2025-06-18"

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

const (
	rpcParseError     = -32700
	rpcInvalidRequest = -32600
	rpcMethodNotFound = -32601
	rpcInvalidParams  = -32602
	rpcInternalError  = -32603
)

func MCPGet(c *gin.Context) {
	c.Header("Allow", "POST")
	c.JSON(http.StatusMethodNotAllowed, gin.H{"error": "此 MCP 服务为无状态实现，请使用 POST"})
}

func MCPDelete(c *gin.Context) {
	c.Status(http.StatusMethodNotAllowed)
}

func MCPPost(c *gin.Context) {
	if v := c.GetHeader("Mcp-Protocol-Version"); v != "" && !mcpVersionSupported(v) {
		c.JSON(http.StatusBadRequest, rpcResponse{JSONRPC: "2.0", ID: json.RawMessage("null"),
			Error: &rpcError{Code: rpcInvalidRequest, Message: "Unsupported MCP-Protocol-Version: " + v}})
		return
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		c.JSON(http.StatusBadRequest, rpcResponse{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{Code: rpcParseError, Message: "read error"}})
		return
	}
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		c.JSON(http.StatusBadRequest, rpcResponse{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{Code: rpcParseError, Message: "empty body"}})
		return
	}

	// 批量（2025-03-26 允许）
	if body[0] == '[' {
		var batch []json.RawMessage
		if err := json.Unmarshal(body, &batch); err != nil || len(batch) == 0 {
			c.JSON(http.StatusBadRequest, rpcResponse{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{Code: rpcParseError, Message: "invalid JSON"}})
			return
		}
		responses := make([]rpcResponse, 0, len(batch))
		for _, raw := range batch {
			if resp := handleMCPMessage(c, raw); resp != nil {
				responses = append(responses, *resp)
			}
		}
		if len(responses) == 0 {
			c.Status(http.StatusAccepted)
			return
		}
		c.JSON(http.StatusOK, responses)
		return
	}

	resp := handleMCPMessage(c, body)
	if resp == nil {
		c.Status(http.StatusAccepted) // 通知 / 响应：无返回体
		return
	}
	c.JSON(http.StatusOK, resp)
}

func handleMCPMessage(c *gin.Context, raw json.RawMessage) *rpcResponse {
	var req rpcRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return &rpcResponse{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{Code: rpcParseError, Message: "invalid JSON"}}
	}
	isNotification := len(req.ID) == 0 || string(req.ID) == "null"
	if req.Method == "" {
		// 客户端发来的响应（我们不发请求，忽略）
		return nil
	}
	if req.JSONRPC != "2.0" {
		if isNotification {
			return nil
		}
		return &rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: rpcInvalidRequest, Message: "jsonrpc must be 2.0"}}
	}
	if isNotification {
		return nil // notifications/initialized、notifications/cancelled 等
	}

	result, rerr := dispatchMCP(c, req)
	resp := &rpcResponse{JSONRPC: "2.0", ID: req.ID}
	if rerr != nil {
		resp.Error = rerr
	} else {
		resp.Result = result
	}
	return resp
}

func dispatchMCP(c *gin.Context, req rpcRequest) (any, *rpcError) {
	p := principal(c)
	switch req.Method {
	case "initialize":
		var params struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &params)
		version := mcpLatestVersion
		if mcpVersionSupported(params.ProtocolVersion) {
			version = params.ProtocolVersion
		}
		return gin.H{
			"protocolVersion": version,
			"capabilities": gin.H{
				"tools":     gin.H{"listChanged": false},
				"resources": gin.H{"listChanged": false},
				"prompts":   gin.H{"listChanged": false},
			},
			"serverInfo": gin.H{
				"name":    "ninimenu",
				"title":   database.GetSetting("app_name", "NiniMenu") + " 食谱助手",
				"version": agentAPIVersion,
			},
			"instructions": mcpInstructions(p),
		}, nil

	case "ping":
		return gin.H{}, nil

	case "tools/list":
		return gin.H{"tools": agent.MCPTools(p)}, nil

	case "tools/call":
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil || params.Name == "" {
			return nil, &rpcError{Code: rpcInvalidParams, Message: "缺少工具名"}
		}
		if _, ok := agent.Get(params.Name); !ok {
			return nil, &rpcError{Code: rpcInvalidParams, Message: "Unknown tool: " + params.Name}
		}
		result, err := agent.Invoke(toolCtx(c, "mcp"), params.Name, params.Arguments)
		if err != nil {
			// 工具执行错误按 MCP 约定放进结果（isError），让模型能看到并自我纠正
			return gin.H{"content": []gin.H{{"type": "text", "text": "错误：" + err.Error()}}, "isError": true}, nil
		}
		return toolResult(result), nil

	case "resources/list":
		return gin.H{"resources": []gin.H{
			{"uri": "ninimenu://profile", "name": "taste-profile", "title": "口味画像", "description": "用户近 90 天的口味画像", "mimeType": "application/json"},
			{"uri": "ninimenu://preferences", "name": "preferences", "title": "饮食偏好", "description": "忌口、过敏原、辣度等", "mimeType": "application/json"},
			{"uri": "ninimenu://week-plan", "name": "week-plan", "title": "本周菜单", "mimeType": "application/json"},
		}}, nil

	case "resources/templates/list":
		return gin.H{"resourceTemplates": []gin.H{
			{"uriTemplate": "ninimenu://dish/{id}", "name": "dish", "title": "菜品详情", "mimeType": "application/json"},
		}}, nil

	case "resources/read":
		var params struct {
			URI string `json:"uri"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil || params.URI == "" {
			return nil, &rpcError{Code: rpcInvalidParams, Message: "缺少 uri"}
		}
		tool, args := "", "{}"
		switch {
		case params.URI == "ninimenu://profile":
			tool = "get_taste_profile"
		case params.URI == "ninimenu://preferences":
			tool = "get_preferences"
		case params.URI == "ninimenu://week-plan":
			tool = "get_week_plan"
		case strings.HasPrefix(params.URI, "ninimenu://dish/"):
			tool = "get_dish"
			args = fmt.Sprintf(`{"dish_id":%s}`, jsonNumber(strings.TrimPrefix(params.URI, "ninimenu://dish/")))
		default:
			return nil, &rpcError{Code: -32002, Message: "Resource not found", Data: gin.H{"uri": params.URI}}
		}
		result, err := agent.Invoke(toolCtx(c, "mcp"), tool, json.RawMessage(args))
		if err != nil {
			var forbidden agent.ErrForbidden
			if errors.As(err, &forbidden) {
				return nil, &rpcError{Code: rpcInvalidRequest, Message: err.Error()}
			}
			return nil, &rpcError{Code: -32002, Message: err.Error(), Data: gin.H{"uri": params.URI}}
		}
		b, _ := json.Marshal(result)
		return gin.H{"contents": []gin.H{{"uri": params.URI, "mimeType": "application/json", "text": string(b)}}}, nil

	case "prompts/list":
		return gin.H{"prompts": mcpPrompts()}, nil

	case "prompts/get":
		var params struct {
			Name      string            `json:"name"`
			Arguments map[string]string `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, &rpcError{Code: rpcInvalidParams, Message: "参数错误"}
		}
		text, ok := renderPrompt(params.Name, params.Arguments)
		if !ok {
			return nil, &rpcError{Code: rpcInvalidParams, Message: "Unknown prompt: " + params.Name}
		}
		return gin.H{"messages": []gin.H{{"role": "user", "content": gin.H{"type": "text", "text": text}}}}, nil

	case "logging/setLevel", "completion/complete":
		return gin.H{}, nil
	}
	return nil, &rpcError{Code: rpcMethodNotFound, Message: "Method not found: " + req.Method}
}

func toolResult(result any) gin.H {
	b, _ := json.Marshal(result)
	out := gin.H{"content": []gin.H{{"type": "text", "text": string(b)}}, "isError": false}
	// structuredContent 必须是 JSON 对象
	if len(b) > 0 && b[0] == '{' {
		out["structuredContent"] = json.RawMessage(b)
	}
	return out
}

func jsonNumber(s string) string {
	for _, r := range s {
		if r < '0' || r > '9' {
			return "0"
		}
	}
	if s == "" {
		return "0"
	}
	return s
}

func mcpVersionSupported(v string) bool {
	for _, s := range mcpSupportedVersions {
		if s == v {
			return true
		}
	}
	return false
}

func mcpInstructions(p *auth.Principal) string {
	return fmt.Sprintf(`你已连接到用户「%s」的私人食谱库（仅能访问该用户本人的数据，权限：%s）。
推荐菜品时先调用 get_context 与 get_preferences，再用 recommend_dishes（会自动遵守过敏原/忌口并避开近期吃过的菜）；
只推荐工具返回的菜，不要编造菜名或 ID。做推荐管理时优先用 create_suggestion 把建议推送到用户的收件箱，由用户确认采纳；
只有在用户明确要求时才调用 log_meal 等写入类工具。`, p.User.DisplayName(), strings.Join(p.Scopes.List(), ", "))
}

type promptDef struct {
	Name, Title, Description string
	Args                     []gin.H
	Template                 string
}

var prompts = []promptDef{
	{
		Name: "tonight_dinner", Title: "今晚吃什么", Description: "按口味画像推荐今晚的菜，并说明理由",
		Args:     []gin.H{{"name": "wish", "description": "额外想法，如 想吃辣、半小时内", "required": false}},
		Template: "请先读取我的上下文和饮食偏好，然后用 recommend_dishes 为我推荐今晚的 3 道菜（meal_type=dinner）。我的想法：{{wish}}。逐道说明推荐理由。",
	},
	{
		Name: "plan_week", Title: "规划一周菜单", Description: "结合口味画像与近期记录规划未来 7 天午晚餐，并推送到建议收件箱",
		Args:     []gin.H{{"name": "notes", "description": "补充要求，如 周三要请客", "required": false}},
		Template: "请分析我的口味画像和最近 30 天用餐记录，为未来 7 天规划午餐和晚餐（避免重复、兼顾菜系多样性）。补充要求：{{notes}}。规划好后，按天调用 create_suggestion 推送到我的建议收件箱（每天一条，写清理由）。",
	},
	{
		Name: "diet_report", Title: "饮食分析报告", Description: "生成最近一段时间的饮食分析报告",
		Args:     []gin.H{{"name": "days", "description": "统计天数，默认 30", "required": false}},
		Template: "请调用 get_taste_profile（days={{days}}）、get_stats 和 list_behavior_events，生成一份饮食分析报告：口味与菜系偏好、重复率、新菜尝试、推荐采纳率、改进建议。",
	},
}

func mcpPrompts() []gin.H {
	out := make([]gin.H, 0, len(prompts))
	for _, p := range prompts {
		out = append(out, gin.H{"name": p.Name, "title": p.Title, "description": p.Description, "arguments": p.Args})
	}
	return out
}

func renderPrompt(name string, args map[string]string) (string, bool) {
	for _, p := range prompts {
		if p.Name != name {
			continue
		}
		text := p.Template
		for _, a := range p.Args {
			key := a["name"].(string)
			val := strings.TrimSpace(args[key])
			if val == "" {
				val = map[string]string{"days": "30"}[key]
				if val == "" {
					val = "无"
				}
			}
			text = strings.ReplaceAll(text, "{{"+key+"}}", val)
		}
		return text, true
	}
	return "", false
}
