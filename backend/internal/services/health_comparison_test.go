package services

import (
	"ninimenu/internal/models"
	"ninimenu/internal/testutil"
	"testing"
	"time"
)

func comparisonFixture(end time.Time, period int, value float64) *HealthReport {
	r := &HealthReport{PeriodDays: period, From: end.AddDate(0, 0, 1-period).Format("2006-01-02"), To: end.Format("2006-01-02")}
	for i := 0; i < period; i++ {
		r.Days = append(r.Days, HealthDay{Date: end.AddDate(0, 0, -i).Format("2006-01-02"), Status: "complete", ItemCount: 1, Evidence: []HealthEvidence{{Source: "journal", Nutrition: &models.NutritionSnapshot{CalculationVersion: NutritionCalculationVersion, Consumed: models.NutrientValues{EnergyKcal: nutrient(value), FatG: nutrient(0)}}}}})
		r.ItemCount++
	}
	enrichNutritionMetrics(r)
	return r
}

func TestHealthComparisonWeekdayCoverageAndZero(t *testing.T) {
	end := time.Date(2026, 1, 3, 0, 0, 0, 0, healthLocation)
	current, previous := comparisonFixture(end, 7, 100), comparisonFixture(end.AddDate(0, 0, -7), 7, 0)
	got := compareHealthPeriods(current, previous)
	if got.To != "2025-12-27" || got.From != "2025-12-21" {
		t.Fatalf("range: %+v", got)
	}
	energy := got.Nutrients[0]
	if energy.MatchedDays != 7 || *energy.PreviousAverage != 0 || *energy.Delta != 100 {
		t.Fatalf("zero comparison: %+v", energy)
	}
	if got.Nutrients[4].Delta != nil {
		t.Fatal("missing fiber compared as zero")
	}
	for i := 0; i < 3; i++ {
		previous.Days[i].Status = "partial"
	}
	if got := compareHealthPeriods(current, previous).Nutrients[0]; got.MatchedDays != 4 || got.Delta == nil {
		t.Fatalf("4 day threshold: %+v", got)
	}
	previous.Days[3].Evidence[0].Nutrition.CalculationVersion = "future-version"
	if got := compareHealthPeriods(current, previous).Nutrients[0]; got.MatchedDays != 3 || got.Delta != nil || got.CurrentAverage != nil {
		t.Fatalf("mixed versions accepted: %+v", got)
	}
	// Enough days in both periods alone is insufficient when weekday overlap is only one day.
	current, previous = comparisonFixture(end, 7, 100), comparisonFixture(end.AddDate(0, 0, -7), 7, 50)
	for i := 4; i < 7; i++ {
		current.Days[i].Status = "partial"
	}
	for i := 0; i < 3; i++ {
		previous.Days[i].Status = "partial"
	}
	if got := compareHealthPeriods(current, previous).Nutrients[0]; got.MatchedDays != 1 || got.Delta != nil {
		t.Fatal("unmatched weekdays compared")
	}
}

func TestHealthComparisonThirtyDaysUsesRecentWeekdayPairs(t *testing.T) {
	end := time.Date(2024, 3, 15, 0, 0, 0, 0, healthLocation)
	current, previous := comparisonFixture(end, 30, 150), comparisonFixture(end.AddDate(0, 0, -30), 30, 100)
	got := compareHealthPeriods(current, previous).Nutrients[0]
	if got.RequiredDays != 18 || got.MatchedDays != 28 || *got.Delta != 50 {
		t.Fatalf("30 day pairing: %+v", got)
	}
	for i, date := range got.CurrentDates {
		a, _ := time.Parse("2006-01-02", date)
		b, _ := time.Parse("2006-01-02", got.PreviousDates[i])
		if a.Weekday() != b.Weekday() || date < current.From || got.PreviousDates[i] > previous.To {
			t.Fatal("invalid date pairing")
		}
	}
	// Adding an unknown recipe item prevents an otherwise complete day from qualifying.
	for i := 0; i < 15; i++ {
		current.Days[i].ItemCount++
	}
	if got := compareHealthPeriods(current, previous).Nutrients[0]; got.Delta != nil {
		t.Fatal("partial nutrient coverage compared")
	}
}

func TestHealthComparisonReadsPreviousPeriodAndInvalidates(t *testing.T) {
	user, _, err := testutil.NewUser("health-comparison@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := testutil.NewUser("health-comparison-other@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	food, err := SaveNutritionFood(user.ID, 0, NutritionFoodInput{Name: "测试标签", BasisUnit: "g", FoodState: "as_sold", SourceReference: "测试包装", Nutrients: models.NutrientValues{EnergyKcal: nutrient(50)}})
	if err != nil {
		t.Fatal(err)
	}
	var latest *models.FoodJournalEntry
	now := healthNow()
	for _, offset := range []int{0, 1, 2, 3, 7, 8, 9, 10} {
		amount := 100.0
		if offset < 7 {
			amount = 200
		}
		entry, err := CreateFoodJournalEntry(user.ID, FoodJournalInput{MealDate: now.AddDate(0, 0, -offset).Format("2006-01-02"), MealType: "lunch", DishName: "测试餐", NutritionMode: "replace", NutritionFoodID: food.ID, NutritionAmount: &amount, NutritionUnit: "g", FoodState: "as_sold", PortionSource: "measured"})
		if err != nil {
			t.Fatal(err)
		}
		if offset == 0 {
			latest = entry
		}
		if err := ConfirmHealthDay(user.ID, entry.MealDate, healthFingerprint([]models.FoodJournalEntry{*entry}, nil), true); err != nil {
			t.Fatal(err)
		}
	}
	report, err := BuildHealthReport(user.ID, 7)
	if err != nil {
		t.Fatal(err)
	}
	metric := report.Comparison.Nutrients[0]
	if report.ItemCount != 4 || metric.MatchedDays != 4 || metric.Delta == nil || *metric.Delta != 50 {
		t.Fatalf("window/average: %+v", report.Comparison)
	}
	if report.Comparison.To != now.AddDate(0, 0, -7).Format("2006-01-02") {
		t.Fatal("periods overlap")
	}
	foreign, err := BuildHealthReport(other.ID, 7)
	if err != nil || foreign.Comparison.Nutrients[0].MatchedDays != 0 {
		t.Fatal("comparison leaks another user")
	}
	_, err = UpdateFoodJournalEntry(user.ID, latest.ID, FoodJournalInput{MealDate: latest.MealDate, MealType: latest.MealType, DishName: latest.DishName, Notes: "补充说明", NutritionMode: "keep"})
	if err != nil {
		t.Fatal(err)
	}
	report, err = BuildHealthReport(user.ID, 7)
	if err != nil {
		t.Fatal(err)
	}
	if report.Comparison.Nutrients[0].Delta != nil || report.Comparison.Nutrients[0].MatchedDays != 3 {
		t.Fatal("stale day confirmation still used")
	}
}
