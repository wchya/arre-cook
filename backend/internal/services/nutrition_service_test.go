package services

import (
	"ninimenu/internal/models"
	"ninimenu/internal/testutil"
	"testing"
)

func nutrient(v float64) *float64 { return &v }
func TestNutritionSnapshotUnitsAndVersions(t *testing.T) {
	u, _, _ := testutil.NewUser("nutrition-snapshot@qq.com")
	other, _, _ := testutil.NewUser("nutrition-snapshot-other@qq.com")
	label := NutritionFoodInput{Name: "示例包装食物", BasisUnit: "g", FoodState: "ready_to_eat", SourceReference: "测试包装标签，每100克", Nutrients: models.NutrientValues{EnergyKcal: nutrient(200), ProteinG: nutrient(10), FatG: nutrient(0)}}
	food, err := SaveNutritionFood(u.ID, 0, label)
	if err != nil {
		t.Fatal(err)
	}
	in := FoodJournalInput{MealDate: healthToday(), MealType: "lunch", DishName: "示例食物", NutritionMode: "replace", NutritionFoodID: food.ID, NutritionAmount: nutrient(50), NutritionUnit: "g", FoodState: "ready_to_eat", PortionSource: "measured"}
	entry, err := CreateFoodJournalEntry(u.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	if entry.Nutrition == nil || *entry.Nutrition.Consumed.EnergyKcal != 100 || *entry.Nutrition.Consumed.ProteinG != 5 || entry.Nutrition.Consumed.FiberG != nil || *entry.Nutrition.Consumed.FatG != 0 {
		t.Fatalf("bad golden calculation: %+v", entry.Nutrition)
	}
	if _, err := CreateFoodJournalEntry(other.ID, in); err == nil {
		t.Fatal("cross-user food allowed")
	}
	wrong := in
	wrong.NutritionUnit = "ml"
	if _, err := CreateFoodJournalEntry(u.ID, wrong); err == nil {
		t.Fatal("mass/volume conversion inferred")
	}
	wrong = in
	wrong.FoodState = "raw"
	if _, err := CreateFoodJournalEntry(u.ID, wrong); err == nil {
		t.Fatal("raw/cooked conversion inferred")
	}
	wrong = in
	wrong.NutritionAmount = nutrient(-1)
	if _, err := CreateFoodJournalEntry(u.ID, wrong); err == nil {
		t.Fatal("negative quantity allowed")
	}
	label.Version = food.Version
	label.Nutrients.EnergyKcal = nutrient(300)
	updated, err := SaveNutritionFood(u.ID, food.ID, label)
	if err != nil || updated.Version != 2 {
		t.Fatalf("food update: %v", err)
	}
	if _, err := SaveNutritionFood(u.ID, food.ID, label); err == nil {
		t.Fatal("stale label version accepted")
	}
	report, err := BuildHealthReport(u.ID, 7)
	if err != nil {
		t.Fatal(err)
	}
	if *report.Nutrients[0].KnownTotal != 100 || report.Nutrients[0].DailyAverage != nil || report.Nutrients[4].KnownTotal != nil || *report.Nutrients[3].KnownTotal != 0 {
		t.Fatalf("snapshot changed or missing confused with zero: %+v", report.Nutrients)
	}
	if err := DeleteNutritionFood(u.ID, food.ID); err != nil {
		t.Fatal(err)
	}
	report, err = BuildHealthReport(u.ID, 7)
	if err != nil || *report.Nutrients[0].KnownTotal != 100 {
		t.Fatal("deleting label rewrote history")
	}
	in.NutritionMode = "keep"
	in.Notes = "备注修改"
	kept, err := UpdateFoodJournalEntry(u.ID, entry.ID, in)
	if err != nil || kept.Nutrition == nil {
		t.Fatalf("notes discarded snapshot: %v", err)
	}
	in.Portion = "份量已改变"
	cleared, err := UpdateFoodJournalEntry(u.ID, entry.ID, in)
	if err != nil || cleared.Nutrition != nil {
		t.Fatal("changing portion retained stale nutrition")
	}
}

func TestNutritionDailyAverageRequiresCompleteCoverage(t *testing.T) {
	u, _, _ := testutil.NewUser("nutrition-average@qq.com")
	food, err := SaveNutritionFood(u.ID, 0, NutritionFoodInput{Name: "测试食品", BasisUnit: "ml", FoodState: "as_sold", SourceReference: "测试标签", Nutrients: models.NutrientValues{EnergyKcal: nutrient(40), ProteinG: nutrient(3)}})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		date := healthNow().AddDate(0, 0, -i).Format("2006-01-02")
		_, err := CreateFoodJournalEntry(u.ID, FoodJournalInput{MealDate: date, MealType: "breakfast", DishName: "测试食品", NutritionMode: "replace", NutritionFoodID: food.ID, NutritionAmount: nutrient(250), NutritionUnit: "ml", FoodState: "as_sold", PortionSource: "measured"})
		if err != nil {
			t.Fatal(err)
		}
		report, err := BuildHealthReport(u.ID, 7)
		if err != nil {
			t.Fatal(err)
		}
		for _, day := range report.Days {
			if day.Date == date {
				if err := ConfirmHealthDay(u.ID, date, day.Fingerprint, true); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	report, err := BuildHealthReport(u.ID, 7)
	if err != nil {
		t.Fatal(err)
	}
	if report.Nutrients[0].DailyAverage == nil || *report.Nutrients[0].DailyAverage != 100 || report.Nutrients[0].CompleteDays != 4 {
		t.Fatalf("average gate: %+v", report.Nutrients[0])
	}
	if report.Nutrients[4].DailyAverage != nil {
		t.Fatal("unknown fiber became average zero")
	}
	_, err = CreateFoodJournalEntry(u.ID, FoodJournalInput{MealDate: healthToday(), MealType: "lunch", DishName: "未提供营养的午餐"})
	if err != nil {
		t.Fatal(err)
	}
	report, _ = BuildHealthReport(u.ID, 7)
	if report.Nutrients[0].DailyAverage != nil || report.Nutrients[0].CoveredItems != 4 || report.Nutrients[0].TotalItems != 5 {
		t.Fatalf("incomplete day counted: %+v", report.Nutrients[0])
	}
}
