package routes_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"ninimenu/internal/services"
	"ninimenu/internal/testutil"
)

// Wire recipes contain JSON arrays, while the persistence model uses strings.
// The protocol contract only needs identity and recommendation metadata here.
type recommendationContract struct {
	RecommendationID string `json:"recommendation_id"`
	SessionID        string `json:"session_id"`
	AlgorithmVersion string `json:"algorithm_version"`
	ServedForDate    string `json:"served_for_date"`
	Items            []struct {
		ItemID string `json:"item_id"`
		Dish   struct {
			ID uint `json:"id"`
		} `json:"dish"`
	} `json:"items"`
}

func recommendationToken(t *testing.T, login string, scopes ...string) string {
	t.Helper()
	status, res := call(t, "POST", "/api/me/agent-tokens", login, map[string]any{"name": "recommendation-contract", "scopes": scopes})
	must(t, status, res, 200)
	return decode[struct {
		Token string `json:"token"`
	}](t, res.Data).Token
}

func recommendationMCP(t *testing.T, token, method string, params any) map[string]json.RawMessage {
	t.Helper()
	data, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 91, "method": method, "params": params})
	req := httptest.NewRequest("POST", "/mcp", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("MCP status %d: %s", w.Code, w.Body.String())
	}
	var rpc struct {
		Result map[string]json.RawMessage `json:"result"`
		Error  json.RawMessage            `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &rpc); err != nil || len(rpc.Error) > 0 {
		t.Fatalf("MCP response %s: %v", w.Body.String(), err)
	}
	return rpc.Result
}

func TestRecommendationFeedbackIsolationIdempotencyAndPlans(t *testing.T) {
	user, login, _ := testutil.NewUser(t.Name() + "@qq.com")
	_, other, _ := testutil.NewUser(t.Name() + "-other@qq.com")
	status, res := call(t, "POST", "/api/pick/smart", login, map[string]any{"count": 3, "meal_type": "dinner", "mode": "home_auto"})
	must(t, status, res, 200)
	recommendation := decode[recommendationContract](t, res.Data)
	if recommendation.RecommendationID == "" || recommendation.SessionID == "" || recommendation.AlgorithmVersion == "" || recommendation.ServedForDate != services.Today() || len(recommendation.Items) == 0 {
		t.Fatalf("missing recommendation contract: %s", res.Data)
	}
	dishID := recommendation.Items[0].Dish.ID
	feedback := map[string]any{"recommendation_id": recommendation.RecommendationID, "dish_id": dishID, "event_type": "impression", "request_key": "visible-card-1"}
	for _, kind := range []string{"impression", "accept", "skip"} {
		status, res = call(t, "POST", "/api/behavior", login, map[string]any{"event_type": kind, "dish_id": dishID, "meta": map[string]any{"recommendation_id": "invented", "session_id": recommendation.SessionID}})
		must(t, status, res, 400)
	}
	status, res = call(t, "POST", "/api/recommendation-feedback", other, feedback)
	must(t, status, res, 400)
	for i := 0; i < 2; i++ {
		status, res = call(t, "POST", "/api/recommendation-feedback", login, feedback)
		must(t, status, res, 200)
		if created := decode[services.RecommendationFeedbackResult](t, res.Data).Created; created != (i == 0) {
			t.Fatalf("retry created=%v", created)
		}
	}
	feedback["event_type"] = "skip"
	status, res = call(t, "POST", "/api/recommendation-feedback", login, feedback)
	must(t, status, res, 400)
	feedback["request_key"] = "skip-card-1"
	status, res = call(t, "POST", "/api/recommendation-feedback", login, feedback)
	must(t, status, res, 200)
	profile := services.BuildTasteProfile(user.ID, 90)
	for _, d := range profile.DislikedDishes {
		if d.ID == dishID {
			t.Fatal("skip became a long-term dislike")
		}
	}
	status, res = call(t, "POST", "/api/pick/smart", login, map[string]any{"count": 3, "meal_type": "dinner", "session_id": recommendation.SessionID, "refresh": true})
	must(t, status, res, 200)
	next := decode[recommendationContract](t, res.Data)
	for _, item := range next.Items {
		if item.Dish.ID == dishID {
			t.Fatal("skipped dish returned in the same session")
		}
	}
	// A lunch recommendation may be explicitly adopted for dinner, with current
	// constraints checked again. The choice is still a plan, not consumption.
	plan := map[string]any{"dish_id": dishID, "meal_date": services.Today(), "meal_type": "lunch", "recommendation_id": recommendation.RecommendationID}
	for i := 0; i < 2; i++ {
		status, res = call(t, "POST", "/api/meal-plans", login, plan)
		must(t, status, res, 200)
	}
	var n int64
	database.DB.Model(&models.MealRecord{}).Where("user_id = ?", user.ID).Count(&n)
	if n != 0 {
		t.Fatal("plan was treated as eaten")
	}
	database.DB.Model(&models.BehaviorEvent{}).Where("user_id = ? AND event_type = ?", user.ID, "accept").Count(&n)
	if n != 1 {
		t.Fatalf("plan acceptance counted %d times", n)
	}
	// An empty replacement clears just the plan and its unused shopping rows.
	status, res = call(t, "PUT", "/api/meal-plans", login, map[string]any{"meal_date": services.Today(), "meal_type": "lunch", "dish_ids": []uint{}})
	must(t, status, res, 200)
	for _, kind := range []string{"dislike", "undo_dislike", "dislike"} {
		status, res = call(t, "POST", "/api/recommendation-feedback", login, map[string]any{"dish_id": dishID, "event_type": kind})
		must(t, status, res, 200)
		if !decode[services.RecommendationFeedbackResult](t, res.Data).Created {
			t.Fatalf("state change %s was mistaken for a retry", kind)
		}
	}
	profile = services.BuildTasteProfile(user.ID, 90)
	foundDislike := false
	for _, d := range profile.DislikedDishes {
		foundDislike = foundDislike || d.ID == dishID
	}
	if !foundDislike {
		t.Fatal("dislike -> undo -> dislike did not restore dislike")
	}
	status, res = call(t, "DELETE", "/api/me", login, map[string]any{"confirm": "注销"})
	must(t, status, res, 200)
	for _, model := range []any{&models.UserDishAffinity{}, &models.RecommendationSnapshot{}, &models.RecommendationFeedback{}} {
		if err := database.DB.Model(model).Where("user_id = ?", user.ID).Count(&n).Error; err != nil || n != 0 {
			t.Fatalf("account cleanup %T: count=%d err=%v", model, n, err)
		}
	}
}

func TestRecommendationMCPCompatibilityScopesAndFutureLogMeal(t *testing.T) {
	user, login, _ := testutil.NewUser(t.Name() + "@qq.com")
	readonly := recommendationToken(t, login, "readonly")
	full := recommendationToken(t, login, "full")
	tools := recommendationMCP(t, full, "tools/list", map[string]any{})
	toolJSON, _ := json.Marshal(tools)
	for _, field := range []string{"recommend_dishes", "recommendation_feedback", "plan_meal", "list_meal_plans", "replace_meal_plan", "cancel_meal_plan", "session_id", "selected_dish_ids"} {
		if !strings.Contains(string(toolJSON), field) {
			t.Fatalf("MCP does not advertise %s", field)
		}
	}
	result := recommendationMCP(t, readonly, "tools/call", map[string]any{"name": "recommend_dishes", "arguments": map[string]any{"count": 1, "meal_type": "dinner"}})
	if string(result["isError"]) == "true" {
		t.Fatalf("readonly recommendation failed: %s", result["content"])
	}
	var content []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(result["content"], &content); err != nil || len(content) == 0 {
		t.Fatal("missing MCP text content")
	}
	var rec recommendationContract
	if err := json.Unmarshal([]byte(content[0].Text), &rec); err != nil || rec.RecommendationID == "" || len(rec.Items) != 1 || rec.Items[0].ItemID == "" {
		t.Fatalf("MCP contract lost metadata: %s %v", content[0].Text, err)
	}
	dishID := rec.Items[0].Dish.ID
	status, res := call(t, "POST", "/api/agent/recommendation-feedback", readonly, map[string]any{"recommendation_id": rec.RecommendationID, "dish_id": dishID, "event_type": "impression"})
	must(t, status, res, 403)
	status, res = call(t, "POST", "/api/agent/meal-plans", readonly, map[string]any{"dish_id": dishID, "meal_type": "dinner", "meal_date": services.Today()})
	must(t, status, res, 403)
	result = recommendationMCP(t, full, "tools/call", map[string]any{"name": "log_meal", "arguments": map[string]any{"dish_id": dishID, "meal_type": "dinner", "meal_date": "tomorrow"}})
	if string(result["isError"]) == "true" {
		t.Fatalf("legacy future meal not adapted: %s", result["content"])
	}
	if !strings.Contains(string(result["content"]), `\"planned\":true`) {
		t.Fatalf("missing explicit planned status: %s", result["content"])
	}
	var n int64
	database.DB.Model(&models.MealRecord{}).Where("user_id = ?", user.ID).Count(&n)
	if n != 0 {
		t.Fatal("MCP future plan polluted actual records")
	}
	status, res = call(t, "GET", "/api/agent/meal-plans", full, nil)
	must(t, status, res, 200)
	if plans := decode[[]services.HealthPlanView](t, res.Data); len(plans) != 1 || plans[0].DishID != dishID {
		t.Fatalf("missing future plan: %s", res.Data)
	}
	openAPI := httptest.NewRecorder()
	router.ServeHTTP(openAPI, httptest.NewRequest("GET", "/api/agent/openapi.json", nil))
	if openAPI.Code != 200 {
		t.Fatalf("OpenAPI status=%d", openAPI.Code)
	}
	for _, tool := range []string{"plan_meal", "recommendation_feedback", "replace_meal_plan"} {
		if !strings.Contains(openAPI.Body.String(), tool) {
			t.Fatalf("OpenAPI missing %s: %s", tool, fmt.Sprint(openAPI.Code))
		}
	}
}

