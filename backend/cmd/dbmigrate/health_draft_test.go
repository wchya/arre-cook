package main

import (
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"ninimenu/internal/models"
	"path/filepath"
	"testing"
)

func TestHealthDraftLedgerMigrationAndCopy(t *testing.T) {
	open := func() *gorm.DB {
		db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "db.sqlite")), &gorm.Config{})
		if err != nil {
			t.Fatal(err)
		}
		pool, _ := db.DB()
		t.Cleanup(func() { pool.Close() })
		for i := 0; i < 2; i++ {
			if err := db.AutoMigrate(models.SchemaModels()...); err != nil {
				t.Fatal(err)
			}
		}
		return db
	}
	source, target := open(), open()
	for _, uid := range []uint{7, 8} {
		if err := source.Create(&models.HealthDraftUsage{UserID: uid, UsageDate: "2026-09-29", Used: 3}).Error; err != nil {
			t.Fatal(err)
		}
		if err := source.Create(&models.HealthJournalBatch{UserID: uid, RequestKey: "same_request_key", RequestHash: "content_hash", EntryIDs: []uint{3, 4}}).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, model := range []any{&models.HealthDraftUsage{}, &models.HealthJournalBatch{}} {
		if err := copyModel(source, target, model, options{batch: 1, truncate: true}); err != nil {
			t.Fatal(err)
		}
	}
	for _, uid := range []uint{7, 8} {
		var usage models.HealthDraftUsage
		var batch models.HealthJournalBatch
		if err := target.First(&usage, "user_id = ?", uid).Error; err != nil {
			t.Fatal(err)
		}
		if err := target.First(&batch, "user_id = ?", uid).Error; err != nil {
			t.Fatal(err)
		}
		if usage.Used != 3 || batch.RequestHash != "content_hash" || len(batch.EntryIDs) != 2 || batch.EntryIDs[1] != 4 {
			t.Fatal("copy changed quota or retry receipt")
		}
	}
}
