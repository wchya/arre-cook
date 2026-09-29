package services

import (
	"errors"
	"gorm.io/gorm"
	"strings"
	"testing"

	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"ninimenu/internal/testutil"
)

func TestHealthProfileVersionsErasureAndValidation(t *testing.T) {
	user, _, _ := testutil.NewUser("health-profile-versions@qq.com")
	other, _, _ := testutil.NewUser("health-profile-isolation@qq.com")
	input := HealthProfileInput{Confirmed: true, Goal: "balanced", EatingPattern: "mixed", Allergies: []string{" 花生 ", "花生"}, DietaryExclusions: []string{"猪肉"}}
	profile, err := SaveHealthProfile(user.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	if !profile.Active || profile.Version != 1 || len(profile.Allergies) != 1 || profile.Allergies[0] != "花生" || profile.ConfirmedAt == nil {
		t.Fatalf("unexpected profile: %+v", profile)
	}
	if _, err := SaveHealthProfile(user.ID, input); !errors.Is(err, ErrHealthProfileConflict) {
		t.Fatalf("stale write: %v", err)
	}
	isolated, err := LoadHealthProfile(other.ID)
	if err != nil || isolated.Active || isolated.Version != 0 || len(isolated.Allergies) != 0 {
		t.Fatal("profile isolation failed")
	}
	invalid := []HealthProfileInput{
		{Version: 1}, {Version: 1, Confirmed: true, Goal: "medical"}, {Version: 1, Confirmed: true, EatingPattern: "unknown"},
		{Version: 1, Confirmed: true, Allergies: []string{""}}, {Version: 1, Confirmed: true, DietaryExclusions: []string{strings.Repeat("字", 41)}},
		{Version: 1, Confirmed: true, Allergies: []string{"花\n生"}}, {Version: 1, Confirmed: true, Allergies: make([]string, 21)},
	}
	for _, in := range invalid {
		if _, err := SaveHealthProfile(user.ID, in); !errors.Is(err, ErrInvalidHealthProfile) {
			t.Fatalf("invalid accepted: %v", err)
		}
	}
	input.Version = 1
	input.Goal = "regular_meals"
	if _, err := SaveHealthProfile(user.ID, input); err != nil {
		t.Fatal(err)
	}
	var history []models.HealthProfileVersion
	database.DB.Where("user_id = ?", user.ID).Order("version").Find(&history)
	if len(history) != 2 || history[0].Snapshot.Goal != "balanced" || history[1].Snapshot.Goal != "regular_meals" {
		t.Fatal("immutable revisions missing")
	}
	if _, err := ClearHealthProfile(user.ID, 1); !errors.Is(err, ErrHealthProfileConflict) {
		t.Fatal("stale erase accepted")
	}
	cleared, err := ClearHealthProfile(user.ID, 2)
	if err != nil || cleared.Active || cleared.Version != 3 || cleared.ConfirmedAt != nil || cleared.Goal != "" || len(cleared.Allergies) != 0 {
		t.Fatalf("erase failed: %+v %v", cleared, err)
	}
	var n int64
	database.DB.Model(&models.HealthProfileVersion{}).Where("user_id = ?", user.ID).Count(&n)
	if n != 0 {
		t.Fatal("erasure retained private history")
	}
	input.Version = 0
	if _, err := SaveHealthProfile(user.ID, input); !errors.Is(err, ErrHealthProfileConflict) {
		t.Fatal("stale new-profile page resurrected erased profile")
	}
	input.Version = 3
	if _, err := SaveHealthProfile(user.ID, input); err != nil {
		t.Fatal(err)
	}
	prefs, err := LoadPreferences(user.ID)
	if err != nil || len(prefs.Allergies) > 0 || len(prefs.AvoidIngredients) > 0 || prefs.Goals != "" {
		t.Fatal("private health fields leaked into legacy AI-visible preferences")
	}
}

func TestHealthProfileFiltersRecommendationsPlansAndCache(t *testing.T) {
	user, _, _ := testutil.NewUser("health-profile-filter@qq.com")
	dish := models.Dish{OwnerID: user.ID, Name: "档案核验餐", Enabled: true, MealType: "all", Ingredients: `[{"name":"花生","amount":"1g"}]`, Seasonings: "[]"}
	if err := database.DB.Create(&dish).Error; err != nil {
		t.Fatal(err)
	}
	in := HealthPlanInput{DishID: dish.ID, MealDate: healthToday(), MealType: "dinner", PeriodDays: 7}
	plan, err := AcceptHealthPlan(user.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.SetUserSetting(user.ID, "week_plan_cache", "old"); err != nil {
		t.Fatal(err)
	}
	profile, err := SaveHealthProfile(user.ID, HealthProfileInput{Confirmed: true, Allergies: []string{"花生"}, DietaryExclusions: []string{"猪肉"}})
	if err != nil {
		t.Fatal(err)
	}
	if cached := database.GetUserSetting(user.ID, "week_plan_cache", "missing"); cached != "missing" {
		t.Fatal("cache not invalidated")
	}
	for _, ignore := range []bool{false, true} {
		recommended, err := RecommendDishes(user.ID, RecommendRequest{Keyword: "档案核验餐", IgnorePreferences: ignore})
		if err != nil || len(recommended.Items) != 0 {
			t.Fatalf("hard restrictions relaxed: %v %+v", err, recommended)
		}
	}
	if _, err := AcceptHealthPlan(user.ID, in); err == nil {
		t.Fatal("old plan retry bypassed changed restrictions")
	}
	plans, err := ListHealthPlans(user.ID, healthToday(), healthToday(), database.DB)
	if err != nil || len(plans) != 1 || plans[0].Available {
		t.Fatal("old plan availability not refreshed")
	}
	report, err := BuildHealthReport(user.ID, 7)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range report.Recommendations {
		if containsAny(dishSearchText(item.Dish), []string{"花生", "猪肉"}) {
			t.Fatal("report recommended excluded ingredient")
		}
	}
	week, err := GenerateWeekPlan(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, day := range week.Days {
		for _, d := range append(day.Lunch, day.Dinner...) {
			if containsAny(dishSearchText(d), []string{"花生", "猪肉"}) {
				t.Fatal("week plan ignored health profile")
			}
		}
	}
	tomorrow, err := PickTomorrowDishes(user.ID, TomorrowPickOptions{Count: 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range tomorrow {
		if containsAny(dishSearchText(d), []string{"花生", "猪肉"}) {
			t.Fatal("tomorrow ignored health profile")
		}
	}
	if _, err := ClearHealthProfile(user.ID, profile.Version); err != nil {
		t.Fatal(err)
	}
	restored, err := AcceptHealthPlan(user.ID, in)
	if err != nil || restored.ID != plan.ID {
		t.Fatal("erasure broke existing plan")
	}
}

func TestHealthProfileWriteAndEraseRollback(t *testing.T) {
	user, _, _ := testutil.NewUser("health-profile-rollback@qq.com")
	input := HealthProfileInput{Confirmed: true, Goal: "balanced", Allergies: []string{"花生"}}
	profile, err := SaveHealthProfile(user.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	db := database.DB
	name := "health_profile_revision_failure"
	if err := db.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "health_profile_versions" {
			tx.AddError(errors.New("test revision failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer db.Callback().Create().Remove(name)
	input.Version = profile.Version
	input.Goal = "regular_meals"
	if _, err := SaveHealthProfile(user.ID, input); err == nil {
		t.Fatal("expected history failure")
	}
	loaded, err := LoadHealthProfile(user.ID)
	if err != nil || loaded.Version != 1 || loaded.Goal != "balanced" {
		t.Fatal("current profile escaped revision rollback")
	}
	eraseName := "health_profile_erase_failure"
	if err := db.Callback().Delete().Before("gorm:delete").Register(eraseName, func(tx *gorm.DB) {
		if tx.Statement.Table == "health_profile_versions" {
			tx.AddError(errors.New("test erase failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer db.Callback().Delete().Remove(eraseName)
	if _, err := ClearHealthProfile(user.ID, 1); err == nil {
		t.Fatal("expected erase failure")
	}
	loaded, err = LoadHealthProfile(user.ID)
	if err != nil || !loaded.Active || loaded.Version != 1 || loaded.Allergies[0] != "花生" {
		t.Fatal("partial erase committed")
	}
}

func TestHealthProfileReadFailureDoesNotBypassFilters(t *testing.T) {
	user, _, _ := testutil.NewUser("health-profile-read-failure@qq.com")
	db := database.DB
	name := "health_profile_read_failure"
	if err := db.Callback().Query().Before("gorm:query").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "health_profiles" {
			tx.AddError(errors.New("test profile unavailable"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer db.Callback().Query().Remove(name)
	if _, err := RecommendDishes(user.ID, RecommendRequest{}); err == nil {
		t.Fatal("recommendation ignored read failure")
	}
	if _, err := GenerateWeekPlan(user.ID); err == nil {
		t.Fatal("menu ignored read failure")
	}
	if len(GetCachedWeekPlan(user.ID).Days) != 0 {
		t.Fatal("failed menu returned cached recommendations")
	}
}
