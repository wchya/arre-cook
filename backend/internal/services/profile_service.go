package services

import (
	"encoding/json"
	"fmt"
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

	tasteWeightMap    map[string]float64
	categoryWeightMap map[string]float64
	favoriteSet       map[uint]bool
	likedSet          map[uint]bool
	dislikedSet       map[uint]bool
	eatenCount        map[uint]int
	viewCount         map[uint]int
	rejectCount       map[uint]int
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
func BuildTasteProfile(uid uint, days int) *TasteProfile {
	days = normalizeProfileDays(days)
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	repeatDays := UserRepeatDays(uid)

	p := &TasteProfile{
		GeneratedAt:       now.Format(time.RFC3339),
		WindowDays:        days,
		RepeatDays:        repeatDays,
		MealTypeCounts:    map[string]int{},
		TasteWeights:      []WeightItem{},
		CategoryWeights:   []WeightItem{},
		DifficultyCounts:  map[string]int{},
		TopIngredients:    []WeightItem{},
		TopDishes:         []DishFrequency{},
		RecentDishIDs:     []uint{},
		LastEaten:         map[uint]string{},
		FavoriteDishes:    []DishBrief{},
		LikedDishes:       []DishBrief{},
		DislikedDishes:    []DishBrief{},
		MoodCounts:        map[string]int{},
		HomeMoodCounts:    map[string]int{},
		BehaviorCounts:    map[string]int{},
		RejectedDishes:    []DishFrequency{},
		MostViewedDishes:  []DishFrequency{},
		Preferences:       GetPreferences(uid),
		tasteWeightMap:    map[string]float64{},
		categoryWeightMap: map[string]float64{},
		favoriteSet:       map[uint]bool{},
		likedSet:          map[uint]bool{},
		dislikedSet:       map[uint]bool{},
		eatenCount:        map[uint]int{},
		viewCount:         map[uint]int{},
		rejectCount:       map[uint]int{},
	}

	var allDishes []models.Dish
	database.DB.Unscoped().Scopes(database.VisibleDishes(uid)).
		Select("id", "name", "category", "taste", "ingredients", "seasonings", "cook_time", "difficulty", "owner_id", "deleted_at").
		Find(&allDishes)
	dishByID := make(map[uint]models.Dish, len(allDishes))
	for _, d := range allDishes {
		dishByID[d.ID] = d
	}

	var records []models.MealRecord
	database.DB.Scopes(database.OwnedBy(uid)).
		Select("id", "dish_id", "dish_name", "meal_type", "meal_date", "rating", "mood", "created_at").
		Order("meal_date ASC, created_at ASC").
		Find(&records)
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
		date, ok := parseMealDate(r.MealDate)
		if !ok {
			continue
		}
		age := today.Sub(date).Hours() / 24
		if age < 0 {
			// 明日菜单等未来日期的记录，按“今天”对待
			age = 0
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

		w := 1.0
		switch r.Mood {
		case "yum", "great":
			w += 0.8
		case "no", "meh":
			w -= 0.7
		}
		if r.Rating > 0 {
			w += float64(r.Rating-3) * 0.25
		}
		if w < 0.15 {
			w = 0.15
		}
		w *= math.Pow(0.5, age/profileHalfLife)
		weightSum += w

		// 喜好判定以最新一次记录为准
		if r.Mood == "yum" || r.Mood == "great" || r.Rating >= 4 {
			p.likedSet[r.DishID] = true
			delete(p.dislikedSet, r.DishID)
		}
		if r.Mood == "no" || r.Mood == "meh" || (r.Rating > 0 && r.Rating <= 2) {
			p.dislikedSet[r.DishID] = true
			delete(p.likedSet, r.DishID)
		}

		d, found := dishByID[r.DishID]
		if !found {
			continue
		}
		for _, t := range tasteTokens(d.Taste) {
			tasteAcc[t] += w
			tasteCnt[t]++
		}
		if c := strings.TrimSpace(d.Category); c != "" {
			catAcc[c] += w
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
			spicyAcc += w
		}
		for _, name := range ingredientNames(d.Ingredients) {
			ingAcc[name] += w
			ingCnt[name]++
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
	p.TopIngredients, _ = normalizeWeights(ingAcc, ingCnt, 12)

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

	for id := range recentDishIDMap(uid, repeatDays) {
		p.RecentDishIDs = append(p.RecentDishIDs, id)
	}
	sort.Slice(p.RecentDishIDs, func(i, j int) bool { return p.RecentDishIDs[i] < p.RecentDishIDs[j] })

	var favorites []models.Favorite
	database.DB.Scopes(database.OwnedBy(uid)).Order("created_at DESC").Find(&favorites)
	for _, f := range favorites {
		p.favoriteSet[f.DishID] = true
		if d, ok := dishByID[f.DishID]; ok && !d.DeletedAt.Valid {
			p.FavoriteDishes = append(p.FavoriteDishes, briefOf(d))
		}
	}
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

	since := today.AddDate(0, 0, -days)
	var ratings []models.DayRating
	database.DB.Scopes(database.OwnedBy(uid)).Select("home_mood", "mood", "meal_date").
		Where("meal_date >= ?", since.Format("2006-01-02")).
		Find(&ratings)
	for _, r := range ratings {
		if r.HomeMood != "" {
			p.HomeMoodCounts[r.HomeMood]++
		}
	}

	var events []models.BehaviorEvent
	database.DB.Scopes(database.OwnedBy(uid)).Select("event_type", "dish_id", "created_at").Where("created_at >= ?", since).Find(&events)
	rejectSince := now.AddDate(0, 0, -30)
	for _, e := range events {
		p.BehaviorCounts[e.EventType]++
		if e.DishID == 0 {
			continue
		}
		switch e.EventType {
		case "view":
			p.viewCount[e.DishID]++
		case "reject":
			// 近 30 天的“不想吃”才影响推荐，避免一次拒绝永久打入冷宫
			if e.CreatedAt.After(rejectSince) {
				p.rejectCount[e.DishID]++
			}
		}
	}
	if rec := p.BehaviorCounts["recommend"]; rec > 0 {
		p.AcceptRate = math.Round(float64(p.BehaviorCounts["accept"])/float64(rec)*100) / 100
		if p.AcceptRate > 1 {
			p.AcceptRate = 1
		}
	}
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
