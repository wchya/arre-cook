package services

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"gorm.io/gorm"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"ninimenu/internal/testutil"
)

func TestHealthDraftBatchAtomicIdempotentAndIsolated(t *testing.T) {
	u, _, err := testutil.NewUser(t.Name() + "@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := testutil.NewUser(t.Name() + "-other@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	in := HealthJournalBatchInput{RequestKey: "draft_batch_case_1", Confirmed: true, MealDate: healthToday(), MealType: "breakfast", Items: []HealthDraftItemInput{{DishName: "鸡蛋"}, {DishName: "牛奶", Portion: "半杯"}}}
	before := mealStatusReport(t, u.ID)
	if err := SetHealthMealOmission(u.ID, in.MealDate, in.MealType, before.Days[6].Fingerprint, true); err != nil {
		t.Fatal(err)
	}
	ids, err := SaveHealthJournalBatch(u.ID, in)
	if err != nil || len(ids) != 2 {
		t.Fatalf("save: %v %v", ids, err)
	}
	again, err := SaveHealthJournalBatch(u.ID, in)
	if err != nil || !reflect.DeepEqual(ids, again) {
		t.Fatal("retry duplicated meal")
	}
	r := mealStatusReport(t, u.ID)
	if r.ItemCount != 2 || r.MealEventCount != 1 || r.NotEatenMeals != 0 || r.CompleteDays != 0 {
		t.Fatalf("bad projection: %+v", r)
	}
	for _, e := range r.Days[6].Evidence {
		if e.Nutrition != nil {
			t.Fatal("model text manufactured nutrition")
		}
	}
	otherIDs, err := SaveHealthJournalBatch(other.ID, in)
	if err != nil || reflect.DeepEqual(ids, otherIDs) {
		t.Fatal("batch key not account scoped")
	}
	changed := in
	changed.Items = append([]HealthDraftItemInput{}, in.Items...)
	changed.Items[0].DishName = "面包"
	if _, err := SaveHealthJournalBatch(u.ID, changed); !errors.Is(err, ErrHealthBatchConflict) {
		t.Fatal("changed retry accepted")
	}
	if err := DeleteFoodJournalEntry(u.ID, ids[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveHealthJournalBatch(u.ID, in); err != nil {
		t.Fatal(err)
	}
	if mealStatusReport(t, u.ID).ItemCount != 1 {
		t.Fatal("retry resurrected deleted food")
	}
	in.RequestKey = "draft_batch_snack_1"
	in.MealType = "snack"
	if _, err := SaveHealthJournalBatch(u.ID, in); err != nil {
		t.Fatal(err)
	}
	if mealStatusReport(t, u.ID).MealEventCount != 2 {
		t.Fatal("same batch snack items became separate meals")
	}
}

func TestHealthDraftBatchValidationAndRollback(t *testing.T) {
	u, _, err := testutil.NewUser(t.Name() + "@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	good := HealthJournalBatchInput{RequestKey: "draft_batch_rollback", Confirmed: true, MealDate: healthToday(), MealType: "breakfast", Items: []HealthDraftItemInput{{DishName: "鸡蛋"}, {DishName: "米饭"}}}
	for _, mutate := range []func(*HealthJournalBatchInput){func(i *HealthJournalBatchInput) { i.Confirmed = false }, func(i *HealthJournalBatchInput) { i.MealDate = "" }, func(i *HealthJournalBatchInput) { i.MealDate = "2099-01-01" }, func(i *HealthJournalBatchInput) { i.Items = nil }, func(i *HealthJournalBatchInput) { i.MealType = "other" }, func(i *HealthJournalBatchInput) {
		i.Items = []HealthDraftItemInput{{DishName: "米饭"}, {DishName: ""}}
	}} {
		in := good
		mutate(&in)
		if _, err := SaveHealthJournalBatch(u.ID, in); !errors.Is(err, ErrHealthDraftInput) {
			t.Fatalf("invalid accepted: %v", err)
		}
	}
	if err := SetHealthMealOmission(u.ID, good.MealDate, good.MealType, mealStatusReport(t, u.ID).Days[6].Fingerprint, true); err != nil {
		t.Fatal(err)
	}
	db := database.DB
	name := "health_batch_failure"
	if err := db.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "health_journal_batches" {
			tx.AddError(errors.New("receipt write failed"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer db.Callback().Create().Remove(name)
	if _, err := SaveHealthJournalBatch(u.ID, good); err == nil {
		t.Fatal("expected rollback")
	}
	r := mealStatusReport(t, u.ID)
	if r.ItemCount != 0 || r.NotEatenMeals != 1 {
		t.Fatal("partial batch or cleared omission escaped transaction")
	}
}

func TestHealthDraftQuotaIndependentUserAndSharedSiteCaps(t *testing.T) {
	u, _, err := testutil.NewUser(t.Name() + "@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2031, 2, 1, 23, 59, 0, 0, assistantLocation)
	for i := 0; i < HealthDraftDailyLimit; i++ {
		if err := ConsumeHealthDraftQuota(database.DB, u.ID, now); err != nil {
			t.Fatal(err)
		}
	}
	if err := ConsumeHealthDraftQuota(database.DB, u.ID, now); !errors.Is(err, ErrHealthDraftQuota) {
		t.Fatal("per-user cap not enforced")
	}
	q, err := GetHealthDraftQuota(database.DB, u.ID, now)
	if err != nil || q.Remaining != 0 || q.Used != HealthDraftDailyLimit {
		t.Fatalf("quota %+v %v", q, err)
	}
	chat, err := GetAssistantQuota(database.DB, u.ID, now)
	if err != nil || chat.Used != 0 {
		t.Fatal("draft consumed personal chat quota")
	}
	var site models.AssistantUsage
	date, _ := assistantQuotaDay(now)
	if err := database.DB.First(&site, "user_id = 0 AND usage_date = ?", date).Error; err != nil {
		t.Fatal(err)
	}
	if site.Used != HealthDraftDailyLimit {
		t.Fatal("failed draft burned site budget")
	}
	if err := ConsumeHealthDraftQuota(database.DB, u.ID, now.Add(2*time.Minute)); err != nil {
		t.Fatal("daily quota did not reset")
	}
	if err := database.DB.Model(&models.AssistantUsage{}).Where("user_id = 0 AND usage_date = ?", date).Update("used", DefaultAssistantSiteLimit).Error; err != nil {
		t.Fatal(err)
	}
	other, _, err := testutil.NewUser(t.Name() + "-other@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	if err := ConsumeHealthDraftQuota(database.DB, other.ID, now); !errors.Is(err, ErrAssistantSiteQuotaExceeded) {
		t.Fatal("shared site cap bypassed")
	}
	q, err = GetHealthDraftQuota(database.DB, other.ID, now)
	if err != nil || q.Used != 0 {
		t.Fatal("rejected site request burned user quota")
	}
}

func TestHealthDraftCandidatesRequireExplicitNutritionChoiceAndAtomicSnapshot(t *testing.T) {
	u, _, err := testutil.NewUser(t.Name() + "@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := testutil.NewUser(t.Name() + "-other@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	energy := 200.0
	food, err := SaveNutritionFood(u.ID, 0, NutritionFoodInput{Name: "鸡蛋标签", BasisUnit: "g", FoodState: "ready_to_eat", SourceReference: "测试标签", Nutrients: models.NutrientValues{EnergyKcal: &energy}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := FindHealthDraftCandidates(u.ID, " "); !errors.Is(err, ErrHealthDraftCandidateQuery) {
		t.Fatal("blank query accepted")
	}
	candidates, err := FindHealthDraftCandidates(u.ID, "鸡蛋")
	if err != nil || len(candidates.Personal) != 1 || candidates.Personal[0].ID != food.ID {
		t.Fatalf("personal candidate missing: %+v %v", candidates, err)
	}
	candidates, err = FindHealthDraftCandidates(other.ID, "鸡蛋")
	if err != nil || len(candidates.Personal) != 0 {
		t.Fatal("personal candidate crossed accounts")
	}
	provenance := models.CatalogProvenance{Dataset: "draft-fixture", Version: "v1", RecordID: "egg-1", URL: "https://example.test/egg", License: "test license", ReviewedBy: "test reviewer", ReviewedAt: healthToday()}
	if err := ImportNutritionCatalog([]CatalogFoodInput{{Name: "熟鸡蛋", Aliases: []string{"水煮蛋"}, BasisUnit: "g", FoodState: "ready_to_eat", EdibleBasis: "edible_portion", Nutrients: models.NutrientValues{EnergyKcal: &energy}, Provenance: provenance}}, database.DB); err != nil {
		t.Fatal(err)
	}
	candidates, err = FindHealthDraftCandidates(other.ID, "水煮蛋")
	if err != nil || len(candidates.Catalog) != 1 || len(candidates.Personal) != 0 {
		t.Fatalf("alias matching did not preserve explicit catalog choice: %+v %v", candidates, err)
	}
	if err := WithdrawNutritionCatalog(candidates.Catalog[0].ID, database.DB); err != nil {
		t.Fatal(err)
	}
	candidates, err = FindHealthDraftCandidates(other.ID, "水煮蛋")
	if err != nil || len(candidates.Catalog) != 0 {
		t.Fatal("withdrawn catalog food remained a candidate")
	}
	amount := 75.0
	base := HealthJournalBatchInput{RequestKey: "draft_nutrition_choice_1", Confirmed: true, MealDate: healthToday(), MealType: "breakfast", Items: []HealthDraftItemInput{{DishName: "鸡蛋", Portion: "一份", NutritionMode: "replace", NutritionFoodID: food.ID, NutritionAmount: &amount, NutritionUnit: "g", FoodState: "ready_to_eat", PortionSource: "estimated"}}}
	if _, err := SaveHealthJournalBatch(other.ID, base); !errors.Is(err, ErrHealthDraftInput) {
		t.Fatal("another account used private label")
	}
	broken := base
	broken.Items = append([]HealthDraftItemInput{}, base.Items...)
	broken.Items[0].NutritionUnit = "ml"
	if _, err := SaveHealthJournalBatch(u.ID, broken); !errors.Is(err, ErrHealthDraftInput) {
		t.Fatal("unit mismatch accepted")
	}
	if n := mealStatusReport(t, u.ID).ItemCount; n != 0 {
		t.Fatal("invalid nutrition left a partial journal")
	}
	ids, err := SaveHealthJournalBatch(u.ID, base)
	if err != nil || len(ids) != 1 {
		t.Fatalf("selected food save failed: %v %v", ids, err)
	}
	var entry models.FoodJournalEntry
	if err := database.DB.First(&entry, ids[0]).Error; err != nil || entry.Nutrition == nil || entry.Nutrition.Consumed.EnergyKcal == nil || *entry.Nutrition.Consumed.EnergyKcal != 150 || entry.Nutrition.PortionSource != "estimated" {
		t.Fatalf("snapshot missing: %+v %v", entry.Nutrition, err)
	}
	if err := DeleteNutritionFood(u.ID, food.ID); err != nil {
		t.Fatal(err)
	}
	again, err := SaveHealthJournalBatch(u.ID, base)
	if err != nil || !reflect.DeepEqual(ids, again) {
		t.Fatal("committed batch retry relied on removed food")
	}
	base.RequestKey = "draft_nutrition_choice_2"
	if _, err := SaveHealthJournalBatch(u.ID, base); !errors.Is(err, ErrHealthDraftInput) {
		t.Fatal("new batch used deleted food")
	}
	portionFood := catalogFixture(t.Name() + "-portion")
	portionFood.Portions = []models.CatalogPortion{{Key: "bowl", Label: "一平碗", Amount: 150, Reference: "测试称量"}}
	if err := ImportNutritionCatalog([]CatalogFoodInput{portionFood}, database.DB); err != nil {
		t.Fatal(err)
	}
	var source models.NutritionCatalogFood
	if err := database.DB.Where("dataset = ?", portionFood.Provenance.Dataset).First(&source).Error; err != nil {
		t.Fatal(err)
	}
	personal, err := AdoptNutritionCatalog(u.ID, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	portionBatch := HealthJournalBatchInput{RequestKey: "draft_portion_choice_1", Confirmed: true, MealDate: healthToday(), MealType: "lunch", Items: []HealthDraftItemInput{{DishName: "未匹配的配菜"}, {DishName: "半碗主食", NutritionMode: "replace", NutritionFoodID: personal.ID, NutritionUnit: "g", FoodState: "raw", PortionSource: "estimated", NutritionPortionKey: "bowl", NutritionPortionCount: nutrient(0.5)}}}
	brokenPortion := portionBatch
	brokenPortion.Items = append([]HealthDraftItemInput{}, portionBatch.Items...)
	brokenPortion.Items[1].NutritionPortionCount = nutrient(0)
	if _, err := SaveHealthJournalBatch(u.ID, brokenPortion); !errors.Is(err, ErrHealthDraftInput) || mealStatusReport(t, u.ID).ItemCount != 1 {
		t.Fatal("invalid standard portion left a partial batch")
	}
	portionIDs, err := SaveHealthJournalBatch(u.ID, portionBatch)
	if err != nil || len(portionIDs) != 2 {
		t.Fatalf("standard portion batch failed: %v %v", portionIDs, err)
	}
	var portionEntry models.FoodJournalEntry
	if err := database.DB.First(&portionEntry, portionIDs[1]).Error; err != nil || portionEntry.Nutrition == nil || portionEntry.Nutrition.Amount != 75 || portionEntry.Nutrition.StandardPortion == nil || portionEntry.Nutrition.StandardPortion.Count != 0.5 || portionEntry.Nutrition.Consumed.EnergyKcal == nil || *portionEntry.Nutrition.Consumed.EnergyKcal != 150 {
		t.Fatalf("batch standard portion snapshot incorrect: %+v %v", portionEntry.Nutrition, err)
	}
}
