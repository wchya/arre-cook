package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"

	"gorm.io/gorm"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"ninimenu/internal/testutil"
)

func mealStatusReport(t *testing.T, uid uint) *HealthReport {
	t.Helper()
	r, err := BuildHealthReport(uid, 7)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestMealOmissionProjectionConflictAndSummary(t *testing.T) {
	u, _, err := testutil.NewUser(t.Name() + "@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := testutil.NewUser(t.Name() + "-other@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	date := healthToday()
	before := mealStatusReport(t, u.ID).Days[6]
	if err := SetHealthMealOmission(u.ID, date, "breakfast", before.Fingerprint, true); err != nil {
		t.Fatal(err)
	}
	r := mealStatusReport(t, u.ID)
	day := r.Days[6]
	if day.Status != "partial" || day.Meals[0].Status != "not_eaten" || day.Fingerprint == before.Fingerprint || r.ItemCount != 0 || r.MealEventCount != 0 || r.LoggedDays != 0 || r.NotEatenMeals != 1 {
		t.Fatalf("omission counted as food: %+v", r)
	}
	for _, n := range r.Nutrients {
		if n.KnownTotal != nil || n.DailyAverage != nil {
			t.Fatal("omission manufactured zero intake")
		}
	}
	if err := ConfirmHealthDay(u.ID, date, day.Fingerprint, true); err == nil {
		t.Fatal("omissions-only day confirmed as intake")
	}
	if err := SetHealthMealOmission(u.ID, date, "breakfast", before.Fingerprint, false); !errors.Is(err, ErrHealthDayChanged) {
		t.Fatalf("stale status accepted: %v", err)
	}
	if r := mealStatusReport(t, other.ID); r.NotEatenMeals != 0 || r.Days[6].Status != "unknown" {
		t.Fatal("cross-user omission exposed")
	}
	if err := SetHealthMealOmission(other.ID, date, "breakfast", day.Fingerprint, false); !errors.Is(err, ErrHealthDayChanged) {
		t.Fatal("other user's fingerprint accepted")
	}
	if _, err := CreateFoodJournalEntry(u.ID, FoodJournalInput{MealDate: date, MealType: "lunch", DishName: "米饭"}); err != nil {
		t.Fatal(err)
	}
	r = mealStatusReport(t, u.ID)
	if err := SetHealthMealOmission(u.ID, date, "lunch", r.Days[6].Fingerprint, true); !errors.Is(err, ErrHealthDayChanged) {
		t.Fatal("actual lunch overwritten")
	}
	if err := ConfirmHealthDay(u.ID, date, r.Days[6].Fingerprint, true); err != nil {
		t.Fatal(err)
	}
	r = mealStatusReport(t, u.ID)
	summary, err := BuildHealthSummary(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if summary.From != r.From || summary.To != r.To || summary.LoggedDays != r.LoggedDays || summary.CompleteDays != 1 || summary.ItemCount != r.ItemCount || summary.MealEventCount != r.MealEventCount || summary.NotEatenMeals != 1 {
		t.Fatalf("summary differs from report: %+v", summary)
	}
	if err := SetHealthMealOmission(u.ID, date, "breakfast", r.Days[6].Fingerprint, false); err != nil {
		t.Fatal(err)
	}
	r = mealStatusReport(t, u.ID)
	if r.NotEatenMeals != 0 || r.CompleteDays != 0 || r.Days[6].Meals[0].Status != "unknown" {
		t.Fatal("undo retained confirmation")
	}
}

func TestActualWritesPermanentlyClearOmissions(t *testing.T) {
	for _, method := range []string{"journal", "move-journal", "recipe"} {
		t.Run(method, func(t *testing.T) {
			u, _, err := testutil.NewUser(t.Name() + "@qq.com")
			if err != nil {
				t.Fatal(err)
			}
			date := healthToday()
			var entry *models.FoodJournalEntry
			if method == "move-journal" {
				entry, err = CreateFoodJournalEntry(u.ID, FoodJournalInput{MealDate: date, MealType: "lunch", DishName: "鸡蛋"})
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := SetHealthMealOmission(u.ID, date, "breakfast", mealStatusReport(t, u.ID).Days[6].Fingerprint, true); err != nil {
				t.Fatal(err)
			}
			var remove func() error
			in := FoodJournalInput{MealDate: date, MealType: "breakfast", DishName: "鸡蛋"}
			switch method {
			case "journal":
				entry, err = CreateFoodJournalEntry(u.ID, in)
			case "move-journal":
				entry, err = UpdateFoodJournalEntry(u.ID, entry.ID, in)
			case "recipe":
				dish := models.Dish{OwnerID: u.ID, Name: "鸡蛋", Enabled: true, MealType: "all", Ingredients: "[]"}
				if err := database.DB.Create(&dish).Error; err != nil {
					t.Fatal(err)
				}
				var record *models.MealRecord
				record, err = CreateMealRecord(u.ID, MealInput{DishID: dish.ID, MealType: "breakfast", MealDate: date}, "agent", "")
				if err == nil {
					remove = func() error { _, err := DeleteMealRecord(u.ID, record.ID); return err }
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			if method != "recipe" {
				remove = func() error { return DeleteFoodJournalEntry(u.ID, entry.ID) }
			}
			r := mealStatusReport(t, u.ID)
			if r.NotEatenMeals != 0 || r.Days[6].Meals[0].Status != "recorded" || r.ItemCount != 1 {
				t.Fatal("actual record did not replace omission")
			}
			if err := remove(); err != nil {
				t.Fatal(err)
			}
			if r := mealStatusReport(t, u.ID); r.NotEatenMeals != 0 || r.Days[6].Meals[0].Status != "unknown" {
				t.Fatal("deleted record resurrected omission")
			}
		})
	}
}

func TestOmissionClearRollsBackWhenActualWriteFails(t *testing.T) {
	u, _, err := testutil.NewUser(t.Name() + "@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	if err := SetHealthMealOmission(u.ID, healthToday(), "breakfast", mealStatusReport(t, u.ID).Days[6].Fingerprint, true); err != nil {
		t.Fatal(err)
	}
	db := database.DB
	name := "test_omission_rollback"
	if err := db.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "food_journal_entries" {
			tx.AddError(errors.New("write failed"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer db.Callback().Create().Remove(name)
	if _, err := CreateFoodJournalEntry(u.ID, FoodJournalInput{MealDate: healthToday(), MealType: "breakfast", DishName: "鸡蛋"}); err == nil {
		t.Fatal("expected write failure")
	}
	if r := mealStatusReport(t, u.ID); r.NotEatenMeals != 1 || r.ItemCount != 0 {
		t.Fatal("omission removal escaped rollback")
	}
}

func TestLegacyHealthFingerprintWithoutOmissions(t *testing.T) {
	u, _, err := testutil.NewUser(t.Name() + "@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	entry, err := CreateFoodJournalEntry(u.ID, FoodJournalInput{MealDate: healthToday(), MealType: "lunch", DishName: "米饭"})
	if err != nil {
		t.Fatal(err)
	}
	js := []models.FoodJournalEntry{*entry}
	rs := []models.MealRecord{}
	raw, _ := json.Marshal(struct {
		Journal []models.FoodJournalEntry
		Records []models.MealRecord
	}{js, rs})
	sum := sha256.Sum256(raw)
	old := hex.EncodeToString(sum[:])
	if got := healthFingerprint(js, rs); got != old {
		t.Fatal("legacy fingerprint changed")
	}
	if err := database.DB.Create(&models.HealthDayConfirmation{UserID: u.ID, MealDate: healthToday(), Fingerprint: old}).Error; err != nil {
		t.Fatal(err)
	}
	if mealStatusReport(t, u.ID).CompleteDays != 1 {
		t.Fatal("legacy confirmation invalidated")
	}
}

func TestBreakfastPlanLifecycle(t *testing.T) {
	u, _, err := testutil.NewUser(t.Name() + "@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	dish := models.Dish{OwnerID: u.ID, Name: "早餐鸡蛋", Enabled: true, MealType: "all", Ingredients: `[{"name":"鸡蛋","amount":"2个"}]`}
	db := database.DB
	if err := db.Create(&dish).Error; err != nil {
		t.Fatal(err)
	}
	for _, d := range GetCachedWeekPlan(u.ID).Days {
		if len(d.Breakfast) != 0 {
			t.Fatal("breakfast generated without explicit choice")
		}
	}
	in := HealthPlanInput{DishID: dish.ID, MealDate: healthToday(), MealType: "breakfast", PeriodDays: 7}
	p, err := AcceptHealthPlan(u.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	again, err := AcceptHealthPlan(u.ID, in)
	if err != nil || again.ID != p.ID {
		t.Fatalf("non-idempotent breakfast adoption: %v", err)
	}
	for i := 0; i < 2; i++ {
		found := false
		for _, d := range GetCachedWeekPlan(u.ID).Days {
			if d.Date == in.MealDate {
				found = len(d.Breakfast) == 1 && d.Breakfast[0].ID == dish.ID
			}
		}
		if !found {
			t.Fatal("breakfast missing on cache read")
		}
	}
	r := mealStatusReport(t, u.ID)
	if r.ItemCount != 0 || len(r.Plans) != 1 || r.Plans[0].Status != "planned" {
		t.Fatal("breakfast plan counted as actual intake")
	}
	if _, err := CreateMealRecord(u.ID, MealInput{DishID: dish.ID, MealDate: in.MealDate, MealType: in.MealType}, "app", ""); err != nil {
		t.Fatal(err)
	}
	r = mealStatusReport(t, u.ID)
	if r.Plans[0].Status != "recorded" || r.Days[6].Meals[0].Status != "recorded" {
		t.Fatal("breakfast consumption not reflected")
	}
	if err := CancelHealthPlan(u.ID, p.ID); err != nil {
		t.Fatal(err)
	}
	for _, model := range []any{&models.MealRecord{}, &models.ShoppingCheck{}} {
		var n int64
		if err := db.Model(model).Where("user_id = ?", u.ID).Count(&n).Error; err != nil || n != 1 {
			t.Fatalf("cancel lost actual data or duplicated procurement: %d %v", n, err)
		}
	}
}
