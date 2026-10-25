package services

import (
	"encoding/json"
	"gorm.io/gorm"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"sort"
	"strings"
)

// RecommendRequest 推荐请求。所有字段可选；空请求 = 按口味画像给 3 道菜。
type RecommendRequest struct {
	confirmedDishIDs   map[uint]bool // Real choices, distinct from earlier candidates in this batch.
	MealDate           string        `json:"meal_date"`
	SessionID          string        `json:"session_id"`
	Refresh            bool          `json:"refresh"`
	SelectedDishIDs    []uint        `json:"selected_dish_ids"`
	MealType           string        `json:"meal_type"`
	Mood               string        `json:"mood"`
	Count              int           `json:"count"`
	Keyword            string        `json:"keyword"`
	Tastes             []string      `json:"tastes"`
	Categories         []string      `json:"categories"`
	MaxCookTime        int           `json:"max_cook_time"`
	Difficulty         string        `json:"difficulty"`
	IncludeIngredients []string      `json:"include_ingredients"`
	ExcludeIngredients []string      `json:"exclude_ingredients"`
	ExcludeDishIDs     []uint        `json:"exclude_dish_ids"`
	ExcludeRecentDays  *int          `json:"exclude_recent_days"`
	ProfileDays        int           `json:"profile_days"`
	Diversity          *bool         `json:"diversity"`
	IgnorePreferences  bool          `json:"ignore_preferences"`
	Jitter             bool          `json:"jitter"`
	Mode               string        `json:"mode"`
	Source             string        `json:"source"`
	Actor              string        `json:"actor"`
}

type RecommendItem struct {
	ItemID  string `json:"item_id"`
	parts   recommendationScoreParts
	Dish    models.Dish `json:"dish"`
	Score   float64     `json:"score"`
	Reasons []string    `json:"reasons"`
}

type RecommendResponse struct {
	RecommendationID string          `json:"recommendation_id"`
	SessionID        string          `json:"session_id"`
	ServedForDate    string          `json:"served_for_date"`
	MealType         string          `json:"meal_type"`
	AlgorithmVersion string          `json:"algorithm_version"`
	Items            []RecommendItem `json:"items"`
	CandidateCount   int             `json:"candidate_count"`
	ProfileSummary   string          `json:"profile_summary"`
	Applied          map[string]any  `json:"applied"`
}

type candidateFilter struct {
	mealType    string
	categories  map[string]bool
	tastes      []string
	include     []string
	exclude     []string
	hardExclude []string // 过敏原：任何放宽都不会去掉
	keyword     string
	difficulty  string
	maxCookTime int
	excluded    map[uint]bool
	recent      map[uint]bool
	applyTastes bool
	applyRecent bool
	applyAvoid  bool
}

// RecommendDishes 站内统一推荐引擎：候选过滤 → 画像打分 → 多样性挑选 → 记录推荐事件。
// 首页午/晚餐推荐、心情推荐、AI 助手与 /api/agent/recommend 共用这一套逻辑；只使用 uid 本人的数据。
func RecommendDishes(uid uint, req RecommendRequest, dbs ...*gorm.DB) (*RecommendResponse, error) {
	ctx, err := NewRecommendationContext(uid, req.ProfileDays, dbs...)
	if err != nil {
		return nil, err
	}
	return RecommendWithContext(ctx, req)
}

func filterCandidates(dishes []models.Dish, f candidateFilter) []models.Dish {
	var out []models.Dish
	for _, d := range dishes {
		if f.excluded[d.ID] {
			continue
		}
		if f.applyRecent && f.recent[d.ID] {
			continue
		}
		if f.mealType != "" && d.MealType != "" && d.MealType != "all" && d.MealType != f.mealType {
			continue
		}
		if len(f.categories) > 0 && !f.categories[d.Category] {
			continue
		}
		if f.difficulty != "" && d.Difficulty != f.difficulty {
			continue
		}
		if f.maxCookTime > 0 && (d.CookTime <= 0 || d.CookTime > f.maxCookTime) {
			continue
		}
		text := dishSearchText(d)
		if f.keyword != "" && !strings.Contains(text, f.keyword) {
			continue
		}
		requiredText := strings.Join(append(ingredientNames(d.Ingredients), ingredientNames(d.Seasonings)...), " ")
		if !containsAll(requiredText, f.include) {
			continue
		}
		if containsAny(text, f.hardExclude) {
			continue
		}
		if f.applyAvoid && containsAny(text, f.exclude) {
			continue
		}
		if f.applyTastes && len(f.tastes) > 0 && !tasteMatchesAny(d, f.tastes) {
			continue
		}
		out = append(out, d)
	}
	return out
}

