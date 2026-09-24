package services

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"sort"
	"strings"
	"time"
)

// RecommendRequest 推荐请求。所有字段可选；空请求 = 按口味画像给 3 道菜。
type RecommendRequest struct {
	MealType           string   `json:"meal_type"`
	Mood               string   `json:"mood"`
	Count              int      `json:"count"`
	Keyword            string   `json:"keyword"`
	Tastes             []string `json:"tastes"`
	Categories         []string `json:"categories"`
	MaxCookTime        int      `json:"max_cook_time"`
	Difficulty         string   `json:"difficulty"`
	IncludeIngredients []string `json:"include_ingredients"`
	ExcludeIngredients []string `json:"exclude_ingredients"`
	ExcludeDishIDs     []uint   `json:"exclude_dish_ids"`
	ExcludeRecentDays  *int     `json:"exclude_recent_days"`
	ProfileDays        int      `json:"profile_days"`
	Diversity          *bool    `json:"diversity"`
	IgnorePreferences  bool     `json:"ignore_preferences"`
	Jitter             bool     `json:"jitter"`
	Mode               string   `json:"mode"`
	Source             string   `json:"source"`
	Actor              string   `json:"actor"`
}

type RecommendItem struct {
	Dish    models.Dish `json:"dish"`
	Score   float64     `json:"score"`
	Reasons []string    `json:"reasons"`
}

