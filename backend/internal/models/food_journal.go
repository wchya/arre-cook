package models

import "time"

// FoodJournalEntry records a meal without requiring a dish in the recipe catalog.
// It is personal data and is never shared with family members.
type FoodJournalEntry struct {
	ID         uint      `json:"id" gorm:"primaryKey"`
	UserID     uint      `json:"-" gorm:"not null;index:idx_food_journal_user_date,priority:1"`
	MealDate   string    `json:"meal_date" gorm:"not null;size:10;index:idx_food_journal_user_date,priority:2"`
	MealType   string    `json:"meal_type" gorm:"not null;size:16"`
	DishName   string    `json:"dish_name" gorm:"not null;size:100"`
	Cuisine    string    `json:"cuisine" gorm:"size:40"`
	FoodGroups []string  `json:"food_groups" gorm:"serializer:json"`
	Notes      string    `json:"notes" gorm:"size:500"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}
