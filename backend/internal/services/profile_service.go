package services

import (
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"math"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"sort"
	"strings"
	"time"
)

const (
	defaultProfileDays = 90
	maxProfileDays     = 365
	// 权重半衰期（天）：45 天前的一餐权重减半，让画像跟随最近口味变化
	profileHalfLife = 45.0
)

// WeightItem 归一化后的偏好项：weight 为占比（0~1），count 为原始次数。
type WeightItem struct {
	Name   string  `json:"name"`
	Weight float64 `json:"weight"`
	Count  int     `json:"count"`
}

// DishBrief 菜品摘要，画像与推荐结果里用它避免携带完整做法。
type DishBrief struct {
	ID         uint   `json:"id"`
	Name       string `json:"name"`
	Category   string `json:"category"`
	Taste      string `json:"taste"`
	CookTime   int    `json:"cook_time"`
	Difficulty string `json:"difficulty"`
}

type DishFrequency struct {
	DishID   uint   `json:"dish_id"`
	DishName string `json:"dish_name"`
	Count    int    `json:"count"`
	LastDate string `json:"last_date"`
}

// TasteProfile 用户口味画像：由该用户的用餐记录、评价、收藏、首页心情、行为事件与显式偏好聚合而成。
// 这是对外（智能体）开放的核心“用户行为信息”，也是站内推荐引擎的打分依据。
type TasteProfile struct {
	GeneratedAt      string          `json:"generated_at"`
	WindowDays       int             `json:"window_days"`
	RepeatDays       int             `json:"repeat_days"`
	TotalRecords     int             `json:"total_records"`
	WindowRecords    int             `json:"window_records"`
	DistinctDishes   int             `json:"distinct_dishes"`
	MealTypeCounts   map[string]int  `json:"meal_type_counts"`
	TasteWeights     []WeightItem    `json:"taste_weights"`
	CategoryWeights  []WeightItem    `json:"category_weights"`
	DifficultyCounts map[string]int  `json:"difficulty_counts"`
	AvgCookTime      float64         `json:"avg_cook_time"`
	SpicyRatio       float64         `json:"spicy_ratio"`
	TopIngredients   []WeightItem    `json:"top_ingredients"`
	TopDishes        []DishFrequency `json:"top_dishes"`
	RecentDishIDs    []uint          `json:"recent_dish_ids"`
	LastEaten        map[uint]string `json:"last_eaten"`
	FavoriteDishes   []DishBrief     `json:"favorite_dishes"`
	LikedDishes      []DishBrief     `json:"liked_dishes"`
	DislikedDishes   []DishBrief     `json:"disliked_dishes"`
	MoodCounts       map[string]int  `json:"mood_counts"`
	HomeMoodCounts   map[string]int  `json:"home_mood_counts"`
	BehaviorCounts   map[string]int  `json:"behavior_counts"`
	RejectedDishes   []DishFrequency `json:"rejected_dishes"`
	MostViewedDishes []DishFrequency `json:"most_viewed_dishes"`
	AcceptRate       float64         `json:"accept_rate"`
	Preferences      Preferences     `json:"preferences"`
	Summary          string          `json:"summary"`

	tasteWeightMap      map[string]float64
	categoryWeightMap   map[string]float64
	favoriteSet         map[uint]bool
	likedSet            map[uint]bool
	dislikedSet         map[uint]bool
	eatenCount          map[uint]int
	viewCount           map[uint]int
	rejectCount         map[uint]int
	dishEvidence        map[uint]*affinityEvidence
	tasteNegative       map[string]float64
	categoryNegative    map[string]float64
	notToday            map[uint]bool
	loadErr             error
	ingredientNegative  map[string]float64
	ingredientWeightMap map[string]float64
	recentMainCounts    map[string]int
}

func normalizeProfileDays(days int) int {
	if days <= 0 {
		return defaultProfileDays
	}
	if days > maxProfileDays {
		return maxProfileDays
	}
	return days
}

