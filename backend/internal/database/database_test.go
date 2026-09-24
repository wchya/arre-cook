package database

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"ninimenu/internal/config"
	"ninimenu/internal/models"
)

type legacyBehaviorEvent struct {
	ID        uint   `gorm:"primaryKey"`
	EventType string `gorm:"not null;index"`
	DishID    uint   `gorm:"index"`
	DishName  string
	Source    string `gorm:"index"`
	Actor     string
	Meta      string    `gorm:"default:'{}'"`
	CreatedAt time.Time `gorm:"index"`
}

func (legacyBehaviorEvent) TableName() string { return "behavior_events" }

func TestInitMigratesLegacyDataAndScopesMealUniquenessByUser(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	legacySQL, err := legacy.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = legacySQL.Close() })

	legacySchema := []string{
		`CREATE TABLE meal_records (
			id INTEGER PRIMARY KEY AUTOINCREMENT, dish_id INTEGER NOT NULL, dish_name TEXT NOT NULL,
			meal_type TEXT NOT NULL, meal_date TEXT NOT NULL, rating INTEGER DEFAULT 0,
			remark TEXT, mood TEXT, photo TEXT, created_at DATETIME
		)`,
		`CREATE UNIQUE INDEX idx_meal_records_unique_day ON meal_records(dish_id, meal_type, meal_date)`,
		`CREATE TABLE favorites (id INTEGER PRIMARY KEY AUTOINCREMENT, dish_id INTEGER NOT NULL UNIQUE, created_at DATETIME)`,
		`CREATE TABLE day_ratings (
			id INTEGER PRIMARY KEY AUTOINCREMENT, meal_date TEXT NOT NULL UNIQUE, home_mood TEXT,
			mood TEXT, remark TEXT, photos TEXT DEFAULT '[]', created_at DATETIME, updated_at DATETIME
		)`,
		`CREATE TABLE home_inventories (
			id INTEGER PRIMARY KEY AUTOINCREMENT, item_name TEXT NOT NULL UNIQUE,
			in_stock NUMERIC DEFAULT TRUE, created_at DATETIME, updated_at DATETIME
		)`,
		`CREATE TABLE settings (
			id INTEGER PRIMARY KEY AUTOINCREMENT, "key" TEXT NOT NULL UNIQUE, value TEXT,
			created_at DATETIME, updated_at DATETIME
		)`,
		`INSERT INTO meal_records (id, dish_id, dish_name, meal_type, meal_date, rating, remark, mood, photo)
			VALUES (11, 15, '旧菜谱', 'lunch', '2026-09-20', 5, '历史记录', 'yum', '/uploads/legacy.jpg')`,
		`INSERT INTO favorites (id, dish_id) VALUES (12, 15)`,
		`INSERT INTO day_ratings (id, meal_date, mood, remark, photos)
			VALUES (13, '2026-09-20', 'yum', '旧评价', '["/uploads/day.jpg"]')`,
		`INSERT INTO home_inventories (id, item_name, in_stock) VALUES (14, '米', TRUE)`,
		`INSERT INTO settings (id, "key", value) VALUES (16, 'voice_enabled', '0')`,
		`INSERT INTO settings (id, "key", value) VALUES (17, 'week_plan_cache', 'legacy-cache')`,
	}
	for _, statement := range legacySchema {
		if err := legacy.Exec(statement).Error; err != nil {
			t.Fatalf("prepare legacy database: %v", err)
		}
	}
	if err := legacy.AutoMigrate(&legacyBehaviorEvent{}); err != nil {
		t.Fatalf("prepare legacy behavior table: %v", err)
	}
	if err := legacy.Create(&legacyBehaviorEvent{ID: 15, EventType: "accept", DishID: 15, DishName: "旧菜谱", Source: "app", Actor: "user", Meta: "{}"}).Error; err != nil {
		t.Fatalf("insert legacy behavior event: %v", err)
	}
	if err := legacySQL.Close(); err != nil {
		t.Fatal(err)
	}

	config.C = config.Config{
		Env:           "production",
		DBPath:        dbPath,
		AdminUsername: "admin",
		AdminPassword: "test-password",
		AdminEmail:    "owner@example.com",
		RepeatDays:    3,
	}
	PasswordHasher = func(password string) (string, error) { return "test-hash:" + password, nil }
	t.Cleanup(func() {
		if DB != nil {
			if sqlDB, err := DB.DB(); err == nil {
				_ = sqlDB.Close()
			}
		}
		DB = nil
		PasswordHasher = nil
	})

	if err := Init(); err != nil {
		t.Fatalf("initialize migrated database: %v", err)
	}

	var admin models.User
	if err := DB.Where("email = ?", "owner@example.com").First(&admin).Error; err != nil {
		t.Fatal(err)
	}
	var migratedRecord models.MealRecord
	if err := DB.Where("id = ?", 11).First(&migratedRecord).Error; err != nil {
		t.Fatal(err)
	}
	if migratedRecord.UserID != admin.ID || migratedRecord.Remark != "历史记录" {
		t.Fatalf("legacy meal record migration = user %d, remark %q; want admin %d and preserved remark", migratedRecord.UserID, migratedRecord.Remark, admin.ID)
	}
	var favorite models.Favorite
	if err := DB.Where("user_id = ? AND dish_id = ?", admin.ID, 15).First(&favorite).Error; err != nil {
		t.Fatalf("legacy favorite was not migrated: %v", err)
	}
	var rating models.DayRating
	if err := DB.Where("user_id = ? AND meal_date = ?", admin.ID, "2026-09-20").First(&rating).Error; err != nil {
		t.Fatalf("legacy daily rating was not migrated: %v", err)
	}
	var inventory models.HomeInventory
	if err := DB.Where("user_id = ? AND item_name = ?", admin.ID, "米").First(&inventory).Error; err != nil {
		t.Fatalf("legacy home inventory was not migrated: %v", err)
	}
	var behavior models.BehaviorEvent
	if err := DB.Where("id = ?", 15).First(&behavior).Error; err != nil {
		t.Fatal(err)
	}
	if behavior.UserID != admin.ID {
		t.Fatalf("legacy behavior owner = %d, want admin %d", behavior.UserID, admin.ID)
	}
	var userSetting models.UserSetting
	if err := DB.Where("user_id = ? AND `key` = ?", admin.ID, "voice_enabled").First(&userSetting).Error; err != nil {
		t.Fatalf("legacy user setting was not migrated: %v", err)
	}
	var stalePlanCount int64
	DB.Model(&models.Setting{}).Where("`key` = ?", "week_plan_cache").Count(&stalePlanCount)
	if stalePlanCount != 0 {
		t.Fatal("legacy global week plan cache should be discarded")
	}

	otherEmail := "other@example.com"
	other := models.User{Email: &otherEmail, Nickname: otherEmail, Role: models.RoleUser, TokenVersion: 1}
	if err := DB.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	meal := models.MealRecord{UserID: admin.ID, DishID: 99, DishName: "同一道菜", MealType: "dinner", MealDate: "2026-09-24"}
	if err := DB.Create(&meal).Error; err != nil {
		t.Fatal(err)
	}
	meal.ID = 0
	meal.UserID = other.ID
	if err := DB.Create(&meal).Error; err != nil {
		t.Fatalf("different users should be able to log the same meal: %v", err)
	}
	if DB.Migrator().HasIndex(&models.MealRecord{}, "idx_meal_records_unique_day") {
		t.Fatal("legacy global meal unique index was not removed")
	}
	if !DB.Migrator().HasIndex(&models.MealRecord{}, "idx_meal_records_user_unique_day") {
		t.Fatal("user-scoped meal unique index was not created")
	}
}
