package services

import (
	"errors"
	"fmt"
	"gorm.io/gorm"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"ninimenu/internal/testutil"
	"testing"
)

func TestHealthReportEvidenceConfirmationAndLink(t *testing.T) {
	u, _, err := testutil.NewUser("health-evidence@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	other, _, _ := testutil.NewUser("health-evidence-other@qq.com")
	db := database.DB
	dish := models.Dish{OwnerID: u.ID, Name: "番茄鸡蛋", Enabled: true, MealType: "all", Ingredients: `[{"name":"鸡蛋","amount":"2个"}]`}
	if err := db.Create(&dish).Error; err != nil {
		t.Fatal(err)
	}
	r, err := CreateMealRecord(u.ID, MealInput{DishID: dish.ID, MealDate: healthToday(), MealType: "lunch"}, "app", "", db)
	if err != nil {
		t.Fatal(err)
	}
	in := FoodJournalInput{MealDate: healthToday(), MealType: "lunch", DishName: dish.Name, Portion: "半盘", FoodGroups: []string{"vegetable", "protein"}, RequestKey: "test-retry"}
	j, err := CreateFoodJournalEntry(u.ID, in, db)
	if err != nil {
		t.Fatal(err)
	}
	again, err := CreateFoodJournalEntry(u.ID, in, db)
	if err != nil || j.ID != again.ID {
		t.Fatalf("retry: %v %+v", err, again)
	}
	in.DishName = "changed"
	if _, err := CreateFoodJournalEntry(u.ID, in, db); err == nil {
		t.Fatal("idempotency key reused for different body")
	}
	in.DishName = dish.Name
	report, err := BuildHealthReport(u.ID, 7, db)
	if err != nil {
		t.Fatal(err)
	}
	if report.PortionCoverage.TextOnlyItems != 1 || report.PortionCoverage.UnknownItems != 1 {
		t.Fatal("unlinked records must remain separate in coverage")
	}
	last := report.Days[6]
	if report.MealCount != 2 || report.MealEventCount != 1 || report.ItemCount != 2 || len(last.Evidence[0].PossibleDuplicateIDs) != 1 {
		t.Fatalf("bad projection: %+v", report)
	}
	if err := ConfirmHealthDay(u.ID, healthToday(), last.Fingerprint, true, db); err != nil {
		t.Fatal(err)
	}
	report, _ = BuildHealthReport(u.ID, 7, db)
	if report.CompleteDays != 1 {
		t.Fatal("confirmation lost")
	}
	in.LinkedRecordID = &r.ID
	if _, err := UpdateFoodJournalEntry(other.ID, j.ID, in, db); err == nil {
		t.Fatal("cross user update")
	}
	if _, err := CreateFoodJournalEntry(other.ID, in, db); err == nil {
		t.Fatal("cross user link")
	}
	if _, err := UpdateFoodJournalEntry(u.ID, j.ID, in, db); err != nil {
		t.Fatal(err)
	}
	report, _ = BuildHealthReport(u.ID, 7, db)
	if report.PortionCoverage.TextOnlyItems != 1 || report.PortionCoverage.UnknownItems != 0 {
		t.Fatal("linked source double counted in portion coverage")
	}
	if report.CompleteDays != 0 || report.ItemCount != 1 || report.MealCount != 2 || report.MealEventCount != 1 {
		t.Fatalf("link projection: %+v", report)
	}
	if err := ConfirmHealthDay(u.ID, healthToday(), last.Fingerprint, true, db); err == nil {
		t.Fatal("stale fingerprint accepted")
	}
	if err := ConfirmHealthDay(u.ID, healthToday(), report.Days[6].Fingerprint, true, db); err != nil {
		t.Fatal(err)
	}
	if _, err := DeleteMealRecord(u.ID, r.ID, db); err != nil {
		t.Fatal(err)
	}
	report, _ = BuildHealthReport(u.ID, 7, db)
	if report.CompleteDays != 0 || report.ItemCount != 1 {
		t.Fatal("source deletion did not invalidate confirmation")
	}
	if err := ConfirmHealthDay(other.ID, healthToday(), report.Days[6].Fingerprint, true, db); err == nil {
		t.Fatal("empty other user day confirmed")
	}
}

func TestHealthReportFullRangeAndFutureExclusion(t *testing.T) {
	u, _, _ := testutil.NewUser("health-full-range@qq.com")
	entries := make([]models.FoodJournalEntry, 510)
	for i := range entries {
		entries[i] = models.FoodJournalEntry{UserID: u.ID, MealDate: healthToday(), MealType: "lunch", DishName: fmt.Sprint("food", i), FoodGroups: []string{"vegetable"}}
	}
	if err := database.DB.CreateInBatches(entries, 100).Error; err != nil {
		t.Fatal(err)
	}
	future := healthNow().AddDate(0, 0, 1).Format("2006-01-02")
	if _, err := CreateFoodJournalEntry(u.ID, FoodJournalInput{MealDate: future, MealType: "lunch", DishName: "future"}); err == nil {
		t.Fatal("future journal allowed")
	}
	database.DB.Create(&models.MealRecord{UserID: u.ID, DishID: 1, DishName: "future", MealDate: future, MealType: "dinner"})
	report, err := BuildHealthReport(u.ID, 7)
	if err != nil {
		t.Fatal(err)
	}
	if report.MealCount != 510 || report.ItemCount != 510 || report.MealEventCount != 1 || report.FoodGroupDays["vegetable"] != 1 {
		t.Fatalf("truncated or future included: %+v", report)
	}
	for _, key := range []string{"snack-a", "snack-a", "snack-b"} {
		_, err := CreateFoodJournalEntry(u.ID, FoodJournalInput{MealDate: healthToday(), MealType: "snack", DishName: "水果", EventKey: key})
		if err != nil {
			t.Fatal(err)
		}
	}
	report, _ = BuildHealthReport(u.ID, 7)
	if report.MealEventCount != 3 {
		t.Fatal("explicit snack events not grouped")
	}
}

func TestHealthPlanShoppingAndRecordsLifecycle(t *testing.T) {
	u, _, _ := testutil.NewUser("health-plan@qq.com")
	other, _, _ := testutil.NewUser("health-plan-other@qq.com")
	db := database.DB
	dish := models.Dish{OwnerID: u.ID, Name: "计划鸡蛋", Enabled: true, MealType: "all", Ingredients: `[{"name":"鸡蛋","amount":"2个"}]`, Seasonings: "[]"}
	db.Create(&dish)
	in := HealthPlanInput{DishID: dish.ID, MealDate: healthToday(), MealType: "dinner", PeriodDays: 7}
	p, err := AcceptHealthPlan(u.ID, in, db)
	if err != nil {
		t.Fatal(err)
	}
	again, err := AcceptHealthPlan(u.ID, in, db)
	if err != nil || p.ID != again.ID {
		t.Fatalf("plan retry %v", err)
	}
	var n int64
	db.Model(&models.ShoppingCheck{}).Where("user_id = ?", u.ID).Count(&n)
	if n != 1 {
		t.Fatalf("shopping duplicated: %d", n)
	}
	report, _ := BuildHealthReport(u.ID, 7, db)
	if report.MealCount != 0 || len(report.Plans) != 1 || report.Plans[0].Status != "planned" {
		t.Fatal("plan counted as eaten")
	}
	if _, err := AcceptHealthPlan(other.ID, in, db); err == nil {
		t.Fatal("private dish planned by other")
	}
	if err := CancelHealthPlan(other.ID, p.ID, db); err == nil {
		t.Fatal("cross user cancel")
	}
	plan := GetCachedWeekPlan(u.ID, db)
	found := false
	for _, d := range plan.Days {
		if d.Date == in.MealDate {
			found = len(d.Dinner) > 0 && d.Dinner[0].ID == dish.ID
		}
	}
	if !found {
		t.Fatal("explicit plan missing from week menu")
	}
	r, err := CreateMealRecord(u.ID, MealInput{DishID: dish.ID, MealDate: in.MealDate, MealType: in.MealType}, "app", "", db)
	if err != nil {
		t.Fatal(err)
	}
	db.Model(&models.ShoppingCheck{}).Where("user_id = ?", u.ID).Count(&n)
	if n != 1 {
		t.Fatal("eating planned dish duplicated shopping")
	}
	report, _ = BuildHealthReport(u.ID, 7, db)
	if report.Plans[0].Status != "recorded" {
		t.Fatal("actual record not reflected")
	}
	if _, err := DeleteMealRecord(u.ID, r.ID, db); err != nil {
		t.Fatal(err)
	}
	db.Model(&models.ShoppingCheck{}).Where("user_id = ?", u.ID).Count(&n)
	if n != 1 {
		t.Fatal("record deletion removed planned shopping")
	}
	if err := CancelHealthPlan(u.ID, p.ID, db); err != nil {
		t.Fatal(err)
	}
	db.Model(&models.ShoppingCheck{}).Where("user_id = ?", u.ID).Count(&n)
	if n != 0 {
		t.Fatal("cancel retained orphan shopping")
	}
}

func TestHealthPlanRollsBackShoppingFailure(t *testing.T) {
	u, _, _ := testutil.NewUser("health-plan-rollback@qq.com")
	db := database.DB
	dish := models.Dish{OwnerID: u.ID, Name: "rollback meal", Enabled: true, MealType: "all", Ingredients: `[{"name":"鸡蛋","amount":"2个"}]`}
	if err := db.Create(&dish).Error; err != nil {
		t.Fatal(err)
	}
	name := "health_plan_shopping_failure"
	if err := db.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "shopping_checks" {
			tx.AddError(errors.New("test shopping failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer db.Callback().Create().Remove(name)
	if _, err := AcceptHealthPlan(u.ID, HealthPlanInput{DishID: dish.ID, MealDate: healthToday(), MealType: "lunch", PeriodDays: 7}, db); err == nil {
		t.Fatal("expected shopping failure")
	}
	var n int64
	db.Model(&models.HealthPlanItem{}).Where("user_id = ?", u.ID).Count(&n)
	if n != 0 {
		t.Fatal("plan escaped rolled back shopping transaction")
	}
}

func TestHealthPlanCancelPreservesActualMeal(t *testing.T) {
	u, _, _ := testutil.NewUser("health-plan-preserve@qq.com")
	db := database.DB
	dish := models.Dish{OwnerID: u.ID, Name: "preserve meal", Enabled: true, MealType: "all", Ingredients: `[{"name":"鸡蛋","amount":"2个"}]`}
	db.Create(&dish)
	p, err := AcceptHealthPlan(u.ID, HealthPlanInput{DishID: dish.ID, MealDate: healthToday(), MealType: "lunch", PeriodDays: 7}, db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CreateMealRecord(u.ID, MealInput{DishID: dish.ID, MealDate: healthToday(), MealType: "lunch"}, "app", "", db); err != nil {
		t.Fatal(err)
	}
	if err := CancelHealthPlan(u.ID, p.ID, db); err != nil {
		t.Fatal(err)
	}
	var n int64
	db.Model(&models.MealRecord{}).Where("user_id = ?", u.ID).Count(&n)
	if n != 1 {
		t.Fatal("cancel deleted actual consumption")
	}
	db.Model(&models.ShoppingCheck{}).Where("user_id = ?", u.ID).Count(&n)
	if n != 1 {
		t.Fatal("cancel deleted actual meal shopping")
	}
}

func TestHealthPortionCoverageKeepsMeasurementAndEstimationSeparate(t *testing.T) {
	user, _, err := testutil.NewUser(t.Name() + "@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	snapshots := []*models.NutritionSnapshot{
		{PortionSource: "measured", Amount: 100},
		{PortionSource: "estimated", Amount: 100},
		{PortionSource: "measured", Amount: 100, StandardPortion: &models.PortionSnapshot{CatalogPortion: models.CatalogPortion{Key: "bowl", Amount: 200}, Count: 0.5}},
		{PortionSource: "measured", Amount: 100, Recipe: &models.NutritionRecipe{YieldG: 200}},
		nil, nil,
	}
	for i, snap := range snapshots {
		entry := models.FoodJournalEntry{UserID: user.ID, MealDate: healthToday(), MealType: "lunch", DishName: fmt.Sprint("food", i), Nutrition: snap}
		if i == 4 {
			entry.Portion = "半碗"
		}
		if err := database.DB.Create(&entry).Error; err != nil {
			t.Fatal(err)
		}
	}
	future := models.FoodJournalEntry{UserID: user.ID, MealDate: healthNow().AddDate(0, 0, 1).Format("2006-01-02"), MealType: "lunch", DishName: "future", Nutrition: snapshots[0]}
	if err := database.DB.Create(&future).Error; err != nil {
		t.Fatal(err)
	}
	report, err := BuildHealthReport(user.ID, 7)
	if err != nil {
		t.Fatal(err)
	}
	expected := HealthPortionCoverage{MeasuredItems: 1, StandardItems: 1, EstimatedItems: 2, TextOnlyItems: 1, UnknownItems: 1}
	if report.PortionCoverage != expected || report.ItemCount != 6 {
		t.Fatalf("unexpected coverage: %+v", report.PortionCoverage)
	}
	if report.PortionKnownItems != 1 {
		t.Fatal("legacy text portion metric changed meaning")
	}
}
