package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"ninimenu/internal/assistant"
	"ninimenu/internal/database"
	"ninimenu/internal/llm"
	"ninimenu/internal/models"
	"ninimenu/internal/services"
	"ninimenu/internal/utils"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GetAssistantStatus 站内 AI 助手是否可用（未配置大模型时退化为本地推荐引擎）。
func GetAssistantStatus(c *gin.Context) {
	quota, err := services.GetAssistantQuota(database.DB.WithContext(c.Request.Context()), uid(c), time.Now())
	if err != nil {
		utils.InternalError(c, "暂时无法读取助手次数，请稍后重试")
		return
	}
	s := llm.Resolve(database.DB.WithContext(c.Request.Context()))
	suggestions := []string{"今晚吃什么？想吃辣的，半小时内", "我最近的饮食报告", "冰箱里有鸡蛋和番茄，能做什么"}
	if s.Enabled() {
		suggestions = append(suggestions, "帮我记下今天午餐吃了番茄面", "帮我保存一道私房菜")
	}
	utils.Success(c, gin.H{
		"llm_enabled": s.Enabled(),
		"model":       map[bool]string{true: s.Model, false: ""}[s.Enabled()],
		"suggestions": suggestions,
		"quota":       quota,
	})
}

// AssistantChat POST /api/assistant/chat：先返回 quota/status/工具进度，审核后返回正文和兼容旧客户端的卡片，最后 done。
func AssistantChat(c *gin.Context) {
	var req struct {
		SessionID uint   `json:"session_id"`
		Message   string `json:"message"`
	}
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		utils.BadRequest(c, "请只提交消息和会话编号")
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF || strings.TrimSpace(req.Message) == "" {
		utils.BadRequest(c, "说点什么吧")
		return
	}
	if utf8.RuneCountInString(strings.TrimSpace(req.Message)) > 1000 {
		utils.BadRequest(c, "每条消息最多 1000 个字，请精简后再发送")
		return
	}
	if err := assistant.CheckInput(req.Message); err != nil {
		services.WriteAudit(services.AuditEntry{UserID: uid(c), Actor: "assistant", Channel: "chat", Tool: "content_input", Status: "denied", Error: "content policy"}, database.DB.WithContext(c.Request.Context()))
		utils.BadRequest(c, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), services.AssistantRequestTimeout)
	defer cancel()
	db := database.DB.WithContext(c.Request.Context()).WithContext(ctx)
	if req.SessionID > 0 {
		var session models.ChatSession
		err := db.Where("id = ? AND user_id = ?", req.SessionID, uid(c)).First(&session).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.NotFound(c, "这段对话已不存在，请新建对话")
			return
		}
		if err != nil {
			utils.InternalError(c, "暂时无法读取对话，请稍后重试")
			return
		}
	}
	release, err := services.AcquireAssistantLease(db, uid(c), time.Now())
	if err != nil {
		if errors.Is(err, services.ErrAssistantBusy) || errors.Is(err, services.ErrAssistantServiceBusy) {
			c.Header("Retry-After", "5")
			utils.Error(c, http.StatusTooManyRequests, 42902, err.Error())
		} else {
			utils.InternalError(c, "助手暂时不可用，请稍后再试")
		}
		return
	}
	defer release()
	quota, err := services.ConsumeAssistantQuota(db, uid(c), time.Now())
	if errors.Is(err, services.ErrAssistantQuotaExceeded) || errors.Is(err, services.ErrAssistantSiteQuotaExceeded) {
		c.Header("Retry-After", strconv.Itoa(max(1, int(time.Until(quota.ResetAt).Seconds()))))
		c.JSON(http.StatusTooManyRequests, utils.Response{Code: 42901, Message: quota.ExhaustedMessage(), Data: gin.H{"quota": quota}})
		return
	}
	if err != nil {
		utils.InternalError(c, "暂时无法确认助手次数，请稍后重试")
		return
	}

	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-store, no-transform")
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
	emit("quota", quota)

	if err := assistant.Run(ctx, principal(c), req.SessionID, req.Message, emit); err != nil {
		if c.Request.Context().Err() == nil {
			emit("error", gin.H{"message": err.Error()})
			emit("done", gin.H{})
		}
	}
}

func ListAssistantSessions(c *gin.Context) {
	var rows []models.ChatSession
	if err := database.DB.WithContext(c.Request.Context()).Scopes(database.OwnedBy(uid(c))).Order("updated_at DESC").Limit(50).Find(&rows).Error; err != nil {
		utils.InternalError(c, "对话记录加载失败，请重试")
		return
	}
	utils.Success(c, rows)
}

type chatMessageView struct {
	models.ChatMessage
	CardsJSON json.RawMessage `json:"cards"`
}

func GetAssistantMessages(c *gin.Context) {
	var s models.ChatSession
	if err := database.DB.WithContext(c.Request.Context()).Scopes(database.OwnedBy(uid(c))).Where("id = ?", c.Param("id")).First(&s).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.NotFound(c, "会话不存在")
		} else {
			utils.InternalError(c, "对话加载失败，请重试")
		}
		return
	}
	// Show the newest messages (the ones the model also remembers), oldest first.
	var rows []models.ChatMessage
	if err := database.DB.WithContext(c.Request.Context()).Scopes(database.OwnedBy(uid(c))).Where("session_id = ?", s.ID).Order("id DESC").Limit(assistantHistoryLimit + 1).Find(&rows).Error; err != nil {
		utils.InternalError(c, "对话加载失败，请重试")
		return
	}
	hasMore := len(rows) > assistantHistoryLimit
	if hasMore {
		rows = rows[:assistantHistoryLimit]
	}
	slices.Reverse(rows)
	out := make([]chatMessageView, 0, len(rows))
	for _, r := range rows {
		cards := json.RawMessage(r.Cards)
		if !json.Valid(cards) {
			cards = json.RawMessage("[]")
		}
		out = append(out, chatMessageView{ChatMessage: r, CardsJSON: cards})
	}
	utils.Success(c, gin.H{"session": s, "messages": out, "has_more": hasMore})
}

const assistantHistoryLimit = 200

func DeleteAssistantSession(c *gin.Context) {
	var s models.ChatSession
	if err := database.DB.WithContext(c.Request.Context()).Scopes(database.OwnedBy(uid(c))).Where("id = ?", c.Param("id")).First(&s).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.NotFound(c, "会话不存在")
		} else {
			utils.InternalError(c, "对话加载失败，请重试")
		}
		return
	}
	if err := database.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("session_id = ? AND user_id = ?", s.ID, uid(c)).Delete(&models.ChatMessage{}).Error; err != nil {
			return err
		}
		return tx.Delete(&s).Error
	}); err != nil {
		utils.InternalError(c, "删除对话失败，请重试")
		return
	}
	utils.SuccessMsg(c, "已删除")
}
