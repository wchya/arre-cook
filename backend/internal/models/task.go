package models

import "time"

// TaskClaim is durable idempotency state; rows are deliberately not expired.
type TaskClaim struct {
	ID        uint   `gorm:"primaryKey"`
	Key       string `gorm:"size:160;uniqueIndex;not null"`
	Done      bool
	CreatedAt time.Time
}

// RequestWindow reserves account-level rate units across API replicas.
type RequestWindow struct {
	ID        uint   `gorm:"primaryKey"`
	Key       string `gorm:"size:160;uniqueIndex;not null"`
	StartedAt time.Time
	Units     int
	ExpiresAt time.Time `gorm:"index"`
}
