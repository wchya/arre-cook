package services

import (
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"ninimenu/internal/models"
)

func historyTestRecipe(id uint, name, ingredients string) models.Dish {
	return models.Dish{ID: id, Name: name, Ingredients: ingredients, Taste: "清淡", Category: "家常菜", Tags: "[]"}
}

func historyTestProfile() *TasteProfile {
	return &TasteProfile{dishEvidence: map[uint]*affinityEvidence{}, favoriteSet: map[uint]bool{}, dislikedSet: map[uint]bool{}}
}

func TestRecommendationHistoryContentSimilarity(t *testing.T) {
	dishes := []models.Dish{
		historyTestRecipe(1, "番茄炖牛腩", `["番茄","牛腩"]`),
		historyTestRecipe(2, "西红柿炖牛腩", `["牛腩","西红柿","番茄","盐"]`),
		historyTestRecipe(3, "番茄炒牛腩", `["番茄","牛腩"]`),
		historyTestRecipe(4, "番茄炖鸡腿", `["番茄","鸡腿"]`),
		historyTestRecipe(5, "清蒸鲈鱼", `["鲈鱼"]`),
		historyTestRecipe(6, "无食材炖菜", `[]`),
	}
	m := newRecommendationHistory(dishes, nil)
	if got := m.similarity(1, 2); got < .999999 {
		t.Fatalf("duplicate ingredients/aliases/order changed identical recipe: %v", got)
	}
	method, ingredient := m.similarity(1, 3), m.similarity(1, 4)
	if method <= 0 || method >= .99 || ingredient <= 0 || ingredient >= method {
		t.Fatalf("meaningful recipe changes not distinguished: method=%v ingredient=%v", method, ingredient)
	}
	if m.similarity(1, 5) != 0 || m.similarity(1, 6) != 0 || m.similarity(99, 1) != 0 {
		t.Fatal("taste/category/method without ingredients created a personal match")
	}
	for i, j := 0, len(dishes)-1; i < j; i, j = i+1, j-1 {
		dishes[i], dishes[j] = dishes[j], dishes[i]
	}
	if m.fingerprint() != newRecommendationHistory(dishes, nil).fingerprint() {
		t.Fatal("catalog iteration order changed model fingerprint")
	}
}

func TestRecommendationHistoryMethodChangesSurviveLongIngredientLists(t *testing.T) {
	ingredients := `["番茄","土豆","胡萝卜","西兰花","香菇","鸡肉"]`
	dishes := []models.Dish{
		historyTestRecipe(1, "六鲜炒鸡肉", ingredients),
		historyTestRecipe(2, "六鲜炖鸡肉", ingredients),
		historyTestRecipe(3, "清蒸鲈鱼", `["鲈鱼"]`),
	}
	m := newRecommendationHistory(dishes, nil)
	similarity := m.similarity(1, 2)
	t.Logf("six-ingredient stir-fry/stew cosine: %.12f", similarity)
	if similarity <= 0 || similarity >= .92 {
		t.Fatalf("ingredient count drowned out a meaningful method change: %.12f", similarity)
	}
}

func TestRecommendationHistoryFeatureChannelsHaveStableWeight(t *testing.T) {
	short := historyTestRecipe(1, "炒鸡肉", `["鸡肉"]`)
	long := historyTestRecipe(2, "什锦炒菜", `["鸡肉","牛肉","虾仁","番茄","西红柿","土豆","香菇"]`)
	long.Taste = "清淡,鲜香,微辣"
	for _, dish := range []models.Dish{short, long} {
		vector := historyRecipeVector(dish, dishFeatures(dish))
		squares := map[string]float64{}
		for _, token := range vector.Tokens {
			channel, _, _ := strings.Cut(token.Name, ":")
			squares[channel] += token.Weight * token.Weight
		}
		for channel, want := range map[string]float64{"ingredient": 3, "protein": 1, "taste": 1, "method": 1.6} {
			if got := math.Sqrt(squares[channel]); math.Abs(got-want) > .000001 {
				t.Fatalf("%s: %s channel strength depends on recipe length: got=%v want=%v", dish.Name, channel, got, want)
			}
		}
	}
}

func TestRecommendationHistoryCondimentsDoNotCreateProteinSimilarity(t *testing.T) {
	dishes := []models.Dish{
		historyTestRecipe(1, "煮豆腐", `["豆腐"]`),
		historyTestRecipe(2, "煮酱汁", `["生抽","老抽","豆瓣酱","豆豉","盐"]`),
		historyTestRecipe(3, "煮豆干", `["豆干"]`),
		historyTestRecipe(4, "煮黄瓜", `["黄瓜","生抽"]`),
	}
	dishes[3].Seasonings = `["豆腐"]`
	m := newRecommendationHistory(dishes, nil)
	if m.similarity(1, 2) != 0 || m.similarity(1, 4) != 0 {
		t.Fatal("seasonings or soy sauce became tofu evidence")
	}
	if m.similarity(1, 3) <= 0 || m.similarity(1, 3) >= .5 {
		t.Fatalf("culinary protein group should be a weak bridge: %v", m.similarity(1, 3))
	}
}

