package models

import "time"

// UserDishAffinity is a rebuildable projection of this user's evidence, never a
// global recipe rating. Raw meal/feedback records remain the source of truth.
type UserDishAffinity struct {
	UserID           uint      `gorm:"primaryKey;autoIncrement:false" json:"-"`
	DishID           uint      `gorm:"primaryKey;autoIncrement:false" json:"dish_id"`
	Affinity         float64   `json:"-"`
	Confidence       float64   `json:"-"`
	EffectiveSamples float64   `json:"-"`
	LastEaten        string    `gorm:"size:10" json:"-"`
	AlgorithmVersion string    `gorm:"size:40" json:"-"`
	UpdatedAt        time.Time `json:"-"`
}

// RecommendationSnapshot stores only lightweight selection evidence. Recipes
// are resolved again on every read so edits and access changes take effect.
type RecommendationSnapshot struct {
	ID               string    `gorm:"primaryKey;size:64" json:"recommendation_id"`
	UserID           uint      `gorm:"not null;index:idx_recommendation_context,priority:1;index:idx_recommendation_session,priority:1" json:"-"`
	MealDate         string    `gorm:"not null;size:10;index:idx_recommendation_context,priority:2" json:"served_for_date"`
	MealType         string    `gorm:"not null;size:16;index:idx_recommendation_context,priority:3" json:"meal_type"`
	ContextKey       string    `gorm:"not null;size:64;index:idx_recommendation_context,priority:4" json:"-"`
	SessionID        string    `gorm:"not null;size:64;index:idx_recommendation_session,priority:2" json:"session_id"`
	Payload          string    `gorm:"type:longtext" json:"-"`
	AlgorithmVersion string    `gorm:"not null;size:40" json:"algorithm_version"`
	CreatedAt        time.Time `gorm:"index" json:"created_at"`
}
