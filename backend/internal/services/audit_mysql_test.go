package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	driver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
)

// Run only against an explicitly supplied, disposable local mysqld socket.
// No production DSN, credentials or network address is accepted by this test.
func TestAuditMySQL(t *testing.T) {
	socket := os.Getenv("ARRE_AUDIT_MYSQL_SOCKET")
	if socket == "" {
		t.Skip("requires isolated local MySQL socket")
	}
	if !filepath.IsAbs(socket) || !strings.HasPrefix(filepath.Base(filepath.Dir(socket)), "arre-audit-mysql-") {
		t.Fatal("not an audit sandbox socket")
	}
	cfg := driver.NewConfig()
	cfg.User, cfg.Net, cfg.Addr, cfg.ParseTime = "root", "unix", socket, true
	cfg.Timeout, cfg.ReadTimeout, cfg.WriteTimeout = 3*time.Second, 10*time.Second, 10*time.Second
	open := func() *gorm.DB {
		db, err := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		if err != nil {
			t.Fatal("open isolated MySQL:", err)
		}
		pool, _ := db.DB()
		pool.SetMaxOpenConns(4)
		t.Cleanup(func() { pool.Close() })
		return db
	}
	admin := open()
	cfg.DBName = fmt.Sprintf("arre_audit_%d", time.Now().UnixNano())
	if err := admin.Exec("CREATE DATABASE " + cfg.DBName).Error; err != nil {
		t.Fatal(err)
	}
	db := open()
	if err := db.AutoMigrate(models.SchemaModels()...); err != nil {
		t.Fatal(err)
	}
	if err := database.RegisterUploadReferences(db); err != nil {
		t.Fatal(err)
	}
	other := open() // independent connection pool, as used by another API replica
	if err := database.RegisterUploadReferences(other); err != nil {
		t.Fatal(err)
	}
	user := func() models.User {
		u := models.User{Nickname: "mysql test"}
		if err := db.Create(&u).Error; err != nil {
			t.Fatal(err)
		}
		return u
	}

	t.Run("quota_with_preexisting_repeatable_read_snapshot", func(t *testing.T) {
		u := user()
		rows := make([]models.Dish, 499)
		for i := range rows {
			rows[i] = models.Dish{OwnerID: u.ID, Name: "quota"}
		}
		if err := db.CreateInBatches(&rows, 100).Error; err != nil {
			t.Fatal(err)
		}
		ready, start, result := make(chan struct{}, 2), make(chan struct{}), make(chan error, 2)
		for _, handle := range []*gorm.DB{db, other} {
			go func(handle *gorm.DB) {
				result <- handle.Transaction(func(tx *gorm.DB) error {
					var n int64
					err := tx.Model(&models.Dish{}).Where("owner_id = ?", u.ID).Count(&n).Error
					ready <- struct{}{}
					<-start
					if err != nil {
						return err
					}
					return InsertDish(tx, &models.Dish{OwnerID: u.ID, Name: "last slot"})
				})
			}(handle)
		}
		<-ready
		<-ready
		close(start)
		accepted, rejected := 0, 0
		for i := 0; i < 2; i++ {
			err := <-result
			if err == nil {
				accepted++
			} else if errors.Is(err, ErrDishQuota) {
				rejected++
			} else {
				t.Error(err)
			}
		}
		var n int64
		db.Model(&models.Dish{}).Where("owner_id = ?", u.ID).Count(&n)
		if accepted != 1 || rejected != 1 || n != 500 {
			t.Fatalf("accepted=%d rejected=%d rows=%d", accepted, rejected, n)
		}
	})

	t.Run("tombstone_overrides_old_snapshot", func(t *testing.T) {
		u := user()
		key := fmt.Sprintf("u/%d/expired.jpg", u.ID)
		if err := database.ReserveUpload(db, u.ID, key, "", 10); err != nil {
			t.Fatal(err)
		}
		if err := database.FinishUpload(db, key); err != nil {
			t.Fatal(err)
		}
		d := models.Dish{OwnerID: u.ID, Name: "unchanged"}
		if err := db.Create(&d).Error; err != nil {
			t.Fatal(err)
		}
		tx := db.Begin()
		defer tx.Rollback()
		var before models.UploadAsset
		if err := tx.Where("`key` = ?", key).First(&before).Error; err != nil {
			t.Fatal(err)
		}
		if err := database.DeleteUpload(context.Background(), other, key); err != nil {
			t.Fatal(err)
		}
		if err := tx.Model(&d).Update("image_url", "/uploads/"+key).Error; !errors.Is(err, database.ErrUploadGone) {
			t.Fatalf("stale reference accepted: %v", err)
		}
	})

	t.Run("transfer_and_leave_preserve_an_owner", func(t *testing.T) {
		for i := 0; i < 10; i++ {
			a, b := user(), user()
			family, err := CreateFamily(a.ID, "mysql family", db)
			if err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&models.FamilyMember{FamilyID: family.ID, UserID: b.ID, JoinedAt: time.Now()}).Error; err != nil {
				t.Fatal(err)
			}
			start, result := make(chan struct{}), make(chan error, 2)
			go func() { <-start; result <- TransferFamily(a.ID, b.ID, db) }()
			go func() { <-start; result <- LeaveFamily(b.ID, other) }()
			close(start)
			first, second := <-result, <-result
			if first == nil && second == nil {
				t.Fatal("both competing mutations committed")
			}
			if err := db.First(family, family.ID).Error; err != nil {
				t.Fatal(err)
			}
			var n int64
			db.Model(&models.FamilyMember{}).Where("family_id = ? AND user_id = ?", family.ID, family.OwnerID).Count(&n)
			if n != 1 {
				t.Fatal("family lost its owner")
			}
		}
	})

	t.Run("shared_windows_and_deployment_idempotency", func(t *testing.T) {
		var allowed atomic.Int32
		var wg sync.WaitGroup
		for i := 0; i < 12; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				handle := []*gorm.DB{db, other}[i%2]
				if database.ReserveWindow(handle, "replica-quota", 24*time.Hour, 3, 1) {
					allowed.Add(1)
				}
				if err := AnnounceDeployment("mysql-one-version", handle); err != nil {
					t.Error(err)
				}
			}(i)
		}
		wg.Wait()
		var users, notices int64
		db.Model(&models.User{}).Count(&users)
		db.Model(&models.Notification{}).Where("type = ?", "system_update").Count(&notices)
		if allowed.Load() != 3 || notices != users {
			t.Fatalf("allowed=%d notices=%d users=%d", allowed.Load(), notices, users)
		}
	})

	t.Run("canceled_query_releases_client_pool", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		start := time.Now()
		if err := db.WithContext(ctx).Exec("SELECT SLEEP(2)").Error; err == nil {
			t.Fatal("query ignored cancellation")
		}
		if time.Since(start) > time.Second {
			t.Fatal("canceled request stayed blocked")
		}
		if err := db.Exec("SELECT 1").Error; err != nil {
			t.Fatal("pool unusable after cancellation")
		}
	})

	t.Run("identical_setting_upserts", func(t *testing.T) {
		for i := 0; i < 2; i++ {
			if err := database.SetUserSetting(1, "same-value", "unchanged", other); err != nil {
				t.Fatal(err)
			}
		}
	})
}