func TestRecommendationHistorySignedSparseEvidence(t *testing.T) {
	dishes := []models.Dish{
		historyTestRecipe(1, "番茄炖牛肉", `["番茄","牛肉"]`),
		historyTestRecipe(2, "番茄炖牛肉新方", `["番茄","牛肉"]`),
		historyTestRecipe(3, "番茄炒牛肉", `["番茄","牛肉"]`),
	}
	p := historyTestProfile()
	neutral := newRecommendationHistory(dishes, p)
	if neutral.signals[2] != (recommendationHistorySignal{}) {
		t.Fatalf("cold start fabricated history: %+v", neutral.signals[2])
	}
	p.dishEvidence[1] = &affinityEvidence{Sum: 50 * 3, Weight: 3}
	if got := newRecommendationHistory(dishes, p).signals[2]; got.Adjustment != 0 || got.Reason != "" {
		t.Fatalf("neutral rating became preference: %+v", got)
	}
	p.dishEvidence[1] = &affinityEvidence{Sum: 60 * .35, Weight: .35}
	weak := newRecommendationHistory(dishes, p).signals[2]
	if weak.Positive <= 0 || weak.Positive >= .08 || weak.Reason != "" {
		t.Fatalf("unrated evidence should be weak and unexplained: %+v", weak)
	}
	p.dishEvidence[1] = &affinityEvidence{Sum: 100 * 3, Weight: 3}
	positiveModel := newRecommendationHistory(dishes, p)
	positive := positiveModel.signals[2]
	if positive.Adjustment <= weak.Adjustment || positive.Positive < .3 || positive.Discovery < .3 || positive.SourceID != 1 || !strings.Contains(positive.Reason, "番茄炖牛肉") || positive.Novelty > .000001 {
		t.Fatalf("positive similar history not represented: %+v", positive)
	}
	if positiveModel.signals[1].Adjustment != 0 {
		t.Fatal("source dish learned from itself")
	}
	if got := positiveModel.signals[3]; got.Novelty <= 0 || got.Positive <= 0 {
		t.Fatalf("familiar ingredient with new method has no discovery signal: %+v", got)
	}
	p.dishEvidence[1] = &affinityEvidence{Sum: 0, Weight: 3}
	negative := newRecommendationHistory(dishes, p).signals[2]
	if negative.Adjustment >= 0 || negative.Negative <= 0 || negative.Positive != 0 || negative.Reason != "" {
		t.Fatalf("negative history lost its sign: %+v", negative)
	}
}

func TestRecommendationHistoryFavoritesAndLatestDislike(t *testing.T) {
	dishes := []models.Dish{
		historyTestRecipe(1, "收藏番茄炒蛋", `["番茄","鸡蛋"]`),
		historyTestRecipe(2, "新番茄炒蛋", `["番茄","鸡蛋"]`),
	}
	p := historyTestProfile()
	p.favoriteSet[1] = true
	favorite := newRecommendationHistory(dishes, p).signals[2]
	if favorite.Positive <= .1 || favorite.Positive >= .3 || favorite.Discovery <= .1 || favorite.Adjustment >= 8 {
		t.Fatalf("favorite should provide a limited prior: %+v", favorite)
	}
	p.dishEvidence[1] = &affinityEvidence{Sum: 90 * 12, Weight: 12}
	p.dislikedSet[1] = true
	disliked := newRecommendationHistory(dishes, p).signals[2]
	if disliked.Positive != 0 || disliked.Discovery != 0 || disliked.Adjustment >= 0 || disliked.Negative <= 0 || disliked.Reason != "" {
		t.Fatalf("latest dislike did not veto favorite and older good ratings: %+v", disliked)
	}
	delete(p.dislikedSet, 1)
	p.dishEvidence[1] = &affinityEvidence{Sum: 25 * 3, Weight: 3}
	if got := newRecommendationHistory(dishes, p).signals[2]; got.Positive != 0 || got.Negative <= 0 {
		t.Fatalf("favorite canceled negative satisfaction: %+v", got)
	}
}

