package services

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
)

var ErrInvalidMealPlan = errors.New("菜单计划无效")

// MealPlanInput is a choice for a future meal, never proof that a meal was eaten.
// Source and Actor are assigned by the authenticated transport, not its caller.
type MealPlanInput struct {
	DishID           uint   `json:"dish_id"`
	MealDate         string `json:"meal_date"`
	MealType         string `json:"meal_type"`
	RecommendationID string `json:"recommendation_id"`
	Source           string `json:"-"`
	Actor            string `json:"-"`
}

type MealPlanReplaceInput struct {
	MealDate          string          `json:"meal_date"`
	MealType          string          `json:"meal_type"`
	DishIDs           []uint          `json:"dish_ids"`
	RecommendationIDs map[uint]string `json:"recommendation_ids"`
	Source            string          `json:"-"`
	Actor             string          `json:"-"`
}

func NormalizeMealPlanDate(raw string) (string, error) {
	switch strings.TrimSpace(raw) {
	case "", "today", "今天":
		return healthToday(), nil
	case "tomorrow", "明天":
		return healthNow().AddDate(0, 0, 1).Format("2006-01-02"), nil
	default:
		date := strings.TrimSpace(raw)
		if parsed, err := time.Parse("2006-01-02", date); err != nil || parsed.Format("2006-01-02") != date {
			return "", ErrInvalidDate
		}
		return date, nil
	}
}

func validateMealPlanSlot(date, meal string) error {
	parsed, err := time.Parse("2006-01-02", date)
	if err != nil || parsed.Format("2006-01-02") != date || date < healthToday() || date > healthNow().AddDate(0, 0, 6).Format("2006-01-02") {
		return errors.Join(ErrInvalidMealPlan, errors.New("请选择今天起七天内的日期"))
	}
	if !isMainMeal(meal) {
		return ErrInvalidMealType
	}
	return nil
}

func ListMealPlans(uid uint, from, to string, dbs ...*gorm.DB) ([]HealthPlanView, error) {
	if from == "" {
		from = healthToday()
	}
	if to == "" {
		to = healthNow().AddDate(0, 0, 6).Format("2006-01-02")
	}
	start, err := time.Parse("2006-01-02", from)
	if err != nil {
		return nil, ErrInvalidMealPlan
	}
	end, err := time.Parse("2006-01-02", to)
	if err != nil || end.Before(start) || end.Sub(start) > 366*24*time.Hour {
		return nil, ErrInvalidMealPlan
	}
	return ListHealthPlans(uid, from, to, database.Handle(dbs...))
}

func AcceptMealPlan(uid uint, in MealPlanInput, dbs ...*gorm.DB) (*models.HealthPlanItem, error) {
	return acceptMealPlan(uid, in, 0, database.Handle(dbs...))
}

func acceptMealPlan(uid uint, in MealPlanInput, reportDays int, db *gorm.DB) (*models.HealthPlanItem, error) {
	date, err := NormalizeMealPlanDate(in.MealDate)
	if err != nil {
		return nil, err
	}
	in.MealDate = date
	if err := validateMealPlanSlot(in.MealDate, in.MealType); err != nil {
		return nil, err
	}
	var result models.HealthPlanItem
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := lockHealthProfileUser(uid, tx); err != nil {
			return err
		}
		blocked, err := loadDietaryExclusions(uid, tx)
		if err != nil {
			return err
		}
		result, err = acceptMealPlanTx(tx, uid, in, reportDays, blocked)
		return err
	})
	return &result, err
}

