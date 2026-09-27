package services

import (
	"context"
	"errors"
	"ninimenu/internal/models"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openQuotaDB(t *testing.T, path string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(4)
	t.Cleanup(func() { sqlDB.Close() })
	if err := db.AutoMigrate(&models.Setting{}, &models.AssistantUsage{}, &models.AssistantLease{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestAssistantQuotaConcurrentPersistentAndDaily(t *testing.T) {
	path := filepath.Join(t.TempDir(), "quota.db")
	db := openQuotaDB(t, path)
	otherProcess := openQuotaDB(t, path)
	now := time.Date(2026, 9, 27, 15, 59, 59, 0, time.UTC)
	start := make(chan struct{})
	results := make(chan error, 48)
	var wg sync.WaitGroup
	for i := 0; i < 48; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			connection := db
			if i%2 == 1 {
				connection = otherProcess
			}
			_, err := ConsumeAssistantQuota(connection, 101, now)
			results <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)
	allowed, denied := 0, 0
	for err := range results {
		if err == nil {
			allowed++
		} else if errors.Is(err, ErrAssistantQuotaExceeded) {
			denied++
		} else {
			t.Fatal(err)
		}
	}
	if allowed != 20 || denied != 28 {
		t.Fatalf("allowed=%d denied=%d", allowed, denied)
	}
	// A fresh DB handle represents a restarted application: the quota is still used.
	restarted := openQuotaDB(t, path)
	quota, err := GetAssistantQuota(restarted, 101, now)
	if err != nil || quota.Used != 20 || quota.Remaining != 0 || quota.Limit != 20 {
		t.Fatalf("persistent quota=%+v err=%v", quota, err)
	}
	if !quota.ResetAt.Equal(now.Add(time.Second)) {
		t.Fatalf("reset must be Beijing midnight: %s", quota.ResetAt)
	}
	next, err := ConsumeAssistantQuota(restarted, 101, now.Add(time.Second))
	if err != nil || next.Used != 1 || next.Remaining != 19 {
		t.Fatalf("next day quota=%+v err=%v", next, err)
	}
	another, err := GetAssistantQuota(restarted, 102, now)
	if err != nil || another.Used != 0 || another.Remaining != 20 {
		t.Fatalf("accounts must be isolated: %+v %v", another, err)
	}
}

func TestAssistantQuotaConfigurationAndStorageFailure(t *testing.T) {
	db := openQuotaDB(t, filepath.Join(t.TempDir(), "quota.db"))
	now := time.Now()
	for _, value := range []string{"21", "-1", "1.5", "", "unlimited"} {
		if _, err := ParseAssistantDailyLimit(value); err == nil {
			t.Fatalf("accepted invalid limit %q", value)
		}
	}
	setting := models.Setting{Key: AssistantDailyLimitKey, Value: "2"}
	if err := db.Create(&setting).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := ConsumeAssistantQuota(db, 1, now); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ConsumeAssistantQuota(db, 1, now); !errors.Is(err, ErrAssistantQuotaExceeded) {
		t.Fatalf("limit not enforced: %v", err)
	}
	if err := db.Model(&models.Setting{}).Where("`key` = ?", AssistantDailyLimitKey).Update("value", "0").Error; err != nil {
		t.Fatal(err)
	}
	quota, err := ConsumeAssistantQuota(db, 2, now)
	if !errors.Is(err, ErrAssistantQuotaExceeded) || quota.Used != 0 || quota.Remaining != 0 {
		t.Fatalf("pause=%+v err=%v", quota, err)
	}
	if err := db.Model(&models.Setting{}).Where("`key` = ?", AssistantDailyLimitKey).Update("value", "20").Error; err != nil {
		t.Fatal(err)
	}
	resumed, err := ConsumeAssistantQuota(db, 1, now)
	if err != nil || resumed.Used != 3 {
		t.Fatalf("changing limit reset usage: %+v %v", resumed, err)
	}
	if err := db.Migrator().DropTable(&models.AssistantUsage{}); err != nil {
		t.Fatal(err)
	}
	if _, err := GetAssistantQuota(db, 1, now); err == nil {
		t.Fatal("storage failure hidden by status")
	}
	if _, err := ConsumeAssistantQuota(db, 1, now); err == nil || errors.Is(err, ErrAssistantQuotaExceeded) {
		t.Fatalf("storage failure must refuse request: %v", err)
	}
}

func TestAssistantSiteQuotaCannotBeBypassedWithMultipleAccounts(t *testing.T) {
	db := openQuotaDB(t, filepath.Join(t.TempDir(), "site.db"))
	if err := db.Create(&models.Setting{Key: AssistantSiteLimitKey, Value: "3"}).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for _, id := range []uint{1, 2, 3} {
		if _, err := ConsumeAssistantQuota(db, id, now); err != nil {
			t.Fatal(err)
		}
	}
	denied, err := ConsumeAssistantQuota(db, 4, now)
	if !errors.Is(err, ErrAssistantSiteQuotaExceeded) || denied.Used != 0 || denied.BlockedReason != "site_limit" {
		t.Fatalf("site limit=%+v err=%v", denied, err)
	}
	status, err := GetAssistantQuota(db, 4, now)
	if err != nil || status.BlockedReason != "site_limit" {
		t.Fatalf("site status=%+v err=%v", status, err)
	}
	if _, err := ConsumeAssistantQuota(db, 4, now.AddDate(0, 0, 1)); err != nil {
		t.Fatal(err)
	}
}

func TestExhaustedUserCannotBurnSiteBudget(t *testing.T) {
	db := openQuotaDB(t, filepath.Join(t.TempDir(), "site.db"))
	if err := db.Create(&models.Setting{Key: AssistantDailyLimitKey, Value: "1"}).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if _, err := ConsumeAssistantQuota(db, 1, now); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if _, err := ConsumeAssistantQuota(db, 1, now); !errors.Is(err, ErrAssistantQuotaExceeded) {
			t.Fatal(err)
		}
	}
	date, reset := assistantQuotaDay(now)
	site, err := quotaForDay(db, 0, date, DefaultAssistantSiteLimit, reset)
	if err != nil || site.Used != 1 {
		t.Fatalf("exhausted user burned site budget: %+v %v", site, err)
	}
}

func TestAssistantLeasesAcrossConnectionsExpireAndReleaseSafely(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lease.db")
	db := openQuotaDB(t, path)
	other := openQuotaDB(t, path)
	now := time.Now()
	release, err := AcquireAssistantLease(db, 1, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AcquireAssistantLease(other, 1, now); !errors.Is(err, ErrAssistantBusy) {
		t.Fatalf("account overlap allowed: %v", err)
	}
	held := []func(){release}
	for _, id := range []uint{2, 3, 4} {
		done, err := AcquireAssistantLease(other, id, now)
		if err != nil {
			t.Fatal(err)
		}
		held = append(held, done)
	}
	if _, err := AcquireAssistantLease(other, 5, now); !errors.Is(err, ErrAssistantServiceBusy) {
		t.Fatalf("service overlap allowed: %v", err)
	}
	// An expired request cannot remove the new owner's lease when its deferred cleanup runs.
	next, err := AcquireAssistantLease(other, 1, now.Add(3*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	release()
	if _, err := AcquireAssistantLease(db, 1, now.Add(3*time.Minute)); !errors.Is(err, ErrAssistantBusy) {
		t.Fatalf("stale cleanup removed current lease: %v", err)
	}
	next()
	for _, done := range held {
		done()
	}
	ctx, cancel := context.WithCancel(context.Background())
	done, err := AcquireAssistantLease(db.WithContext(ctx), 9, now.Add(4*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	done()
	after, err := AcquireAssistantLease(other, 9, now.Add(4*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	after()
}
