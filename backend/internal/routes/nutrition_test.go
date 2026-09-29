package routes_test

import (
	"fmt"
	"net/http"
	"net/url"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"ninimenu/internal/services"
	"ninimenu/internal/testutil"
	"testing"
)

func TestNutritionRoutesIsolationVersionsAndCleanup(t *testing.T) {
	user, alice, err := testutil.NewUser("nutrition-routes@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	_, bob, err := testutil.NewUser("nutrition-routes-other@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	body := map[string]any{"name": "测试牛奶", "basis_unit": "ml", "food_state": "as_sold", "source_reference": "测试包装", "nutrients": map[string]any{"energy_kcal": 50, "fat_g": 0, "fiber_g": nil}}
	status, response := call(t, "POST", "/api/health/foods", alice, body)
	must(t, status, response, http.StatusOK)
	food := decode[models.NutritionFood](t, response.Data)
	endpoint := fmt.Sprintf("/api/health/foods/%d", food.ID)
	status, response = call(t, "GET", "/api/health/foods", bob, nil)
	must(t, status, response, http.StatusOK)
	if len(decode[[]models.NutritionFood](t, response.Data)) != 0 {
		t.Fatal("foreign labels visible")
	}
	body["version"] = food.Version
	for _, method := range []string{"PUT", "DELETE"} {
		status, response = call(t, method, endpoint, bob, body)
		must(t, status, response, http.StatusNotFound)
	}
	status, response = call(t, "PUT", endpoint, alice, body)
	must(t, status, response, http.StatusOK)
	if decode[models.NutritionFood](t, response.Data).Version != 2 {
		t.Fatal("version did not advance")
	}
	status, response = call(t, "PUT", endpoint, alice, body)
	must(t, status, response, http.StatusBadRequest)
	status, response = call(t, "GET", "/api/me/export", alice, nil)
	must(t, status, response, http.StatusOK)
	exported := decode[struct {
		Foods []models.NutritionFood `json:"nutrition_foods"`
	}](t, response.Data)
	if len(exported.Foods) != 1 || exported.Foods[0].Nutrients.FiberG != nil || *exported.Foods[0].Nutrients.FatG != 0 {
		t.Fatal("export lost label or null/zero semantics")
	}
	status, response = call(t, "DELETE", endpoint, alice, nil)
	must(t, status, response, http.StatusOK)
	body["version"] = 2
	status, response = call(t, "PUT", endpoint, alice, body)
	must(t, status, response, http.StatusNotFound)
	status, response = call(t, "POST", "/api/health/foods", alice, body)
	must(t, status, response, http.StatusOK)
	status, response = call(t, "DELETE", "/api/me", alice, map[string]any{"confirm": "注销"})
	must(t, status, response, http.StatusOK)
	var count int64
	if err := database.DB.Model(&models.NutritionFood{}).Where("user_id = ?", user.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("cleanup: %d %v", count, err)
	}
}

func TestNutritionRecipeRoutesAndExport(t *testing.T) {
	_, alice, err := testutil.NewUser("nutrition-recipe-routes@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	_, bob, err := testutil.NewUser("nutrition-recipe-route-other@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	ingredients := []map[string]any{}
	for i := 0; i < 2; i++ {
		status, response := call(t, "POST", "/api/health/foods", alice, map[string]any{"name": fmt.Sprintf("原料%d", i), "basis_unit": "g", "food_state": "ready_to_eat", "source_reference": "测试标签", "nutrients": map[string]any{"energy_kcal": 100}})
		must(t, status, response, http.StatusOK)
		food := decode[models.NutritionFood](t, response.Data)
		ingredients = append(ingredients, map[string]any{"food_id": food.ID, "food_version": food.Version, "amount": 100, "unit": "g", "food_state": "ready_to_eat"})
	}
	body := map[string]any{"name": "混合食品", "source_reference": "直接混合", "yield_g": 200, "method": "unheated_all_retained", "ingredients": ingredients}
	status, response := call(t, "POST", "/api/health/recipes", alice, body)
	must(t, status, response, http.StatusOK)
	food := decode[models.NutritionFood](t, response.Data)
	if food.Recipe == nil || *food.Recipe.Total.EnergyKcal != 200 {
		t.Fatal("recipe response missing calculation")
	}
	endpoint := fmt.Sprintf("/api/health/recipes/%d", food.ID)
	body["version"] = 1
	status, response = call(t, "PUT", endpoint, bob, body)
	must(t, status, response, http.StatusNotFound)
	status, response = call(t, "POST", "/api/health/recipes", bob, body)
	must(t, status, response, http.StatusBadRequest)
	status, response = call(t, "PUT", endpoint, alice, body)
	must(t, status, response, http.StatusOK)
	status, response = call(t, "PUT", endpoint, alice, body)
	must(t, status, response, http.StatusBadRequest)
	status, response = call(t, "GET", "/api/me/export", alice, nil)
	must(t, status, response, http.StatusOK)
	exported := decode[struct {
		Foods []models.NutritionFood `json:"nutrition_foods"`
	}](t, response.Data)
	found := false
	for _, f := range exported.Foods {
		if f.ID == food.ID {
			found = f.Recipe != nil && len(f.Recipe.Ingredients) == 2
		}
	}
	if !found {
		t.Fatal("export omitted recipe source evidence")
	}
	status, response = call(t, "DELETE", fmt.Sprintf("/api/health/foods/%d", food.ID), alice, nil)
	must(t, status, response, http.StatusOK)
	status, response = call(t, "PUT", endpoint, alice, body)
	must(t, status, response, http.StatusNotFound)
}

func TestNutritionCatalogRoutesSearchAdoptAndHistoricalSnapshot(t *testing.T) {
	energy := 120.0
	in := services.CatalogFoodInput{
		Name:        "路由目录食物",
		Aliases:     []string{"路由100%_别名"},
		BasisUnit:   "g",
		FoodState:   "raw",
		EdibleBasis: "edible_portion",
		Nutrients:   models.NutrientValues{EnergyKcal: &energy},
		Provenance: models.CatalogProvenance{
			Dataset:    "route-catalog-test",
			Version:    "v1",
			RecordID:   "route-001",
			URL:        "https://example.org/route-catalog-test",
			License:    "Test fixture only",
			ReviewedBy: "Route test",
			ReviewedAt: "2026-01-01",
		},
	}
	if err := services.ImportNutritionCatalog([]services.CatalogFoodInput{in}, database.DB); err != nil {
		t.Fatal(err)
	}
	var source models.NutritionCatalogFood
	if err := database.DB.Where("dataset = ? AND record_id = ?", in.Provenance.Dataset, in.Provenance.RecordID).First(&source).Error; err != nil {
		t.Fatal(err)
	}
	_, alice, err := testutil.NewUser("nutrition-catalog-route-alice@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	_, bob, err := testutil.NewUser("nutrition-catalog-route-bob@qq.com")
	if err != nil {
		t.Fatal(err)
	}

	status, response := call(t, "GET", "/api/health/catalog?q="+url.QueryEscape("100%_别名")+"&state=raw", alice, nil)
	must(t, status, response, http.StatusOK)
	rows := decode[[]models.NutritionCatalogFood](t, response.Data)
	if len(rows) != 1 || rows[0].ID != source.ID {
		t.Fatalf("literal alias search returned %#v", rows)
	}
	status, response = call(t, "GET", "/api/health/catalog?q="+url.QueryEscape("100%_别名")+"&state=cooked", alice, nil)
	must(t, status, response, http.StatusOK)
	if rows := decode[[]models.NutritionCatalogFood](t, response.Data); len(rows) != 0 {
		t.Fatalf("state filter returned %d rows", len(rows))
	}

	status, response = call(t, "POST", fmt.Sprintf("/api/health/catalog/%d/adopt", source.ID), alice, nil)
	must(t, status, response, http.StatusOK)
	adopted := decode[models.NutritionFood](t, response.Data)
	if adopted.CatalogID == nil || *adopted.CatalogID != source.ID || adopted.Catalog == nil {
		t.Fatal("adoption did not return immutable provenance")
	}
	status, response = call(t, "POST", fmt.Sprintf("/api/health/catalog/%d/adopt", source.ID), alice, nil)
	must(t, status, response, http.StatusOK)
	if replay := decode[models.NutritionFood](t, response.Data); replay.ID != adopted.ID {
		t.Fatalf("repeated adoption created another personal food: %d != %d", replay.ID, adopted.ID)
	}
	status, response = call(t, "GET", "/api/health/foods", bob, nil)
	must(t, status, response, http.StatusOK)
	if foods := decode[[]models.NutritionFood](t, response.Data); len(foods) != 0 {
		t.Fatalf("personal catalog copy leaked across accounts: %d", len(foods))
	}

	status, response = call(t, "POST", "/api/food-journal", alice, map[string]any{
		"meal_date": services.Today(), "meal_type": "lunch", "dish_name": "路由目录食物",
		"nutrition_mode": "replace", "nutrition_food_id": adopted.ID, "nutrition_amount": 50,
		"nutrition_unit": "g", "food_state": "raw", "portion_source": "measured",
	})
	must(t, status, response, http.StatusOK)
	status, response = call(t, "GET", "/api/food-journal", alice, nil)
	must(t, status, response, http.StatusOK)
	entries := decode[[]models.FoodJournalEntry](t, response.Data)
	if len(entries) == 0 || entries[len(entries)-1].Nutrition == nil || entries[len(entries)-1].Nutrition.Catalog == nil {
		t.Fatal("journal did not retain adopted catalog provenance")
	}

	if err := database.DB.Model(&models.NutritionCatalogFood{}).Where("id = ?", source.ID).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	status, response = call(t, "GET", "/api/health/catalog?q="+url.QueryEscape("路由目录食物"), alice, nil)
	must(t, status, response, http.StatusOK)
	if rows := decode[[]models.NutritionCatalogFood](t, response.Data); len(rows) != 0 {
		t.Fatal("withdrawn catalog item still searchable")
	}
	status, response = call(t, "POST", fmt.Sprintf("/api/health/catalog/%d/adopt", source.ID), bob, nil)
	must(t, status, response, http.StatusNotFound)
}
