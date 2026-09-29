package routes_test

import (
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"ninimenu/internal/services"
	"ninimenu/internal/testutil"
	"testing"
)

func TestHealthMealStatusPrivacyValidationExportAndCleanup(t *testing.T) {
	user, alice, err := testutil.NewUser(t.Name() + "@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	_, bob, err := testutil.NewUser(t.Name() + "-other@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	r, err := services.BuildHealthReport(user.ID, 7)
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/health/days/" + r.To + "/meals/breakfast/status"
	input := map[string]any{"fingerprint": r.Days[6].Fingerprint, "not_eaten": true, "user_id": 999}
	for _, endpoint := range []struct {
		method, path string
		body         any
	}{{"GET", "/api/health/summary", nil}, {"PUT", path, input}} {
		status, res := call(t, endpoint.method, endpoint.path, "", endpoint.body)
		must(t, status, res, 401)
	}
	for _, invalid := range []any{map[string]any{"fingerprint": r.Days[6].Fingerprint}, map[string]any{"not_eaten": true}, map[string]any{"fingerprint": "x", "not_eaten": "true"}} {
		status, res := call(t, "PUT", path, alice, invalid)
		must(t, status, res, 400)
	}
	status, res := call(t, "PUT", path, alice, input)
	must(t, status, res, 200)
	status, res = call(t, "PUT", path, alice, input)
	must(t, status, res, 409)
	status, res = call(t, "GET", "/api/health/summary", bob, nil)
	must(t, status, res, 200)
	if decode[services.HealthSummary](t, res.Data).NotEatenMeals != 0 {
		t.Fatal("other user's omission exposed")
	}
	status, res = call(t, "GET", "/api/health/summary", alice, nil)
	must(t, status, res, 200)
	if decode[services.HealthSummary](t, res.Data).NotEatenMeals != 1 {
		t.Fatal("own omission missing")
	}
	for _, invalidPath := range []string{"/api/health/days/2099-01-01/meals/lunch/status", "/api/health/days/" + r.To + "/meals/snack/status"} {
		status, res = call(t, "PUT", invalidPath, alice, input)
		must(t, status, res, 400)
	}
	status, res = call(t, "GET", "/api/me/export", alice, nil)
	must(t, status, res, 200)
	exported := decode[struct {
		Omissions []models.HealthMealOmission `json:"health_meal_omissions"`
	}](t, res.Data)
	if len(exported.Omissions) != 1 || exported.Omissions[0].MealType != "breakfast" {
		t.Fatal("export lost omission")
	}
	status, res = call(t, "DELETE", "/api/me", alice, map[string]any{"confirm": "注销"})
	must(t, status, res, 200)
	var n int64
	if err := database.DB.Model(&models.HealthMealOmission{}).Where("user_id = ?", user.ID).Count(&n).Error; err != nil || n != 0 {
		t.Fatalf("account cleanup: %d %v", n, err)
	}
}
