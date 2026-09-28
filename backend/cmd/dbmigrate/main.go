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
	if opts.batch < 1 || opts.batch > 1000 {
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

	modelsToCopy := models.SchemaModels()
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

	if opts.batch < 1 || opts.batch > 1000 {
		return fmt.Errorf("batch size must be 1-1000")
	}
	if opts.truncate {
		if err := target.Unscoped().Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(prototype).Error; err != nil {
			return err
		}
	}
	// Stream rows rather than keyset helpers: several tables have composite or
	// string primary keys, which GORM FindInBatches cannot advance correctly.
	cursor, err := source.Unscoped().Model(prototype).Rows()
	if err != nil {
		return err
	}
	defer cursor.Close()
	sliceType := reflect.SliceOf(reflect.TypeOf(prototype))
	batch := reflect.MakeSlice(sliceType, 0, opts.batch)
	count := int64(0)
	flush := func() error {
		if batch.Len() == 0 {
			return nil
		}
		if err := target.Create(batch.Interface()).Error; err != nil {
			return fmt.Errorf("insert %s after %d rows: %w", stmt.Schema.Table, count, err)
		}
		count += int64(batch.Len())
		batch = reflect.MakeSlice(sliceType, 0, opts.batch)
		return nil
	}
	for cursor.Next() {
		row := reflect.New(reflect.TypeOf(prototype).Elem())
		if err := source.ScanRows(cursor, row.Interface()); err != nil {
			return err
		}
		batch = reflect.Append(batch, row)
		if batch.Len() == opts.batch {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	if err := cursor.Err(); err != nil {
		return err
	}
	if err := cursor.Close(); err != nil {
		return err
	}
	if err := flush(); err != nil {
		return err
	}
	var sourceCount, targetCount int64
	if err := source.Unscoped().Model(prototype).Count(&sourceCount).Error; err != nil {
		return err
	}
	if err := target.Unscoped().Model(prototype).Count(&targetCount).Error; err != nil {
		return err
	}
	if sourceCount != count || targetCount != sourceCount {
		return fmt.Errorf("row count mismatch in %s: source=%d copied=%d target=%d", stmt.Schema.Table, sourceCount, count, targetCount)
	}
	log.Printf("%s: %d rows", stmt.Schema.Table, count)
	return nil
}
