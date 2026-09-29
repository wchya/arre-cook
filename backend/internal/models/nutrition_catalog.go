package models

import "time"

// CatalogProvenance is copied into personal foods and journal snapshots.
// Review fields record the supplied content review; they are not medical certification.
type CatalogProvenance struct {
	Dataset    string `json:"dataset"`
	Version    string `json:"version"`
	RecordID   string `json:"record_id"`
	URL        string `json:"url"`
	License    string `json:"license"`
	ReviewedBy string `json:"reviewed_by"`
	ReviewedAt string `json:"reviewed_at"`
}

// CatalogPortion is a reviewed serving of this exact food state and edible basis.
// Amount uses the parent food's basis unit; no density or cooking conversion occurs.
type CatalogPortion struct {
	Key       string  `json:"key"`
	Label     string  `json:"label"`
	Amount    float64 `json:"amount"`
	Reference string  `json:"reference"`
}

type PortionSnapshot struct {
	CatalogPortion
	Count float64 `json:"count"`
}

type NutritionCatalogFood struct {
	Portions       []CatalogPortion  `json:"portions,omitempty" gorm:"serializer:json;type:longtext"`
	ID             uint              `json:"id" gorm:"primaryKey"`
	Dataset        string            `json:"dataset" gorm:"size:80;not null;uniqueIndex:idx_catalog_revision,priority:1"`
	DatasetVersion string            `json:"dataset_version" gorm:"size:80;not null;uniqueIndex:idx_catalog_revision,priority:2"`
	RecordID       string            `json:"record_id" gorm:"size:80;not null;uniqueIndex:idx_catalog_revision,priority:3"`
	Name           string            `json:"name" gorm:"size:100;not null"`
	Aliases        []string          `json:"aliases" gorm:"serializer:json;type:longtext"`
	SearchText     string            `json:"-" gorm:"type:text"`
	BasisUnit      string            `json:"basis_unit" gorm:"size:8;not null"`
	FoodState      string            `json:"food_state" gorm:"size:24;not null"`
	EdibleBasis    string            `json:"edible_basis" gorm:"size:24;not null"`
	Nutrients      NutrientValues    `json:"nutrients" gorm:"serializer:json;type:longtext"`
	Provenance     CatalogProvenance `json:"provenance" gorm:"serializer:json;type:longtext"`
	ContentHash    string            `json:"content_hash" gorm:"size:64;not null"`
	Enabled        bool              `json:"enabled" gorm:"not null;index"`
	CreatedAt      time.Time         `json:"created_at"`
}
