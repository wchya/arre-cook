package services

import (
	"encoding/json"
	"errors"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"strings"
	"time"
)

var (
	ErrDishNotFound    = errors.New("菜品不存在")
	ErrRecordNotFound  = errors.New("记录不存在")
	ErrInvalidMealType = errors.New("餐次只能是 lunch 或 dinner")
	ErrInvalidDate     = errors.New("日期格式应为 YYYY-MM-DD")
	ErrDuplicateMeal   = errors.New("该菜品已在当日该餐次中记录过")
)

// MealInput 记一餐的输入。菜名以数据库为准，不信任调用方传入。
type MealInput struct {
	DishID   uint   `json:"dish_id"`
	MealType string `json:"meal_type"`
	MealDate string `json:"meal_date"`
	Rating   int    `json:"rating"`
	Remark   string `json:"remark"`
	Mood     string `json:"mood"`
	Photo    string `json:"photo"`
}

func Today() string { return time.Now().Format("2006-01-02") }

func normalizeMealDate(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "today" || raw == "今天" {
		return Today(), nil
	}
	if raw == "tomorrow" || raw == "明天" {
		return time.Now().AddDate(0, 0, 1).Format("2006-01-02"), nil
	}
	if _, err := time.Parse("2006-01-02", raw); err != nil {
		return "", ErrInvalidDate
	}
	return raw, nil
}

// CreateMealRecord 记一餐：校验菜品可见性 → 去重 → 写记录 → 生成买菜清单 → 成就检测。
// source 非 app 时（智能体/站内助手代记）同时写一条 accept 行为事件；站内前端自己埋点。
func CreateMealRecord(uid uint, in MealInput, source, actor string) (*models.MealRecord, error) {
	if in.MealType != "lunch" && in.MealType != "dinner" {
		return nil, ErrInvalidMealType
	}
	date, err := normalizeMealDate(in.MealDate)
	if err != nil {
		return nil, err
	}
	dish, err := FindVisibleDish(uid, in.DishID)
	if err != nil || in.DishID == 0 {
		return nil, ErrDishNotFound
	}
	var existing int64
	database.DB.Model(&models.MealRecord{}).Scopes(database.OwnedBy(uid)).
		Where("dish_id = ? AND meal_type = ? AND meal_date = ?", dish.ID, in.MealType, date).
		Count(&existing)
	if existing > 0 {
		return nil, ErrDuplicateMeal
	}
	record := models.MealRecord{
		UserID:   uid,
		DishID:   dish.ID,
		DishName: dish.Name,
		MealType: in.MealType,
		MealDate: date,
		Rating:   clampInt(in.Rating, 0, 5),
		Remark:   truncateRunes(strings.TrimSpace(in.Remark), 500),
		Mood:     strings.TrimSpace(in.Mood),
		Photo:    strings.TrimSpace(in.Photo),
	}
	if err := database.DB.Create(&record).Error; err != nil {
		return nil, err
	}
	entries := addShoppingItems(uid, dish, in.MealType, date)
	notifyMealShopping(uid, dish, in.MealType, date, entries)
	if source != "" && source != "app" {
		LogBehavior(uid, "accept", dish.ID, dish.Name, source, actor, map[string]any{"meal_type": in.MealType, "meal_date": date})
	}
	QueueAutoAchievementSync(uid)
	return &record, nil
}

type MealPatch struct {
	Rating *int    `json:"rating"`
	Remark *string `json:"remark"`
	Mood   *string `json:"mood"`
	Photo  *string `json:"photo"`
}

func UpdateMealRecord(uid uint, id any, patch MealPatch) (*models.MealRecord, error) {
	var record models.MealRecord
	if err := database.DB.Scopes(database.OwnedBy(uid)).First(&record, id).Error; err != nil {
		return nil, ErrRecordNotFound
	}
	if patch.Rating != nil {
		record.Rating = clampInt(*patch.Rating, 0, 5)
	}
	if patch.Remark != nil {
		record.Remark = truncateRunes(strings.TrimSpace(*patch.Remark), 500)
	}
	if patch.Mood != nil {
		record.Mood = strings.TrimSpace(*patch.Mood)
	}
	if patch.Photo != nil {
		record.Photo = strings.TrimSpace(*patch.Photo)
	}
	if err := database.DB.Save(&record).Error; err != nil {
		return nil, err
	}
	QueueAutoAchievementSync(uid)
	return &record, nil
}

