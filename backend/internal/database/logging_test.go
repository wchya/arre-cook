package database

import (
	"bytes"
	"context"
	"github.com/glebarez/sqlite"
	mysql "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"log"
	"strings"
	"testing"
	"time"
)

func TestSQLLogsNeverExpandParametersOrDriverValues(t *testing.T) {
	var output bytes.Buffer
	safe := SafeLogger{logger.New(log.New(&output, "", 0), logger.Config{LogLevel: logger.Error, ParameterizedQueries: true})}
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: safe})
	if err != nil {
		t.Fatal(err)
	}
	pool, _ := db.DB()
	defer pool.Close()
	type secret struct {
		ID    uint   `gorm:"primaryKey"`
		Value string `gorm:"uniqueIndex"`
	}
	if err := db.AutoMigrate(&secret{}); err != nil {
		t.Fatal(err)
	}
	const sentinel = "DO-NOT-LOG-sensitive-fixture"
	if err := db.Create(&secret{Value: sentinel}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&secret{Value: sentinel}).Error; err == nil {
		t.Fatal("real uniqueness error expected")
	}
	safe.Trace(context.Background(), time.Now(), func() (string, int64) { return "INSERT INTO secrets(value) VALUES (?)", 0 }, &mysql.MySQLError{Number: 1062, Message: "duplicate value " + sentinel})
	if strings.Contains(output.String(), sentinel) || !strings.Contains(output.String(), "INSERT") || !strings.Contains(output.String(), "1062") {
		t.Fatalf("unsafe or unhelpful log: %s", output.String())
	}
}
