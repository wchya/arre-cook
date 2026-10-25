package services

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"ninimenu/internal/models"
)

const RecommendationAlgorithmVersion = "personal-meal-v2"

var recommendationLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

func recommendationNow() time.Time { return time.Now().In(recommendationLocation) }
func recommendationDay(t time.Time) time.Time {
	t = t.In(recommendationLocation)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, recommendationLocation)
}
func boundedScore(n float64) float64 { return math.Max(0, math.Min(100, n)) }

type affinityEvidence struct{ Sum, Weight float64 }

func (p *TasteProfile) addDishEvidence(id uint, value, weight float64) {
	if p.dishEvidence[id] == nil {
		p.dishEvidence[id] = &affinityEvidence{}
	}
	p.dishEvidence[id].Sum += value * weight
	p.dishEvidence[id].Weight += weight
}
func mealSatisfaction(r models.MealRecord) (float64, float64) {
	// An explicit numeric rating wins if older clients send both channels.
	if r.Rating >= 1 && r.Rating <= 5 {
		return float64(r.Rating-1) * 25, 3
	}
	switch r.Mood {
	case "yum", "great":
		return 90, 3
	case "no", "meh":
		return 10, 3
	case "ok":
		return 50, 3
	}
	return 60, 0.35
}
func affinityForDish(d models.Dish, p *TasteProfile) (float64, float64, float64) {
	prior := 50.0
	best := 0.0
	penalty := 0.0
	for _, t := range tasteTokens(d.Taste) {
		best = math.Max(best, p.tasteWeightMap[t])
		n := p.tasteNegative[t]
		penalty = math.Max(penalty, 15*n/(n+3))
	}
	prior += 20*best + 10*p.categoryWeightMap[d.Category] - penalty
	n := p.categoryNegative[d.Category]
	prior -= 10 * n / (n + 3)
	if p.favoriteSet[d.ID] {
		prior += 15
	}
	if d.OwnerID != 0 || d.FamilyID != 0 {
		prior += 3
	}
	ingredientBest, ingredientPenalty := 0.0, 0.0
	for _, name := range ingredientNames(d.Ingredients) {
		key := canonicalIngredient(name)
		ingredientBest = math.Max(ingredientBest, p.ingredientWeightMap[key])
		n := p.ingredientNegative[key]
		ingredientPenalty = math.Max(ingredientPenalty, 5*n/(n+3))
	}
	prior += 5*ingredientBest - ingredientPenalty
	prior = boundedScore(prior)
	e := p.dishEvidence[d.ID]
	if e == nil {
		return prior, 0, 0
	}
	const priorSamples = 5.0
	return boundedScore((priorSamples*prior + e.Sum) / (priorSamples + e.Weight)), e.Weight / (priorSamples + e.Weight), e.Weight
}

type recommendationFeatures struct {
	Role, Main, Method string
	Proteins           []string
	Spice              int
	Ingredients        []string
}

// Canonical names cover common produce aliases and ingredient forms. Filtering
// still inspects the original text as well, including compound seasonings.
var ingredientAliasGroups = [][]string{
	{"番茄", "西红柿"}, {"土豆", "马铃薯"}, {"香菜", "芫荽"}, {"西兰花", "绿花菜", "青花菜"},
	{"猪肉", "五花肉", "猪肉末", "猪肉馅", "猪里脊", "排骨", "腊肉", "培根", "火腿"},
	{"牛肉", "牛腩", "牛腱", "牛里脊", "肥牛", "牛排"}, {"鸡肉", "鸡腿", "鸡翅", "鸡胸", "鸡胗", "鸡爪"},
	{"羊肉", "羊排", "肥羊"}, {"鸭肉", "鸭腿", "鸭胸", "鸭胗"}, {"虾", "虾仁", "虾米", "虾皮", "海米"},
	{"鱼", "鱼片", "鱼头", "鲈鱼", "鲫鱼", "草鱼", "鳜鱼", "鲤鱼", "带鱼", "鳝鱼", "鳗鱼", "三文鱼", "金枪鱼"},
	{"鸡蛋", "鸭蛋", "鹌鹑蛋", "蛋液", "蛋黄", "蛋清"}, {"大豆", "黄豆", "豆腐", "豆干", "腐竹", "豆皮", "酱油", "生抽", "老抽", "豆瓣酱", "豆豉"},
	{"花生", "花生油", "花生酱"}, {"芝麻", "香油", "麻油", "芝麻油", "芝麻酱"}, {"牛奶", "奶酪", "乳酪", "黄油", "奶油", "酸奶", "乳粉"},
	{"辣椒", "青椒", "红椒", "小米辣", "干辣椒", "辣椒面", "辣椒粉", "剁椒", "泡椒"},
	{"蟹", "螃蟹", "梭子蟹", "大闸蟹", "青蟹", "蟹肉", "蟹黄"},
	{"贝类", "蛤蜊", "花蛤", "花甲", "生蚝", "牡蛎", "扇贝", "干贝", "蛏子", "贻贝", "青口", "蚬子", "鲍鱼", "蚌"},
	{"坚果", "核桃", "腰果", "杏仁", "巴旦木", "榛子", "开心果", "碧根果", "夏威夷果", "松子", "栗子"},
	{"内脏", "猪肝", "猪心", "猪肺", "猪腰", "鸡肝", "鸡心", "鸡胗", "鸭肝", "鸭心", "鸭胗", "牛肚", "猪肚", "毛肚", "肥肠", "大肠", "小肠"},
	{"肥肉", "五花肉", "肥膘", "猪油"},
}

