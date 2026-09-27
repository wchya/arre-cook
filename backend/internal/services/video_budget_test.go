package services

import (
	"context"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestVideoBudgetAtomicSharedPersistentAndIndependentOfUsers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "video.db")
	db, other := openQuotaDB(t, path), openQuotaDB(t, path)
	if err := db.AutoMigrate(&models.VideoPlatformBudget{}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)
	var wg sync.WaitGroup
	var granted atomic.Int32
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			connection := db
			if i%2 == 1 {
				connection = other
			}
			delay, err := reserveVideoRequest(connection, "bilibili", now)
			if err != nil {
				t.Error(err)
			}
			if err == nil && delay == 0 {
				granted.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if granted.Load() != 1 {
		t.Fatalf("concurrent slots granted=%d", granted.Load())
	}
	for i := 1; i < videoHourlyRequests; i++ {
		delay, err := reserveVideoRequest(db, "bilibili", now.Add(time.Duration(i)*videoRequestSpacing))
		if delay != 0 || err != nil {
			t.Fatalf("reservation %d: %s %v", i, delay, err)
		}
	}
	if delay, err := reserveVideoRequest(other, "bilibili", now.Add(5*time.Minute)); err != nil || delay < 50*time.Minute {
		t.Fatalf("hourly limit bypassed: %s %v", delay, err)
	}
	if delay, err := reserveVideoRequest(other, "douyin", now); delay != 0 || err != nil {
		t.Fatal("platform budgets not independent")
	}
	if delay, err := reserveVideoRequest(db, "bilibili", now.Add(time.Hour)); delay != 0 || err != nil {
		t.Fatal("hour reset failed")
	}
	var row models.VideoPlatformBudget
	if err := db.Where("platform = ?", "bilibili").Take(&row).Error; err != nil || row.HourUsed != 1 || row.DayUsed != 61 {
		t.Fatalf("bucket labels/counts corrupted: %+v %v", row, err)
	}
	if err := db.Model(&row).Update("day_used", videoDailyRequests).Error; err != nil {
		t.Fatal(err)
	}
	if delay, err := reserveVideoRequest(db, "bilibili", now.Add(2*time.Hour)); delay <= 0 || err != nil {
		t.Fatal("daily limit bypassed")
	}
	if delay, err := reserveVideoRequest(other, "bilibili", now.Add(24*time.Hour)); delay != 0 || err != nil {
		t.Fatal("day reset failed")
	}
}

func TestVideoCooldownSurvivesCanceledRequestAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cooldown.db")
	db := openQuotaDB(t, path)
	if err := db.AutoMigrate(&models.VideoPlatformBudget{}); err != nil {
		t.Fatal(err)
	}
	old := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = old })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := (videoBudgetGate{}).CoolDown(ctx, "bilibili", time.Hour); err != nil {
		t.Fatal(err)
	}
	restarted := openQuotaDB(t, path)
	if delay, err := reserveVideoRequest(restarted, "bilibili", time.Now()); err != nil || delay < 59*time.Minute {
		t.Fatalf("cooldown lost: %s %v", delay, err)
	}
}
