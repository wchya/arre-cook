package main

import (
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"ninimenu/internal/models"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestOmissionMigrationPreservesLegacyRecordsAndCopiesCompositeKeys(t *testing.T) {
	open := func() *gorm.DB {
		db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "data.db")), &gorm.Config{})
		if err != nil {
			t.Fatal(err)
		}
		pool, _ := db.DB()
		t.Cleanup(func() { pool.Close() })
		return db
	}
	source, target := open(), open()
	if err := source.AutoMigrate(&models.MealRecord{}, &models.HealthDayConfirmation{}); err != nil {
		t.Fatal(err)
	}
	record := models.MealRecord{UserID: 7, MealDate: "2026-09-28", MealType: "lunch", DishName: "旧记录"}
	confirmation := models.HealthDayConfirmation{UserID: 7, MealDate: record.MealDate, Fingerprint: "unchanged"}
	if err := source.Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	if err := source.Create(&confirmation).Error; err != nil {
		t.Fatal(err)
	}
	for _, db := range []*gorm.DB{source, target} {
		for i := 0; i < 2; i++ {
			if err := db.AutoMigrate(models.SchemaModels()...); err != nil {
				t.Fatal(err)
			}
		}
	}
	var gotRecord models.MealRecord
	var gotConfirmation models.HealthDayConfirmation
	if err := source.First(&gotRecord, record.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := source.First(&gotConfirmation).Error; err != nil {
		t.Fatal(err)
	}
	if gotRecord.DishName != record.DishName || gotRecord.MealType != record.MealType || gotConfirmation.Fingerprint != confirmation.Fingerprint {
		t.Fatal("migration rewrote legacy data")
	}
	now := time.Now().Truncate(time.Second)
	rows := []models.HealthMealOmission{{UserID: 7, MealDate: "2026-09-28", MealType: "breakfast", ConfirmedAt: now}, {UserID: 7, MealDate: "2026-09-28", MealType: "dinner", ConfirmedAt: now}, {UserID: 7, MealDate: "2026-09-29", MealType: "breakfast", ConfirmedAt: now}, {UserID: 8, MealDate: "2026-09-28", MealType: "breakfast", ConfirmedAt: now}}
	if err := source.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if err := copyModel(source, target, &models.HealthMealOmission{}, options{batch: 1, truncate: true}); err != nil {
		t.Fatal(err)
	}
	var got []models.HealthMealOmission
	if err := target.Order("user_id, meal_date, meal_type").Find(&got).Error; err != nil {
		t.Fatal(err)
	}
	if len(got) != len(rows) {
		t.Fatalf("copy dropped composite rows: %+v", got)
	}
	for i := range rows {
		if !reflect.DeepEqual([]any{rows[i].UserID, rows[i].MealDate, rows[i].MealType}, []any{got[i].UserID, got[i].MealDate, got[i].MealType}) || !rows[i].ConfirmedAt.Equal(got[i].ConfirmedAt) {
			t.Fatal("copy changed omission")
		}
	}
}