func DeleteMealRecord(uid uint, id any) (*models.MealRecord, error) {
	var record models.MealRecord
	if err := database.DB.Scopes(database.OwnedBy(uid)).First(&record, id).Error; err != nil {
		return nil, ErrRecordNotFound
	}
	if err := database.DB.Delete(&record).Error; err != nil {
		return nil, err
	}
	database.DB.Scopes(database.OwnedBy(uid)).
		Where("dish_id = ? AND meal_type = ? AND meal_date = ?", record.DishID, record.MealType, record.MealDate).
		Delete(&models.ShoppingCheck{})
	QueueAutoAchievementSync(uid)
	return &record, nil
}

type nameAmount struct {
	Name   string `json:"name"`
	Amount string `json:"amount"`
}

// addShoppingItems 把这一餐的食材与调料写入个人买菜清单，返回写入的条目。
func addShoppingItems(uid uint, dish models.Dish, mealType, mealDate string) []nameAmount {
	entries := dishShoppingEntries(dish)
	items := make([]models.ShoppingCheck, 0, len(entries))
	for _, it := range entries {
		items = append(items, models.ShoppingCheck{
			UserID: uid, MealDate: mealDate, MealType: mealType,
			DishID: dish.ID, DishName: dish.Name, ItemName: it.Name, ItemAmount: it.Amount,
		})
	}
	if len(items) > 0 {
		database.DB.Create(&items)
	}
	return entries
}

// SetFavorite 收藏 / 取消收藏（幂等）。
func SetFavorite(uid, dishID uint, favorite bool) error {
	if favorite {
		dish, err := FindVisibleDish(uid, dishID)
		if err != nil {
			return ErrDishNotFound
		}
		fav := models.Favorite{UserID: uid, DishID: dish.ID}
		database.DB.Where("user_id = ? AND dish_id = ?", uid, dish.ID).FirstOrCreate(&fav)
	} else {
		database.DB.Scopes(database.OwnedBy(uid)).Where("dish_id = ?", dishID).Delete(&models.Favorite{})
	}
	QueueAutoAchievementSync(uid)
	return nil
}

var validEventTypes = map[string]bool{
	"view": true, "recommend": true, "accept": true, "reject": true,
	"search": true, "chat": true, "feedback": true, "custom": true,
}

func IsValidEventType(t string) bool { return validEventTypes[strings.TrimSpace(t)] }

// LogBehavior 写一条行为事件（归属 uid）。dish 名称缺省时从可见菜品里补齐。
func LogBehavior(uid uint, eventType string, dishID uint, dishName, source, actor string, meta map[string]any) {
	eventType = strings.TrimSpace(eventType)
	if eventType == "" || uid == 0 {
		return
	}
	if !validEventTypes[eventType] {
		eventType = "custom"
	}
	if dishID > 0 && dishName == "" {
		var d models.Dish
		if err := database.DB.Unscoped().Scopes(database.VisibleDishes(uid)).Select("name").First(&d, dishID).Error; err == nil {
			dishName = d.Name
		} else {
			// 看不到的菜（别人的私房菜）不记 ID，避免借行为接口探测
			dishID = 0
		}
	}
	metaJSON := "{}"
	if meta != nil {
		if b, err := json.Marshal(meta); err == nil && len(b) <= 4096 {
			metaJSON = string(b)
		}
	}
	if source == "" {
		source = "app"
	}
	database.DB.Create(&models.BehaviorEvent{
		UserID: uid, EventType: eventType, DishID: dishID, DishName: truncateRunes(dishName, 64),
		Source: truncateRunes(strings.TrimSpace(source), 64), Actor: truncateRunes(strings.TrimSpace(actor), 64), Meta: metaJSON,
	})
}