func TestRecommendationHistoryUnratedMealsNeverBecomeDiscoveryEvidence(t *testing.T) {
	dishes := []models.Dish{
		historyTestRecipe(1, "常吃番茄炖牛肉", `["番茄","牛肉"]`),
		historyTestRecipe(2, "常吃番茄炖鸡肉", `["番茄","鸡肉"]`),
		historyTestRecipe(3, "常吃番茄炖土豆", `["番茄","土豆"]`),
		historyTestRecipe(4, "常吃番茄炒蛋", `["番茄","鸡蛋"]`),
		historyTestRecipe(5, "番茄炒牛肉", `["番茄","牛肉"]`),
	}
	for _, sourceCount := range []int{1, 4} {
		p := historyTestProfile()
		for i := 1; i <= sourceCount; i++ {
			p.dishEvidence[uint(i)] = &affinityEvidence{Sum: 60 * .35 * 200, Weight: .35 * 200}
		}
		got := newRecommendationHistory(dishes, p).signals[5]
		if got.Positive < .08 || got.Discovery != 0 {
			t.Fatalf("%d repeated unrated sources should affect relevance but never discovery: %+v", sourceCount, got)
		}
		if strings.Contains(got.Reason, "喜欢") || strings.Contains(got.Reason, "评价") {
			t.Fatalf("weak evidence fabricated satisfaction: %s", got.Reason)
		}
		p.dishEvidence[1] = &affinityEvidence{Sum: 75 * 3, Weight: 3}
		if trusted := newRecommendationHistory(dishes, p).signals[5]; trusted.Discovery < .08 {
			t.Fatalf("explicit positive rating failed to enable discovery: %+v", trusted)
		}
		p.dislikedSet[1] = true
		if vetoed := newRecommendationHistory(dishes, p).signals[5]; vetoed.Discovery != 0 {
			t.Fatalf("latest dislike failed to veto discovery: %+v", vetoed)
		}
	}
}

func TestRecommendationHistoryFeaturesUseCurrentCatalogAndFallback(t *testing.T) {
	old := historyTestRecipe(1, "番茄炖牛肉", `["番茄","牛肉"]`)
	current := historyTestRecipe(1, "番茄炒鸡蛋", `["番茄","鸡蛋"]`)
	missing := historyTestRecipe(2, "清蒸鲈鱼", `["鲈鱼"]`)
	m := newRecommendationHistory([]models.Dish{old, current}, nil)
	if !reflect.DeepEqual(m.featuresFor(current), dishFeatures(current)) || m.featuresFor(current).Method != "炒" {
		t.Fatal("historical duplicate replaced current cached recipe features")
	}
	if !reflect.DeepEqual(m.featuresFor(missing), dishFeatures(missing)) {
		t.Fatal("missing feature cache did not fall back to recipe extraction")
	}
	var absent *recommendationHistoryModel
	if !reflect.DeepEqual(absent.featuresFor(missing), dishFeatures(missing)) {
		t.Fatal("nil feature cache did not fall back to recipe extraction")
	}
	if m.fingerprint() == "" || m.fingerprint() != m.buildFingerprint() {
		t.Fatal("cached model fingerprint differs from model content")
	}
}

func TestRecommendationHistoryKeepsOpposingNeighborsSeparate(t *testing.T) {
	dishes := []models.Dish{
		historyTestRecipe(1, "喜欢番茄炒蛋", `["番茄","鸡蛋"]`),
		historyTestRecipe(2, "不喜欢番茄炒蛋", `["番茄","鸡蛋"]`),
		historyTestRecipe(3, "未吃番茄炒蛋", `["番茄","鸡蛋"]`),
	}
	p := historyTestProfile()
	p.dishEvidence[1] = &affinityEvidence{Sum: 300, Weight: 3}
	p.dishEvidence[2] = &affinityEvidence{Sum: 0, Weight: 3}
	m := newRecommendationHistory(dishes, p)
	if got := m.signals[3]; got.Positive <= 0 || got.Negative <= 0 || math.Abs(got.Adjustment) > .000001 || got.Reason != "" {
		t.Fatalf("opposing sources were collapsed or falsely explained: %+v", got)
	}
	before := m.fingerprint()
	p.dishEvidence[1] = &affinityEvidence{Sum: 300 * .25, Weight: 3 * .25}
	if got := newRecommendationHistory(dishes, p); got.signals[3].Positive >= m.signals[3].Positive || got.fingerprint() == before {
		t.Fatal("decayed evidence failed to shrink or invalidate model")
	}
}

func TestRecommendationHistoryRecentSimilarityAndNilSafety(t *testing.T) {
	dishes := []models.Dish{
		historyTestRecipe(1, "番茄炒蛋", `["番茄","鸡蛋"]`),
		historyTestRecipe(2, "西红柿炒蛋", `["西红柿","鸡蛋"]`),
	}
	m := newRecommendationHistory(dishes, nil)
	date := time.Date(2026, 11, 17, 18, 0, 0, 0, recommendationLocation)
	if got := m.recentSimilarity(1, date, map[uint]string{1: "2026-11-17"}); got != 0 {
		t.Fatalf("recent similarity matched self: %v", got)
	}
	for _, last := range []string{"2026-11-18", "2026-11-01", "invalid"} {
		if got := m.recentSimilarity(1, date, map[uint]string{2: last}); got != 0 {
			t.Fatalf("invalid/future/old meal caused fatigue: %s %v", last, got)
		}
	}
	if got := m.recentSimilarity(1, date, map[uint]string{2: "2026-11-10"}); math.Abs(got-.5) > .000001 {
		t.Fatalf("seven-day fatigue did not halve: %v", got)
	}
	var absent *recommendationHistoryModel
	if absent.fingerprint() != "" || absent.similarity(1, 2) != 0 || absent.recentSimilarity(1, date, nil) != 0 {
		t.Fatal("nil model methods are not inert")
	}
}
