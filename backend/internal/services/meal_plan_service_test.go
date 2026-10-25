package services

import (
	"encoding/json"
	"errors"
	"testing"

	"gorm.io/gorm"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"ninimenu/internal/testutil"
)

func planTestDish(t *testing.T, owner uint, name, ingredient string) models.Dish {
	t.Helper()
	d := models.Dish{OwnerID: owner, Name: name, Enabled: true, MealType: "all", CookTime: 20, Difficulty: "easy", Ingredients: `[{"name":"` + ingredient + `","amount":"1个"}]`, Seasonings: "[]", Tags: "[]", Images: "[]", Steps: `[{"text":"洗净炒熟"}]`}
	if err := database.DB.Create(&d).Error; err != nil {
		t.Fatal(err)
	}
	return d
}

func TestMealPlanIsSeparateFromConsumptionAndRetrySafe(t *testing.T) {
	u, _, err := testutil.NewUser(t.Name() + "@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	d := planTestDish(t, u.ID, "待做番茄", "番茄")
	date := healthNow().AddDate(0, 0, 1).Format("2006-01-02")
	in := MealPlanInput{DishID: d.ID, MealDate: date, MealType: "dinner", Source: "agent:test", Actor: "test"}
	p, err := AcceptMealPlan(u.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	again, err := AcceptMealPlan(u.ID, in)
	if err != nil || again.ID != p.ID {
		t.Fatalf("retry failed: %+v %v", again, err)
	}
	for model, want := range map[any]int64{&models.HealthPlanItem{}: 1, &models.MealRecord{}: 0, &models.FoodJournalEntry{}: 0, &models.ShoppingCheck{}: 1, &models.BehaviorEvent{}: 1} {
		var n int64
		if err := database.DB.Model(model).Where("user_id = ?", u.ID).Count(&n).Error; err != nil || n != want {
			t.Fatalf("%T count=%d want=%d err=%v", model, n, want, err)
		}
	}
	listed, err := ListMealPlans(u.ID, date, date)
	if err != nil || len(listed) != 1 || listed[0].Status != "planned" || !listed[0].Available || listed[0].Source != "agent:test" {
		t.Fatalf("incorrect plan view: %+v %v", listed, err)
	}
	if _, err := AcceptHealthPlan(u.ID, HealthPlanInput{DishID: d.ID, MealDate: date, MealType: "dinner", PeriodDays: 0}); err == nil {
		t.Fatal("generic plans accidentally removed report validation")
	}
}

func TestMealPlanKeepsRecommendationAndActualSlotAttribution(t *testing.T) {
	u, _, _ := testutil.NewUser(t.Name() + "@qq.com")
	a := planTestDish(t, u.ID, t.Name()+"番茄", "番茄")
	b := planTestDish(t, u.ID, t.Name()+"土豆", "土豆")
	recommended, err := RecommendDishes(u.ID, RecommendRequest{Keyword: t.Name(), MealType: "lunch", MealDate: healthToday(), Count: 2, Source: "app"})
	if err != nil || len(recommended.Items) != 2 || recommended.RecommendationID == "" {
		t.Fatalf("recommendation unavailable: %+v %v", recommended, err)
	}
	date, err := NormalizeMealPlanDate("tomorrow")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AcceptMealPlan(u.ID, MealPlanInput{DishID: a.ID, MealDate: "tomorrow", MealType: "dinner", RecommendationID: recommended.RecommendationID}); err != nil {
		t.Fatalf("explicit choice for a different meal was rejected: %v", err)
	}
	in := MealPlanReplaceInput{MealDate: date, MealType: "dinner", DishIDs: []uint{a.ID, b.ID}, RecommendationIDs: map[uint]string{b.ID: recommended.RecommendationID}}
	for i := 0; i < 2; i++ {
		plans, err := ReplaceMealPlan(u.ID, in)
		if err != nil || len(plans) != 2 || plans[1].RecommendationID != recommended.RecommendationID {
			t.Fatalf("replacement attribution lost: %+v %v", plans, err)
		}
	}
	var events []models.BehaviorEvent
	if err := database.DB.Where("user_id = ? AND event_type = ?", u.ID, "accept").Find(&events).Error; err != nil || len(events) != 2 {
		t.Fatalf("acceptance events=%d err=%v", len(events), err)
	}
	for _, event := range events {
		var meta map[string]any
		if err := json.Unmarshal([]byte(event.Meta), &meta); err != nil {
			t.Fatal(err)
		}
		if meta["meal_date"] != date || meta["meal_type"] != "dinner" || meta["served_for_date"] != healthToday() || meta["recommended_meal_type"] != "lunch" || meta["session_id"] != recommended.SessionID {
			t.Fatalf("recommendation slot confused with actual plan: %+v", meta)
		}
	}
}

func TestReplaceMealPlanAtomicAndPreservesActualShopping(t *testing.T) {
	u, _, _ := testutil.NewUser(t.Name() + "@qq.com")
	other, _, _ := testutil.NewUser(t.Name() + "-other@qq.com")
	a := planTestDish(t, u.ID, "原菜单", "番茄")
	b := planTestDish(t, u.ID, "替换菜单", "土豆")
	private := planTestDish(t, other.ID, "别人菜单", "鸡蛋")
	in := MealPlanReplaceInput{MealDate: healthToday(), MealType: "lunch", DishIDs: []uint{a.ID}}
	first, err := ReplaceMealPlan(u.ID, in)
	if err != nil || len(first) != 1 {
		t.Fatalf("initial replace: %v", err)
	}
	in.DishIDs = []uint{b.ID, private.ID}
	if _, err := ReplaceMealPlan(u.ID, in); !errors.Is(err, ErrDishNotFound) {
		t.Fatalf("private replacement accepted: %v", err)
	}
	check := func(wantID uint) {
		t.Helper()
		plans, err := ListMealPlans(u.ID, healthToday(), healthToday())
		if err != nil || len(plans) != 1 || plans[0].DishID != wantID {
			t.Fatalf("replacement lost previous slot: %+v %v", plans, err)
		}
	}
	check(a.ID)
	if _, err := CreateMealRecord(u.ID, MealInput{DishID: a.ID, MealDate: healthToday(), MealType: "lunch"}, "app", ""); err != nil {
		t.Fatal(err)
	}
	in.DishIDs = []uint{b.ID}
	changed, err := ReplaceMealPlan(u.ID, in)
	if err != nil || len(changed) != 1 {
		t.Fatal(err)
	}
	check(b.ID)
	var n int64
	database.DB.Model(&models.ShoppingCheck{}).Where("user_id = ?", u.ID).Count(&n)
	if n != 2 {
		t.Fatalf("replacing plan removed actual food shopping: %d", n)
	}
	again, err := ReplaceMealPlan(u.ID, in)
	if err != nil || len(again) != 1 || again[0].ID != changed[0].ID {
		t.Fatalf("replace retry changed identity: %+v %v", again, err)
	}
	database.DB.Model(&models.BehaviorEvent{}).Where("user_id = ? AND event_type = ?", u.ID, "plan").Count(&n)
	if n != 2 {
		t.Fatalf("replace retries duplicated feedback: %d", n)
	}
	in.DishIDs = []uint{}
	if result, err := ReplaceMealPlan(u.ID, in); err != nil || len(result) != 0 {
		t.Fatalf("clear slot failed: %v", err)
	}
	database.DB.Model(&models.ShoppingCheck{}).Where("user_id = ?", u.ID).Count(&n)
	if n != 1 {
		t.Fatalf("clear slot lost actual shopping: %d", n)
	}
}

func TestReplaceMealPlanRollsBackOnFeedbackFailure(t *testing.T) {
	u, _, _ := testutil.NewUser(t.Name() + "@qq.com")
	a := planTestDish(t, u.ID, "保留菜单", "番茄")
	b := planTestDish(t, u.ID, "新菜单", "土豆")
	in := MealPlanReplaceInput{MealDate: healthToday(), MealType: "dinner", DishIDs: []uint{a.ID}}
	if _, err := ReplaceMealPlan(u.ID, in); err != nil {
		t.Fatal(err)
	}
	db := database.DB
	name := "meal_plan_feedback_failure"
	if err := db.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "behavior_events" {
			tx.AddError(errors.New("test feedback failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer db.Callback().Create().Remove(name)
	in.DishIDs = []uint{b.ID}
	if _, err := ReplaceMealPlan(u.ID, in); err == nil {
		t.Fatal("expected replacement failure")
	}
	plans, err := ListMealPlans(u.ID, healthToday(), healthToday())
	if err != nil || len(plans) != 1 || plans[0].DishID != a.ID {
		t.Fatalf("partial replacement committed: %+v %v", plans, err)
	}
}

func TestTomorrowNeverResurrectsExplicitExclusions(t *testing.T) {
	u, _, _ := testutil.NewUser(t.Name() + "@qq.com")
	ids := []uint{}
	if err := database.DB.Model(&models.Dish{}).Scopes(database.VisibleDishes(u.ID)).Pluck("id", &ids).Error; err != nil {
		t.Fatal(err)
	}
	for _, profile := range []string{"balanced", "quick", "light", "spicy", "favorite"} {
		items, err := PickTomorrowDishes(u.ID, TomorrowPickOptions{Profile: profile, Count: 10, ExcludeIDs: ids})
		if err != nil || len(items) != 0 {
			t.Fatalf("%s exclusions relaxed: %+v %v", profile, items, err)
		}
	}
}

func TestWeekPlanReservesExplicitChoiceAndAvoidsSameDayDuplicates(t *testing.T) {
	u, _, _ := testutil.NewUser(t.Name() + "@qq.com")
	d := planTestDish(t, u.ID, "固定的午餐", "番茄")
	if _, err := AcceptMealPlan(u.ID, MealPlanInput{DishID: d.ID, MealDate: healthToday(), MealType: "lunch"}); err != nil {
		t.Fatal(err)
	}
	if err := database.SetUserSetting(u.ID, "lunch_dishes_per_day", "3"); err != nil {
		t.Fatal(err)
	}
	if err := database.SetUserSetting(u.ID, "dinner_dishes_per_day", "3"); err != nil {
		t.Fatal(err)
	}
	for pass := 0; pass < 2; pass++ {
		plan, err := GenerateWeekPlan(u.ID)
		if err != nil || len(plan.Days) != 7 {
			t.Fatalf("generate week: %+v %v", plan, err)
		}
		found := false
		for _, day := range plan.Days {
			seen := map[uint]bool{}
			for _, meal := range [][]models.Dish{day.Breakfast, day.Lunch, day.Dinner} {
				for _, dish := range meal {
					if seen[dish.ID] {
						t.Fatalf("same day duplicate on %s: %d", day.Date, dish.ID)
					}
					seen[dish.ID] = true
				}
			}
			if day.Date == healthToday() {
				found = len(day.Lunch) == 1 && day.Lunch[0].ID == d.ID
			}
		}
		if !found {
			t.Fatal("generated menu displaced explicit choice")
		}
	}
}
