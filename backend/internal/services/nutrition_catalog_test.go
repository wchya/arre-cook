package services

import (
	"encoding/json"
	"math"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"ninimenu/internal/testutil"
	"testing"
)

func catalogFixture(dataset string) CatalogFoodInput {
	return CatalogFoodInput{Name: "测试可食部分", Aliases: []string{"目录别名", "100%_测试"}, BasisUnit: "g", FoodState: "raw", EdibleBasis: "edible_portion", Nutrients: models.NutrientValues{EnergyKcal: nutrient(200), FatG: nutrient(0)}, Provenance: models.CatalogProvenance{Dataset: dataset, Version: "v1", RecordID: "001", URL: "https://example.org/test-only", License: "Synthetic test fixture only", ReviewedBy: "Test fixture", ReviewedAt: "2026-01-01"}}
}

func TestNutritionCatalogImmutableImportAndValidation(t *testing.T) {
	in := catalogFixture(t.Name())
	if err := ImportNutritionCatalog([]CatalogFoodInput{in}, database.DB); err != nil {
		t.Fatal(err)
	}
	if err := ImportNutritionCatalog([]CatalogFoodInput{in}, database.DB); err != nil {
		t.Fatal(err)
	}
	var count int64
	database.DB.Model(&models.NutritionCatalogFood{}).Where("dataset = ?", in.Provenance.Dataset).Count(&count)
	if count != 1 {
		t.Fatal("reimport duplicated rows")
	}
	another := in
	another.Provenance.RecordID = "002"
	changed := in
	changed.Nutrients.EnergyKcal = nutrient(999)
	if err := ImportNutritionCatalog([]CatalogFoodInput{another, changed}, database.DB); err == nil {
		t.Fatal("same revision changed")
	}
	database.DB.Model(&models.NutritionCatalogFood{}).Where("dataset = ?", in.Provenance.Dataset).Count(&count)
	if count != 1 {
		t.Fatal("conflicting batch partially committed")
	}
	checks := []func(*CatalogFoodInput){
		func(x *CatalogFoodInput) { x.Provenance.License = "" },
		func(x *CatalogFoodInput) { x.Provenance.ReviewedBy = "" },
		func(x *CatalogFoodInput) { x.Provenance.ReviewedAt = "2999-01-01" },
		func(x *CatalogFoodInput) { x.Provenance.URL = "https://user:pass@example.org" },
		func(x *CatalogFoodInput) { x.EdibleBasis = "whole_with_bones" },
		func(x *CatalogFoodInput) { x.Nutrients = models.NutrientValues{} },
	}
	for i, change := range checks {
		candidate := in
		change(&candidate)
		if _, err := ValidateCatalogFoods([]CatalogFoodInput{candidate}); err == nil {
			t.Fatalf("invalid metadata case %d accepted", i)
		}
	}
}

