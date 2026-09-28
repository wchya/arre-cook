package database

import (
	"crypto/sha256"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ninimenu/internal/models"
	"time"
)

// ReserveWindow is a shared fixed window, not a per-process approximation.
// Fail closed if the storage operation fails. Expired rows can be purged hourly.
func ReserveWindow(db *gorm.DB, key string, window time.Duration, limit, units int) bool {
	if limit <= 0 || units < 1 || units > limit {
		return false
	}
	now := time.Now()
	start := now.Truncate(window)
	key = fmt.Sprintf("%x:%d", sha256.Sum256([]byte(key)), start.UnixNano())
	allowed := false
	err := db.Transaction(func(tx *gorm.DB) error {
		row := models.RequestWindow{Key: key, StartedAt: start, ExpiresAt: start.Add(window)}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
		result := tx.Model(&models.RequestWindow{}).Where("`key` = ? AND units <= ?", key, limit-units).UpdateColumn("units", gorm.Expr("units + ?", units))
		allowed = result.RowsAffected == 1
		return result.Error
	})
	return err == nil && allowed
}

// LockKey provides a stable row lock for short multi-row invariants. The row
// insert/update is a current read on MySQL and a write lock on SQLite.
func LockKey(db *gorm.DB, key string) error {
	row := models.TaskClaim{Key: key}
	return db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "key"}}, DoUpdates: clause.Assignments(map[string]any{"key": key})}).Create(&row).Error
}