type RecommendResponse struct {
	Items          []RecommendItem `json:"items"`
	CandidateCount int             `json:"candidate_count"`
	ProfileSummary string          `json:"profile_summary"`
	Applied        map[string]any  `json:"applied"`
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
func RecommendDishes(uid uint, req RecommendRequest) (*RecommendResponse, error) {
	count := req.Count
	if count < 1 {
		count = 3
	}
	if count > 10 {
		count = 10
	}
	mealType := strings.TrimSpace(req.MealType)
	if mealType != "lunch" && mealType != "dinner" {
		mealType = ""
	}
	excludeRecentDays := UserRepeatDays(uid)
	if req.ExcludeRecentDays != nil {
		excludeRecentDays = *req.ExcludeRecentDays
		if excludeRecentDays < 0 {
			excludeRecentDays = 0
		}
	}
	diversity := true
	if req.Diversity != nil {
		diversity = *req.Diversity
	}

	profile := BuildTasteProfile(uid, req.ProfileDays)
	prefs := profile.Preferences

	applied := map[string]any{
		"meal_type":           mealType,
		"mood":                strings.TrimSpace(req.Mood),
		"count":               count,
		"exclude_recent_days": excludeRecentDays,
		"diversity":           diversity,
		"profile_days":        profile.WindowDays,
		"preferences_applied": !req.IgnorePreferences,
		"relaxed":             []string{},
	}
	empty := &RecommendResponse{Items: []RecommendItem{}, ProfileSummary: profile.Summary, Applied: applied}

	dishes := VisibleEnabledDishes(uid)
	if len(dishes) == 0 {
		return empty, nil
	}

	excluded := make(map[uint]bool, len(req.ExcludeDishIDs))
	for _, id := range req.ExcludeDishIDs {
		if id > 0 {
			excluded[id] = true
		}
	}
	recent := map[uint]bool{}
	if excludeRecentDays > 0 {
		recent = recentDishIDMap(uid, excludeRecentDays)
	}

	exclude := cleanList(req.ExcludeIngredients)
	var hardExclude []string
	if !req.IgnorePreferences {
		hardExclude = prefs.Allergies
		exclude = append(exclude, prefs.AvoidIngredients...)
	}
	maxCook := req.MaxCookTime

	filter := candidateFilter{
		mealType:    mealType,
		categories:  stringSet(req.Categories),
		tastes:      cleanList(req.Tastes),
		include:     cleanList(req.IncludeIngredients),
		exclude:     exclude,
		hardExclude: hardExclude,
		keyword:     strings.ToLower(strings.TrimSpace(req.Keyword)),
		difficulty:  strings.TrimSpace(req.Difficulty),
		maxCookTime: maxCook,
		excluded:    excluded,
		recent:      recent,
		applyTastes: true,
		applyRecent: true,
		applyAvoid:  true,
	}

	// 逐级放宽：先去掉口味硬过滤，再去掉近期去重，最后放开忌口（过敏原永不放开），保证尽量给得出结果
	relaxed := []string{}
	candidates := filterCandidates(dishes, filter)
	if len(candidates) == 0 && len(filter.tastes) > 0 {
		filter.applyTastes = false
		relaxed = append(relaxed, "tastes")
		candidates = filterCandidates(dishes, filter)
	}
	if len(candidates) == 0 && len(recent) > 0 {
		filter.applyRecent = false
		relaxed = append(relaxed, "recent")
		candidates = filterCandidates(dishes, filter)
	}
	if len(candidates) == 0 && len(filter.exclude) > 0 {
		filter.applyAvoid = false
		relaxed = append(relaxed, "avoid_ingredients")
		candidates = filterCandidates(dishes, filter)
	}
	applied["relaxed"] = relaxed
	if len(candidates) == 0 {
		return empty, nil
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	scored := make([]RecommendItem, 0, len(candidates))
	for _, d := range candidates {
		score, reasons := scoreDish(d, req, mealType, profile, today)
		if req.Jitter {
			score += rng.Float64() * 6
		}
		d.Favorite = profile.favoriteSet[d.ID]
		scored = append(scored, RecommendItem{Dish: d, Score: math.Round(score*10) / 10, Reasons: reasons})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].Score != scored[j].Score {
			return scored[i].Score > scored[j].Score
		}
		return scored[i].Dish.ID < scored[j].Dish.ID
	})

	picked := pickDiverse(scored, count, diversity)
	logRecommendEvents(uid, picked, req, mealType)

	return &RecommendResponse{
		Items:          picked,
		CandidateCount: len(candidates),
		ProfileSummary: profile.Summary,
		Applied:        applied,
	}, nil
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
		if f.maxCookTime > 0 && d.CookTime > f.maxCookTime {
			continue
		}
		text := dishSearchText(d)
		if f.keyword != "" && !strings.Contains(text, f.keyword) {
			continue
		}
		if !containsAll(text, f.include) {
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

func scoreDish(d models.Dish, req RecommendRequest, mealType string, p *TasteProfile, today time.Time) (float64, []string) {
	score := 10.0
	reasons := make([]string, 0, 4)
	add := func(points float64, reason string) {
		score += points
		if reason != "" && len(reasons) < 3 {
			reasons = append(reasons, reason)
		}
	}

	// 口味画像：取该菜最匹配的口味权重
	best, bestName := 0.0, ""
	for _, t := range tasteTokens(d.Taste) {
		if w := p.tasteWeightMap[t]; w > best {
			best, bestName = w, t
		}
	}
	if best > 0 {
		if best >= 0.12 {
			add(30*best, "常吃口味「"+bestName+"」")
		} else {
			score += 30 * best
		}
	}
	if w := p.categoryWeightMap[d.Category]; w > 0 {
		score += 15 * w
	}

	for _, t := range cleanList(req.Tastes) {
		if tasteMatches(d, t) {
			add(12, "符合想要的口味「"+t+"」")
			break
		}
	}

	if d.FamilyID != 0 {
		add(6, "家里的共享菜谱")
	} else if d.OwnerID != 0 {
		add(6, "你的私房菜")
	}
	if p.favoriteSet[d.ID] {
		add(15, "已收藏的心头好")
	}
	if p.likedSet[d.ID] {
		add(10, "上次吃评价很高")
	}
	if p.dislikedSet[d.ID] {
		add(-25, "之前吃过评价一般")
	}
	if n := p.rejectCount[d.ID]; n > 0 {
		score -= math.Min(float64(n)*8, 24)
	}

	if last, ok := p.LastEaten[d.ID]; ok {
		if t, ok2 := parseMealDate(last); ok2 {
			age := today.Sub(t).Hours() / 24
			switch {
			case age < 0:
				add(-30, "已经排在明天的菜单里")
			case age <= 3:
				score -= 30
			case age <= 14:
				score -= 12 * (1 - age/14)
			case age >= 21:
				add(4, fmt.Sprintf("有 %d 天没吃了，换换口味", int(age)))
			}
		}
	} else if p.WindowRecords > 0 {
		add(5, "还没做过，尝个新")
	}

	// 显式偏好
	if !req.IgnorePreferences {
		pr := p.Preferences
		switch pr.SpiceLevel {
		case 0:
			if isSpicyDish(d) {
				score -= 30
			} else {
				add(8, "不辣，合你口味")
			}
		case 3:
			if isSpicyDish(d) {
				add(8, "够辣够过瘾")
			}
		}
		if pr.MaxCookTime > 0 && d.CookTime > 0 {
			if d.CookTime <= pr.MaxCookTime {
				score += 5
			} else {
				score -= math.Min(float64(d.CookTime-pr.MaxCookTime)/2, 15)
			}
		}
		if pr.Goals != "" && strings.Contains(pr.Goals, "减") && (containsTaste(d.Taste, "清淡") || hasAnyTag(d, "清淡", "健康", "低脂")) {
			add(6, "清爽，贴合"+pr.Goals+"目标")
		}
	}

	switch strings.TrimSpace(req.Mood) {
	case "tired", "lazy":
		if d.Difficulty == "easy" {
			add(12, "做法简单，不费劲")
		}
		if d.CookTime > 0 && d.CookTime <= 25 {
			add(8, fmt.Sprintf("%d 分钟就能上桌", d.CookTime))
		}
		if d.Difficulty == "hard" {
			score -= 15
		}
	case "spicy":
		if isSpicyDish(d) {
			add(20, "够辣，正对胃口")
		} else {
			score -= 12
		}
	case "healthy":
		if containsTaste(d.Taste, "清淡") || containsTaste(d.Taste, "鲜") || hasAnyTag(d, "汤品", "汤菜", "清淡", "健康") {
			add(15, "清爽少负担")
		}
		if isSpicyDish(d) {
			score -= 8
		}
	case "happy":
		if hasAnyTag(d, "硬菜", "宴客", "聚餐") {
			add(8, "开心就吃顿硬的")
		}
	}

	if strings.TrimSpace(req.Mood) != "healthy" && p.SpicyRatio >= 0.5 && isSpicyDish(d) {
		score += 6
	}
	if req.MaxCookTime > 0 && d.CookTime > 0 && d.CookTime <= req.MaxCookTime {
		score += 4
	}
	if p.AvgCookTime > 0 && d.CookTime > 0 {
		diff := math.Abs(float64(d.CookTime) - p.AvgCookTime)
		if diff <= 10 {
			score += 3
		} else if diff >= 40 {
			score -= 3
		}
	}
	if mealType != "" && d.MealType == mealType {
		score += 3
	}

	if len(reasons) == 0 {
		reasons = append(reasons, "菜单里的一道好菜")
	}
	return score, reasons
}

// pickDiverse 按分数贪心挑选，同一菜系最多占一半名额，避免一次推荐全是同一菜系。
func pickDiverse(scored []RecommendItem, count int, diversity bool) []RecommendItem {
	if !diversity {
		if len(scored) > count {
			return scored[:count]
		}
		return scored
	}
	perCategoryCap := int(math.Ceil(float64(count) / 2))
	if perCategoryCap < 1 {
		perCategoryCap = 1
	}
	perCategory := map[string]int{}
	used := map[uint]bool{}
	picked := make([]RecommendItem, 0, count)
	for _, it := range scored {
		if len(picked) >= count {
			break
		}
		if perCategory[it.Dish.Category] >= perCategoryCap {
			continue
		}
		picked = append(picked, it)
		perCategory[it.Dish.Category]++
		used[it.Dish.ID] = true
	}
	for _, it := range scored {
		if len(picked) >= count {
			break
		}
		if used[it.Dish.ID] {
			continue
		}
		picked = append(picked, it)
		used[it.Dish.ID] = true
	}
	return picked
}

func logRecommendEvents(uid uint, items []RecommendItem, req RecommendRequest, mealType string) {
	if len(items) == 0 {
		return
	}
	// 首页自动加载的推荐不算用户主动行为，不写事件，避免污染采纳率
	if strings.TrimSpace(req.Mode) == "home_auto" {
		return
	}
	source := strings.TrimSpace(req.Source)
	if source == "" {
		source = "app"
	}
	events := make([]models.BehaviorEvent, 0, len(items))
	for _, it := range items {
		meta, _ := json.Marshal(map[string]any{
			"score":     it.Score,
			"reasons":   it.Reasons,
			"meal_type": mealType,
			"mood":      strings.TrimSpace(req.Mood),
			"mode":      strings.TrimSpace(req.Mode),
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
	database.DB.Create(&events)
	if strings.HasPrefix(source, "agent") || source == "assistant" {
		RecordAchievementEvent(uid, "agent_recommend", "")
	}
}

// RecentDishIDs 返回用户最近 days 天（含未来日期的计划）吃过的菜品 ID。
func RecentDishIDs(uid uint, days int) []uint {
	recent := recentDishIDMap(uid, days)
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
		if !strings.Contains(text, strings.ToLower(n)) {
			return false
		}
	}
	return true
}

func containsAny(text string, needles []string) bool {
	for _, n := range needles {
		if n != "" && strings.Contains(text, strings.ToLower(n)) {
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
