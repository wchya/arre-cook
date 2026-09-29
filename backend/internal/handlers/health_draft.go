package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"ninimenu/internal/assistant"
	"ninimenu/internal/database"
	"ninimenu/internal/llm"
	"ninimenu/internal/services"
	"ninimenu/internal/utils"
)

func GetHealthDraftStatus(c *gin.Context) {
	db := database.DB.WithContext(c.Request.Context())
	quota, err := services.GetHealthDraftQuota(db, uid(c), time.Now())
	if err != nil {
		utils.InternalError(c, "暂时无法读取文字记餐额度")
		return
	}
	utils.Success(c, gin.H{"enabled": llm.Resolve(db).Enabled(), "quota": quota, "max_chars": assistant.HealthDraftMaxChars, "max_items": assistant.HealthDraftMaxItems})
}

func decodeHealthDraftBody(c *gin.Context, dest any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if decoder.Decode(dest) != nil || decoder.Decode(new(any)) != io.EOF {
		utils.BadRequest(c, "记餐请求格式不正确或内容过长")
		return false
	}
	return true
}

func ParseHealthMealDraft(c *gin.Context) {
	var in struct {
		Text string `json:"text"`
	}
	if !decodeHealthDraftBody(c, &in) {
		return
	}
	if err := assistant.CheckHealthDraftText(in.Text); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 40*time.Second)
	defer cancel()
	db := database.DB.WithContext(ctx)
	settings := llm.Resolve(db)
	if !settings.Enabled() {
		utils.Error(c, 503, 50300, "文字整理尚未启用，可以继续手动记餐")
		return
	}
	release, err := services.AcquireAssistantLease(db, uid(c), time.Now())
	if err != nil {
		if errors.Is(err, services.ErrAssistantBusy) || errors.Is(err, services.ErrAssistantServiceBusy) {
			c.Header("Retry-After", "5")
			utils.Error(c, 429, 42902, "当前有 AI 请求正在处理，请稍后重试")
		} else {
			utils.InternalError(c, "文字整理暂时不可用")
		}
		return
	}
	defer release()
	err = services.ConsumeHealthDraftQuota(db, uid(c), time.Now())
	if err != nil {
		if errors.Is(err, services.ErrHealthDraftQuota) || errors.Is(err, services.ErrAssistantSiteQuotaExceeded) {
			utils.Error(c, 429, 42901, "今日文字记餐或全站 AI 额度已用完，可以继续手动记餐")
		} else {
			utils.InternalError(c, "无法确认文字记餐额度，请稍后重试")
		}
		return
	}
	draft, err := assistant.ParseHealthMealDraft(ctx, settings, in.Text)
	if errors.Is(c.Request.Context().Err(), context.Canceled) {
		return
	}
	if ctx.Err() != nil {
		utils.Error(c, 504, 50400, "文字整理超时，请缩短描述或手动记餐")
		return
	}
	if err != nil {
		utils.Error(c, 502, 50200, err.Error())
		return
	}
	c.Header("Cache-Control", "no-store")
	utils.Success(c, draft)
}

func SaveHealthMealDraft(c *gin.Context) {
	var in services.HealthJournalBatchInput
	if !decodeHealthDraftBody(c, &in) {
		return
	}
	ids, err := services.SaveHealthJournalBatch(uid(c), in, database.DB.WithContext(c.Request.Context()))
	if errors.Is(err, services.ErrHealthBatchConflict) {
		utils.Error(c, 409, 40900, err.Error())
		return
	}
	if errors.Is(err, services.ErrHealthDraftInput) {
		utils.BadRequest(c, "未保存，请核对食物名称、个人份量、日期及餐次后重试")
		return
	}
	if err != nil {
		utils.InternalError(c, "保存暂时失败，请重试同一份内容")
		return
	}
	utils.Success(c, gin.H{"entry_ids": ids})
}
