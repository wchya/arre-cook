package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"ninimenu/internal/assistant"
	"ninimenu/internal/database"
	"ninimenu/internal/llm"
	"ninimenu/internal/models"
	"ninimenu/internal/utils"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

// GetAssistantStatus 站内 AI 助手是否可用（未配置大模型时退化为本地推荐引擎）。
func GetAssistantStatus(c *gin.Context) {
	s := llm.Resolve()
	utils.Success(c, gin.H{
		"llm_enabled": s.Enabled(),
		"model":       map[bool]string{true: s.Model, false: ""}[s.Enabled()],
		"suggestions": []string{"今晚吃什么？想吃辣的，半小时内", "冰箱里有鸡蛋和番茄，能做什么", "帮我规划这周的晚餐", "我最近吃得健康吗", "我不吃香菜，帮我记住"},
	})
}

// AssistantChat POST /api/assistant/chat —— SSE 流式返回：session / delta / tool_start / tool_end / error / done。
func AssistantChat(c *gin.Context) {
	var req struct {
		SessionID uint   `json:"session_id"`
		Message   string `json:"message"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Message) == "" {
		utils.BadRequest(c, "说点什么吧")
		return
	}

	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache, no-transform")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no") // 让 Nginx 不缓冲 SSE
	c.Status(http.StatusOK)

	var mu sync.Mutex
	emit := func(event string, data any) {
		mu.Lock()
		defer mu.Unlock()
		b, err := json.Marshal(data)
		if err != nil {
			return
		}
		fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event, b)
		c.Writer.Flush()
	}

	if err := assistant.Run(c.Request.Context(), principal(c), req.SessionID, req.Message, emit); err != nil {
		if c.Request.Context().Err() == nil {
			emit("error", gin.H{"message": err.Error()})
			emit("done", gin.H{})
		}
	}
}

func ListAssistantSessions(c *gin.Context) {
	var rows []models.ChatSession
	database.DB.Scopes(database.OwnedBy(uid(c))).Order("updated_at DESC").Limit(50).Find(&rows)
	utils.Success(c, rows)
}

type chatMessageView struct {
	models.ChatMessage
	CardsJSON json.RawMessage `json:"cards"`
}

func GetAssistantMessages(c *gin.Context) {
	var s models.ChatSession
	if err := database.DB.Scopes(database.OwnedBy(uid(c))).First(&s, c.Param("id")).Error; err != nil {
		utils.NotFound(c, "会话不存在")
		return
	}
	var rows []models.ChatMessage
	database.DB.Scopes(database.OwnedBy(uid(c))).Where("session_id = ?", s.ID).Order("id ASC").Limit(200).Find(&rows)
	out := make([]chatMessageView, 0, len(rows))
	for _, r := range rows {
		cards := json.RawMessage(r.Cards)
		if !json.Valid(cards) {
			cards = json.RawMessage("[]")
		}
		out = append(out, chatMessageView{ChatMessage: r, CardsJSON: cards})
	}
	utils.Success(c, gin.H{"session": s, "messages": out})
}

func DeleteAssistantSession(c *gin.Context) {
	var s models.ChatSession
	if err := database.DB.Scopes(database.OwnedBy(uid(c))).First(&s, c.Param("id")).Error; err != nil {
		utils.NotFound(c, "会话不存在")
		return
	}
	database.DB.Where("session_id = ? AND user_id = ?", s.ID, uid(c)).Delete(&models.ChatMessage{})
	database.DB.Delete(&s)
	utils.SuccessMsg(c, "已删除")
}