// BuildTasteProfile 聚合用户 uid 最近 days 天的行为数据生成口味画像。只读取该用户自己的数据。
func BuildTasteProfile(uid uint, days int, dbs ...*gorm.DB) *TasteProfile {
	requestDB := database.Handle(dbs...)

	days = normalizeProfileDays(days)
	now := recommendationNow()
	today := recommendationDay(now)
	repeatDays := UserRepeatDays(uid, requestDB)

	p := &TasteProfile{
		GeneratedAt:         now.Format(time.RFC3339),
		WindowDays:          days,
		RepeatDays:          repeatDays,
		MealTypeCounts:      map[string]int{},
		TasteWeights:        []WeightItem{},
		CategoryWeights:     []WeightItem{},
		DifficultyCounts:    map[string]int{},
		TopIngredients:      []WeightItem{},
		TopDishes:           []DishFrequency{},
		RecentDishIDs:       []uint{},
		LastEaten:           map[uint]string{},
		FavoriteDishes:      []DishBrief{},
		LikedDishes:         []DishBrief{},
		DislikedDishes:      []DishBrief{},
		MoodCounts:          map[string]int{},
		HomeMoodCounts:      map[string]int{},
		BehaviorCounts:      map[string]int{},
		RejectedDishes:      []DishFrequency{},
		MostViewedDishes:    []DishFrequency{},
		Preferences:         GetPreferences(uid, requestDB),
		tasteWeightMap:      map[string]float64{},
		categoryWeightMap:   map[string]float64{},
		favoriteSet:         map[uint]bool{},
		likedSet:            map[uint]bool{},
		dislikedSet:         map[uint]bool{},
		eatenCount:          map[uint]int{},
		viewCount:           map[uint]int{},
		rejectCount:         map[uint]int{},
		dishEvidence:        map[uint]*affinityEvidence{},
		tasteNegative:       map[string]float64{},
		categoryNegative:    map[string]float64{},
		notToday:            map[uint]bool{},
		ingredientNegative:  map[string]float64{},
		ingredientWeightMap: map[string]float64{},
		recentMainCounts:    map[string]int{},
	}

	var allDishes []models.Dish
	if err := requestDB.Unscoped().Scopes(database.VisibleDishes(uid)).
		Select("id", "name", "category", "taste", "ingredients", "seasonings", "cook_time", "difficulty", "owner_id", "deleted_at").
		Find(&allDishes).Error; err != nil {
		p.loadErr = err
		return p
	}
	dishByID := make(map[uint]models.Dish, len(allDishes))
	for _, d := range allDishes {
		dishByID[d.ID] = d
	}

	var records []models.MealRecord
	if err := requestDB.Scopes(database.OwnedBy(uid)).
		Select("id", "dish_id", "dish_name", "meal_type", "meal_date", "rating", "mood", "created_at").
		Where("meal_date <= ?", today.Format("2006-01-02")).
		Order("meal_date ASC, created_at ASC, id ASC").
		Find(&records).Error; err != nil {
		p.loadErr = err
		return p
	}
	p.TotalRecords = len(records)

	tasteAcc := map[string]float64{}
	tasteCnt := map[string]int{}
	catAcc := map[string]float64{}
	catCnt := map[string]int{}
	ingAcc := map[string]float64{}
	ingCnt := map[string]int{}
	windowDistinct := map[uint]bool{}
	var weightSum, spicyAcc float64
	var cookSum, cookN int

	for _, r := range records {
		p.eatenCount[r.DishID]++
		if r.MealDate > p.LastEaten[r.DishID] {
			p.LastEaten[r.DishID] = r.MealDate
		}
		date, dateErr := time.ParseInLocation("2006-01-02", r.MealDate, recommendationLocation)
		if dateErr != nil {
			continue
		}
		age := today.Sub(date).Hours() / 24
		if age < 0 {
			continue // An intention to eat is not evidence of eating or liking.
		}
		if age > float64(days) {
			continue
		}
		p.WindowRecords++
		windowDistinct[r.DishID] = true
		p.MealTypeCounts[r.MealType]++
		if r.Mood != "" {
			p.MoodCounts[r.Mood]++
		}

		value, strength := mealSatisfaction(r)
		decay := math.Pow(0.5, age/profileHalfLife)
		p.addDishEvidence(r.DishID, value, strength*decay)
		// Explicit poor ratings contribute negative evidence rather than a
		// smaller positive vote for the same taste/cuisine.
		w := (value - 50) / 50 * strength * decay
		if strength < 1 {
			w = 0.25 * decay
		}
		weightSum += math.Max(0, w)
		delete(p.likedSet, r.DishID)
		delete(p.dislikedSet, r.DishID)
		if value >= 75 && strength >= 1 {
			p.likedSet[r.DishID] = true
		} else if value <= 25 && strength >= 1 {
			p.dislikedSet[r.DishID] = true
		}

		d, found := dishByID[r.DishID]
		if !found {
			continue
		}
		for _, t := range tasteTokens(d.Taste) {
			if w < 0 {
				p.tasteNegative[t] += -w
			} else {
				tasteAcc[t] += w
			}
			tasteCnt[t]++
		}
		if c := strings.TrimSpace(d.Category); c != "" {
			if w < 0 {
				p.categoryNegative[c] += -w
			} else {
				catAcc[c] += w
			}
			catCnt[c]++
		}
		if d.Difficulty != "" {
			p.DifficultyCounts[d.Difficulty]++
		}
		if d.CookTime > 0 {
			cookSum += d.CookTime
			cookN++
		}
		if isSpicyDish(d) {
			spicyAcc += math.Max(0, w)
		}
		for _, name := range ingredientNames(d.Ingredients) {
			name = canonicalIngredient(name)
			if w < 0 {
				p.ingredientNegative[name] += -w
			} else {
				ingAcc[name] += w
			}
			ingCnt[name]++
		}
		if age <= 7 {
			f := dishFeatures(d)
			if f.Main != "" {
				p.recentMainCounts[f.Main]++
			}
		}
	}

	// 显式偏好的口味作为先验加进画像（相当于 2 餐的权重），新用户也能有个性化推荐
	for _, t := range p.Preferences.FavoriteTastes {
		tasteAcc[t] += 2
	}

	p.DistinctDishes = len(windowDistinct)
	if cookN > 0 {
		p.AvgCookTime = math.Round(float64(cookSum)/float64(cookN)*10) / 10
	}
	if weightSum > 0 {
		p.SpicyRatio = math.Round(spicyAcc/weightSum*100) / 100
	}
	p.TasteWeights, p.tasteWeightMap = normalizeWeights(tasteAcc, tasteCnt, 10)
	p.CategoryWeights, p.categoryWeightMap = normalizeWeights(catAcc, catCnt, 8)
	p.TopIngredients, p.ingredientWeightMap = normalizeWeights(ingAcc, ingCnt, 12)

	for id, cnt := range p.eatenCount {
		name := ""
		if d, ok := dishByID[id]; ok {
			name = d.Name
		}
		p.TopDishes = append(p.TopDishes, DishFrequency{DishID: id, DishName: name, Count: cnt, LastDate: p.LastEaten[id]})
	}
	sortFrequencies(p.TopDishes)
	if len(p.TopDishes) > 10 {
		p.TopDishes = p.TopDishes[:10]
	}

	for id := range recentDishIDMap(uid, repeatDays, requestDB) {
		p.RecentDishIDs = append(p.RecentDishIDs, id)
	}
	sort.Slice(p.RecentDishIDs, func(i, j int) bool { return p.RecentDishIDs[i] < p.RecentDishIDs[j] })

	var favorites []models.Favorite
	if err := requestDB.Scopes(database.OwnedBy(uid)).Order("created_at DESC").Find(&favorites).Error; err != nil {
		p.loadErr = err
		return p
	}
	for _, f := range favorites {
		p.favoriteSet[f.DishID] = true
		if d, ok := dishByID[f.DishID]; ok && !d.DeletedAt.Valid {
			p.FavoriteDishes = append(p.FavoriteDishes, briefOf(d))
		}
	}

	since := today.AddDate(0, 0, -days)
	var ratings []models.DayRating
	if err := requestDB.Scopes(database.OwnedBy(uid)).Select("home_mood", "mood", "meal_date").
		Where("meal_date >= ?", since.Format("2006-01-02")).
		Find(&ratings).Error; err != nil {
		p.loadErr = err
		return p
	}
	for _, r := range ratings {
		if r.HomeMood != "" {
			p.HomeMoodCounts[r.HomeMood]++
		}
	}

	var events []models.BehaviorEvent
	if err := requestDB.Scopes(database.OwnedBy(uid)).Select("id", "event_type", "dish_id", "meta", "created_at").Where("created_at >= ?", since).Order("created_at ASC, id ASC").Find(&events).Error; err != nil {
		p.loadErr = err
		return p
	}
	rejectSince := now.AddDate(0, 0, -1)
	dislikeEvents := map[uint]models.BehaviorEvent{}
	for _, e := range events {
		p.BehaviorCounts[e.EventType]++
		if e.DishID == 0 {
			continue
		}
		switch e.EventType {
		case "view":
			p.viewCount[e.DishID]++
		case "reject":
			// Legacy reject also meant “show me another”; only weak, one-day fatigue.
			if e.CreatedAt.After(rejectSince) {
				p.rejectCount[e.DishID]++
			}
		case "not_today":
			if e.CreatedAt.In(recommendationLocation).Format("2006-01-02") == today.Format("2006-01-02") {
				p.notToday[e.DishID] = true
			}
		case "dislike":
			dislikeEvents[e.DishID] = e
		case "undo_dislike":
			delete(dislikeEvents, e.DishID)
		}
	}
	for dishID, e := range dislikeEvents {
		age := today.Sub(recommendationDay(e.CreatedAt)).Hours() / 24
		p.addDishEvidence(dishID, 0, 4*math.Pow(0.5, math.Max(0, age)/profileHalfLife))
		p.dislikedSet[dishID] = true
		delete(p.likedSet, dishID)
	}
	// A served batch is not an impression; only attributed, visible cards form
	// the denominator. Reliable feedback is idempotent in the feedback service.
	impressions, accepts := map[string]bool{}, map[string]bool{}
	for _, e := range events {
		var meta struct {
			RecommendationID string `json:"recommendation_id"`
		}
		_ = json.Unmarshal([]byte(e.Meta), &meta)
		if meta.RecommendationID == "" || e.DishID == 0 {
			continue
		}
		key := fmt.Sprintf("%s:%d", meta.RecommendationID, e.DishID)
		if e.EventType == "impression" {
			impressions[key] = true
		}
		if e.EventType == "accept" {
			accepts[key] = true
		}
	}
	if len(impressions) > 0 {
		n := 0
		for key := range accepts {
			if impressions[key] {
				n++
			}
		}
		p.AcceptRate = math.Round(float64(n)/float64(len(impressions))*100) / 100
	}

	p.LikedDishes = []DishBrief{}
	p.DislikedDishes = []DishBrief{}
	for id := range p.likedSet {
		if d, ok := dishByID[id]; ok && !d.DeletedAt.Valid {
			p.LikedDishes = append(p.LikedDishes, briefOf(d))
		}
	}
	for id := range p.dislikedSet {
		if d, ok := dishByID[id]; ok && !d.DeletedAt.Valid {
			p.DislikedDishes = append(p.DislikedDishes, briefOf(d))
		}
	}
	sortBriefs(p.LikedDishes)
	sortBriefs(p.DislikedDishes)
	p.MostViewedDishes = frequencyList(p.viewCount, dishByID, p.LastEaten, 8)
	p.RejectedDishes = frequencyList(p.rejectCount, dishByID, p.LastEaten, 8)

	p.Summary = buildProfileSummary(p)
	return p
}

