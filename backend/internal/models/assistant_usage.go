package models

import "time"

// AssistantUsage is shared by all page clients and survives session deletion and deployment.
type AssistantUsage struct {
	UserID    uint   `gorm:"primaryKey;autoIncrement:false"`
	UsageDate string `gorm:"primaryKey;size:10"`
	Used      int    `gorm:"not null;default:0"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// AssistantLease provides one active request per account and four shared service slots.
// Owners are random per request; an expired request cannot release a newer owner's slot.
type AssistantLease struct {
	Key       string    `gorm:"primaryKey;size:64"`
	Owner     string    `gorm:"not null;size:64"`
	ExpiresAt time.Time `gorm:"not null;index"`
}