// canonicalIngredient only merges true naming aliases. A pantry bottle of
// soy sauce is not interchangeable with tofu or soybeans.
func canonicalIngredient(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	for _, g := range ingredientAliasGroups[:4] {
		for _, alias := range g {
			if raw == alias {
				return g[0]
			}
		}
	}
	return raw
}

// Food groups are intentionally broader and only used for recipe features.
func ingredientFoodGroup(raw string) string {
	raw = canonicalIngredient(raw)
	best, group := 0, raw
	for _, g := range ingredientAliasGroups {
		for _, alias := range g {
			if strings.Contains(raw, alias) && len(alias) > best {
				best, group = len(alias), g[0]
			}
		}
	}
	return group
}
func matchesRequiredIngredientTerm(text, term string) bool {
	term = strings.ToLower(strings.TrimSpace(term))
	text = strings.ToLower(text)
	if term == "" {
		return false
	}
	if strings.Contains(text, term) {
		return true
	}
	for _, g := range ingredientAliasGroups[:4] {
		for _, alias := range g {
			if term == alias {
				for _, other := range g {
					if strings.Contains(text, other) {
						return true
					}
				}
			}
		}
	}
	return false
}

func expandedIngredientText(raw string) string {
	out := strings.ToLower(raw)
	for _, g := range ingredientAliasGroups {
		for _, alias := range g {
			if strings.Contains(out, alias) {
				out += " " + g[0]
				break
			}
		}
	}
	return out
}
func matchesIngredientTerm(text, term string) bool {
	term = strings.ToLower(strings.TrimSpace(term))
	if term == "" {
		return false
	}
	if strings.Contains(strings.ToLower(text), term) {
		return true
	}
	// Only a canonical group term expands to all members. Excluding duck eggs,
	// for example, must not silently exclude every chicken egg recipe.
	for _, g := range ingredientAliasGroups {
		if term == g[0] {
			return strings.Contains(expandedIngredientText(text), term)
		}
	}
	// True synonyms (rather than broad food groups) match in both directions.
	for _, g := range ingredientAliasGroups[:4] {
		for _, alias := range g {
			if term == alias {
				for _, other := range g {
					if strings.Contains(strings.ToLower(text), other) {
						return true
					}
				}
			}
		}
	}
	return false
}
func DishViolatesDietaryExclusions(d models.Dish, terms []string) bool {
	return containsAny(dishSearchText(d), terms)
}
func dishFeatures(d models.Dish) recommendationFeatures {
	f := recommendationFeatures{Spice: -1}
	f.Ingredients = ingredientNames(d.Ingredients)
	text := strings.Join(append(append([]string{}, f.Ingredients...), ingredientNames(d.Seasonings)...), " ")
	seen := map[string]bool{}
	for _, name := range f.Ingredients {
		c := ingredientFoodGroup(name)
		if f.Main == "" {
			f.Main = c
		}
		for _, protein := range []string{"猪肉", "牛肉", "鸡肉", "羊肉", "鸭肉", "虾", "鱼", "蟹", "贝类", "鸡蛋", "大豆"} {
			if c == protein && !seen[c] {
				f.Proteins = append(f.Proteins, c)
				seen[c] = true
			}
		}
	}
	// The meat check precedes any legacy "素菜" tag.
	meat := false
	for _, name := range f.Ingredients {
		if strings.Contains(name, "肉末") || strings.Contains(name, "肉馅") || strings.Contains(name, "肉丝") {
			meat = true
		}
	}
	for _, protein := range f.Proteins {
		if protein != "鸡蛋" && protein != "大豆" {
			meat = true
		}
	}
	if strings.Contains(d.Name, "汤") || hasAnyTag(d, "汤品", "汤菜") {
		f.Role = "soup"
	} else if strings.Contains(d.Name, "饭") || strings.Contains(d.Name, "面") || hasAnyTag(d, "主食") {
		f.Role = "staple"
	} else if meat {
		f.Role = "meat"
	} else if len(f.Proteins) > 0 {
		f.Role = "protein"
	} else {
		for _, name := range f.Ingredients {
			for _, veg := range []string{"番茄", "西红柿", "土豆", "马铃薯", "黄瓜", "茄子", "白菜", "包菜", "青菜", "菠菜", "生菜", "菜心", "芹菜", "芦笋", "竹笋", "莴笋", "韭菜", "西兰花", "花菜", "豆角", "豇豆", "四季豆", "秋葵", "丝瓜", "苦瓜", "冬瓜", "南瓜", "西葫芦", "萝卜", "菌", "菇", "木耳", "藕", "山药", "芋头"} {
				if strings.Contains(name, veg) {
					f.Role = "vegetable"
					break
				}
			}
			if f.Role != "" {
				break
			}
		}
	}
	search := d.Name + " " + strings.Join(parseTags(d.Tags), " ")
	for _, method := range []string{"凉拌", "干锅", "红烧", "清蒸", "煎", "炖", "焖", "蒸", "炒", "煮", "烤", "炸"} {
		if strings.Contains(search, method) {
			f.Method = method
			break
		}
	}
	if strings.Contains(d.Taste, "特辣") || strings.Contains(d.Taste, "重辣") {
		f.Spice = 3
	} else if strings.Contains(d.Taste, "微辣") {
		f.Spice = 1
	} else if isSpicyDish(d) || matchesIngredientTerm(text, "辣椒") {
		f.Spice = 2
	} else if d.Taste != "" || len(f.Ingredients) > 0 {
		f.Spice = 0
	}
	return f
}

