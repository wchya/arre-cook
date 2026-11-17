package services

import (
	"fmt"
	"math"
	"testing"

	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"ninimenu/internal/testutil"
)

func recommendationV3User(t testing.TB, suffix string) models.User {
	t.Helper()
	u, _, err := testutil.NewUser(t.Name() + suffix + "@recommendation.test")
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func recommendationV3Meal(t testing.TB, uid uint, dish models.Dish, daysAgo, rating int) models.MealRecord {
	t.Helper()
	r := models.MealRecord{UserID: uid, DishID: dish.ID, DishName: dish.Name, MealDate: recommendationNow().AddDate(0, 0, -daysAgo).Format("2006-01-02"), MealType: "dinner", Rating: rating}
	if err := database.DB.Create(&r).Error; err != nil {
		t.Fatal(err)
	}
	return r
}

func recommendationV3Request(keyword string, excluded ...uint) RecommendRequest {
	off, zero := false, 0
	return RecommendRequest{Keyword: keyword, Count: 10, Diversity: &off, ExcludeRecentDays: &zero, ExcludeDishIDs: excluded}
}

func recommendationV3Result(t testing.TB, uid uint, req RecommendRequest) *RecommendResponse {
	t.Helper()
	res, err := RecommendDishes(uid, req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func recommendationV3Item(t testing.TB, res *RecommendResponse, id uint) RecommendItem {
	t.Helper()
	for _, item := range res.Items {
		if item.Dish.ID == id {
			return item
		}
	}
	t.Fatalf("expected dish %d in recommendation: %+v", id, res.Items)
	return RecommendItem{}
}

func TestRecommendationV3HistoryTransfersToUnseenRecipes(t *testing.T) {
	u := recommendationV3User(t, "")
	source := recommendationTestDish(t, u.ID, t.Name()+"历史番茄炒蛋", `[{"name":"番茄"},{"name":"鸡蛋"}]`)
	keyword := t.Name() + "候选"
	near := recommendationTestDish(t, u.ID, keyword+"西红柿炒鸡蛋", `[{"name":"西红柿"},{"name":"鸡蛋"}]`)
	far := recommendationTestDish(t, u.ID, keyword+"番茄炒黄瓜", `[{"name":"番茄"},{"name":"黄瓜"}]`)
	recommendationV3Meal(t, u.ID, source, 10, 5)
	req := recommendationV3Request(keyword, source.ID)
	res := recommendationV3Result(t, u.ID, req)
	if res.AlgorithmVersion != "personal-meal-v3" {
		t.Fatalf("unexpected algorithm version: %s", res.AlgorithmVersion)
	}
	nearItem, farItem := recommendationV3Item(t, res, near.ID), recommendationV3Item(t, res, far.ID)
	if nearItem.Score <= farItem.Score || len(res.Items) != 2 || res.Items[0].Dish.ID != near.ID {
		t.Fatalf("liked recipe did not transfer to an unseen related recipe: near=%+v far=%+v", nearItem, farItem)
	}
	// Selected dishes must not change pairing or trigger exploration when disabled.
	req.SelectedDishIDs = []uint{source.ID}
	req.Refresh = true
	for attempt := 0; attempt < 3; attempt++ {
		res = recommendationV3Result(t, u.ID, req)
		if res.Items[0].Dish.ID != near.ID {
			t.Fatalf("disabled exploration changed the best recipe: %+v", res.Items)
		}
		for _, item := range res.Items {
			if item.parts.Pairing != 50 {
				t.Fatalf("disabled diversity still adjusted pairing: %+v", item)
			}
		}
	}
}

func TestRecommendationV3UsersWithOppositeHistoriesGetDifferentRecipes(t *testing.T) {
	for _, tc := range []struct {
		name, seedName, seedIngredients string
		wantTomato                      bool
	}{
		{"tomato", "番茄炒蛋", `[{"name":"番茄"},{"name":"鸡蛋"}]`, true},
		{"beef", "土豆炖牛肉", `[{"name":"土豆"},{"name":"牛肉"}]`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u := recommendationV3User(t, "")
			source := recommendationTestDish(t, u.ID, t.Name()+"历史"+tc.seedName, tc.seedIngredients)
			keyword := t.Name() + "候选"
			tomato := recommendationTestDish(t, u.ID, keyword+"西红柿炒鸡蛋", `[{"name":"西红柿"},{"name":"鸡蛋"}]`)
			beef := recommendationTestDish(t, u.ID, keyword+"马铃薯炖牛肉", `[{"name":"马铃薯"},{"name":"牛肉"}]`)
			recommendationV3Meal(t, u.ID, source, 10, 5)
			res := recommendationV3Result(t, u.ID, recommendationV3Request(keyword, source.ID))
			want := beef.ID
			if tc.wantTomato {
				want = tomato.ID
			}
			if len(res.Items) != 2 || res.Items[0].Dish.ID != want {
				t.Fatalf("history-specific preference was lost: want=%d items=%+v", want, res.Items)
			}
		})
	}
}

func TestRecommendationV3FavoritesTransferWithoutMealHistory(t *testing.T) {
	u := recommendationV3User(t, "")
	source := recommendationTestDish(t, u.ID, t.Name()+"收藏清蒸鲈鱼", `[{"name":"鲈鱼"},{"name":"姜"}]`)
	keyword := t.Name() + "候选"
	near := recommendationTestDish(t, u.ID, keyword+"清蒸鲈鱼新做法", `[{"name":"鲈鱼"},{"name":"姜"}]`)
	far := recommendationTestDish(t, u.ID, keyword+"凉拌黄瓜", `[{"name":"黄瓜"},{"name":"蒜"}]`)
	if err := SetFavorite(u.ID, source.ID, true); err != nil {
		t.Fatal(err)
	}
	res := recommendationV3Result(t, u.ID, recommendationV3Request(keyword, source.ID))
	if recommendationV3Item(t, res, near.ID).Score <= recommendationV3Item(t, res, far.ID).Score {
		t.Fatalf("favorite-only user received no related-recipe preference: %+v", res.Items)
	}
}

func TestRecommendationV3DislikeTransfersAndUndoRebuilds(t *testing.T) {
	u := recommendationV3User(t, "")
	source := recommendationTestDish(t, u.ID, t.Name()+"历史红烧茄子", `[{"name":"茄子"},{"name":"蒜"}]`)
	keyword := t.Name() + "候选"
	near := recommendationTestDish(t, u.ID, keyword+"红烧茄子新做法", `[{"name":"茄子"},{"name":"蒜"}]`)
	recommendationTestDish(t, u.ID, keyword+"清蒸土豆", `[{"name":"土豆"},{"name":"胡萝卜"}]`)
	req := recommendationV3Request(keyword, source.ID)
	before := recommendationV3Result(t, u.ID, req)
	if _, err := RecordRecommendationFeedback(u.ID, RecommendationFeedbackInput{DishID: source.ID, EventType: "dislike"}, "app", ""); err != nil {
		t.Fatal(err)
	}
	negative := recommendationV3Result(t, u.ID, req)
	beforeScore, negativeScore := recommendationV3Item(t, before, near.ID).Score, recommendationV3Item(t, negative, near.ID).Score
	if negativeScore >= beforeScore {
		t.Fatalf("explicit dislike did not lower a near variant: before=%v after=%v", beforeScore, negativeScore)
	}
	if _, err := RecordRecommendationFeedback(u.ID, RecommendationFeedbackInput{DishID: source.ID, EventType: "undo_dislike"}, "app", ""); err != nil {
		t.Fatal(err)
	}
	restored := recommendationV3Item(t, recommendationV3Result(t, u.ID, req), near.ID)
	if math.Abs(restored.Score-beforeScore) > .01 {
		t.Fatalf("undo retained negative similarity evidence: before=%v restored=%v", beforeScore, restored.Score)
	}
}

func TestRecommendationV3DeletedHistoryRemovesTransferredPreference(t *testing.T) {
	u := recommendationV3User(t, "")
	source := recommendationTestDish(t, u.ID, t.Name()+"历史香菇炖鸡", `[{"name":"鸡肉"},{"name":"香菇"}]`)
	keyword := t.Name() + "候选"
	near := recommendationTestDish(t, u.ID, keyword+"香菇炖鸡新做法", `[{"name":"鸡肉"},{"name":"香菇"}]`)
	req := recommendationV3Request(keyword, source.ID)
	baseline := recommendationV3Item(t, recommendationV3Result(t, u.ID, req), near.ID)
	meal := recommendationV3Meal(t, u.ID, source, 10, 5)
	learned := recommendationV3Item(t, recommendationV3Result(t, u.ID, req), near.ID)
	if learned.Score <= baseline.Score {
		t.Fatalf("history did not improve related recipe: baseline=%v learned=%v", baseline.Score, learned.Score)
	}
	if err := database.DB.Delete(&meal).Error; err != nil {
		t.Fatal(err)
	}
	restored := recommendationV3Item(t, recommendationV3Result(t, u.ID, req), near.ID)
	if math.Abs(restored.Score-baseline.Score) > .01 {
		t.Fatalf("deleted history still affects related recipe: baseline=%v restored=%v", baseline.Score, restored.Score)
	}
}

func TestRecommendationV3HistoricalRecipeEditInvalidatesSnapshot(t *testing.T) {
	u := recommendationV3User(t, "")
	source := recommendationTestDish(t, u.ID, t.Name()+"收藏历史菜", `[{"name":"番茄"},{"name":"鸡蛋"}]`)
	keyword := t.Name() + "候选"
	near := recommendationTestDish(t, u.ID, keyword+"番茄炒蛋", `[{"name":"番茄"},{"name":"鸡蛋"}]`)
	recommendationTestDish(t, u.ID, keyword+"清蒸西兰花", `[{"name":"西兰花"},{"name":"胡萝卜"}]`)
	if err := SetFavorite(u.ID, source.ID, true); err != nil {
		t.Fatal(err)
	}
	req := recommendationV3Request(keyword, source.ID)
	before := recommendationV3Result(t, u.ID, req)
	if err := database.DB.Model(&source).Update("ingredients", `[{"name":"西兰花"},{"name":"胡萝卜"}]`).Error; err != nil {
		t.Fatal(err)
	}
	after := recommendationV3Result(t, u.ID, req)
	if before.RecommendationID == after.RecommendationID {
		t.Fatalf("editing a history recipe outside the candidate keyword reused the stale snapshot: %s", before.RecommendationID)
	}
	if recommendationV3Item(t, after, near.ID).Score >= recommendationV3Item(t, before, near.ID).Score {
		t.Fatalf("updated history features did not change related-recipe preference: before=%+v after=%+v", before.Items, after.Items)
	}
}

func TestRecommendationV3HistoricalIngredientOrderInvalidatesFreshness(t *testing.T) {
	u := recommendationV3User(t, "")
	source := recommendationTestDish(t, u.ID, t.Name()+"历史番茄炖鸡", `[{"name":"番茄"},{"name":"鸡肉"}]`)
	keyword := t.Name() + "候选"
	candidate := recommendationTestDish(t, u.ID, keyword+"番茄炒蛋", `[{"name":"番茄"},{"name":"鸡蛋"}]`)
	recommendationV3Meal(t, u.ID, source, 1, 5)
	req := recommendationV3Request(keyword, source.ID)
	before := recommendationV3Result(t, u.ID, req)
	if repeated := recommendationV3Result(t, u.ID, req); repeated.RecommendationID != before.RecommendationID {
		t.Fatal("unchanged history unexpectedly invalidated the snapshot")
	}
	// Ingredient sets stay identical, but first-ingredient fatigue moves from tomato to chicken.
	if err := database.DB.Model(&source).Update("ingredients", `[{"name":"鸡肉"},{"name":"番茄"}]`).Error; err != nil {
		t.Fatal(err)
	}
	after := recommendationV3Result(t, u.ID, req)
	if after.RecommendationID == before.RecommendationID {
		t.Fatalf("historical main-ingredient change reused stale freshness: %s", before.RecommendationID)
	}
	beforeFreshness := recommendationV3Item(t, before, candidate.ID).parts.Freshness
	afterFreshness := recommendationV3Item(t, after, candidate.ID).parts.Freshness
	if afterFreshness <= beforeFreshness {
		t.Fatalf("historical ingredient order did not rebuild ingredient fatigue: before=%v after=%v", beforeFreshness, afterFreshness)
	}
}

func TestRecommendationV3RecentContentDuplicatesAreLessFresh(t *testing.T) {
	u := recommendationV3User(t, "")
	source := recommendationTestDish(t, u.ID, t.Name()+"历史番茄炒蛋", `[{"name":"番茄"},{"name":"鸡蛋"}]`)
	keyword := t.Name() + "候选"
	duplicate := recommendationTestDish(t, u.ID, keyword+"西红柿炒鸡蛋", `[{"name":"西红柿"},{"name":"鸡蛋"}]`)
	novel := recommendationTestDish(t, u.ID, keyword+"番茄炖豆腐", `[{"name":"番茄"},{"name":"豆腐"}]`)
	recommendationV3Meal(t, u.ID, source, 1, 5)
	req := recommendationV3Request(keyword, source.ID)
	req.Diversity = nil
	res := recommendationV3Result(t, u.ID, req)
	duplicateFreshness := recommendationV3Item(t, res, duplicate.ID).parts.Freshness
	novelFreshness := recommendationV3Item(t, res, novel.ID).parts.Freshness
	if duplicateFreshness >= novelFreshness {
		t.Fatalf("new recipe ID hid recently eaten duplicate content: duplicate=%v meaningfulVariant=%v", duplicateFreshness, novelFreshness)
	}
}

func TestRecommendationV3SimilarRecipesCannotBypassAllergens(t *testing.T) {
	u := recommendationV3User(t, "")
	source := recommendationTestDish(t, u.ID, t.Name()+"历史花生炒鸡", `[{"name":"鸡肉"},{"name":"花生"}]`)
	keyword := t.Name() + "候选"
	unsafe := recommendationTestDish(t, u.ID, keyword+"花生炒鸡新做法", `[{"name":"鸡肉"},{"name":"花生"}]`)
	safe := recommendationTestDish(t, u.ID, keyword+"清蒸西兰花", `[{"name":"西兰花"}]`)
	recommendationV3Meal(t, u.ID, source, 10, 5)
	allergies := []string{"花生"}
	if _, err := SavePreferences(u.ID, PreferencesPatch{Allergies: &allergies}); err != nil {
		t.Fatal(err)
	}
	req := recommendationV3Request(keyword, source.ID)
	req.IgnorePreferences = true
	res := recommendationV3Result(t, u.ID, req)
	if len(res.Items) != 1 || res.Items[0].Dish.ID != safe.ID {
		t.Fatalf("historical appeal bypassed a hard allergen exclusion for dish %d: %+v", unsafe.ID, res.Items)
	}
}

func TestRecommendationV3DiscoveryKeepsRelevanceAndUsefulNovelty(t *testing.T) {
	leader := models.Dish{ID: 1, Name: "清蒸西兰花", Ingredients: `[{"name":"西兰花"},{"name":"胡萝卜"}]`, Taste: "清淡", Category: "家常菜"}
	variant := models.Dish{ID: 2, Name: "番茄炖豆腐", Ingredients: `[{"name":"番茄"},{"name":"豆腐"}]`, Taste: "清淡", Category: "家常菜"}
	duplicate := models.Dish{ID: 3, Name: "西红柿炖豆腐", Ingredients: `[{"name":"西红柿"},{"name":"豆腐"}]`, Taste: "清淡", Category: "家常菜"}
	for _, tc := range []struct {
		name       string
		score      float64
		positive   float64
		discovery  float64
		negative   float64
		novelty    float64
		similarity float64
		selected   []models.Dish
		want       bool
	}{
		{name: "supported change within six points", score: 80, positive: .5, discovery: .5, novelty: .4, similarity: .6, want: true},
		{name: "relevance loss exceeds six points", score: 79.9, positive: .5, discovery: .5, novelty: .4, similarity: .6},
		{name: "unrelated new recipe", score: 85, novelty: .8, similarity: .1},
		{name: "insufficient trusted support", score: 85, positive: .5, discovery: .07, novelty: .4, similarity: .6},
		{name: "negative historical support", score: 85, positive: .5, discovery: .5, negative: .3, novelty: .4, similarity: .6},
		{name: "renamed duplicate", score: 85, positive: .5, discovery: .5, novelty: .4, similarity: .93},
		{name: "no meaningful change", score: 85, positive: .5, discovery: .5, novelty: .05, similarity: .8},
		{name: "duplicate of any selected recipe", score: 85, positive: .5, discovery: .5, novelty: .4, similarity: .6, selected: []models.Dish{leader, duplicate}},
		{name: "different from selected recipe", score: 80, positive: .5, discovery: .5, novelty: .4, similarity: .6, selected: []models.Dish{leader}, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &TasteProfile{eatenCount: map[uint]int{leader.ID: 1}}
			p.historyModel = newRecommendationHistory([]models.Dish{leader, variant, duplicate}, p)
			p.historyModel.signals[variant.ID] = recommendationHistorySignal{Positive: tc.positive, Discovery: tc.discovery, Negative: tc.negative, Novelty: tc.novelty, Similarity: tc.similarity}
			scored := []RecommendItem{{Dish: leader, Score: 86}, {Dish: variant, Score: tc.score}}
			got, ok := historyDiscovery(scored, tc.selected, p)
			if ok != tc.want || ok && got.Dish.ID != variant.ID {
				t.Fatalf("discovery eligibility differs: got=%+v eligible=%v want=%v", got, ok, tc.want)
			}
		})
	}
}

func BenchmarkRecommendationV3ReusedContext(b *testing.B) {
	fixtureID := newRecommendationID()
	u := recommendationV3User(b, fixtureID)
	keyword := b.Name() + fixtureID + "候选"
	ingredients := []string{`[{"name":"番茄"},{"name":"鸡蛋"}]`, `[{"name":"土豆"},{"name":"牛肉"}]`, `[{"name":"西兰花"},{"name":"胡萝卜"}]`, `[{"name":"黄瓜"},{"name":"蒜"}]`}
	methods := []string{"炒", "炖", "蒸", "凉拌"}
	excluded := make([]uint, 0, 20)
	for i := 0; i < 100; i++ {
		d := models.Dish{OwnerID: u.ID, Name: fmt.Sprintf("%s%s%d", keyword, methods[i%len(methods)], i), Enabled: true, MealType: "all", Category: "测试菜系", Taste: "清淡", Ingredients: ingredients[i%len(ingredients)], Seasonings: "[]", Steps: `[{"text":"做好"}]`, CookTime: 20, Difficulty: "easy", Images: "[]", Tags: "[]"}
		if err := database.DB.Create(&d).Error; err != nil {
			b.Fatal(err)
		}
		if i < 20 {
			recommendationV3Meal(b, u.ID, d, 10+i, 3+i%3)
			excluded = append(excluded, d.ID)
		}
	}
	ctx, err := NewRecommendationContext(u.ID, 90)
	if err != nil {
		b.Fatal(err)
	}
	req := recommendationV3Request(keyword, excluded...)
	req.Mode, req.Count, req.Diversity = "week_plan", 3, nil
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res, err := RecommendWithContext(ctx, req)
		if err != nil || len(res.Items) != 3 {
			b.Fatalf("reused context failed: response=%+v err=%v", res, err)
		}
	}
}
