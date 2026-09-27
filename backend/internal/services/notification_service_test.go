package services

import (
	"testing"

	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"ninimenu/internal/testutil"
)

func TestAnnounceDeploymentIsIdempotent(t *testing.T) {
	user, _, err := testutil.NewUser("deploy-notice@qq.com")
	if err != nil {
		t.Fatal(err)
	}

	if err := AnnounceDeployment("test-build-1"); err != nil {
		t.Fatal(err)
	}
	if err := AnnounceDeployment("test-build-1"); err != nil {
		t.Fatal(err)
	}
	var count int64
	database.DB.Model(&models.Notification{}).Where("user_id = ? AND title = ?", user.ID, "ss-menu 已更新").Count(&count)
	if count != 1 {
		t.Fatalf("deployment notification count = %d, want 1", count)
	}

	if err := AnnounceDeployment("test-build-2"); err != nil {
		t.Fatal(err)
	}
	database.DB.Model(&models.Notification{}).Where("user_id = ? AND title = ?", user.ID, "ss-menu 已更新").Count(&count)
	if count != 2 {
		t.Fatalf("new deployment notification count = %d, want 2", count)
	}
}
