package services

import (
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"strings"
)

// DishQuery 菜品检索条件（智能体与 AI 助手使用；站内列表页用分页查询）。
type DishQuery struct {
	Keyword           string   `json:"keyword"`
	Categories        []string `json:"categories"`
	Tastes            []string `json:"tastes"`
	Ingredients       []string `json:"ingredients"`
	ExcludeIngredient []string `json:"exclude_ingredients"`
	MaxCookTime       int      `json:"max_cook_time"`
	Difficulty        string   `json:"difficulty"`
	MealType          string   `json:"meal_type"`
	OnlyFavorites     bool     `json:"only_favorites"`
	OnlyMine          bool     `json:"only_mine"`
	ExcludeRecent     bool     `json:"exclude_recent"`
	IDs               []uint   `json:"ids"`
	IncludeDisabled   bool     `json:"include_disabled"`
	Offset            int      `json:"offset"`
	Limit             int      `json:"limit"`
}

// SearchDishes 在用户可见菜品中检索，返回当前页与总数。
func SearchDishes(uid uint, q DishQuery) ([]models.Dish, int) {
	query := database.DB.Scopes(database.VisibleDishes(uid))
	if !q.IncludeDisabled {
		query = query.Where("enabled = ?", true)
	}
	if cats := cleanList(q.Categories); len(cats) > 0 {
		query = query.Where("category IN ?", cats)
	}
	if q.Difficulty != "" {
		query = query.Where("difficulty = ?", q.Difficulty)
	}
	if q.MealType == "lunch" || q.MealType == "dinner" {
		query = query.Where("meal_type IN ?", []string{q.MealType, "all", ""})
	}
	if q.MaxCookTime > 0 {
		query = query.Where("cook_time > 0 AND cook_time <= ?", q.MaxCookTime)
	}
	if len(q.IDs) > 0 {
		query = query.Where("id IN ?", q.IDs)
	}
	if q.OnlyMine {
		query = query.Where("owner_id = ?", uid)
	}

	var all []models.Dish
	query.Order("sort_order ASC, id ASC").Find(&all)

	favs := FavoriteIDSet(uid)
	recent := map[uint]bool{}
	if q.ExcludeRecent {
		recent = recentDishIDMap(uid, UserRepeatDays(uid))
	}
	keyword := strings.ToLower(strings.TrimSpace(q.Keyword))
	tastes := cleanList(q.Tastes)
	include := cleanList(q.Ingredients)
	exclude := cleanList(q.ExcludeIngredient)

	filtered := make([]models.Dish, 0, len(all))
	for _, d := range all {
		d.Favorite = favs[d.ID]
		if q.OnlyFavorites && !d.Favorite {
			continue
		}
		if recent[d.ID] {
			continue
		}
		text := dishSearchText(d)
		if keyword != "" && !matchesKeyword(text, keyword) {
			continue
		}
		if len(tastes) > 0 && !tasteMatchesAny(d, tastes) {
			continue
		}
		if !containsAll(text, include) || containsAny(text, exclude) {
			continue
		}
		filtered = append(filtered, d)
	}

	total := len(filtered)
	offset := q.Offset
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 500 {
		limit = 500
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return filtered[offset:end], total
}

// matchesKeyword 关键词按空格拆开后全部命中（如“牛肉 辣”）。
func matchesKeyword(text, keyword string) bool {
	for _, part := range strings.Fields(keyword) {
		if !strings.Contains(text, part) {
			return false
		}
	}
	return true
}

// DishUserStats 某道菜在当前用户下的统计。
type DishUserStats struct {
	TotalCount int     `json:"total_count"`
	YumCount   int     `json:"yum_count"`
	OkCount    int     `json:"ok_count"`
	NoCount    int     `json:"no_count"`
	AvgRating  float64 `json:"avg_rating"`
	LastDate   string  `json:"last_date"`
	ViewCount  int64   `json:"view_count"`
	Favorite   bool    `json:"favorite"`
}

func DishStatsForUser(uid uint, dishID uint) (DishUserStats, []models.MealRecord) {
	var records []models.MealRecord
	database.DB.Scopes(database.OwnedBy(uid)).Where("dish_id = ?", dishID).
		Order("meal_date DESC, created_at DESC").Limit(50).Find(&records)

	s := DishUserStats{TotalCount: len(records)}
	ratingSum, ratingN := 0, 0
	for _, r := range records {
		switch r.Mood {
		case "yum", "great":
			s.YumCount++
		case "ok":
			s.OkCount++
		case "no", "meh":
			s.NoCount++
		}
		if r.Rating > 0 {
			ratingSum += r.Rating
			ratingN++
		}
	}
	if ratingN > 0 {
		s.AvgRating = float64(ratingSum) / float64(ratingN)
	}
	if len(records) > 0 {
		s.LastDate = records[0].MealDate
	}
	database.DB.Model(&models.BehaviorEvent{}).Scopes(database.OwnedBy(uid)).
		Where("dish_id = ? AND event_type = ?", dishID, "view").Count(&s.ViewCount)
	s.Favorite = FavoriteIDSet(uid)[dishID]
	return s, records
}

// DishCard 给大模型与智能体的菜品摘要（不含完整步骤，控制 token）。
type DishCard struct {
	ID          uint     `json:"id"`
	Name        string   `json:"name"`
	Category    string   `json:"category"`
	Taste       string   `json:"taste"`
	CookTime    int      `json:"cook_time"`
	Difficulty  string   `json:"difficulty"`
	MealType    string   `json:"meal_type"`
	Ingredients []string `json:"ingredients"`
	Image       string   `json:"image,omitempty"`
	Favorite    bool     `json:"favorite"`
	Private     bool     `json:"private,omitempty"`
}

func ToDishCard(d models.Dish) DishCard {
	ings := ingredientNames(d.Ingredients)
	if len(ings) > 8 {
		ings = ings[:8]
	}
	if ings == nil {
		ings = []string{}
	}
	return DishCard{
		ID: d.ID, Name: d.Name, Category: d.Category, Taste: d.Taste, CookTime: d.CookTime,
		Difficulty: d.Difficulty, MealType: d.MealType, Ingredients: ings,
		Image: DishImageURL(d), Favorite: d.Favorite, Private: d.OwnerID != 0,
	}
}

func ToDishCards(list []models.Dish) []DishCard {
	out := make([]DishCard, 0, len(list))
	for _, d := range list {
		out = append(out, ToDishCard(d))
	}
	return out
}

// DishImageURL 菜品封面图：image_url 优先，其次 images 第一张。
func DishImageURL(d models.Dish) string {
	if isImageURL(d.ImageURL) {
		return d.ImageURL
	}
	for _, img := range parseStringArray(d.Images) {
		if isImageURL(img) {
			return img
		}
	}
	return ""
}

func isImageURL(url string) bool {
	if url == "" {
		return false
	}
	if strings.HasPrefix(url, "/uploads/") || strings.HasPrefix(url, "http") {
		return true
	}
	lower := strings.ToLower(url)
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".webp", ".gif", ".svg"} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}
