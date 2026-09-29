package models

import "time"

// Counters only; draft descriptions and model output are never stored here.
type HealthDraftUsage struct {
	UserID    uint   `gorm:"primaryKey;autoIncrement:false" json:"-"`
	UsageDate string `gorm:"primaryKey;size:10" json:"usage_date"`
	Used      int    `gorm:"not null;default:0" json:"used"`
}

// Keeps successful batch retries idempotent even after a user deletes an entry.
type HealthJournalBatch struct {
	UserID      uint      `gorm:"primaryKey;autoIncrement:false" json:"-"`
	RequestKey  string    `gorm:"primaryKey;size:80" json:"request_key"`
	RequestHash string    `gorm:"size:64;not null" json:"-"`
	EntryIDs    []uint    `gorm:"serializer:json;type:json" json:"entry_ids"`
	CreatedAt   time.Time `json:"created_at"`
}
