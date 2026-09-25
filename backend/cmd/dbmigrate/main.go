// Command dbmigrate copies a NiniMenu SQLite database into a MySQL database.
// It is intentionally separate from the web server so the production cutover
// can be rehearsed against a disposable MySQL schema first.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"reflect"

	"github.com/glebarez/sqlite"
	mysqlDriver "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"ninimenu/internal/models"
)

type options struct {
	source   string
	target   string
	batch    int
	truncate bool
}

func main() {
	opts := options{}
	flag.StringVar(&opts.source, "source", "data/ninimenu.db", "source SQLite database path")
	flag.StringVar(&opts.target, "target-dsn", os.Getenv("MYSQL_DSN"), "target MySQL DSN")
	flag.IntVar(&opts.batch, "batch-size", 500, "rows per insert batch")
	flag.BoolVar(&opts.truncate, "truncate", false, "delete target rows before copying (only use on an isolated database)")
	flag.Parse()
	if opts.target == "" {
		log.Fatal("-target-dsn or MYSQL_DSN is required")
	}
	if opts.batch < 1 {
		opts.batch = 500
	}

	source, err := gorm.Open(sqlite.Open(opts.source), &gorm.Config{})
	if err != nil {
		log.Fatalf("open SQLite: %v", err)
	}
	target, err := gorm.Open(mysqlDriver.Open(opts.target), &gorm.Config{TranslateError: true})
	if err != nil {
		log.Fatalf("open MySQL: %v", err)
	}

	modelsToCopy := []any{
		&models.User{}, &models.EmailCode{}, &models.UserPreference{}, &models.UserSetting{},
		&models.AgentToken{}, &models.AgentAuditLog{}, &models.AgentSuggestion{},
		&models.Notification{}, &models.ChatSession{}, &models.ChatMessage{},
		&models.Family{}, &models.FamilyMember{}, &models.FamilyInvitation{},
		&models.FamilyPlanItem{}, &models.FamilyShoppingItem{}, &models.FoodJournalEntry{},
		&models.Dish{}, &models.MealRecord{}, &models.Favorite{}, &models.Quote{},
		&models.Achievement{}, &models.UserAchievement{}, &models.AchievementEvent{},
		&models.BlindBox{}, &models.Holiday{}, &models.Setting{}, &models.DayRating{},
		&models.ShoppingCheck{}, &models.HomeInventory{}, &models.ShoppingItemCategory{},
		&models.BehaviorEvent{},
	}
	if err := target.AutoMigrate(modelsToCopy...); err != nil {
		log.Fatalf("migrate MySQL schema: %v", err)
	}

	for _, prototype := range modelsToCopy {
		if err := copyModel(source, target, prototype, opts); err != nil {
			log.Fatalf("copy %T: %v", prototype, err)
		}
	}
	fmt.Println("SQLite to MySQL migration completed")
}

func copyModel(source, target *gorm.DB, prototype any, opts options) error {
	stmt := &gorm.Statement{DB: source}
	if err := stmt.Parse(prototype); err != nil {
		return err
	}
	if !source.Migrator().HasTable(stmt.Schema.Table) {
		return nil
	}

	sliceType := reflect.SliceOf(reflect.TypeOf(prototype))
	rows := reflect.New(sliceType).Interface()
	if err := source.Find(rows).Error; err != nil {
		return err
	}
	values := reflect.ValueOf(rows).Elem()
	if values.Len() == 0 {
		return nil
	}
	if opts.truncate {
		if err := target.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(reflect.New(reflect.TypeOf(prototype).Elem()).Interface()).Error; err != nil {
			return err
		}
	}
	for start := 0; start < values.Len(); start += opts.batch {
		end := start + opts.batch
		if end > values.Len() {
			end = values.Len()
		}
		batch := reflect.MakeSlice(sliceType, end-start, end-start)
		reflect.Copy(batch, values.Slice(start, end))
		if err := target.Create(batch.Interface()).Error; err != nil {
			return fmt.Errorf("insert %s rows %d-%d: %w", stmt.Schema.Table, start, end, err)
		}
	}
	log.Printf("%s: %d rows", stmt.Schema.Table, values.Len())
	return nil
}
