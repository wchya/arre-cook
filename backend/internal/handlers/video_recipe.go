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
	"ninimenu/internal/services"
	"ninimenu/internal/utils"
	"ninimenu/internal/video"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

func GetVideoRecipeStatus(c *gin.Context) {
	quota, err := services.GetAssistantQuota(database.DB.WithContext(c.Request.Context()), uid(c), time.Now())
	if err != nil {
		utils.InternalError(c, "暂时无法读取提炼次数，请稍后重试")
		return
	}
	utils.Success(c, gin.H{"enabled": llm.Resolve().Enabled(), "asr_enabled": services.VideoASRSettings().Enabled(), "max_duration_seconds": video.MaxDuration, "max_transcript_chars": video.MaxTranscriptRunes, "quota": quota})
}

func ExtractVideoRecipe(c *gin.Context) {
	var req struct {
		URL        string `json:"url"`
		Transcript string `json:"transcript"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 40<<10))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&req) != nil || decoder.Decode(new(any)) != io.EOF {
		utils.BadRequest(c, "请只提交视频链接和可选的字幕正文")
		return
	}
	input, err := video.Parse(req.URL)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	if strings.TrimSpace(req.Transcript) != "" {
		if _, err := video.ValidateTranscript(req.Transcript); err != nil {
			utils.BadRequest(c, err.Error())
			return
		}
		if err := assistant.CheckInput(req.Transcript); err != nil {
			utils.BadRequest(c, err.Error())
			return
		}
	}
	settings := llm.Resolve()
	if !settings.Enabled() {
		utils.Error(c, http.StatusServiceUnavailable, 50300, "视频提炼尚未启用，请先在管理后台配置 AI 模型")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), services.AssistantRequestTimeout)
	defer cancel()
	db := database.DB.WithContext(ctx)
	quota, err := services.GetAssistantQuota(db, uid(c), time.Now())
	if err != nil {
		utils.InternalError(c, "暂时无法读取提炼次数，请稍后重试")
		return
	}
	if quota.Remaining == 0 || quota.BlockedReason != "" {
		c.Header("Retry-After", strconv.Itoa(max(1, int(time.Until(quota.ResetAt).Seconds()))))
		c.JSON(http.StatusTooManyRequests, utils.Response{Code: 42901, Message: quota.ExhaustedMessage(), Data: gin.H{"quota": quota}})
		return
	}
	release, err := services.AcquireAssistantLease(db, uid(c), time.Now())
	if err != nil {
		if errors.Is(err, services.ErrAssistantBusy) || errors.Is(err, services.ErrAssistantServiceBusy) {
			c.Header("Retry-After", "5")
			utils.Error(c, http.StatusTooManyRequests, 42902, err.Error())
		} else {
			utils.InternalError(c, "提炼服务暂时不可用，请稍后重试")
		}
		return
	}
	defer release()
	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-store, no-transform")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	var mu sync.Mutex
	emit := func(event string, data any) {
		mu.Lock()
		defer mu.Unlock()
		if ctx.Err() != nil && event != "error" && event != "done" {
			return
		}
		body, err := json.Marshal(data)
		if err != nil {
			return
		}
		if _, err := fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event, body); err != nil {
			cancel()
			return
		}
		c.Writer.Flush()
	}
	stop, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				emit("ping", gin.H{})
			}
		}
	}()
	defer func() { close(stop); <-stopped }()
	emit("quota", quota)
	status := func(message string) { emit("status", gin.H{"message": message}) }
	consumed := false
	beforeAI := func() error {
		if consumed {
			return nil
		}
		var err error
		quota, err = services.ConsumeAssistantQuota(db, uid(c), time.Now())
		if errors.Is(err, services.ErrAssistantQuotaExceeded) || errors.Is(err, services.ErrAssistantSiteQuotaExceeded) {
			emit("quota", quota)
			return &video.Error{Code: "quota_exceeded", Message: quota.ExhaustedMessage()}
		}
		if err != nil {
			return &video.Error{Code: "unavailable", Message: "暂时无法确认提炼次数，请稍后重试"}
		}
		consumed = true
		emit("quota", quota)
		return nil
	}
	source, err := services.ReadVideoTranscript(ctx, input.URL, req.Transcript, status, beforeAI)
	var result assistant.VideoRecipeResult
	if err == nil {
		err = assistant.CheckInput(source.Text)
	}
	if err == nil {
		err = beforeAI()
	}
	if err == nil {
		result, err = assistant.ExtractVideoRecipe(ctx, settings, source, status)
	}
	if c.Request.Context().Err() != nil {
		return
	}
	if ctx.Err() != nil {
		err = &video.Error{Code: "timeout", Message: "这次提炼用时较长，已停止处理；可缩短字幕后重试"}
	}
	if err != nil {
		public := video.PublicError(err)
		// Only our extraction/guardrail messages are exposed; transport errors are masked.
		if source.Text != "" && ctx.Err() == nil {
			var known *video.Error
			if !errors.As(err, &known) {
				public = &video.Error{Code: "extraction_failed", Message: err.Error()}
			}
		}
		emit("error", public)
	} else {
		emit("recipe", result)
	}
	emit("done", gin.H{})
}
