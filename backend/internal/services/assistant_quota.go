package services

import (
	"errors"
	"fmt"
	"ninimenu/internal/models"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const AssistantDailyLimitKey = "assistant_daily_limit"
const MaxAssistantDailyLimit = 20
const AssistantSiteLimitKey = "assistant_site_daily_limit"
const DefaultAssistantSiteLimit = 200

var ErrAssistantQuotaExceeded = errors.New("assistant daily quota exceeded")
var ErrAssistantSiteQuotaExceeded = errors.New("assistant site daily quota exceeded")
var assistantLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

type AssistantQuota struct {
	Limit         int       `json:"limit"`
	Used          int       `json:"used"`
	Remaining     int       `json:"remaining"`
	ResetAt       time.Time `json:"reset_at"`
	BlockedReason string    `json:"blocked_reason,omitempty"`
}

func ParseAssistantDailyLimit(value string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || n < 0 || n > MaxAssistantDailyLimit {
		return 0, errors.New("AI 助手每日上限必须为 0–20 之间的整数")
	}
	return n, nil
}

func ParseAssistantSiteLimit(value string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || n < 0 || n > 10000 {
		return 0, errors.New("全站每日上限必须为 0–10000 之间的整数")
	}
	return n, nil
}

func assistantSiteLimit(db *gorm.DB) (int, error) {
	var setting models.Setting
	err := db.Where("`key` = ?", AssistantSiteLimitKey).First(&setting).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return DefaultAssistantSiteLimit, nil
	}
	if err != nil {
		return 0, err
	}
	return ParseAssistantSiteLimit(setting.Value)
}

// A missing setting uses the default; storage failures must never open unlimited access.
func AssistantDailyLimit(db *gorm.DB) (int, error) {
	var setting models.Setting
	err := db.Where("`key` = ?", AssistantDailyLimitKey).First(&setting).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return MaxAssistantDailyLimit, nil
	}
	if err != nil {
		return 0, err
	}
	return ParseAssistantDailyLimit(setting.Value)
}

func assistantQuotaDay(now time.Time) (string, time.Time) {
	local := now.In(assistantLocation)
	year, month, day := local.Date()
	return local.Format("2006-01-02"), time.Date(year, month, day+1, 0, 0, 0, 0, assistantLocation)
}

func quotaForDay(db *gorm.DB, userID uint, date string, limit int, resetAt time.Time) (AssistantQuota, error) {
	quota := AssistantQuota{Limit: limit, ResetAt: resetAt}
	var usage models.AssistantUsage
	err := db.Where("user_id = ? AND usage_date = ?", userID, date).First(&usage).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return quota, err
	}
	quota.Used = usage.Used
	quota.Remaining = max(0, limit-usage.Used)
	return quota, nil
}

func GetAssistantQuota(db *gorm.DB, userID uint, now time.Time) (AssistantQuota, error) {
	limit, err := AssistantDailyLimit(db)
	if err != nil {
		return AssistantQuota{}, err
	}
	siteLimit, err := assistantSiteLimit(db)
	if err != nil {
		return AssistantQuota{}, err
	}
	date, resetAt := assistantQuotaDay(now)
	quota, err := quotaForDay(db, userID, date, limit, resetAt)
	if err != nil {
		return quota, err
	}
	site, err := quotaForDay(db, 0, date, siteLimit, resetAt)
	if err != nil {
		return quota, err
	}
	if site.Remaining == 0 {
		quota.BlockedReason = "site_limit"
	}
	return quota, nil
}

// The user and site reservations commit together, so an exhausted account cannot
// burn the shared budget. SQL conditions enforce both limits across containers.
func ConsumeAssistantQuota(db *gorm.DB, userID uint, now time.Time) (AssistantQuota, error) {
	limit, err := AssistantDailyLimit(db)
	if err != nil {
		return AssistantQuota{}, err
	}
	siteLimit, err := assistantSiteLimit(db)
	if err != nil {
		return AssistantQuota{}, err
	}
	date, resetAt := assistantQuotaDay(now)
	if userID == 0 {
		return AssistantQuota{}, errors.New("missing assistant account")
	}
	reserveErr := db.Transaction(func(tx *gorm.DB) error {
		if limit == 0 {
			return ErrAssistantQuotaExceeded
		}
		if siteLimit == 0 {
			return ErrAssistantSiteQuotaExceeded
		}
		// Always lock the shared row first to keep a consistent lock order.
		for _, entry := range []struct {
			id        uint
			limit     int
			exhausted error
		}{{0, siteLimit, ErrAssistantSiteQuotaExceeded}, {userID, limit, ErrAssistantQuotaExceeded}} {
			usage := models.AssistantUsage{UserID: entry.id, UsageDate: date}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&usage).Error; err != nil {
				return err
			}
			result := tx.Model(&models.AssistantUsage{}).
				Where("user_id = ? AND usage_date = ? AND used < ?", entry.id, date, entry.limit).
				Updates(map[string]any{"used": gorm.Expr("used + 1"), "updated_at": now})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return entry.exhausted
			}
		}
		return nil
	})
	if reserveErr != nil && !errors.Is(reserveErr, ErrAssistantQuotaExceeded) && !errors.Is(reserveErr, ErrAssistantSiteQuotaExceeded) {
		return AssistantQuota{}, reserveErr
	}
	quota, err := quotaForDay(db, userID, date, limit, resetAt)
	if err != nil {
		return quota, err
	}
	if errors.Is(reserveErr, ErrAssistantSiteQuotaExceeded) {
		quota.BlockedReason = "site_limit"
	}
	return quota, reserveErr
}

func (q AssistantQuota) ExhaustedMessage() string {
	if q.BlockedReason == "site_limit" {
		return "今天助手服务的总次数已用完或已暂停，你仍可浏览菜谱和使用选菜工具"
	}
	if q.Limit == 0 {
		return "管理员已暂停 AI 助手请求，你仍可使用菜谱和选菜工具"
	}
	return fmt.Sprintf("今天的 AI 助手次数已用完（%d 次），北京时间明日 00:00 恢复", q.Limit)
}