func frequencyList(counts map[uint]int, dishByID map[uint]models.Dish, lastEaten map[uint]string, top int) []DishFrequency {
	out := make([]DishFrequency, 0, len(counts))
	for id, cnt := range counts {
		name := ""
		if d, ok := dishByID[id]; ok {
			name = d.Name
		}
		out = append(out, DishFrequency{DishID: id, DishName: name, Count: cnt, LastDate: lastEaten[id]})
	}
	sortFrequencies(out)
	if len(out) > top {
		out = out[:top]
	}
	return out
}

// ingredientNames 从 JSON 配料数组里取出名称（兼容对象数组与字符串数组）。
func ingredientNames(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var objects []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(raw), &objects); err == nil {
		names := make([]string, 0, len(objects))
		for _, o := range objects {
			if n := strings.TrimSpace(o.Name); n != "" {
				names = append(names, n)
			}
		}
		return names
	}
	var strs []string
	if err := json.Unmarshal([]byte(raw), &strs); err == nil {
		names := make([]string, 0, len(strs))
		for _, s := range strs {
			if n := strings.TrimSpace(s); n != "" {
				names = append(names, n)
			}
		}
		return names
	}
	return nil
}

func normalizeWeights(acc map[string]float64, cnt map[string]int, top int) ([]WeightItem, map[string]float64) {
	total := 0.0
	for _, v := range acc {
		total += v
	}
	weights := make(map[string]float64, len(acc))
	items := make([]WeightItem, 0, len(acc))
	for name, v := range acc {
		w := 0.0
		if total > 0 {
			w = math.Round(v/total*1000) / 1000
		}
		weights[name] = w
		items = append(items, WeightItem{Name: name, Weight: w, Count: cnt[name]})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Weight != items[j].Weight {
			return items[i].Weight > items[j].Weight
		}
		return items[i].Name < items[j].Name
	})
	if len(items) > top {
		items = items[:top]
	}
	return items, weights
}