func TestNutritionCatalogAdoptionAndHistoricalProvenance(t *testing.T) {
	in := catalogFixture(t.Name())
	in.Name = "特殊搜索测试"
	in.Aliases = []string{"100%_唯一别名"}
	if err := ImportNutritionCatalog([]CatalogFoodInput{in}, database.DB); err != nil {
		t.Fatal(err)
	}
	rows, err := SearchNutritionCatalog("100%_唯一别名", "raw")
	if err != nil || len(rows) != 1 {
		t.Fatalf("literal alias search: %v %v", rows, err)
	}
	source := rows[0]
	rows, err = SearchNutritionCatalog("100%_唯一别名", "cooked")
	if err != nil || len(rows) != 0 {
		t.Fatal("state filtering failed")
	}
	user, _, err := testutil.NewUser("catalog-adopt@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := testutil.NewUser("catalog-other@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	food, err := AdoptNutritionCatalog(user.ID, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := AdoptNutritionCatalog(user.ID, source.ID)
	if err != nil || replay.ID != food.ID {
		t.Fatal("adoption not idempotent")
	}
	otherFood, err := AdoptNutritionCatalog(other.ID, source.ID)
	if err != nil || otherFood.ID == food.ID {
		t.Fatal("personal food shared across accounts")
	}
	input := FoodJournalInput{MealDate: healthToday(), MealType: "lunch", DishName: food.Name, NutritionMode: "replace", NutritionFoodID: food.ID, NutritionAmount: nutrient(50), NutritionUnit: "g", FoodState: "raw", PortionSource: "measured"}
	entry, err := CreateFoodJournalEntry(user.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	if entry.Nutrition.Catalog == nil || entry.Nutrition.Catalog.Version != "v1" || *entry.Nutrition.Consumed.EnergyKcal != 100 || entry.Nutrition.Consumed.FiberG != nil {
		t.Fatal("snapshot lost source or nullable values")
	}
	if _, err := CreateFoodJournalEntry(other.ID, input); err == nil {
		t.Fatal("foreign personal food accepted")
	}
	if _, err := SaveNutritionFood(user.ID, food.ID, NutritionFoodInput{Name: food.Name, BasisUnit: "g", FoodState: "raw", SourceReference: "test", Version: 1, Nutrients: food.Nutrients}); err == nil {
		t.Fatal("catalog copy overwritten as label")
	}
	if err := database.DB.Model(&models.NutritionCatalogFood{}).Where("id = ?", source.ID).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := ImportNutritionCatalog([]CatalogFoodInput{in}, database.DB); err != nil {
		t.Fatal(err)
	}
	if _, err := AdoptNutritionCatalog(user.ID, source.ID); err == nil {
		t.Fatal("withdrawn item adopted or re-enabled by reimport")
	}
	if err := DeleteNutritionFood(user.ID, food.ID); err != nil {
		t.Fatal(err)
	}
	report, err := BuildHealthReport(user.ID, 7)
	if err != nil || *report.Nutrients[0].KnownTotal != 100 {
		t.Fatal("withdrawal changed historical nutrition")
	}
	if report.Days[len(report.Days)-1].Evidence[0].Nutrition.Catalog.RecordID != "001" {
		t.Fatal("history lost catalog provenance")
	}
}

func TestCatalogMixturePreservesOriginalSourceAfterRevisionAndWithdrawal(t *testing.T) {
	uid, recipeInput, _ := recipeTestData(t)
	in := catalogFixture(t.Name())
	in.FoodState = "ready_to_eat"
	if err := ImportNutritionCatalog([]CatalogFoodInput{in}, database.DB); err != nil {
		t.Fatal(err)
	}
	var source models.NutritionCatalogFood
	if err := database.DB.Where("dataset = ?", in.Provenance.Dataset).First(&source).Error; err != nil {
		t.Fatal(err)
	}
	food, err := AdoptNutritionCatalog(uid, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	recipeInput.Ingredients[0].FoodID = food.ID
	recipe, err := SaveNutritionRecipe(uid, 0, recipeInput)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := CreateFoodJournalEntry(uid, FoodJournalInput{MealDate: healthToday(), MealType: "breakfast", DishName: recipe.Name, NutritionMode: "replace", NutritionFoodID: recipe.ID, NutritionAmount: nutrient(50), NutritionUnit: "g", FoodState: "ready_to_eat", PortionSource: "measured"})
	if err != nil {
		t.Fatal(err)
	}
	if *entry.Nutrition.Consumed.EnergyKcal != 75 || entry.Nutrition.Consumed.FiberG != nil {
		t.Fatal("catalog mixture must preserve golden amount and unknown fiber")
	}
	if err := WithdrawNutritionCatalog(source.ID, database.DB); err != nil {
		t.Fatal(err)
	}
	in.Provenance.Version = "v2"
	in.Nutrients.EnergyKcal = nutrient(900)
	if err := ImportNutritionCatalog([]CatalogFoodInput{in}, database.DB); err != nil {
		t.Fatal(err)
	}
	if err := DeleteNutritionFood(uid, food.ID); err != nil {
		t.Fatal(err)
	}
	if err := DeleteNutritionFood(uid, recipe.ID); err != nil {
		t.Fatal(err)
	}
	report, err := BuildHealthReport(uid, 7)
	if err != nil {
		t.Fatal(err)
	}
	snap := report.Days[len(report.Days)-1].Evidence[0].Nutrition
	original := snap.Recipe.Ingredients[0]
	if *snap.Consumed.EnergyKcal != 75 || original.Catalog == nil || original.Catalog.Version != "v1" || original.Catalog.License != in.Provenance.License || original.CalculationVersion != CatalogCalculationVersion {
		t.Fatalf("historical mixture lost immutable catalog source: %+v", original)
	}
}

func TestCatalogStandardPortionsGoldenValidationAndHistory(t *testing.T) {
	in := catalogFixture(t.Name())
	in.Portions = []models.CatalogPortion{{Key: "bowl", Label: "一平碗", Amount: 150, Reference: "测试来源：同状态可食部分称量"}}
	if err := ImportNutritionCatalog([]CatalogFoodInput{in}, database.DB); err != nil {
		t.Fatal(err)
	}
	var source models.NutritionCatalogFood
	if err := database.DB.Where("dataset = ?", in.Provenance.Dataset).First(&source).Error; err != nil {
		t.Fatal(err)
	}
	user, _, err := testutil.NewUser(t.Name() + "@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	food, err := AdoptNutritionCatalog(user.ID, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	input := FoodJournalInput{MealDate: healthToday(), MealType: "lunch", DishName: food.Name, NutritionMode: "replace", NutritionFoodID: food.ID, NutritionUnit: "g", FoodState: "raw", PortionSource: "measured", NutritionPortionKey: "bowl", NutritionPortionCount: nutrient(0.5), RequestKey: "half-bowl"}
	entry, err := CreateFoodJournalEntry(user.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	snap := entry.Nutrition
	if snap.Amount != 75 || *snap.Consumed.EnergyKcal != 150 || *snap.Consumed.FatG != 0 || snap.Consumed.FiberG != nil || snap.PortionSource != "estimated" || snap.StandardPortion.Count != 0.5 || snap.StandardPortion.Reference != in.Portions[0].Reference {
		t.Fatalf("incorrect half-bowl snapshot: %+v", snap)
	}
	cases := []func(*FoodJournalInput){
		func(x *FoodJournalInput) { x.NutritionPortionKey = "missing" },
		func(x *FoodJournalInput) { x.NutritionPortionCount = nil },
		func(x *FoodJournalInput) { x.NutritionPortionCount = nutrient(0) },
		func(x *FoodJournalInput) { x.NutritionPortionCount = nutrient(math.NaN()) },
		func(x *FoodJournalInput) { x.NutritionPortionCount = nutrient(math.Inf(1)) },
		func(x *FoodJournalInput) { x.NutritionPortionCount = nutrient(-1) },
		func(x *FoodJournalInput) { x.NutritionPortionCount = nutrient(101) },
		func(x *FoodJournalInput) { x.NutritionPortionCount = nutrient(100) }, // 15000g exceeds intake bound
		func(x *FoodJournalInput) { x.NutritionAmount = nutrient(75) },
		func(x *FoodJournalInput) { x.NutritionUnit = "ml" },
		func(x *FoodJournalInput) { x.FoodState = "cooked" },
	}
	for i, mutate := range cases {
		next := input
		next.RequestKey = ""
		mutate(&next)
		if _, err := CreateFoodJournalEntry(user.ID, next); err == nil {
			t.Fatalf("invalid portion case %d accepted", i)
		}
	}
	// Volume servings use the same unit and never convert ml to grams.
	volume := in
	volume.Provenance.RecordID = "volume"
	volume.BasisUnit = "ml"
	volume.Portions = []models.CatalogPortion{{Key: "cup", Label: "一杯", Amount: 200, Reference: "测试量杯刻度"}}
	if err := ImportNutritionCatalog([]CatalogFoodInput{volume}, database.DB); err != nil {
		t.Fatal(err)
	}
	var volumeSource models.NutritionCatalogFood
	if err := database.DB.Where("dataset = ? AND record_id = ?", in.Provenance.Dataset, "volume").First(&volumeSource).Error; err != nil {
		t.Fatal(err)
	}
	volumeFood, err := AdoptNutritionCatalog(user.ID, volumeSource.ID)
	if err != nil {
		t.Fatal(err)
	}
	volumeInput := input
	volumeInput.RequestKey = "half-cup"
	volumeInput.NutritionFoodID = volumeFood.ID
	volumeInput.NutritionUnit = "ml"
	volumeInput.NutritionPortionKey = "cup"
	volumeEntry, err := CreateFoodJournalEntry(user.ID, volumeInput)
	if err != nil {
		t.Fatal(err)
	}
	if volumeEntry.Nutrition.Amount != 100 || volumeEntry.Nutrition.Unit != "ml" || *volumeEntry.Nutrition.Consumed.EnergyKcal != 200 {
		t.Fatal("volume serving changed basis")
	}
	if err := WithdrawNutritionCatalog(source.ID, database.DB); err != nil {
		t.Fatal(err)
	}
	if err := DeleteNutritionFood(user.ID, food.ID); err != nil {
		t.Fatal(err)
	}
	replay, err := CreateFoodJournalEntry(user.ID, input)
	if err != nil || replay.ID != entry.ID || replay.Nutrition.StandardPortion.Amount != 150 {
		t.Fatalf("replay lost standard portion: %v", err)
	}
}

func TestCatalogPortionSourceValidationAndImmutableRevision(t *testing.T) {
	in := catalogFixture(t.Name())
	in.Portions = []models.CatalogPortion{{Key: "piece", Label: "一个", Amount: 50, Reference: "测试可食部称量"}}
	cases := []func(*CatalogFoodInput){
		func(x *CatalogFoodInput) { x.Portions[0].Reference = "" },
		func(x *CatalogFoodInput) { x.Portions[0].Label = "" },
		func(x *CatalogFoodInput) { x.Portions[0].Key = "" },
		func(x *CatalogFoodInput) { x.Portions[0].Amount = 0 },
		func(x *CatalogFoodInput) { x.Portions[0].Amount = math.NaN() },
		func(x *CatalogFoodInput) { x.Portions[0].Amount = math.Inf(1) },
		func(x *CatalogFoodInput) { x.Portions[0].Amount = 10001 },
		func(x *CatalogFoodInput) { x.Portions = append(x.Portions, x.Portions[0]) },
	}
	for i, mutate := range cases {
		next := in
		next.Portions = append([]models.CatalogPortion{}, in.Portions...)
		mutate(&next)
		if _, err := ValidateCatalogFoods([]CatalogFoodInput{next}); err == nil {
			t.Fatalf("invalid portion metadata %d accepted", i)
		}
	}
	if err := ImportNutritionCatalog([]CatalogFoodInput{in}, database.DB); err != nil {
		t.Fatal(err)
	}
	in.Portions[0].Amount = 60
	if err := ImportNutritionCatalog([]CatalogFoodInput{in}, database.DB); err == nil {
		t.Fatal("serving changed within immutable revision")
	}
}

// Adding optional fields must not change the hash of older journal retries or
// immutable catalog imports that predate serving support.
func TestPortionFieldsOmittedForLegacyHashCompatibility(t *testing.T) {
	in := catalogFixture(t.Name())
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var old map[string]json.RawMessage
	if err := json.Unmarshal(data, &old); err != nil {
		t.Fatal(err)
	}
	if _, ok := old["portions"]; ok {
		t.Fatal("absent portions changed catalog hash payload")
	}
	journal := FoodJournalInput{DishName: "旧记录"}
	data, err = json.Marshal(journal)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"nutrition_portion_key", "nutrition_portion_count"} {
		if _, ok := payload[key]; ok {
			t.Fatal("new optional field changed legacy retry payload")
		}
	}
}
