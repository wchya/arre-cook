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

func TestCopyEveryRegisteredModelIncludingSoftDeletedHistory(t *testing.T) {
	open := func(name string) *gorm.DB {
		db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), name)), &gorm.Config{})
		if err != nil {
			t.Fatal(err)
		}
		pool, _ := db.DB()
		t.Cleanup(func() { pool.Close() })
		if err := db.AutoMigrate(models.SchemaModels()...); err != nil {
			t.Fatal(err)
		}
		return db
	}
	source, target := open("source.db"), open("target.db")
	seen := map[string]bool{}
	for _, model := range models.SchemaModels() {
		stmt := &gorm.Statement{DB: source}
		if err := stmt.Parse(model); err != nil {
			t.Fatal(err)
		}
		seen[stmt.Schema.Table] = true
		row := reflect.New(reflect.TypeOf(model).Elem()).Interface()
		if d, ok := row.(*models.Dish); ok {
			d.Name = "deleted history"
			d.DeletedAt = gorm.DeletedAt{Time: time.Now(), Valid: true}
		}
		if err := source.Create(row).Error; err != nil {
			t.Fatalf("seed %s: %v", stmt.Schema.Table, err)
		}
		if err := copyModel(source, target, model, options{batch: 1, truncate: true}); err != nil {
			t.Fatalf("copy %s: %v", stmt.Schema.Table, err)
		}
		var count int64
		if err := target.Unscoped().Model(model).Count(&count).Error; err != nil || count != 1 {
			t.Fatalf("%s count=%d err=%v", stmt.Schema.Table, count, err)
		}
	}
	if !seen["family_shopping_checks"] || !seen["dish_delete_requests"] {
		t.Fatal("family data omitted from schema registry")
	}
	var count int64
	target.Model(&models.Dish{}).Count(&count)
	if count != 0 {
		t.Fatal("soft deletion state lost during copy")
	}
}
func TestCopyModelReadsSeveralBatchesAndTruncatesEmptySource(t *testing.T) {
	dbs := make([]*gorm.DB, 2)
	for i := range dbs {
		db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "batch.db")), &gorm.Config{})
		if err != nil {
			t.Fatal(err)
		}
		pool, _ := db.DB()
		t.Cleanup(func() { pool.Close() })
		if err := db.AutoMigrate(&models.FamilyShoppingCheck{}); err != nil {
			t.Fatal(err)
		}
		dbs[i] = db
	}
	rows := make([]models.FamilyShoppingCheck, 11)
	for i := range rows {
		rows[i].FamilyID = 1
		rows[i].ItemName = "sentinel"
	}
	if err := dbs[0].Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if err := copyModel(dbs[0], dbs[1], &models.FamilyShoppingCheck{}, options{batch: 3, truncate: true}); err != nil {
		t.Fatal(err)
	}
	dbs[0].Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&models.FamilyShoppingCheck{})
	if err := copyModel(dbs[0], dbs[1], &models.FamilyShoppingCheck{}, options{batch: 3, truncate: true}); err != nil {
		t.Fatal(err)
	}
	var count int64
	dbs[1].Model(&models.FamilyShoppingCheck{}).Count(&count)
	if count != 0 {
		t.Fatal("empty source left stale target rows")
	}
}
