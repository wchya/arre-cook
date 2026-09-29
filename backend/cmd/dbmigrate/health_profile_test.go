package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"ninimenu/internal/models"
)

func TestHealthProfileMigrationAndCopy(t *testing.T) {
	open := func(name string) *gorm.DB {
		db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), name)), &gorm.Config{})
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
	source, target := open("source.db"), open("target.db")
	now := time.Now().Truncate(time.Second)
	row := models.HealthProfile{UserID: 77, Version: 2, Active: true, Goal: "balanced", EatingPattern: "mixed", Allergies: []string{"花生"}, DietaryExclusions: []string{}, ConfirmedAt: &now}
	if err := source.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	history := models.HealthProfileVersion{UserID: row.UserID, Version: row.Version, Snapshot: row}
	if err := source.Create(&history).Error; err != nil {
		t.Fatal(err)
	}
	for _, model := range []any{&models.HealthProfile{}, &models.HealthProfileVersion{}} {
		if err := copyModel(source, target, model, options{batch: 1, truncate: true}); err != nil {
			t.Fatal(err)
		}
	}
	var copied models.HealthProfile
	if err := target.First(&copied, "user_id = ?", 77).Error; err != nil {
		t.Fatal(err)
	}
	if copied.Version != 2 || !copied.Active || copied.Allergies[0] != "花生" || !copied.ConfirmedAt.Equal(now) || copied.DietaryExclusions == nil {
		t.Fatal("copy lost current profile")
	}
	var revision models.HealthProfileVersion
	if err := target.First(&revision).Error; err != nil {
		t.Fatal(err)
	}
	if revision.Snapshot.Goal != "balanced" || revision.Snapshot.Allergies[0] != "花生" || revision.Snapshot.Version != 2 {
		t.Fatal("copy lost version snapshot")
	}
}
