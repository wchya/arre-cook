package models

import "time"

// Nil means unavailable. Zero is a supplied, known value (for example zero fat).
type NutrientValues struct {
	EnergyKcal    *float64 `json:"energy_kcal"`
	ProteinG      *float64 `json:"protein_g"`
	CarbohydrateG *float64 `json:"carbohydrate_g"`
	FatG          *float64 `json:"fat_g"`
	FiberG        *float64 `json:"fiber_g"`
	SodiumMg      *float64 `json:"sodium_mg"`
}

// Personal foods retain either transcribed labels or immutable catalog provenance.
type NutritionFood struct {
	Portions        []CatalogPortion   `json:"portions,omitempty" gorm:"serializer:json;type:longtext"`
	CatalogID       *uint              `json:"catalog_id,omitempty" gorm:"uniqueIndex:idx_personal_catalog,priority:2"`
	Catalog         *CatalogProvenance `json:"catalog,omitempty" gorm:"serializer:json;type:longtext"`
	Recipe          *NutritionRecipe   `json:"recipe,omitempty" gorm:"serializer:json;type:longtext"`
	ID              uint               `json:"id" gorm:"primaryKey"`
	UserID          uint               `json:"-" gorm:"not null;index;uniqueIndex:idx_personal_catalog,priority:1"`
	Name            string             `json:"name" gorm:"size:100;not null"`
	BasisUnit       string             `json:"basis_unit" gorm:"size:8;not null"`
	FoodState       string             `json:"food_state" gorm:"size:24;not null"`
	Source          string             `json:"source" gorm:"size:32;not null"`
	SourceReference string             `json:"source_reference" gorm:"size:500;not null"`
	Nutrients       NutrientValues     `json:"nutrients" gorm:"serializer:json;type:longtext"`
	Version         uint               `json:"version" gorm:"not null;default:1"`
	Enabled         bool               `json:"enabled" gorm:"not null;default:true"`
	CreatedAt       time.Time          `json:"created_at"`
	UpdatedAt       time.Time          `json:"updated_at"`
}

type NutritionSnapshot struct {
	StandardPortion    *PortionSnapshot   `json:"standard_portion,omitempty"`
	Catalog            *CatalogProvenance `json:"catalog,omitempty"`
	Recipe             *NutritionRecipe   `json:"recipe,omitempty"`
	FoodID             uint               `json:"food_id"`
	FoodVersion        uint               `json:"food_version"`
	FoodName           string             `json:"food_name"`
	Amount             float64            `json:"amount"`
	Unit               string             `json:"unit"`
	FoodState          string             `json:"food_state"`
	PortionSource      string             `json:"portion_source"`
	Source             string             `json:"source"`
	SourceReference    string             `json:"source_reference"`
	CalculationVersion string             `json:"calculation_version"`
	Per100             NutrientValues     `json:"per_100"`
	Consumed           NutrientValues     `json:"consumed"`
}

// NutritionRecipe stores the exact inputs used for a retained, unheated mixture.
// Components cannot themselves be recipes, keeping snapshots bounded.
type NutritionRecipe struct {
	YieldG      float64             `json:"yield_g"`
	Method      string              `json:"method"`
	Ingredients []NutritionSnapshot `json:"ingredients"`
	Total       NutrientValues      `json:"total"`
}
