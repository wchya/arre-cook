package models

import "time"

// HealthProfile is private, voluntary data, separate from AI-visible taste preferences.
// An inactive row retains only its revision to prevent stale writes after erasure.
type HealthProfile struct {
	UserID            uint       `json:"-" gorm:"primaryKey;autoIncrement:false"`
	Version           int        `json:"version" gorm:"not null"`
	Active            bool       `json:"active"`
	Goal              string     `json:"goal" gorm:"size:32"`
	EatingPattern     string     `json:"eating_pattern" gorm:"size:32"`
	Allergies         []string   `json:"allergies" gorm:"serializer:json;type:text"`
	DietaryExclusions []string   `json:"dietary_exclusions" gorm:"serializer:json;type:text"`
	ConfirmedAt       *time.Time `json:"confirmed_at"`
}

type HealthProfileVersion struct {
	ID       uint          `json:"-" gorm:"primaryKey"`
	UserID   uint          `json:"-" gorm:"not null;uniqueIndex:idx_health_profile_revision,priority:1"`
	Version  int           `json:"version" gorm:"not null;uniqueIndex:idx_health_profile_revision,priority:2"`
	Snapshot HealthProfile `json:"snapshot" gorm:"serializer:json;type:text;not null"`
}