func acceptMealPlanTx(tx *gorm.DB, uid uint, in MealPlanInput, reportDays int, blocked []string) (models.HealthPlanItem, error) {
	var result models.HealthPlanItem
	var dish models.Dish
	sessionID := ""
	servedForDate, recommendedMealType := "", ""
	if err := tx.Scopes(database.VisibleDishes(uid)).Where("enabled = ?", true).First(&dish, in.DishID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return result, ErrDishNotFound
		}
		return result, err
	}
	if DishViolatesDietaryExclusions(dish, blocked) {
		return result, errors.Join(ErrInvalidMealPlan, errors.New("这道菜与当前忌口冲突，请换一道"))
	}
	if in.RecommendationID != "" {
		if err := ValidateRecommendationItem(uid, in.RecommendationID, dish.ID, tx); err != nil {
			return result, err
		}
		var snapshot models.RecommendationSnapshot
		if err := tx.Where("user_id = ? AND id = ?", uid, in.RecommendationID).First(&snapshot).Error; err != nil {
			return result, err
		}
		sessionID = snapshot.SessionID
		servedForDate, recommendedMealType = snapshot.MealDate, snapshot.MealType
	}
	q := tx.Where("user_id = ? AND meal_date = ? AND meal_type = ? AND dish_id = ?", uid, in.MealDate, in.MealType, dish.ID)
	if err := q.First(&result).Error; err == nil {
		return result, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return result, err
	}
	var count int64
	if err := tx.Model(&models.HealthPlanItem{}).Where("user_id = ? AND meal_date = ? AND meal_type = ?", uid, in.MealDate, in.MealType).Count(&count).Error; err != nil {
		return result, err
	}
	if count >= int64(MaxDishesPerMeal) {
		return result, errors.Join(ErrInvalidMealPlan, errors.New("这餐已安排足够菜品，请先撤销再添加"))
	}
	source := strings.TrimSpace(in.Source)
	if source == "" {
		source = "app"
	}
	result = models.HealthPlanItem{UserID: uid, MealDate: in.MealDate, MealType: in.MealType, DishID: dish.ID, DishName: dish.Name, Source: source, RecommendationID: in.RecommendationID}
	if reportDays > 0 {
		result.ReportFrom = healthNow().AddDate(0, 0, 1-reportDays).Format("2006-01-02")
		result.ReportTo = healthToday()
	}
	if err := tx.Create(&result).Error; err != nil {
		return result, err
	}
	var existing int64
	if err := tx.Model(&models.ShoppingCheck{}).Where("user_id = ? AND meal_date = ? AND meal_type = ? AND dish_id = ?", uid, in.MealDate, in.MealType, dish.ID).Count(&existing).Error; err != nil {
		return result, err
	}
	if existing == 0 {
		if _, err := addShoppingItems(tx, uid, dish, in.MealType, in.MealDate); err != nil {
			return result, err
		}
	}
	meta, _ := json.Marshal(map[string]any{"plan_id": result.ID, "meal_date": result.MealDate, "meal_type": result.MealType, "recommendation_id": in.RecommendationID, "session_id": sessionID, "served_for_date": servedForDate, "recommended_meal_type": recommendedMealType})
	events := []models.BehaviorEvent{{UserID: uid, EventType: "plan", DishID: dish.ID, DishName: dish.Name, Source: source, Actor: in.Actor, Meta: string(meta)}}
	if in.RecommendationID != "" {
		events = append(events, models.BehaviorEvent{UserID: uid, EventType: "accept", DishID: dish.ID, DishName: dish.Name, Source: source, Actor: in.Actor, Meta: string(meta)})
	}
	if err := tx.Create(&events).Error; err != nil {
		return result, err
	}
	if err := tx.Where("user_id = ? AND `key` = ?", uid, "week_plan_cache").Delete(&models.UserSetting{}).Error; err != nil {
		return result, err
	}
	return result, nil
}

