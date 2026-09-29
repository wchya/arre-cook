package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
)

const HealthDraftDailyLimit = 10

var ErrHealthDraftQuota = errors.New("今天的文字记餐次数已用完，仍可手动记餐")
var ErrHealthDraftInput = errors.New("请核对日期、餐次及 1–12 项实际吃过的食物后确认")
var ErrHealthBatchConflict = errors.New("这次确认已保存过其他内容，请重新打开记餐草稿")

type HealthDraftQuota struct {
	Limit     int       `json:"limit"`
	Used      int       `json:"used"`
	Remaining int       `json:"remaining"`
	ResetAt   time.Time `json:"reset_at"`
}

func GetHealthDraftQuota(db *gorm.DB, uid uint, now time.Time) (HealthDraftQuota, error) {
	date, reset := assistantQuotaDay(now)
	var row models.HealthDraftUsage
	err := db.Where("user_id = ? AND usage_date = ?", uid, date).First(&row).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return HealthDraftQuota{}, err
	}
	return HealthDraftQuota{Limit: HealthDraftDailyLimit, Used: row.Used, Remaining: max(0, HealthDraftDailyLimit-row.Used), ResetAt: reset}, nil
}

// Shares the site's cap and lease pool, but does not spend personal chat quota.
func ConsumeHealthDraftQuota(db *gorm.DB, uid uint, now time.Time) error {
	siteLimit, err := assistantSiteLimit(db)
	if err != nil {
		return err
	}
	date, _ := assistantQuotaDay(now)
	return db.Transaction(func(tx *gorm.DB) error {
		if uid == 0 {
			return errors.New("missing account")
		}
		site := models.AssistantUsage{UserID: 0, UsageDate: date}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&site).Error; err != nil {
			return err
		}
		result := tx.Model(&models.AssistantUsage{}).Where("user_id = 0 AND usage_date = ? AND used < ?", date, siteLimit).Updates(map[string]any{"used": gorm.Expr("used + 1"), "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrAssistantSiteQuotaExceeded
		}
		// Same account lock used by deletion, so a deleted account cannot regain usage.
		if err := lockHealthProfileUser(uid, tx); err != nil {
			return err
		}
		row := models.HealthDraftUsage{UserID: uid, UsageDate: date}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
		result = tx.Model(&models.HealthDraftUsage{}).Where("user_id = ? AND usage_date = ? AND used < ?", uid, date, HealthDraftDailyLimit).UpdateColumn("used", gorm.Expr("used + 1"))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrHealthDraftQuota
		}
		return nil
	})
}

type HealthDraftItemInput struct {
	DishName              string   `json:"dish_name"`
	Portion               string   `json:"portion"`
	NutritionMode         string   `json:"nutrition_mode,omitempty"`
	NutritionFoodID       uint     `json:"nutrition_food_id,omitempty"`
	NutritionAmount       *float64 `json:"nutrition_amount,omitempty"`
	NutritionUnit         string   `json:"nutrition_unit,omitempty"`
	FoodState             string   `json:"food_state,omitempty"`
	PortionSource         string   `json:"portion_source,omitempty"`
	NutritionPortionKey   string   `json:"nutrition_portion_key,omitempty"`
	NutritionPortionCount *float64 `json:"nutrition_portion_count,omitempty"`
}
type HealthJournalBatchInput struct {
	RequestKey string                 `json:"request_key"`
	Confirmed  bool                   `json:"confirmed"`
	MealDate   string                 `json:"meal_date"`
	MealType   string                 `json:"meal_type"`
	Items      []HealthDraftItemInput `json:"items"`
}

func SaveHealthJournalBatch(uid uint, in HealthJournalBatchInput, dbs ...*gorm.DB) ([]uint, error) {
	if strings.TrimSpace(in.MealDate) == "" || !in.Confirmed || len(in.Items) < 1 || len(in.Items) > 12 || len(in.RequestKey) < 16 || !validHealthKey(in.RequestKey) {
		return nil, ErrHealthDraftInput
	}
	in.MealDate = strings.TrimSpace(in.MealDate)
	for i := range in.Items {
		in.Items[i].DishName = strings.TrimSpace(in.Items[i].DishName)
		in.Items[i].Portion = strings.TrimSpace(in.Items[i].Portion)
		item := &in.Items[i]
		if item.NutritionFoodID == 0 {
			if item.NutritionMode != "" || item.NutritionAmount != nil || item.NutritionUnit != "" || item.FoodState != "" || item.PortionSource != "" || item.NutritionPortionKey != "" || item.NutritionPortionCount != nil {
				return nil, ErrHealthDraftInput
			}
		} else if item.NutritionMode != "replace" || item.NutritionUnit == "" || item.FoodState == "" || (item.NutritionAmount == nil && (item.NutritionPortionKey == "" || item.NutritionPortionCount == nil)) {
			return nil, ErrHealthDraftInput
		}
	}
	raw, _ := json.Marshal(in)
	sum := sha256.Sum256(raw)
	hash := hex.EncodeToString(sum[:])
	var ids []uint
	err := database.Handle(dbs...).Transaction(func(tx *gorm.DB) error {
		if err := lockHealthProfileUser(uid, tx); err != nil {
			return err
		}
		var batch models.HealthJournalBatch
		err := tx.Where("user_id = ? AND request_key = ?", uid, in.RequestKey).First(&batch).Error
		if err == nil {
			if batch.RequestHash != hash {
				return ErrHealthBatchConflict
			}
			ids = batch.EntryIDs
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		entries := make([]*models.FoodJournalEntry, 0, len(in.Items))
		for _, item := range in.Items {
			entry, err := prepareFoodJournal(uid, FoodJournalInput{MealDate: in.MealDate, MealType: in.MealType, DishName: item.DishName, Portion: item.Portion, EventKey: in.RequestKey, NutritionMode: item.NutritionMode, NutritionFoodID: item.NutritionFoodID, NutritionAmount: item.NutritionAmount, NutritionUnit: item.NutritionUnit, FoodState: item.FoodState, PortionSource: item.PortionSource, NutritionPortionKey: item.NutritionPortionKey, NutritionPortionCount: item.NutritionPortionCount}, tx)
			if err != nil {
				return errors.Join(ErrHealthDraftInput, err)
			}
			entries = append(entries, entry)
		}
		if err := clearMealOmission(tx, uid, entries[0].MealDate, in.MealType); err != nil {
			return err
		}
		for _, entry := range entries {
			if err := tx.Create(entry).Error; err != nil {
				return err
			}
			ids = append(ids, entry.ID)
		}
		batch = models.HealthJournalBatch{UserID: uid, RequestKey: in.RequestKey, RequestHash: hash, EntryIDs: ids}
		return tx.Create(&batch).Error
	})
	return ids, err
}