type recommendationScoreParts struct {
	Personal    float64 `json:"personal"`
	Context     float64 `json:"context"`
	Freshness   float64 `json:"freshness"`
	Convenience float64 `json:"convenience"`
	Pairing     float64 `json:"pairing"`
	Quality     float64 `json:"quality"`
	Confidence  float64 `json:"confidence"`
}
type scoreReason struct {
	points float64
	text   string
}

func scoreDishV2(d models.Dish, req RecommendRequest, p *TasteProfile, date time.Time, selected []models.Dish, inventory map[string]bool, quality float64, exposures map[uint]int) (float64, []string, recommendationScoreParts) {
	f := dishFeatures(d)
	personal, confidence, _ := affinityForDish(d, p)
	reasons := []scoreReason{}
	reason := func(points float64, text string) {
		if points > 0 {
			reasons = append(reasons, scoreReason{points, text})
		}
	}
	if p.favoriteSet[d.ID] {
		reason(5.25, "你收藏过的菜")
	}
	if evidence := p.dishEvidence[d.ID]; personal >= 65 && evidence != nil && evidence.Weight >= 1 && evidence.Sum/evidence.Weight >= 75 && p.likedSet[d.ID] {
		reason((personal-50)*.35, "之前吃过，评价不错")
	}
	best := 0.0
	bestTaste := ""
	for _, t := range tasteTokens(d.Taste) {
		if p.tasteWeightMap[t] > best {
			best = p.tasteWeightMap[t]
			bestTaste = t
		}
	}
	if best >= .12 {
		reason(7*best, "符合你常吃的「"+bestTaste+"」口味")
	}
	context := 50.0
	if req.MealType != "" && d.MealType == req.MealType {
		context += 10
	}
	if len(req.Tastes) > 0 && tasteMatchesAny(d, req.Tastes) {
		context += 25
		reason(5, "符合这次想吃的口味")
	}
	switch req.Mood {
	case "tired", "lazy":
		if d.Difficulty == "easy" {
			context += 20
			reason(4, "做法简单，适合轻松做一餐")
		}
		if d.CookTime > 0 && d.CookTime <= 25 {
			context += 20
		}
		if d.Difficulty == "hard" {
			context -= 25
		}
	case "spicy":
		if f.Spice > 0 {
			context += 35
			reason(7, "符合这次想吃辣的心情")
		} else {
			context -= 25
		}
	case "healthy":
		if containsTaste(d.Taste, "清淡") {
			context += 25
			reason(5, "符合这次想吃清淡的口味")
		}
		if f.Spice >= 2 {
			context -= 15
		}
	case "happy":
		if hasAnyTag(d, "硬菜", "宴客", "聚餐") {
			context += 20
			reason(4, "适合这次想吃丰盛一点")
		}
	}
	if !req.IgnorePreferences && p.Preferences.SpiceLevel >= 0 && f.Spice >= 0 {
		context += 15 - 12*math.Abs(float64(f.Spice-p.Preferences.SpiceLevel))
	}
	freshness := 85.0
	if last, ok := p.LastEaten[d.ID]; ok {
		if t, err := time.ParseInLocation("2006-01-02", last, recommendationLocation); err == nil {
			age := date.Sub(t).Hours() / 24
			freshness = boundedScore(age / 21 * 100)
			if age >= 14 {
				reason(3, "有一阵子没吃了，换换口味")
			}
		}
	} else if p.WindowRecords > 0 {
		freshness = 95
		reason(2.5, "还没尝过，可以试试新菜")
	}
	freshness -= math.Min(float64(exposures[d.ID])*4, 24)
	freshness -= math.Min(float64(p.recentMainCounts[f.Main])*5, 15)
	freshness -= math.Min(float64(p.rejectCount[d.ID])*2, 6)
	if p.notToday[d.ID] && req.MealDate == recommendationNow().Format("2006-01-02") {
		freshness = 0
	}
	convenience := 50.0
	if d.CookTime > 0 {
		switch {
		case d.CookTime <= 25:
			convenience = 85
			reason(4, fmt.Sprintf("%d 分钟能做好", d.CookTime))
		case d.CookTime <= 45:
			convenience = 70
		case d.CookTime <= 90:
			convenience = 45
		default:
			convenience = 25
		}
	}
	if d.Difficulty == "easy" {
		convenience += 10
	} else if d.Difficulty == "hard" {
		convenience -= 15
	}
	if !req.IgnorePreferences && p.Preferences.MaxCookTime > 0 && d.CookTime > p.Preferences.MaxCookTime {
		convenience -= math.Min(30, float64(d.CookTime-p.Preferences.MaxCookTime))
	}
	available := 0
	for _, name := range f.Ingredients {
		if inventory[canonicalIngredient(name)] {
			available++
		}
	}
	if len(f.Ingredients) > 0 && available > 0 {
		convenience += 10 * float64(available) / float64(len(f.Ingredients))
		reason(2, "有你标记为家中常备的食材")
	}
	pairing := 50.0
	if req.Diversity == nil || *req.Diversity {
		pairing = pairingScore(d, selected)
	}
	if len(selected) > 0 && pairing >= 70 {
		text := "换一种主食材或做法"
		if req.Mode == "week_plan" || strings.HasPrefix(req.Mode, "tomorrow") {
			text = "为这一餐搭配不同的主食材或做法"
		}
		confirmed := make([]models.Dish, 0, len(selected))
		explicit := make(map[uint]bool, len(req.SelectedDishIDs))
		for _, id := range req.SelectedDishIDs {
			explicit[id] = true
		}
		for _, other := range selected {
			if req.confirmedDishIDs[other.ID] || explicit[other.ID] {
				confirmed = append(confirmed, other)
			}
		}
		if len(confirmed) > 0 && pairingScore(d, confirmed) >= 70 {
			text = "和已经选好的菜搭配更丰富"
		}
		reason((pairing-50)*.1, text)
	}
	parts := recommendationScoreParts{boundedScore(personal), boundedScore(context), boundedScore(freshness), boundedScore(convenience), boundedScore(pairing), boundedScore(quality), confidence}
	score := .35*parts.Personal + .20*parts.Context + .15*parts.Freshness + .15*parts.Convenience + .10*parts.Pairing + .05*parts.Quality
	sort.SliceStable(reasons, func(i, j int) bool { return reasons[i].points > reasons[j].points })
	explanation := []string{}
	seenReasons := map[string]bool{}
	for _, r := range reasons {
		if !seenReasons[r.text] {
			explanation = append(explanation, r.text)
			seenReasons[r.text] = true
		}
		if len(explanation) == 2 {
			break
		}
	}
	if len(explanation) == 0 {
		explanation = append(explanation, "符合本餐条件，给菜单换一点花样")
	}
	return math.Round(boundedScore(score)*10) / 10, explanation, parts
}
func pairingScore(d models.Dish, selected []models.Dish) float64 {
	if len(selected) == 0 {
		return 50
	}
	f := dishFeatures(d)
	total := 0.0
	for _, other := range selected {
		o := dishFeatures(other)
		value := 60.0
		if f.Role != "" && o.Role != "" {
			if f.Role == o.Role {
				value -= 12
			} else {
				value += 20
			}
		}
		if f.Main != "" && f.Main == o.Main {
			value -= 20
		}
		for _, a := range f.Proteins {
			for _, b := range o.Proteins {
				if a == b {
					value -= 20
				}
			}
		}
		if f.Method != "" && f.Method == o.Method {
			value -= 10
		}
		if f.Spice >= 2 && o.Spice >= 2 {
			value -= 10
		}
		if d.Category == other.Category {
			value -= 5
		}
		total += boundedScore(value)
	}
	return total / float64(len(selected))
}
