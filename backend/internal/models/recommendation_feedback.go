package models

import "time"

// RecommendationFeedback makes an attributed user action retry-safe. BehaviorEvent
// remains the shared event stream consumed by profiles and external agents.
type RecommendationFeedback struct {
	ID               uint      `json:"id" gorm:"primaryKey"`
	UserID           uint      `json:"-" gorm:"not null;uniqueIndex:idx_recommendation_feedback_request,priority:1"`
	RequestKey       string    `json:"-" gorm:"size:128;not null;uniqueIndex:idx_recommendation_feedback_request,priority:2"`
	RequestHash      string    `json:"-" gorm:"size:64;not null"`
	RecommendationID string    `json:"recommendation_id" gorm:"size:64;index"`
	DishID           uint      `json:"dish_id" gorm:"not null"`
	EventType        string    `json:"event_type" gorm:"size:32;not null"`
	CreatedAt        time.Time `json:"created_at"`
}