func TestLegacyFutureRecordWritesBecomePlansAcrossTransports(t *testing.T) {
	u, login, _ := testutil.NewUser(t.Name() + "@qq.com")
	var dish models.Dish
	if err := database.DB.Where("enabled = ?", true).First(&dish).Error; err != nil {
		t.Fatal(err)
	}
	body := map[string]any{"dish_id": dish.ID, "meal_date": "tomorrow", "meal_type": "lunch"}
	status, res := call(t, "POST", "/api/records", login, body)
	must(t, status, res, 200)
	if !decode[struct {
		Planned bool `json:"planned"`
	}](t, res.Data).Planned {
		t.Fatalf("legacy app did not get plan marker: %s", res.Data)
	}
	status, res = call(t, "POST", "/api/records/batch", login, map[string]any{"records": []any{body}})
	must(t, status, res, 200)
	batch := decode[struct {
		Created []json.RawMessage `json:"created"`
		Planned []json.RawMessage `json:"planned"`
	}](t, res.Data)
	if len(batch.Created) != 0 || len(batch.Planned) != 1 {
		t.Fatalf("batch plan/actual mixed: %s", res.Data)
	}
	recorder := recommendationToken(t, login, "records:write")
	status, res = call(t, "POST", "/api/agent/records", recorder, body)
	must(t, status, res, 403)
	full := recommendationToken(t, login, "full")
	status, res = call(t, "POST", "/api/agent/records", full, body)
	must(t, status, res, 200)
	batch = decode[struct {
		Created []json.RawMessage `json:"created"`
		Planned []json.RawMessage `json:"planned"`
	}](t, res.Data)
	if len(batch.Created) != 0 || len(batch.Planned) != 1 {
		t.Fatalf("Agent REST future record did not become a plan: %s", res.Data)
	}
	var n int64
	database.DB.Model(&models.MealRecord{}).Where("user_id = ?", u.ID).Count(&n)
	if n != 0 {
		t.Fatal("a transport bypassed actual-record date validation")
	}
	database.DB.Model(&models.HealthPlanItem{}).Where("user_id = ?", u.ID).Count(&n)
	if n != 1 {
		t.Fatalf("legacy retries created %d plans", n)
	}
}
