package models

import "time"

// Confirmation is valid only while the underlying day's records match Fingerprint.
type HealthDayConfirmation struct {
	UserID      uint      `json:"-" gorm:"primaryKey"`
	MealDate    string    `json:"meal_date" gorm:"primaryKey;size:10"`
	Fingerprint string    `json:"fingerprint" gorm:"size:64;not null"`
	ConfirmedAt time.Time `json:"confirmed_at"`
}

// HealthPlanItem is a user's explicit plan, independent of generated menu caches.
type HealthPlanItem struct {
	ID         uint      `json:"id" gorm:"primaryKey"`
	UserID     uint      `json:"-" gorm:"not null;uniqueIndex:idx_health_plan_slot,priority:1;index"`
	MealDate   string    `json:"meal_date" gorm:"size:10;not null;uniqueIndex:idx_health_plan_slot,priority:2"`
	MealType   string    `json:"meal_type" gorm:"size:16;not null;uniqueIndex:idx_health_plan_slot,priority:3"`
	DishID     uint      `json:"dish_id" gorm:"not null;uniqueIndex:idx_health_plan_slot,priority:4"`
	DishName   string    `json:"dish_name" gorm:"size:200"`
	ReportFrom string    `json:"report_from" gorm:"size:10"`
	ReportTo   string    `json:"report_to" gorm:"size:10"`
	CreatedAt  time.Time `json:"created_at"`
}

// HealthMealOmission records an explicit declaration, never a zero-nutrient food item.
type HealthMealOmission struct {
	UserID      uint      `json:"-" gorm:"primaryKey;autoIncrement:false"`
	MealDate    string    `json:"meal_date" gorm:"primaryKey;size:10"`
	MealType    string    `json:"meal_type" gorm:"primaryKey;size:16"`
	ConfirmedAt time.Time `json:"confirmed_at"`
}
