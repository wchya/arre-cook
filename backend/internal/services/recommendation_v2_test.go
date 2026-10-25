package services

import (
	"encoding/json"
	"fmt"
	"math"
	"testing"

	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"ninimenu/internal/testutil"
)

func recommendationTestDish(t *testing.T, uid uint, name, ingredients string) models.Dish {
	t.Helper()
	d := models.Dish{OwnerID: uid, Name: name, Enabled: true, MealType: "all", Category: "测试菜系", Taste: "清淡", Ingredients: ingredients, Seasonings: "[]", Steps: `[{"text":"备料后做好"}]`, CookTime: 20, Difficulty: "easy", Images: "[]", Tags: "[]"}
	if err := database.DB.Create(&d).Error; err != nil {
		t.Fatal(err)
	}
	return d
}
func TestRecommendationV2HardConstraintsAndUnknownTime(t *testing.T) {
	u, _, _ := testutil.NewUser("recommend-v2-hard@qq.com")
	beef := recommendationTestDish(t, u.ID, "v2硬过滤牛腩", `[{"name":"牛腩"}]`)
	safe := recommendationTestDish(t, u.ID, "v2硬过滤番茄", `[{"name":"西红柿"}]`)
	unknown := recommendationTestDish(t, u.ID, "v2硬过滤未知耗时", `[{"name":"马铃薯"}]`)
	if err := database.DB.Model(&unknown).Update("cook_time", 0).Error; err != nil {
		t.Fatal(err)
	}
	avoid := []string{"牛肉"}
	if _, err := SavePreferences(u.ID, PreferencesPatch{AvoidIngredients: &avoid}); err != nil {
		t.Fatal(err)
	}
	for _, ignore := range []bool{false, true} {
		response, err := RecommendDishes(u.ID, RecommendRequest{Keyword: "v2硬过滤", Count: 10, MaxCookTime: 30, IgnorePreferences: ignore})
		if err != nil || len(response.Items) != 1 || response.Items[0].Dish.ID != safe.ID {
			t.Fatalf("hard filter bypass: ignore=%v response=%+v err=%v", ignore, response, err)
		}
		response, err = RecommendDishes(u.ID, RecommendRequest{Keyword: "v2硬过滤", Count: 10, MaxCookTime: 30, IgnorePreferences: ignore, ExcludeDishIDs: []uint{safe.ID}, ExcludeIngredients: []string{"土豆"}})
		if err != nil || len(response.Items) != 0 {
			t.Fatalf("empty pool relaxed explicit exclusions: %+v %v", response, err)
		}
	}
	if !DishViolatesDietaryExclusions(beef, []string{"牛肉"}) || !DishViolatesDietaryExclusions(safe, []string{"番茄"}) {
		t.Fatal("canonical ingredients were not matched")
	}
}

