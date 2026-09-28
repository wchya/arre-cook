package database

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"
	"ninimenu/internal/models"
)

func TestDissolvedFamilyReleasesOnlyItsImageReferences(t *testing.T) {
	db := uploadTestDB(t)
	u := assetUser(t, db)
	family := models.Family{OwnerID: u.ID, Name: "family"}
	if err := db.Create(&family).Error; err != nil {
		t.Fatal(err)
	}
	key := "u/1/shared.jpg"
	if err := ReserveUpload(db, u.ID, key, "", 10); err != nil {
		t.Fatal(err)
	}
	if err := FinishUpload(db, key); err != nil {
		t.Fatal(err)
	}
	dishes := []models.Dish{
		{OwnerID: u.ID, FamilyID: family.ID, Name: "family copy", ImageURL: "/uploads/" + key},
		{OwnerID: u.ID, Name: "personal copy", ImageURL: "/uploads/" + key},
	}
	if err := db.Create(&dishes).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&family).Error; err != nil {
			return err
		}
		return tx.Where("family_id = ?", family.ID).Delete(&models.Dish{}).Error
	}); err != nil {
		t.Fatal(err)
	}
	if err := DeleteUpload(context.Background(), db, key); !errors.Is(err, ErrUploadReferenced) {
		t.Fatalf("personal reference lost: %v", err)
	}
	if err := db.Unscoped().Delete(&dishes[1]).Error; err != nil {
		t.Fatal(err)
	}
	if err := DeleteUpload(context.Background(), db, key); err != nil {
		t.Fatal(err)
	}
	if err := FinishUpload(db, key); !errors.Is(err, ErrUploadGone) {
		t.Fatalf("deleted upload revived: %v", err)
	}
}

func TestSettingsRepeatedWritesAndUserIsolation(t *testing.T) {
	db := uploadTestDB(t)
	if err := db.AutoMigrate(&models.Setting{}, &models.UserSetting{}); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"first", "first", "changed"} {
		if err := SetSetting("key", value, db); err != nil {
			t.Fatal(err)
		}
		if err := SetUserSetting(1, "key", value, db); err != nil {
			t.Fatal(err)
		}
	}
	if err := SetUserSetting(2, "key", "another", db); err != nil {
		t.Fatal(err)
	}
	if GetSetting("key", "", db) != "changed" || GetUserSetting(1, "key", "", db) != "changed" || GetUserSetting(2, "key", "", db) != "another" {
		t.Fatal("setting upsert lost data or mixed accounts")
	}
}
