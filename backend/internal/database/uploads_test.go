package database

import (
	"context"
	"errors"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"ninimenu/internal/config"
	"ninimenu/internal/models"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func uploadTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	old := config.C
	t.Cleanup(func() { config.C = old })
	config.C = config.Config{UploadDir: t.TempDir(), BackupDir: t.TempDir()}
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "uploads.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	pool, _ := db.DB()
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() { pool.Close() })
	if err := db.AutoMigrate(&models.User{}, &models.Family{}, &models.Dish{}, &models.MealRecord{}, &models.DayRating{}, &models.TaskClaim{}, &models.UploadAsset{}, &models.UploadReference{}, &models.RequestWindow{}); err != nil {
		t.Fatal(err)
	}
	if err := RegisterUploadReferences(db); err != nil {
		t.Fatal(err)
	}
	return db
}
func assetUser(t *testing.T, db *gorm.DB) models.User {
	t.Helper()
	u := models.User{Nickname: "upload owner"}
	if err := db.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	return u
}
func expireAsset(t *testing.T, db *gorm.DB, key string) {
	t.Helper()
	if err := db.Model(&models.UploadAsset{}).Where("`key` = ?", key).Updates(map[string]any{"expires_at": time.Now().Add(-time.Minute), "state": "ready"}).Error; err != nil {
		t.Fatal(err)
	}
}
func TestUploadQuotaCountsPendingReservations(t *testing.T) {
	db := uploadTestDB(t)
	u := assetUser(t, db)
	if err := db.Create(&models.UploadAsset{Key: "occupied", OwnerID: u.ID, Bytes: UserUploadBytes - 100, State: "pending"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := ReserveUpload(db, u.ID, "u/1/too-large.jpg", "", 101); !errors.Is(err, ErrUploadQuota) {
		t.Fatalf("quota error=%v", err)
	}
	if err := ReserveUpload(db, u.ID, "u/1/fits.jpg", "", 100); err != nil {
		t.Fatal(err)
	}
	if err := ReserveUpload(db, u.ID, "u/1/overflow.jpg", "", 1); !errors.Is(err, ErrUploadQuota) {
		t.Fatalf("pending reservation bypassed quota: %v", err)
	}
}
func TestUploadReferencesProtectCopiesStepPhotosAndMapUpdates(t *testing.T) {
	db := uploadTestDB(t)
	u := assetUser(t, db)
	key := "u/1/photo.jpg"
	if err := ReserveUpload(db, u.ID, key, "", 10); err != nil {
		t.Fatal(err)
	}
	expireAsset(t, db, key)
	d := models.Dish{OwnerID: u.ID, Name: "step photo", Steps: `[{"text":"cook","images":["/uploads/u/1/photo.jpg"]}]`}
	if err := db.Create(&d).Error; err != nil {
		t.Fatal(err)
	}
	copy := d
	copy.ID = 0
	if err := db.Create(&copy).Error; err != nil {
		t.Fatal(err)
	}
	if err := DeleteUpload(context.Background(), db, key); !errors.Is(err, ErrUploadReferenced) {
		t.Fatalf("shared photo deletion allowed: %v", err)
	}
	if err := db.Unscoped().Delete(&d).Error; err != nil {
		t.Fatal(err)
	}
	if err := DeleteUpload(context.Background(), db, key); !errors.Is(err, ErrUploadReferenced) {
		t.Fatalf("copy reference lost: %v", err)
	}
	if err := db.Model(&models.Dish{}).Where("id = ?", copy.ID).Update("steps", "[]").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.User{}).Where("id = ?", u.ID).Update("avatar", "/uploads/"+key).Error; err != nil {
		t.Fatal(err)
	}
	if err := DeleteUpload(context.Background(), db, key); !errors.Is(err, ErrUploadReferenced) {
		t.Fatalf("avatar map update not bound: %v", err)
	}
	if err := db.Model(&u).Update("avatar", "").Error; err != nil {
		t.Fatal(err)
	}
	if err := DeleteUpload(context.Background(), db, key); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&copy).Update("image_url", "/uploads/"+key).Error; !errors.Is(err, ErrUploadGone) {
		t.Fatalf("deleted asset rebound: %v", err)
	}
	var fresh models.Dish
	db.First(&fresh, copy.ID)
	if fresh.ImageURL != "" {
		t.Fatal("invalid reference did not roll back source update")
	}
}
func TestAbandonedUploadCleanupDeletesOriginalAndBackup(t *testing.T) {
	db := uploadTestDB(t)
	u := assetUser(t, db)
	key, backup := "u/1/orphan.jpg", "u/1/original.png"
	if err := ReserveUpload(db, u.ID, key, backup, 10); err != nil {
		t.Fatal(err)
	}
	for root, k := range map[string]string{config.C.UploadDir: key, config.C.BackupDir: backup} {
		p := filepath.Join(root, k)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("image"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	expireAsset(t, db, key)
	if err := CleanupUploads(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	for root, k := range map[string]string{config.C.UploadDir: key, config.C.BackupDir: backup} {
		if _, err := os.Stat(filepath.Join(root, k)); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("orphan bytes retained")
		}
	}
	var asset models.UploadAsset
	db.Where("`key` = ?", key).First(&asset)
	if asset.State != "deleted" {
		t.Fatal("quota was not released")
	}
}
func TestSharedWindowUsesDatabaseAcrossHandles(t *testing.T) {
	db := uploadTestDB(t)
	first, second := db.Session(&gorm.Session{}), db.Session(&gorm.Session{})
	if !ReserveWindow(first, "same-account", time.Hour, 3, 2) {
		t.Fatal("initial reservation failed")
	}
	if ReserveWindow(second, "same-account", time.Hour, 3, 2) {
		t.Fatal("second handle bypassed shared quota")
	}
	if !ReserveWindow(second, "same-account", time.Hour, 3, 1) {
		t.Fatal("last unit was unavailable")
	}
}