// ReplaceMealPlan commits the whole slot and its shopping rows together. Existing
// choices retain their identities, so retries do not create new feedback.
func ReplaceMealPlan(uid uint, in MealPlanReplaceInput, dbs ...*gorm.DB) ([]models.HealthPlanItem, error) {
	date, err := NormalizeMealPlanDate(in.MealDate)
	if err != nil {
		return nil, err
	}
	in.MealDate = date
	if err := validateMealPlanSlot(in.MealDate, in.MealType); err != nil {
		return nil, err
	}
	if len(in.DishIDs) > MaxDishesPerMeal {
		return nil, errors.Join(ErrInvalidMealPlan, errors.New("每餐最多安排10道菜"))
	}
	seen := make(map[uint]bool, len(in.DishIDs))
	for _, id := range in.DishIDs {
		if id == 0 || seen[id] {
			return nil, errors.Join(ErrInvalidMealPlan, errors.New("菜品不能为空或重复"))
		}
		seen[id] = true
	}
	for id := range in.RecommendationIDs {
		if !seen[id] {
			return nil, errors.Join(ErrInvalidMealPlan, errors.New("推荐关联必须属于本餐选择的菜品"))
		}
	}
	result := []models.HealthPlanItem{}
	err = database.Handle(dbs...).Transaction(func(tx *gorm.DB) error {
		if err := lockHealthProfileUser(uid, tx); err != nil {
			return err
		}
		blocked, err := loadDietaryExclusions(uid, tx)
		if err != nil {
			return err
		}
		var previous []models.HealthPlanItem
		if err := tx.Where("user_id = ? AND meal_date = ? AND meal_type = ?", uid, in.MealDate, in.MealType).Find(&previous).Error; err != nil {
			return err
		}
		for _, p := range previous {
			if !seen[p.DishID] {
				if err := cancelMealPlanTx(tx, uid, p); err != nil {
					return err
				}
			}
		}
		for _, id := range in.DishIDs {
			p, err := acceptMealPlanTx(tx, uid, MealPlanInput{DishID: id, MealDate: in.MealDate, MealType: in.MealType, RecommendationID: in.RecommendationIDs[id], Source: in.Source, Actor: in.Actor}, 0, blocked)
			if err != nil {
				return err
			}
			result = append(result, p)
		}
		return nil
	})
	return result, err
}

func CancelMealPlan(uid, id uint, dbs ...*gorm.DB) error {
	return database.Handle(dbs...).Transaction(func(tx *gorm.DB) error {
		if err := lockHealthProfileUser(uid, tx); err != nil {
			return err
		}
		var p models.HealthPlanItem
		if err := tx.Scopes(database.OwnedBy(uid)).First(&p, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrRecordNotFound
			}
			return err
		}
		return cancelMealPlanTx(tx, uid, p)
	})
}

func cancelMealPlanTx(tx *gorm.DB, uid uint, p models.HealthPlanItem) error {
	if err := tx.Delete(&p).Error; err != nil {
		return err
	}
	if err := tx.Where("user_id = ? AND `key` = ?", uid, "week_plan_cache").Delete(&models.UserSetting{}).Error; err != nil {
		return err
	}
	var records int64
	if err := tx.Model(&models.MealRecord{}).Where("user_id = ? AND meal_date = ? AND meal_type = ? AND dish_id = ?", uid, p.MealDate, p.MealType, p.DishID).Count(&records).Error; err != nil {
		return err
	}
	if records == 0 {
		return tx.Where("user_id = ? AND meal_date = ? AND meal_type = ? AND dish_id = ?", uid, p.MealDate, p.MealType, p.DishID).Delete(&models.ShoppingCheck{}).Error
	}
	return nil
}

func visibleMealPlanSlots(uid uint, from, to string, db *gorm.DB) (map[string][]models.Dish, error) {
	var plans []models.HealthPlanItem
	if err := db.Where("user_id = ? AND meal_date >= ? AND meal_date <= ?", uid, from, to).Order("id").Find(&plans).Error; err != nil {
		return nil, err
	}
	blocked, err := loadDietaryExclusions(uid, db)
	if err != nil {
		return nil, err
	}
	ids := make([]uint, 0, len(plans))
	for _, p := range plans {
		ids = append(ids, p.DishID)
	}
	dishes := map[uint]models.Dish{}
	for _, dish := range menuCandidates(uid, ids, db) {
		if !DishViolatesDietaryExclusions(dish, blocked) {
			dishes[dish.ID] = dish
		}
	}
	slots := map[string][]models.Dish{}
	for _, p := range plans {
		if dish, ok := dishes[p.DishID]; ok {
			key := p.MealDate + ":" + p.MealType
			slots[key] = append(slots[key], dish)
		}
	}
	return slots, nil
}
