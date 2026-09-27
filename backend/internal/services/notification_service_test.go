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
	database.DB.Model(&models.Notification{}).Where("user_id = ? AND title = ?", user.ID, "arre食谱推荐小助手 已更新").Count(&count)
	if count != 1 {
		t.Fatalf("deployment notification count = %d, want 1", count)
	}

	if err := AnnounceDeployment("test-build-2"); err != nil {
		t.Fatal(err)
	}
	database.DB.Model(&models.Notification{}).Where("user_id = ? AND title = ?", user.ID, "arre食谱推荐小助手 已更新").Count(&count)
	if count != 2 {
		t.Fatalf("new deployment notification count = %d, want 2", count)
	}
}

func TestAnnounceDeploymentWithNotesUsesReadableSummary(t *testing.T) {
	user, _, err := testutil.NewUser("deploy-notes@qq.com")
	if err != nil {
		t.Fatal(err)
	}

	if err := AnnounceDeploymentWithNotes("notes-build", "优化站内信详情查看体验；修复头像选择无响应问题"); err != nil {
		t.Fatal(err)
	}
	var notice models.Notification
	if err := database.DB.Where("user_id = ? AND title = ?", user.ID, "arre食谱推荐小助手 已更新").Order("id DESC").First(&notice).Error; err != nil {
		t.Fatal(err)
	}
	want := "系统已更新到版本 v1。\n\n本次更新：\n- 优化站内信详情查看体验\n- 修复头像选择无响应问题"
	if notice.Content != want {
		t.Fatalf("notice content = %q, want %q", notice.Content, want)
	}
}

func TestAnnounceDeploymentWithNotesFallsBackWhenEmpty(t *testing.T) {
	user, _, err := testutil.NewUser("deploy-fallback@qq.com")
	if err != nil {
		t.Fatal(err)
	}

	if err := AnnounceDeploymentWithNotes("fallback-build", "\n ; "); err != nil {
		t.Fatal(err)
	}
	var notice models.Notification
	if err := database.DB.Where("user_id = ? AND title = ?", user.ID, "arre食谱推荐小助手 已更新").Order("id DESC").First(&notice).Error; err != nil {
		t.Fatal(err)
	}
	if notice.Content != "系统已更新到版本 v1。\n\n本次更新了一些内容，并修复了一些 bug。" {
		t.Fatalf("fallback notice content = %q", notice.Content)
	}
}

func TestDisplayDeploymentVersion(t *testing.T) {
	for raw, want := range map[string]string{"2": "v2", "v003": "v003", "v7": "v7", "commit-abc": "v1"} {
		if got := displayDeploymentVersion(raw); got != want {
			t.Fatalf("displayDeploymentVersion(%q) = %q, want %q", raw, got, want)
		}
	}
}