func logRecommendEvents(uid uint, items []RecommendItem, req RecommendRequest, mealType string, dbs ...*gorm.DB) {
	requestDB := database.Handle(dbs...)

	if len(items) == 0 {
		return
	}
	// 首页自动加载的推荐不算用户主动行为，不写事件，避免污染采纳率
	if strings.TrimSpace(req.Mode) == "home_auto" || strings.TrimSpace(req.Mode) == "week_plan" {
		return
	}
	source := strings.TrimSpace(req.Source)
	if source == "" {
		source = "app"
	}
	events := make([]models.BehaviorEvent, 0, len(items))
	for _, it := range items {
		meta, _ := json.Marshal(map[string]any{
			"score":             it.Score,
			"item_id":           it.ItemID,
			"recommendation_id": strings.SplitN(it.ItemID, ":", 2)[0],
			"session_id":        req.SessionID,
			"algorithm_version": RecommendationAlgorithmVersion,
			"reasons":           it.Reasons,
			"meal_type":         mealType,
			"mood":              strings.TrimSpace(req.Mood),
			"mode":              strings.TrimSpace(req.Mode),
		})
		events = append(events, models.BehaviorEvent{
			UserID:    uid,
			EventType: "recommend",
			DishID:    it.Dish.ID,
			DishName:  it.Dish.Name,
			Source:    source,
			Actor:     strings.TrimSpace(req.Actor),
			Meta:      string(meta),
		})
	}
	requestDB.Create(&events)
	if strings.HasPrefix(source, "agent") || source == "assistant" {
		RecordAchievementEvent(uid, "agent_recommend", "", requestDB)
	}
}

// RecentDishIDs 返回用户最近 days 天（含未来日期的计划）吃过的菜品 ID。
func RecentDishIDs(uid uint, days int, dbs ...*gorm.DB) []uint {
	requestDB := database.Handle(dbs...)

	recent := recentDishIDMap(uid, days, requestDB)
	ids := make([]uint, 0, len(recent))
	for id := range recent {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func cleanList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		for _, part := range strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == '，' || r == '、' || r == '|' }) {
			if p := strings.TrimSpace(part); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

func stringSet(values []string) map[string]bool {
	set := make(map[string]bool)
	for _, v := range cleanList(values) {
		set[v] = true
	}
	return set
}

func dishSearchText(d models.Dish) string {
	parts := []string{d.Name, d.Category, d.Taste, d.Remark}
	parts = append(parts, ingredientNames(d.Ingredients)...)
	parts = append(parts, ingredientNames(d.Seasonings)...)
	parts = append(parts, parseTags(d.Tags)...)
	return strings.ToLower(strings.Join(parts, " "))
}

// DishSearchText 菜名、菜系、口味、食材、调料、标签拼成的检索文本（小写）。
func DishSearchText(d models.Dish) string { return dishSearchText(d) }

func containsAll(text string, needles []string) bool {
	for _, n := range needles {
		if !matchesRequiredIngredientTerm(text, n) {
			return false
		}
	}
	return true
}

func containsAny(text string, needles []string) bool {
	for _, n := range needles {
		if matchesIngredientTerm(text, n) {
			return true
		}
	}
	return false
}

func tasteMatches(d models.Dish, taste string) bool {
	taste = strings.TrimSpace(taste)
	if taste == "" {
		return false
	}
	if taste == "辣" && isSpicyDish(d) {
		return true
	}
	return containsTaste(d.Taste, taste) || strings.Contains(d.Name, taste)
}

func tasteMatchesAny(d models.Dish, tastes []string) bool {
	for _, t := range tastes {
		if tasteMatches(d, t) {
			return true
		}
	}
	return false
}

// TasteMatchesAny 导出给 handlers 的口味匹配（含“辣”的宽松匹配）。
func TasteMatchesAny(d models.Dish, tastes []string) bool { return tasteMatchesAny(d, tastes) }

func hasAnyTag(d models.Dish, targets ...string) bool {
	tags := parseTags(d.Tags)
	for _, t := range targets {
		if containsTag(tags, t) {
			return true
		}
	}
	return false
}