func briefOf(d models.Dish) DishBrief {
	return DishBrief{ID: d.ID, Name: d.Name, Category: d.Category, Taste: d.Taste, CookTime: d.CookTime, Difficulty: d.Difficulty}
}

func sortBriefs(list []DishBrief) {
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
}

func sortFrequencies(list []DishFrequency) {
	sort.Slice(list, func(i, j int) bool {
		if list[i].Count != list[j].Count {
			return list[i].Count > list[j].Count
		}
		if list[i].LastDate != list[j].LastDate {
			return list[i].LastDate > list[j].LastDate
		}
		return list[i].DishID < list[j].DishID
	})
}

func topNames(items []WeightItem, n int) []string {
	names := make([]string, 0, n)
	for i, it := range items {
		if i >= n {
			break
		}
		names = append(names, it.Name)
	}
	return names
}

func buildProfileSummary(p *TasteProfile) string {
	prefs := preferenceSummary(p.Preferences)
	if p.WindowRecords == 0 {
		s := fmt.Sprintf("近 %d 天没有用餐记录，暂无行为画像；收藏 %d 道。", p.WindowDays, len(p.FavoriteDishes))
		if prefs != "" {
			s += "已声明偏好：" + prefs + "。"
		}
		return s
	}
	parts := []string{fmt.Sprintf("近 %d 天记录了 %d 餐、%d 道不同的菜", p.WindowDays, p.WindowRecords, p.DistinctDishes)}
	if names := topNames(p.TasteWeights, 3); len(names) > 0 {
		parts = append(parts, "偏好口味："+strings.Join(names, "、"))
	}
	if names := topNames(p.CategoryWeights, 2); len(names) > 0 {
		parts = append(parts, "常吃菜系："+strings.Join(names, "、"))
	}
	parts = append(parts, fmt.Sprintf("辣味占比 %d%%", int(math.Round(p.SpicyRatio*100))))
	if p.AvgCookTime > 0 {
		parts = append(parts, fmt.Sprintf("平均烹饪 %d 分钟", int(math.Round(p.AvgCookTime))))
	}
	if len(p.FavoriteDishes) > 0 {
		parts = append(parts, fmt.Sprintf("收藏 %d 道", len(p.FavoriteDishes)))
	}
	if len(p.RecentDishIDs) > 0 {
		parts = append(parts, fmt.Sprintf("近 %d 天已吃过 %d 道（推荐时避开）", p.RepeatDays, len(p.RecentDishIDs)))
	}
	if prefs != "" {
		parts = append(parts, "已声明偏好："+prefs)
	}
	return strings.Join(parts, "；") + "。"
}

func preferenceSummary(pr Preferences) string {
	var parts []string
	if label := SpiceLevelLabel(pr.SpiceLevel); label != "" {
		parts = append(parts, label)
	}
	if len(pr.Allergies) > 0 {
		parts = append(parts, "过敏 "+strings.Join(pr.Allergies, "/"))
	}
	if len(pr.AvoidIngredients) > 0 {
		parts = append(parts, "忌口 "+strings.Join(pr.AvoidIngredients, "/"))
	}
	if pr.MaxCookTime > 0 {
		parts = append(parts, fmt.Sprintf("做饭不超过 %d 分钟", pr.MaxCookTime))
	}
	if pr.HouseholdSize > 0 {
		parts = append(parts, fmt.Sprintf("%d 人吃饭", pr.HouseholdSize))
	}
	if pr.Goals != "" {
		parts = append(parts, "目标 "+pr.Goals)
	}
	return strings.Join(parts, "，")
}
