package models

import "time"

type UploadAsset struct {
	ID        uint   `gorm:"primaryKey"`
	Key       string `gorm:"size:512;not null;uniqueIndex"`
	OwnerID   uint   `gorm:"index:idx_upload_owner_state"`
	Bytes     int64
	BackupKey string    `gorm:"size:512"`
	State     string    `gorm:"size:16;index:idx_upload_owner_state;index:idx_upload_gc"` // pending, ready, deleting, deleted
	CreatedAt time.Time `gorm:"index:idx_upload_gc"`
	ExpiresAt time.Time `gorm:"index"`
	UpdatedAt time.Time
}

type UploadReference struct {
	ID       uint   `gorm:"primaryKey"`
	Source   string `gorm:"size:32;uniqueIndex:idx_upload_reference,priority:1"`
	SourceID uint   `gorm:"uniqueIndex:idx_upload_reference,priority:2"`
	Key      string `gorm:"size:512;uniqueIndex:idx_upload_reference,priority:3;index"`
}
