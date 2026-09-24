package services

import (
	"encoding/json"
	"errors"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
)

// Preferences 用户饮食偏好（对外 JSON 形态）。
type Preferences struct {
	AvoidIngredients []string  `json:"avoid_ingredients"`
	Allergies        []string  `json:"allergies"`
	FavoriteTastes   []string  `json:"favorite_tastes"`
	SpiceLevel       int       `json:"spice_level"`
	HouseholdSize    int       `json:"household_size"`
	MaxCookTime      int       `json:"max_cook_time"`
	Goals            string    `json:"goals"`
	Notes            string    `json:"notes"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// SpiceLevelLabel 辣度描述，给 AI 与画像摘要使用。
func SpiceLevelLabel(level int) string {
	switch level {
	case 0:
		return "不吃辣"
	case 1:
		return "微辣"
	case 2:
		return "中辣"
	case 3:
		return "特辣"
	}
	return ""
}

func GetPreferences(uid uint) Preferences {
	var row models.UserPreference
	if err := database.DB.Where("user_id = ?", uid).First(&row).Error; err != nil {
		return Preferences{AvoidIngredients: []string{}, Allergies: []string{}, FavoriteTastes: []string{}, SpiceLevel: -1}
	}
	return Preferences{
		AvoidIngredients: parseStringArray(row.AvoidIngredients),
		Allergies:        parseStringArray(row.Allergies),
		FavoriteTastes:   parseStringArray(row.FavoriteTastes),
		SpiceLevel:       row.SpiceLevel,
		HouseholdSize:    row.HouseholdSize,
		MaxCookTime:      row.MaxCookTime,
		Goals:            row.Goals,
		Notes:            row.Notes,
		UpdatedAt:        row.UpdatedAt,
	}
}

// PreferencesPatch 部分更新：nil 字段保持不变（智能体可以只改其中一项）。
type PreferencesPatch struct {
	AvoidIngredients *[]string `json:"avoid_ingredients"`
	Allergies        *[]string `json:"allergies"`
	FavoriteTastes   *[]string `json:"favorite_tastes"`
	SpiceLevel       *int      `json:"spice_level"`
	HouseholdSize    *int      `json:"household_size"`
	MaxCookTime      *int      `json:"max_cook_time"`
	Goals            *string   `json:"goals"`
	Notes            *string   `json:"notes"`
}

func SavePreferences(uid uint, patch PreferencesPatch) (Preferences, error) {
	var row models.UserPreference
	err := database.DB.Where("user_id = ?", uid).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row = models.UserPreference{UserID: uid, AvoidIngredients: "[]", Allergies: "[]", FavoriteTastes: "[]", SpiceLevel: -1}
	} else if err != nil {
		return Preferences{}, err
	}
	if patch.AvoidIngredients != nil {
		row.AvoidIngredients = jsonStrings(cleanTerms(*patch.AvoidIngredients, 30))
	}
	if patch.Allergies != nil {
		row.Allergies = jsonStrings(cleanTerms(*patch.Allergies, 20))
	}
	if patch.FavoriteTastes != nil {
		row.FavoriteTastes = jsonStrings(cleanTerms(*patch.FavoriteTastes, 20))
	}
	if patch.SpiceLevel != nil {
		row.SpiceLevel = clampInt(*patch.SpiceLevel, -1, 3)
	}
	if patch.HouseholdSize != nil {
		row.HouseholdSize = clampInt(*patch.HouseholdSize, 0, 20)
	}
	if patch.MaxCookTime != nil {
		row.MaxCookTime = clampInt(*patch.MaxCookTime, 0, 240)
	}
	if patch.Goals != nil {
		row.Goals = truncateRunes(strings.TrimSpace(*patch.Goals), 100)
	}
	if patch.Notes != nil {
		row.Notes = truncateRunes(strings.TrimSpace(*patch.Notes), 500)
	}
	if err := database.DB.Save(&row).Error; err != nil {
		return Preferences{}, err
	}
	return GetPreferences(uid), nil
}

func cleanTerms(values []string, max int) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, v := range cleanList(values) {
		v = truncateRunes(v, 20)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
		if len(out) >= max {
			break
		}
	}
	return out
}

func jsonStrings(v []string) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// ---------- 收藏 / 菜品可见性 ----------

// FavoriteIDSet 用户收藏的菜品 ID 集合。
func FavoriteIDSet(uid uint) map[uint]bool {
	var ids []uint
	database.DB.Model(&models.Favorite{}).Scopes(database.OwnedBy(uid)).Pluck("dish_id", &ids)
	set := make(map[uint]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return set
}

// MarkFavorites 按当前用户填充菜品的 favorite 视图字段。
func MarkFavorites(uid uint, dishes []models.Dish) {
	if len(dishes) == 0 {
		return
	}
	set := FavoriteIDSet(uid)
	for i := range dishes {
		dishes[i].Favorite = set[dishes[i].ID]
	}
}

func MarkFavorite(uid uint, dish *models.Dish) {
	if dish == nil {
		return
	}
	var n int64
	database.DB.Model(&models.Favorite{}).Scopes(database.OwnedBy(uid)).Where("dish_id = ?", dish.ID).Count(&n)
	dish.Favorite = n > 0
}

// FindVisibleDish 按 ID 取当前用户可见的菜品（公共或本人私有）。
func FindVisibleDish(uid uint, id any) (models.Dish, error) {
	var dish models.Dish
	err := database.DB.Scopes(database.VisibleDishes(uid)).First(&dish, id).Error
	return dish, err
}

// VisibleEnabledDishes 当前用户可用于推荐的全部菜品。
func VisibleEnabledDishes(uid uint) []models.Dish {
	var dishes []models.Dish
	database.DB.Scopes(database.VisibleDishes(uid)).Where("enabled = ?", true).Find(&dishes)
	return dishes
}

// UserRepeatDays 推荐去重天数：用户设置优先，其次站点设置/环境变量。
func UserRepeatDays(uid uint) int {
	return getUserSettingInt(uid, "repeat_days", getSettingInt("repeat_days", defaultRepeatDays()))
}
