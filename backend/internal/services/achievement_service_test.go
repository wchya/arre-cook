package services

import (
	"reflect"
	"strings"
	"testing"

	"ninimenu/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestFindAutoAchievementsMySQL(t *testing.T) {
	// Use the production dialect without connecting or replacing the shared test DB.
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       "test:test@tcp(127.0.0.1:3306)/test",
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	var query string
	var args []interface{}
	if err := db.Callback().Query().After("gorm:query").Register("test:capture_achievement_query", func(tx *gorm.DB) {
		query = tx.Statement.SQL.String()
		args = append([]interface{}(nil), tx.Statement.Vars...)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := findAutoAchievements(db, []string{"first_recommend", "week_plan_first"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(query, "`condition` = ?") || !strings.Contains(query, "code IN (?,?)") {
		t.Fatalf("MySQL query must quote the reserved column and bind the code list: %s", query)
	}
	if want := []interface{}{"auto", "first_recommend", "week_plan_first"}; !reflect.DeepEqual(args, want) {
		t.Fatalf("query arguments = %#v, want %#v", args, want)
	}
}

func TestFindAutoAchievementsFiltersCandidates(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.Close() })
	if err := db.AutoMigrate(&models.Achievement{}); err != nil {
		t.Fatal(err)
	}
	rows := []models.Achievement{
		{Code: "eligible", Name: "Eligible", Condition: "auto"},
		{Code: "manual", Name: "Manual", Condition: "manual"},
		{Code: "unrelated", Name: "Unrelated", Condition: "auto"},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	got, err := findAutoAchievements(db, []string{"eligible", "manual", "missing"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Code != "eligible" {
		t.Fatalf("automatic candidates = %+v, want only eligible", got)
	}
	if got, err := findAutoAchievements(db, nil); err != nil || len(got) != 0 {
		t.Fatalf("empty candidates = %+v, %v", got, err)
	}
	// A query failure must remain distinguishable from an empty result.
	if err := db.Migrator().DropTable(&models.Achievement{}); err != nil {
		t.Fatal(err)
	}
	if _, err := findAutoAchievements(db, []string{"eligible"}); err == nil {
		t.Fatal("missing table must return a query error")
	}
}
