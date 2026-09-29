package services

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
)

var ErrHealthDayChanged = errors.New("当天记录已变化，请刷新后重新确认")

func isMainMeal(meal string) bool { return meal == "breakfast" || meal == "lunch" || meal == "dinner" }

type HealthMealState struct {
	MealType string `json:"meal_type"`
	Status   string `json:"status"`
}

func loadMealOmissions(uid uint, from, to string, db *gorm.DB) ([]models.HealthMealOmission, error) {
	rows := []models.HealthMealOmission{}
	err := db.Scopes(database.OwnedBy(uid)).Where("meal_date >= ? AND meal_date <= ?", from, to).Order("meal_date, meal_type").Find(&rows).Error
	return rows, err
}

// A later actual record permanently clears the declaration, including legacy/Agent writes.
// Call only inside the same user-locked transaction as the actual record write.
func clearMealOmission(tx *gorm.DB, uid uint, date, meal string) error {
	return tx.Where("user_id = ? AND meal_date = ? AND meal_type = ?", uid, date, meal).Delete(&models.HealthMealOmission{}).Error
}

func SetHealthMealOmission(uid uint, date, meal, fingerprint string, notEaten bool, dbs ...*gorm.DB) error {
	parsed, err := time.Parse("2006-01-02", date)
	if err != nil || date > healthToday() || parsed.Before(healthNow().AddDate(0, 0, -365)) {
		return ErrInvalidDate
	}
	if !isMainMeal(meal) {
		return ErrInvalidMealType
	}
	return database.Handle(dbs...).Transaction(func(tx *gorm.DB) error {
		if err := lockHealthProfileUser(uid, tx); err != nil {
			return err
		}
		js := []models.FoodJournalEntry{}
		rs := []models.MealRecord{}
		if err := tx.Where("user_id = ? AND meal_date = ?", uid, date).Find(&js).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ? AND meal_date = ?", uid, date).Find(&rs).Error; err != nil {
			return err
		}
		omissions, err := loadMealOmissions(uid, date, date, tx)
		if err != nil {
			return err
		}
		if fingerprint != healthFingerprint(js, rs, omissions...) {
			return ErrHealthDayChanged
		}
		if notEaten {
			for _, j := range js {
				if j.MealType == meal {
					return ErrHealthDayChanged
				}
			}
			for _, r := range rs {
				if r.MealType == meal {
					return ErrHealthDayChanged
				}
			}
			row := models.HealthMealOmission{UserID: uid, MealDate: date, MealType: meal, ConfirmedAt: time.Now()}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
				return err
			}
		} else if err := clearMealOmission(tx, uid, date, meal); err != nil {
			return err
		}
		return tx.Where("user_id = ? AND meal_date = ?", uid, date).Delete(&models.HealthDayConfirmation{}).Error
	})
}

type HealthSummary struct {
	From           string `json:"from"`
	To             string `json:"to"`
	LoggedDays     int    `json:"logged_days"`
	CompleteDays   int    `json:"complete_days"`
	MealEventCount int    `json:"meal_event_count"`
	ItemCount      int    `json:"item_count"`
	NotEatenMeals  int    `json:"not_eaten_meals"`
	Headline       string `json:"headline"`
}

// Same report projection, without recommendations, previous-period reads or model calls.
func BuildHealthSummary(uid uint, dbs ...*gorm.DB) (*HealthSummary, error) {
	var out *HealthSummary
	err := database.Handle(dbs...).Transaction(func(tx *gorm.DB) error {
		report, err := buildHealthReportWindow(uid, 7, healthNow(), false, tx)
		if err != nil {
			return err
		}
		headline := "从今天吃过的一餐开始记录"
		if report.ItemCount > 0 {
			headline = "先补齐漏记，再确认当天完整"
		}
		if report.CompleteDays > 0 {
			headline = "查看本周记录与下一餐安排"
		}
		out = &HealthSummary{From: report.From, To: report.To, LoggedDays: report.LoggedDays, CompleteDays: report.CompleteDays, MealEventCount: report.MealEventCount, ItemCount: report.ItemCount, NotEatenMeals: report.NotEatenMeals, Headline: headline}
		return nil
	})
	return out, err
}