func TestRecommendationV2SnapshotsSharedAndRevalidated(t *testing.T) {
	u, _, _ := testutil.NewUser("recommend-v2-snap@qq.com")
	other, _, _ := testutil.NewUser("recommend-v2-snap-other@qq.com")
	for i := 0; i < 4; i++ {
		recommendationTestDish(t, u.ID, fmt.Sprintf("v2稳定菜单%d", i), `[{"name":"番茄"}]`)
	}
	req := RecommendRequest{Keyword: "v2稳定菜单", Count: 2, MealType: "dinner", Mode: "home_auto", Jitter: true, Source: "app"}
	first, err := RecommendDishes(u.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	req.Source = "agent:test"
	req.Mode = "agent_tool"
	req.Jitter = false
	second, err := RecommendDishes(u.ID, req)
	if err != nil || first.RecommendationID == "" || first.SessionID != first.RecommendationID || first.RecommendationID != second.RecommendationID {
		t.Fatalf("same context not stable across sources: first=%+v second=%+v err=%v", first, second, err)
	}
	if err := ValidateRecommendationItem(other.ID, first.RecommendationID, first.Items[0].Dish.ID); err == nil {
		t.Fatal("snapshot leaked between users")
	}
	if err := ValidateRecommendationItem(u.ID, first.RecommendationID, first.Items[0].Dish.ID); err != nil {
		t.Fatal(err)
	}
	for _, item := range first.Items {
		if item.Score < 0 || item.Score > 100 || item.ItemID == "" {
			t.Fatalf("invalid internal score or item: %+v", item)
		}
	}
	if _, err := RecordRecommendationFeedback(u.ID, RecommendationFeedbackInput{RecommendationID: first.RecommendationID, DishID: first.Items[0].Dish.ID, EventType: "skip"}, "app", ""); err != nil {
		t.Fatal(err)
	}
	req.SessionID = first.RecommendationID
	req.Refresh = true
	third, err := RecommendDishes(u.ID, req)
	if err != nil || third.RecommendationID == first.RecommendationID || third.SessionID != first.SessionID {
		t.Fatalf("refresh/session association failed: %+v %v", third, err)
	}
	for _, item := range third.Items {
		if item.Dish.ID == first.Items[0].Dish.ID {
			t.Fatal("skipped recipe returned within session")
		}
	}
	// Reopening home without a client session must retain the daily session's skips.
	req.SessionID = ""
	req.Refresh = false
	req.Mode = "home_auto"
	reopened, err := RecommendDishes(u.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range reopened.Items {
		if item.Dish.ID == first.Items[0].Dish.ID {
			t.Fatal("home reopen lost daily session skips")
		}
	}
	id := reopened.Items[0].Dish.ID
	if err := database.DB.Model(&models.Dish{}).Where("id = ?", id).Update("seasonings", `[{"name":"花生油"}]`).Error; err != nil {
		t.Fatal(err)
	}
	allergies := []string{"花生"}
	if _, err := SavePreferences(u.ID, PreferencesPatch{Allergies: &allergies}); err != nil {
		t.Fatal(err)
	}
	updated, err := RecommendDishes(u.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range updated.Items {
		if item.Dish.ID == id {
			t.Fatal("cached recipe bypassed newly saved allergy")
		}
	}
}

func TestRecommendationV2RebuildsAffinityAndIgnoresFutureIntentions(t *testing.T) {
	u, _, _ := testutil.NewUser("recommend-v2-evidence@qq.com")
	good := recommendationTestDish(t, u.ID, "v2评价好菜", `[{"name":"鸡蛋"}]`)
	bad := recommendationTestDish(t, u.ID, "v2评价差菜", `[{"name":"牛肉"}]`)
	future := recommendationTestDish(t, u.ID, "v2评价未来计划", `[{"name":"虾"}]`)
	yesterday := recommendationNow().AddDate(0, 0, -1).Format("2006-01-02")
	rows := []models.MealRecord{{UserID: u.ID, DishID: good.ID, DishName: good.Name, MealDate: yesterday, MealType: "lunch", Mood: "great"}, {UserID: u.ID, DishID: bad.ID, DishName: bad.Name, MealDate: yesterday, MealType: "dinner", Mood: "meh"}, {UserID: u.ID, DishID: future.ID, DishName: future.Name, MealDate: recommendationNow().AddDate(0, 0, 1).Format("2006-01-02"), MealType: "dinner", Mood: "great", Rating: 5}}
	if err := database.DB.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	p := BuildTasteProfile(u.ID, 90)
	if p.TotalRecords != 2 || p.WindowRecords != 2 || p.LastEaten[future.ID] != "" || p.eatenCount[future.ID] != 0 {
		t.Fatalf("future menu polluted taste profile: %+v", p)
	}
	ga, _, _ := affinityForDish(good, p)
	ba, _, _ := affinityForDish(bad, p)
	if ga <= ba || p.tasteWeightMap["清淡"] <= 0 || p.categoryNegative[bad.Category] <= 0 {
		t.Fatalf("signed evidence failed: good=%v bad=%v", ga, ba)
	}
	zero := 0
	req := RecommendRequest{Keyword: "v2评价", Count: 10, ExcludeRecentDays: &zero, MealDate: recommendationNow().AddDate(0, 0, 2).Format("2006-01-02")}
	if _, err := RecommendDishes(u.ID, req); err != nil {
		t.Fatal(err)
	}
	var before models.UserDishAffinity
	if err := database.DB.Where("user_id = ? AND dish_id = ?", u.ID, bad.ID).First(&before).Error; err != nil {
		t.Fatal(err)
	}
	if before.Confidence <= 0 || before.Affinity >= 50 {
		t.Fatalf("negative preference not persisted: %+v", before)
	}
	if err := database.DB.Delete(&rows[1]).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := RecommendDishes(u.ID, req); err != nil {
		t.Fatal(err)
	}
	var after models.UserDishAffinity
	database.DB.Where("user_id = ? AND dish_id = ?", u.ID, bad.ID).First(&after)
	if after.EffectiveSamples != 0 || after.Confidence != 0 || after.Affinity <= before.Affinity {
		t.Fatalf("deleted feedback kept accumulating: before=%+v after=%+v", before, after)
	}
}

func TestRecommendationV2PairingUsesIngredientsAndWeekContextDoesNotWrite(t *testing.T) {
	u, _, _ := testutil.NewUser("recommend-v2-combo@qq.com")
	pork := recommendationTestDish(t, u.ID, "v2搭配小炒猪肉", `[{"name":"猪肉"}]`)
	another := recommendationTestDish(t, u.ID, "v2搭配假素菜", `[{"name":"茄子"},{"name":"肉末"}]`)
	vegetable := recommendationTestDish(t, u.ID, "v2搭配凉拌黄瓜", `[{"name":"黄瓜"}]`)
	another.Tags = `["素菜"]`
	if got := dishFeatures(another); got.Role != "meat" {
		t.Fatalf("meat dish treated as vegetarian because of legacy tags: %+v", got)
	}
	if pairingScore(vegetable, []models.Dish{pork}) <= pairingScore(another, []models.Dish{pork}) {
		t.Fatal("complementary ingredients did not improve pairing")
	}
	var before, after int64
	database.DB.Model(&models.RecommendationSnapshot{}).Where("user_id = ?", u.ID).Count(&before)
	ctx, err := NewRecommendationContext(u.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	got, err := RecommendWithContext(ctx, RecommendRequest{Keyword: "v2搭配", Mode: "week_plan", Count: 3})
	if err != nil || len(got.Items) != 3 || got.RecommendationID != "" {
		t.Fatalf("week core failed: %+v %v", got, err)
	}
	database.DB.Model(&models.RecommendationSnapshot{}).Where("user_id = ?", u.ID).Count(&after)
	if before != after {
		t.Fatal("weekly context generated per-meal snapshots")
	}
	for _, item := range got.Items {
		sum := .35*item.parts.Personal + .20*item.parts.Context + .15*item.parts.Freshness + .15*item.parts.Convenience + .10*item.parts.Pairing + .05*item.parts.Quality
		if math.Abs(item.Score-sum) > .051 {
			t.Fatalf("score weights differ from contract: %+v", item)
		}
	}
	// Snapshot audit keeps score contributions, while normal responses hide them.
	raw, _ := json.Marshal(got)
	var wire map[string]any
	_ = json.Unmarshal(raw, &wire)
	if _, ok := wire["parts"]; ok {
		t.Fatal("internal score contributions leaked")
	}
}

func TestRecommendationV2ConcurrentDailyReadsAndExpiry(t *testing.T) {
	u, _, _ := testutil.NewUser("recommend-v2-concurrent@qq.com")
	recommendationTestDish(t, u.ID, "v2并发菜单", `[{"name":"番茄"}]`)
	expired := models.RecommendationSnapshot{ID: newRecommendationID(), UserID: u.ID, MealDate: "2000-01-01", MealType: "dinner", ContextKey: "old", SessionID: "old", Payload: "[]", AlgorithmVersion: RecommendationAlgorithmVersion, CreatedAt: recommendationNow().AddDate(0, 0, -31)}
	if err := database.DB.Create(&expired).Error; err != nil {
		t.Fatal(err)
	}
	type result struct {
		response *RecommendResponse
		err      error
	}
	results := make(chan result, 4)
	for i := 0; i < 4; i++ {
		go func() {
			r, e := RecommendDishes(u.ID, RecommendRequest{Keyword: "v2并发菜单", MealType: "dinner", Count: 1, Mode: "home_auto"})
			results <- result{r, e}
		}()
	}
	id := ""
	for i := 0; i < 4; i++ {
		r := <-results
		if r.err != nil {
			t.Fatal(r.err)
		}
		if id == "" {
			id = r.response.RecommendationID
		}
		if r.response.RecommendationID != id {
			t.Fatal("concurrent daily reads created different snapshots")
		}
	}
	var n int64
	database.DB.Model(&models.RecommendationSnapshot{}).Where("user_id = ?", u.ID).Count(&n)
	if n != 1 {
		t.Fatalf("expired or duplicate snapshots retained: %d", n)
	}
}

func TestRecommendationV2ExplicitDislikeCanBeReversedWithoutChangingEatingFacts(t *testing.T) {
	u, _, _ := testutil.NewUser("recommend-v2-undo@qq.com")
	dish := recommendationTestDish(t, u.ID, "v2撤销差评", `[{"name":"番茄"}]`)
	before := BuildTasteProfile(u.ID, 90)
	baseline, _, _ := affinityForDish(dish, before)
	if _, err := RecordRecommendationFeedback(u.ID, RecommendationFeedbackInput{DishID: dish.ID, EventType: "dislike", RequestKey: "dislike-one"}, "agent:test", ""); err != nil {
		t.Fatal(err)
	}
	p := BuildTasteProfile(u.ID, 90)
	lowered, _, _ := affinityForDish(dish, p)
	if lowered >= baseline || len(p.DislikedDishes) != 1 || p.TotalRecords != 0 {
		t.Fatalf("explicit feedback not projected: baseline=%v lowered=%v profile=%+v", baseline, lowered, p)
	}
	if _, err := RecordRecommendationFeedback(u.ID, RecommendationFeedbackInput{DishID: dish.ID, EventType: "undo_dislike", RequestKey: "undo-one"}, "agent:test", ""); err != nil {
		t.Fatal(err)
	}
	p = BuildTasteProfile(u.ID, 90)
	restored, _, _ := affinityForDish(dish, p)
	if math.Abs(restored-baseline) > .001 || len(p.DislikedDishes) != 0 {
		t.Fatalf("undo failed to rebuild preference: baseline=%v restored=%v", baseline, restored)
	}
}

func TestRecommendationV2PresetAllergensAndIngredientSemantics(t *testing.T) {
	for _, tc := range []struct{ term, ingredient string }{
		{"花生", "花生油"}, {"虾", "海米"}, {"蟹", "大闸蟹"}, {"贝类", "蛤蜊"}, {"贝类", "生蚝"}, {"贝类", "鲍鱼"},
		{"鸡蛋", "蛋液"}, {"牛奶", "乳酪"}, {"大豆", "生抽"}, {"芝麻", "香油"}, {"坚果", "核桃"}, {"坚果", "腰果"}, {"内脏", "猪肝"}, {"肥肉", "五花肉"},
	} {
		t.Run(tc.term+tc.ingredient, func(t *testing.T) {
			d := models.Dish{Ingredients: `[{"name":"` + tc.ingredient + `"}]`}
			if !DishViolatesDietaryExclusions(d, []string{tc.term}) {
				t.Fatalf("preset exclusion %s did not cover %s", tc.term, tc.ingredient)
			}
		})
	}
	if containsAll("豆腐 生抽", []string{"大豆"}) || containsAll("鸭蛋", []string{"鸡蛋"}) {
		t.Fatal("broad allergy classes incorrectly satisfied required raw ingredients")
	}
	if !containsAll("西红柿", []string{"番茄"}) {
		t.Fatal("true ingredient aliases no longer match")
	}
	u, _, _ := testutil.NewUser("recommend-v2-ingredient-semantics@qq.com")
	d := recommendationTestDish(t, u.ID, "v2原料语义豆腐", `[{"name":"豆腐"}]`)
	p := BuildTasteProfile(u.ID, 90)
	request := RecommendRequest{MealDate: recommendationNow().Format("2006-01-02")}
	_, _, without := scoreDishV2(d, request, p, recommendationDay(recommendationNow()), nil, nil, 100, nil)
	_, _, withSauce := scoreDishV2(d, request, p, recommendationDay(recommendationNow()), nil, map[string]bool{canonicalIngredient("生抽"): true}, 100, nil)
	if without.Convenience != withSauce.Convenience {
		t.Fatal("pantry soy sauce treated as available tofu")
	}
	if canonicalIngredient("番茄酱") == canonicalIngredient("番茄") {
		t.Fatal("pantry processed condiment replaced a raw ingredient")
	}
	response, err := RecommendDishes(u.ID, RecommendRequest{Keyword: "v2原料语义", IncludeIngredients: []string{"大豆"}})
	if err != nil || len(response.Items) != 0 {
		t.Fatalf("soybean request returned tofu due to allergy expansion: %+v %v", response, err)
	}
}

func TestRecommendationV2DisabledDiversityAndHonestReasons(t *testing.T) {
	u, _, _ := testutil.NewUser("recommend-v2-explanations@qq.com")
	d := recommendationTestDish(t, u.ID, "v2理由收藏菜", `[{"name":"鸡蛋"}]`)
	if err := SetFavorite(u.ID, d.ID, true); err != nil {
		t.Fatal(err)
	}
	tastes := []string{"清淡"}
	if _, err := SavePreferences(u.ID, PreferencesPatch{FavoriteTastes: &tastes}); err != nil {
		t.Fatal(err)
	}
	p := BuildTasteProfile(u.ID, 90)
	off := false
	_, reasons, parts := scoreDishV2(d, RecommendRequest{Diversity: &off}, p, recommendationDay(recommendationNow()), []models.Dish{d}, nil, 100, nil)
	if parts.Pairing != 50 {
		t.Fatalf("disabled diversity changed pairing: %v", parts.Pairing)
	}
	for _, reason := range reasons {
		if reason == "之前吃过，评价不错" {
			t.Fatal("favorite prior fabricated a previous good meal")
		}
	}
	if got := dishFeatures(models.Dish{Name: "未知配方", Ingredients: `[{"name":"面粉"}]`}); got.Role != "" {
		t.Fatalf("unclassified recipe assumed vegetable dish: %+v", got)
	}
}

func TestRecommendationV2AlternativeReasonsDoNotInventConfirmedSelections(t *testing.T) {
	u, _, _ := testutil.NewUser("recommend-v2-alternative-reasons@qq.com")
	pork := recommendationTestDish(t, u.ID, "v2备选理由红烧猪肉", `[{"name":"猪肉"}]`)
	vegetable := recommendationTestDish(t, u.ID, "v2备选理由凉拌黄瓜", `[{"name":"黄瓜"}]`)
	// Avoid unrelated reasons eclipsing the pairing contribution in this test.
	if err := database.DB.Model(&vegetable).Update("cook_time", 60).Error; err != nil {
		t.Fatal(err)
	}
	vegetable.CookTime = 60
	p := BuildTasteProfile(u.ID, 90)
	for _, mode := range []string{"home_auto", "home_change", "home_pick", "wheel"} {
		t.Run(mode, func(t *testing.T) {
			_, reasons, parts := scoreDishV2(vegetable, RecommendRequest{Mode: mode}, p, recommendationDay(recommendationNow()), []models.Dish{pork}, nil, 100, nil)
			if parts.Pairing < 70 {
				t.Fatal("fixture failed to exercise a positive pairing contribution")
			}
			found := false
			for _, reason := range reasons {
				if reason == "和已经选好的菜搭配更丰富" {
					t.Fatal("earlier alternative was claimed to be a confirmed choice")
				}
				if reason == "换一种主食材或做法" {
					found = true
				}
			}
			if !found {
				t.Fatalf("missing truthful alternative explanation: %v", reasons)
			}
		})
	}
	for _, req := range []RecommendRequest{
		{Mode: "home_auto", SelectedDishIDs: []uint{pork.ID}},
		{Mode: "home_auto", confirmedDishIDs: map[uint]bool{pork.ID: true}},
	} {
		_, reasons, _ := scoreDishV2(vegetable, req, p, recommendationDay(recommendationNow()), []models.Dish{pork}, nil, 100, nil)
		found := false
		for _, reason := range reasons {
			if reason == "和已经选好的菜搭配更丰富" {
				found = true
			}
		}
		if !found {
			t.Fatalf("real explicit/plan choice lost its factual explanation: %v", reasons)
		}
	}
	for _, mode := range []string{"week_plan", "tomorrow"} {
		_, reasons, parts := scoreDishV2(vegetable, RecommendRequest{Mode: mode}, p, recommendationDay(recommendationNow()), []models.Dish{pork}, nil, 100, nil)
		found := false
		for _, reason := range reasons {
			if reason == "为这一餐搭配不同的主食材或做法" {
				found = true
			}
		}
		if parts.Pairing < 70 || !found {
			t.Fatalf("meal combination was weakened: score=%v reasons=%v", parts.Pairing, reasons)
		}
	}
}
