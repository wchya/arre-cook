package main

import (
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"ninimenu/internal/models"
	"path/filepath"
	"testing"
)

func TestNutritionRecipeColumnUpgradeAndSnapshotCopy(t *testing.T) {
	open := func(name string) *gorm.DB {
		db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), name)), &gorm.Config{})
		if err != nil {
			t.Fatal(err)
		}
		pool, err := db.DB()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { pool.Close() })
		if err := db.AutoMigrate(&models.NutritionFood{}, &models.NutritionCatalogFood{}, &models.FoodJournalEntry{}); err != nil {
			t.Fatal(err)
		}
		return db
	}
	source, target := open("source.db"), open("target.db")
	zero, energy := 0.0, 100.0
	label := models.NutritionFood{UserID: 7, Name: "升级前包装标签", BasisUnit: "g", FoodState: "ready_to_eat", Source: "package_label", SourceReference: "原包装", Version: 2, Enabled: true, Nutrients: models.NutrientValues{EnergyKcal: &energy, FatG: &zero}}
	if err := source.Create(&label).Error; err != nil {
		t.Fatal(err)
	}
	journal := models.FoodJournalEntry{UserID: 7, MealDate: "2026-09-28", MealType: "lunch", DishName: "旧日记", Nutrition: nil}
	if err := source.Create(&journal).Error; err != nil {
		t.Fatal(err)
	}
	// Reproduce the previous deployed label table, which had no recipe column.
	if err := source.Migrator().DropColumn(&models.NutritionFood{}, "Recipe"); err != nil {
		t.Fatal(err)
	}
	for _, model := range []any{&models.NutritionFood{}, &models.NutritionCatalogFood{}} {
		if err := source.Migrator().DropColumn(model, "Portions"); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		if err := source.AutoMigrate(&models.NutritionFood{}, &models.NutritionCatalogFood{}, &models.FoodJournalEntry{}); err != nil {
			t.Fatal(err)
		}
	}
	var restored models.NutritionFood
	if err := source.First(&restored, label.ID).Error; err != nil {
		t.Fatal(err)
	}
	if len(restored.Portions) != 0 || restored.Recipe != nil || restored.Version != 2 || restored.Nutrients.FiberG != nil || *restored.Nutrients.FatG != 0 {
		t.Fatal("upgrade changed legacy label values")
	}
	var old models.FoodJournalEntry
	if err := source.First(&old, journal.ID).Error; err != nil || old.Nutrition != nil {
		t.Fatal("upgrade invented nutrition for legacy journal")
	}
	recipe := models.NutritionRecipe{YieldG: 200, Method: "unheated_all_retained", Total: models.NutrientValues{EnergyKcal: &energy}, Ingredients: []models.NutritionSnapshot{{FoodID: label.ID, FoodVersion: 2, FoodName: label.Name, Amount: 100, Unit: "g", SourceReference: "原包装", Per100: label.Nutrients, Consumed: label.Nutrients}}}
	mixture := models.NutritionFood{UserID: 7, Name: "混合食品", BasisUnit: "g", FoodState: "ready_to_eat", Source: "retained_mixture", SourceReference: "完整配方", Version: 1, Enabled: true, Recipe: &recipe}
	if err := source.Create(&mixture).Error; err != nil {
		t.Fatal(err)
	}
	license := "fixture"
	catalog := models.NutritionCatalogFood{
		Dataset: "migration-catalog", DatasetVersion: "v1", RecordID: "001", Name: "目录食物",
		Aliases: []string{"目录别名"}, SearchText: "目录食物\n目录别名", BasisUnit: "g", FoodState: "raw", EdibleBasis: "edible_portion",
		Nutrients: models.NutrientValues{EnergyKcal: &energy}, Provenance: models.CatalogProvenance{Dataset: "migration-catalog", Version: "v1", RecordID: "001", URL: "https://example.org/migration", License: license, ReviewedBy: "migration test", ReviewedAt: "2026-01-01"},
		ContentHash: "catalog-hash", Enabled: true, Portions: []models.CatalogPortion{{Key: "bowl", Label: "一碗", Amount: 150, Reference: "migration fixture"}},
	}
	if err := source.Create(&catalog).Error; err != nil {
		t.Fatal(err)
	}
	journal.ID = 0
	journal.Nutrition = &models.NutritionSnapshot{StandardPortion: &models.PortionSnapshot{CatalogPortion: catalog.Portions[0], Count: 0.5}, FoodID: mixture.ID, FoodVersion: 1, Recipe: &recipe, CalculationVersion: "retained-mixture-v1", Consumed: models.NutrientValues{EnergyKcal: &energy}}
	if err := source.Create(&journal).Error; err != nil {
		t.Fatal(err)
	}
	for _, model := range []any{&models.NutritionFood{}, &models.NutritionCatalogFood{}, &models.FoodJournalEntry{}} {
		if err := copyModel(source, target, model, options{batch: 1, truncate: true}); err != nil {
			t.Fatal(err)
		}
	}
	var copied models.FoodJournalEntry
	if err := target.First(&copied, journal.ID).Error; err != nil {
		t.Fatal(err)
	}
	if copied.Nutrition == nil || copied.Nutrition.StandardPortion == nil || copied.Nutrition.StandardPortion.Count != 0.5 || copied.Nutrition.StandardPortion.Amount != 150 || copied.Nutrition.Recipe == nil || copied.Nutrition.Recipe.Ingredients[0].FoodVersion != 2 || copied.Nutrition.Recipe.Ingredients[0].Per100.FiberG != nil || *copied.Nutrition.Consumed.EnergyKcal != 100 {
		t.Fatal("copy lost nested provenance or nullable nutrition")
	}
	var copiedCatalog models.NutritionCatalogFood
	if err := target.First(&copiedCatalog, catalog.ID).Error; err != nil {
		t.Fatal(err)
	}
	if len(copiedCatalog.Portions) != 1 || copiedCatalog.Portions[0].Reference != "migration fixture" || len(copiedCatalog.Aliases) != 1 || copiedCatalog.Aliases[0] != "目录别名" || copiedCatalog.Provenance.License != license || copiedCatalog.Nutrients.EnergyKcal == nil || *copiedCatalog.Nutrients.EnergyKcal != 100 || !copiedCatalog.Enabled {
		t.Fatal("copy lost catalog provenance or nullable nutrition")
	}
}
