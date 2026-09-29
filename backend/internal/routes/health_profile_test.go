package routes_test

import (
	"net/http"
	"net/http/httptest"
	"ninimenu/internal/auth"
	"strings"
	"testing"
	"time"

	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"ninimenu/internal/services"
	"ninimenu/internal/testutil"
)

func TestHealthProfileRoutesPrivacyConflictsExportAndCleanup(t *testing.T) {
	user, alice, _ := testutil.NewUser("health-profile-route@qq.com")
	_, bob, _ := testutil.NewUser("health-profile-route-bob@qq.com")
	body := map[string]any{"confirmed": true, "version": 0, "goal": "balanced", "allergies": []string{"花生"}, "dietary_exclusions": []string{"猪肉"}, "user_id": 999}
	status, res := call(t, "GET", "/api/health/profile", "", nil)
	must(t, status, res, http.StatusUnauthorized)
	status, res = call(t, "PUT", "/api/health/profile", alice, body)
	must(t, status, res, http.StatusOK)
	if decode[models.HealthProfile](t, res.Data).Version != 1 {
		t.Fatal("profile not saved")
	}
	status, res = call(t, "PUT", "/api/health/profile", alice, body)
	must(t, status, res, http.StatusConflict)
	status, res = call(t, "GET", "/api/health/profile", bob, nil)
	must(t, status, res, http.StatusOK)
	if decode[models.HealthProfile](t, res.Data).Active {
		t.Fatal("other user profile exposed")
	}
	status, res = call(t, "GET", "/api/me/export", alice, nil)
	must(t, status, res, http.StatusOK)
	exported := decode[struct {
		Profile  models.HealthProfile          `json:"health_profile"`
		Versions []models.HealthProfileVersion `json:"health_profile_versions"`
	}](t, res.Data)
	if !exported.Profile.Active || len(exported.Versions) != 1 || exported.Versions[0].Snapshot.Allergies[0] != "花生" {
		t.Fatal("export lost version or private data")
	}
	status, res = call(t, "DELETE", "/api/health/profile?version=0", alice, nil)
	must(t, status, res, http.StatusConflict)
	status, res = call(t, "DELETE", "/api/health/profile", alice, nil)
	must(t, status, res, http.StatusBadRequest)
	status, res = call(t, "DELETE", "/api/health/profile?version=1", alice, nil)
	must(t, status, res, http.StatusOK)
	if decode[models.HealthProfile](t, res.Data).Active {
		t.Fatal("clear failed")
	}
	body["version"] = 2
	status, res = call(t, "PUT", "/api/health/profile", alice, body)
	must(t, status, res, http.StatusOK)
	status, res = call(t, "DELETE", "/api/me", alice, map[string]any{"confirm": "注销"})
	must(t, status, res, http.StatusOK)
	for _, model := range []any{&models.HealthProfile{}, &models.HealthProfileVersion{}} {
		var n int64
		if err := database.DB.Model(model).Where("user_id = ?", user.ID).Count(&n).Error; err != nil || n != 0 {
			t.Fatalf("cleanup failed: %d %v", n, err)
		}
	}
	if _, err := services.SaveHealthProfile(user.ID, services.HealthProfileInput{Confirmed: true}); err == nil {
		t.Fatal("deleted account resurrected health profile")
	}
}

func TestHealthProfileUnavailableToLegacyAgentCredentials(t *testing.T) {
	user, _, _ := testutil.NewUser("health-profile-agent@qq.com")
	_, err := services.SaveHealthProfile(user.ID, services.HealthProfileInput{Confirmed: true, Allergies: []string{"私密测试食材"}})
	if err != nil {
		t.Fatal(err)
	}
	pat, _, err := services.CreateAgentToken(user.ID, "health privacy", []string{"readonly"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	session, _, err := auth.IssueAgentSession(&user, []string{"readonly"}, "health privacy", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{pat, session} {
		for _, method := range []string{"GET", "PUT", "DELETE"} {
			req := httptest.NewRequest(method, "/api/health/profile", strings.NewReader(`{"version":1,"confirmed":true}`))
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("Agent reached private profile: %s %d", method, w.Code)
			}
		}
	}
	for _, endpoint := range []string{"/api/agent/preferences", "/api/agent/profile", "/api/agent/export"} {
		req := httptest.NewRequest("GET", endpoint, nil)
		req.Header.Set("X-Agent-Token", pat)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK || strings.Contains(w.Body.String(), "私密测试食材") || strings.Contains(w.Body.String(), "health_profile") {
			t.Fatalf("legacy Agent response boundary: %s %d", endpoint, w.Code)
		}
	}
}
