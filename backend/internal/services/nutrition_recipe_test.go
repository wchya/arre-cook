package services

import (
	"math"
	"ninimenu/internal/models"
	"ninimenu/internal/testutil"
	"testing"
)

func recipeTestData(t *testing.T) (uint, NutritionRecipeInput, NutritionFoodInput) {
	t.Helper()
	user, _, err := testutil.NewUser(t.Name() + "@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	label := NutritionFoodInput{Name: "测试酸奶", BasisUnit: "g", FoodState: "ready_to_eat", SourceReference: "测试包装", Nutrients: models.NutrientValues{EnergyKcal: nutrient(200), FatG: nutrient(0), FiberG: nutrient(2)}}
	a, err := SaveNutritionFood(user.ID, 0, label)
	if err != nil {
		t.Fatal(err)
	}
	label.Name = "测试麦片"
	label.Nutrients.EnergyKcal = nutrient(100)
	label.Nutrients.FiberG = nil
	b, err := SaveNutritionFood(user.ID, 0, label)
	if err != nil {
		t.Fatal(err)
	}
	in := NutritionRecipeInput{Name: "混合早餐", SourceReference: "直接混合，全部保留", YieldG: 200, Method: "unheated_all_retained", Ingredients: []NutritionRecipeIngredientInput{{FoodID: a.ID, FoodVersion: 1, Amount: 100, Unit: "g", FoodState: "ready_to_eat"}, {FoodID: b.ID, FoodVersion: 1, Amount: 100, Unit: "g", FoodState: "ready_to_eat"}}}
	return user.ID, in, label
}

func TestNutritionRecipeGoldenHistoryAndMissingValues(t *testing.T) {
	uid, in, label := recipeTestData(t)
	recipe, err := SaveNutritionRecipe(uid, 0, in)
	if err != nil {
		t.Fatal(err)
	}
	if *recipe.Recipe.Total.EnergyKcal != 300 || *recipe.Nutrients.EnergyKcal != 150 || recipe.Nutrients.FiberG != nil || *recipe.Nutrients.FatG != 0 {
		t.Fatalf("incorrect recipe: %+v", recipe)
	}
	journalInput := FoodJournalInput{MealDate: healthToday(), MealType: "breakfast", DishName: in.Name, NutritionMode: "replace", NutritionFoodID: recipe.ID, NutritionAmount: nutrient(50), NutritionUnit: "g", FoodState: "ready_to_eat", PortionSource: "measured", RequestKey: "recipe-repeat"}
	entry, err := CreateFoodJournalEntry(uid, journalInput)
	if err != nil {
		t.Fatal(err)
	}
	if *entry.Nutrition.Consumed.EnergyKcal != 75 || entry.Nutrition.PortionSource != "estimated" || entry.Nutrition.Recipe.YieldG != 200 || entry.Nutrition.Consumed.FiberG != nil {
		t.Fatalf("wrong portion snapshot: %+v", entry.Nutrition)
	}
	// Label edits leave both the saved recipe and historical journal unchanged.
	label.Version = 1
	label.Nutrients.EnergyKcal = nutrient(300)
	_, err = SaveNutritionFood(uid, in.Ingredients[1].FoodID, label)
	if err != nil {
		t.Fatal(err)
	}
	in.Version = 1
	if _, err = SaveNutritionRecipe(uid, recipe.ID, in); err == nil {
		t.Fatal("stale ingredient accepted")
	}
	in.Ingredients[1].FoodVersion = 2
	updated, err := SaveNutritionRecipe(uid, recipe.ID, in)
	if err != nil || updated.Version != 2 || *updated.Recipe.Total.EnergyKcal != 500 {
		t.Fatalf("recipe revision: %v", err)
	}
	if _, err = SaveNutritionRecipe(uid, recipe.ID, in); err == nil {
		t.Fatal("stale recipe version accepted")
	}
	if _, err = SaveNutritionFood(uid, recipe.ID, label); err == nil {
		t.Fatal("recipe overwritten through label API")
	}
	for _, id := range []uint{recipe.ID, in.Ingredients[0].FoodID, in.Ingredients[1].FoodID} {
		if err = DeleteNutritionFood(uid, id); err != nil {
			t.Fatal(err)
		}
	}
	replay, err := CreateFoodJournalEntry(uid, journalInput)
	if err != nil || replay.ID != entry.ID || *replay.Nutrition.Consumed.EnergyKcal != 75 {
		t.Fatalf("recipe deletion broke idempotent replay: %v", err)
	}
	changed := journalInput
	changed.DishName = "different"
	if _, err := CreateFoodJournalEntry(uid, changed); err == nil {
		t.Fatal("changed idempotent payload accepted")
	}
	report, err := BuildHealthReport(uid, 7)
	if err != nil {
		t.Fatal(err)
	}
	if *report.Nutrients[0].KnownTotal != 75 || report.Nutrients[4].KnownTotal != nil {
		t.Fatal("history rewritten or missing converted to zero")
	}
	snapshot := report.Days[len(report.Days)-1].Evidence[0].Nutrition
	if snapshot.Recipe == nil || snapshot.Recipe.Ingredients[1].FoodVersion != 1 || snapshot.FoodVersion != 1 {
		t.Fatal("original provenance lost")
	}
}

func TestNutritionRecipeValidationIsolationAndBoundedInputs(t *testing.T) {
	uid, in, _ := recipeTestData(t)
	other, _, err := testutil.NewUser("recipe-other@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = SaveNutritionRecipe(other.ID, 0, in); err == nil {
		t.Fatal("foreign ingredients accepted")
	}
	cases := []func(*NutritionRecipeInput){
		func(x *NutritionRecipeInput) { x.Method = "fried" },
		func(x *NutritionRecipeInput) { x.YieldG = 0 },
		func(x *NutritionRecipeInput) { x.YieldG = 50 },
		func(x *NutritionRecipeInput) { x.YieldG = math.Inf(1) },
		func(x *NutritionRecipeInput) { x.Ingredients[0].Amount = -1 },
		func(x *NutritionRecipeInput) { x.Ingredients[0].Unit = "ml" },
		func(x *NutritionRecipeInput) { x.Ingredients[0].FoodState = "raw" },
		func(x *NutritionRecipeInput) { x.Ingredients[0].FoodVersion = 0 },
		func(x *NutritionRecipeInput) { x.Ingredients[1] = x.Ingredients[0] },
		func(x *NutritionRecipeInput) { x.Ingredients = x.Ingredients[:1] },
	}
	for i, mutate := range cases {
		candidate := in
		candidate.Ingredients = append([]NutritionRecipeIngredientInput{}, in.Ingredients...)
		mutate(&candidate)
		if _, err := SaveNutritionRecipe(uid, 0, candidate); err == nil {
			t.Fatalf("invalid case %d accepted", i)
		}
	}
	recipe, err := SaveNutritionRecipe(uid, 0, in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = SaveNutritionRecipe(other.ID, recipe.ID, in); err == nil {
		t.Fatal("foreign recipe updated")
	}
	in.Ingredients[0].FoodID = recipe.ID
	if _, err = SaveNutritionRecipe(uid, 0, in); err == nil {
		t.Fatal("nested recipe accepted")
	}
	if _, err = CreateFoodJournalEntry(uid, FoodJournalInput{MealDate: healthToday(), MealType: "lunch", DishName: "too much", NutritionMode: "replace", NutritionFoodID: recipe.ID, NutritionAmount: nutrient(201), NutritionUnit: "g", FoodState: "ready_to_eat", PortionSource: "estimated"}); err == nil {
		t.Fatal("portion larger than batch accepted")
	}
}
