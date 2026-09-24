package routes_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"ninimenu/internal/routes"
	"ninimenu/internal/services"
	"ninimenu/internal/testutil"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

var router *gin.Engine

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	cleanup, err := testutil.InitDB()
	if err != nil {
		fmt.Println("init db:", err)
		os.Exit(1)
	}
	router = gin.New()
	routes.Setup(router)
	code := m.Run()
	cleanup()
	os.Exit(code)
}

type apiResp struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func call(t *testing.T, method, path, token string, body any) (int, apiResp) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var r apiResp
	_ = json.Unmarshal(w.Body.Bytes(), &r)
	return w.Code, r
}

func must(t *testing.T, status int, r apiResp, want int) {
	t.Helper()
	if status != want {
		t.Fatalf("status = %d, want %d (message: %s, data: %s)", status, want, r.Message, string(r.Data))
	}
}

func decode[T any](t *testing.T, raw json.RawMessage) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("decode %s: %v", string(raw), err)
	}
	return v
}

type page struct {
	Items []struct {
		ID      uint   `json:"id"`
		Name    string `json:"name"`
		DishID  uint   `json:"dish_id"`
		OwnerID uint   `json:"owner_id"`
	} `json:"items"`
	Total int `json:"total"`
}

// 核心保证：用户之间的数据完全隔离，智能体令牌只能访问签发者本人的数据且受 scope 限制。
func TestMultiUserIsolationAndAgentScopes(t *testing.T) {
	alice, aliceTok, err := testutil.NewUser("alice@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	_, bobTok, err := testutil.NewUser("bob@qq.com")
	if err != nil {
		t.Fatal(err)
	}

	// 未登录访问
	st, r := call(t, "GET", "/api/records", "", nil)
	must(t, st, r, http.StatusUnauthorized)

	// 公共菜谱对所有人可见
	st, r = call(t, "GET", "/api/dishes?pageSize=1&sort=sort_order&order=asc", aliceTok, nil)
	must(t, st, r, http.StatusOK)
	publicDish := decode[page](t, r.Data).Items[0]

	// Alice 的私房菜，Bob 看不到也改不了
	st, r = call(t, "POST", "/api/dishes", aliceTok, map[string]any{"name": "Alice 秘制红烧肉", "category": "湘菜", "taste": "咸香", "ingredients": `[{"name":"五花肉","amount":"500g"}]`})
	must(t, st, r, http.StatusOK)
	private := decode[struct {
		ID      uint `json:"id"`
		OwnerID uint `json:"owner_id"`
	}](t, r.Data)
	if private.OwnerID != alice.ID {
		t.Fatalf("private dish owner = %d, want %d", private.OwnerID, alice.ID)
	}
	st, r = call(t, "GET", fmt.Sprintf("/api/dishes/%d", private.ID), bobTok, nil)
	must(t, st, r, http.StatusNotFound)
	st, r = call(t, "GET", "/api/dishes?search="+"Alice", bobTok, nil)
	must(t, st, r, http.StatusOK)
	if got := decode[page](t, r.Data).Total; got != 0 {
		t.Fatalf("bob sees %d of alice's private dishes", got)
	}
	st, r = call(t, "PUT", fmt.Sprintf("/api/dishes/%d", publicDish.ID), bobTok, map[string]any{"name": "被篡改"})
	must(t, st, r, http.StatusForbidden)

	// 用餐记录隔离
	today := services.Today()
	st, r = call(t, "POST", "/api/records", aliceTok, map[string]any{"dish_id": publicDish.ID, "meal_type": "lunch", "meal_date": today})
	must(t, st, r, http.StatusOK)
	aliceRecord := decode[struct {
		ID uint `json:"id"`
	}](t, r.Data)
	st, r = call(t, "GET", "/api/records", bobTok, nil)
	must(t, st, r, http.StatusOK)
	if got := decode[page](t, r.Data).Total; got != 0 {
		t.Fatalf("bob sees %d of alice's records", got)
	}
	st, r = call(t, "POST", "/api/records", bobTok, map[string]any{"dish_id": publicDish.ID, "meal_type": "lunch", "meal_date": today})
	must(t, st, r, http.StatusOK)
	bobRecord := decode[struct {
		ID uint `json:"id"`
	}](t, r.Data)
	st, r = call(t, "GET", "/api/records", bobTok, nil)
	must(t, st, r, http.StatusOK)
	if got := decode[page](t, r.Data).Total; got != 1 {
		t.Fatalf("bob sees %d records after logging a meal, want 1", got)
	}
	st, r = call(t, "DELETE", fmt.Sprintf("/api/records/%d", bobRecord.ID), aliceTok, nil)
	must(t, st, r, http.StatusNotFound)
	st, r = call(t, "DELETE", fmt.Sprintf("/api/records/%d", aliceRecord.ID), bobTok, nil)
	must(t, st, r, http.StatusNotFound)
	st, r = call(t, "POST", "/api/records", bobTok, map[string]any{"dish_id": private.ID, "meal_type": "lunch", "meal_date": today})
	must(t, st, r, http.StatusNotFound)

	// 收藏是每个人自己的
	st, r = call(t, "POST", fmt.Sprintf("/api/favorites/%d", publicDish.ID), aliceTok, nil)
	must(t, st, r, http.StatusOK)
	st, r = call(t, "GET", fmt.Sprintf("/api/dishes/%d", publicDish.ID), bobTok, nil)
	must(t, st, r, http.StatusOK)
	if decode[struct {
		Favorite bool `json:"favorite"`
	}](t, r.Data).Favorite {
		t.Fatal("alice's favorite leaked to bob")
	}

	// 管理接口
	st, r = call(t, "GET", "/api/admin/dashboard", bobTok, nil)
	must(t, st, r, http.StatusForbidden)

	// ---------- 智能体：只读令牌 ----------
	st, r = call(t, "POST", "/api/me/agent-tokens", aliceTok, map[string]any{"name": "Hermes", "scopes": []string{"readonly"}})
	must(t, st, r, http.StatusOK)
	created := decode[struct {
		Token string `json:"token"`
		Info  struct {
			ID uint `json:"id"`
		} `json:"info"`
	}](t, r.Data)
	pat := created.Token
	if !strings.HasPrefix(pat, "nm_") {
		t.Fatalf("unexpected token %q", pat)
	}

	st, r = call(t, "GET", "/api/agent/records", pat, nil)
	must(t, st, r, http.StatusOK)
	if got := decode[page](t, r.Data).Total; got != 1 {
		t.Fatalf("agent sees %d records, want 1 (alice's only)", got)
	}
	st, r = call(t, "POST", "/api/agent/records", pat, map[string]any{"dish_id": publicDish.ID, "meal_type": "dinner"})
	must(t, st, r, http.StatusForbidden)
	st, r = call(t, "POST", "/api/agent/tools/log_meal", pat, map[string]any{"dish_id": publicDish.ID, "meal_type": "dinner"})
	must(t, st, r, http.StatusForbidden)
	st, r = call(t, "GET", "/api/agent/tools", pat, nil)
	must(t, st, r, http.StatusOK)
	if strings.Contains(string(r.Data), `"log_meal"`) {
		t.Fatal("readonly token should not list write tools")
	}
	// 个性化推荐会返回画像摘要，因此必须同时授权菜品和画像读取。
	st, r = call(t, "POST", "/api/me/agent-tokens", aliceTok, map[string]any{"name": "Dishes only", "scopes": []string{"dishes:read"}})
	must(t, st, r, http.StatusOK)
	dishesOnly := decode[struct {
		Token string `json:"token"`
	}](t, r.Data).Token
	st, r = call(t, "POST", "/api/agent/recommend", dishesOnly, map[string]any{"meal_type": "dinner", "count": 1})
	must(t, st, r, http.StatusForbidden)
	st, r = call(t, "POST", "/api/agent/tools/get_context", dishesOnly, nil)
	must(t, st, r, http.StatusForbidden)
	st, r = call(t, "POST", "/api/agent/tools/recommend_dishes", dishesOnly, map[string]any{"meal_type": "dinner", "count": 1})
	must(t, st, r, http.StatusForbidden)
	st, r = call(t, "GET", "/api/agent/tools", dishesOnly, nil)
	must(t, st, r, http.StatusOK)
	if strings.Contains(string(r.Data), `"recommend_dishes"`) {
		t.Fatal("dishes-only token should not list profile-based recommendation tool")
	}
	if strings.Contains(string(r.Data), `"get_context"`) {
		t.Fatal("dishes-only token should not list today's private meal context")
	}
	st, r = call(t, "POST", "/api/agent/recommend", pat, map[string]any{"meal_type": "dinner", "count": 1, "ignore_preferences": true})
	must(t, st, r, http.StatusOK)
	recommendation := decode[struct {
		Applied struct {
			PreferencesApplied bool `json:"preferences_applied"`
		} `json:"applied"`
	}](t, r.Data)
	if !recommendation.Applied.PreferencesApplied {
		t.Fatal("external agent must not bypass user dietary preferences")
	}
	st, r = call(t, "GET", "/api/agent/capabilities", dishesOnly, nil)
	must(t, st, r, http.StatusOK)
	if strings.Contains(string(r.Data), `"recommend_dishes"`) {
		t.Fatal("dishes-only token should not see profile-based recommendation in capabilities")
	}
	// 智能体看不到 Bob 的私房菜 / Alice 看得到自己的
	st, r = call(t, "POST", "/api/agent/tools/get_dish", pat, map[string]any{"dish_id": private.ID})
	must(t, st, r, http.StatusOK)
	// 令牌不能冒充用户登录态
	st, r = call(t, "GET", "/api/me", pat, nil)
	must(t, st, r, http.StatusUnauthorized)

	// MCP
	mcp := func(body string) (int, map[string]any) {
		req := httptest.NewRequest("POST", "/mcp", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("Authorization", "Bearer "+pat)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}
	code, out := mcp(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`)
	if code != 200 || out["result"].(map[string]any)["protocolVersion"] != "2025-06-18" {
		t.Fatalf("mcp initialize failed: %d %v", code, out)
	}
	if code, _ := mcp(`{"jsonrpc":"2.0","method":"notifications/initialized"}`); code != http.StatusAccepted {
		t.Fatalf("notification status = %d", code)
	}
	_, out = mcp(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	toolsJSON, _ := json.Marshal(out)
	if !strings.Contains(string(toolsJSON), "recommend_dishes") || strings.Contains(string(toolsJSON), `"log_meal"`) {
		t.Fatalf("unexpected mcp tools: %s", toolsJSON)
	}
	_, out = mcp(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"get_context","arguments":{}}}`)
	ctxJSON, _ := json.Marshal(out)
	if !strings.Contains(string(ctxJSON), "alice@qq.com") {
		t.Fatalf("mcp get_context should be alice's: %s", ctxJSON)
	}
	_, out = mcp(`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"log_meal","arguments":{"dish_id":1,"meal_type":"dinner"}}}`)
	if out["result"].(map[string]any)["isError"] != true {
		t.Fatalf("forbidden tool should return isError: %v", out)
	}
	_, out = mcp(`{"jsonrpc":"2.0","id":5,"method":"nope"}`)
	if out["error"] == nil {
		t.Fatal("unknown method should error")
	}

	// 撤销后立即失效
	st, r = call(t, "DELETE", fmt.Sprintf("/api/me/agent-tokens/%d", created.Info.ID), aliceTok, nil)
	must(t, st, r, http.StatusOK)
	st, r = call(t, "GET", "/api/agent/me", pat, nil)
	must(t, st, r, http.StatusUnauthorized)

	// ---------- 智能体：顾问令牌推送建议，用户采纳 ----------
	st, r = call(t, "POST", "/api/me/agent-tokens", aliceTok, map[string]any{"name": "DeepSeek", "scopes": []string{"advisor"}})
	must(t, st, r, http.StatusOK)
	advisor := decode[struct {
		Token string `json:"token"`
	}](t, r.Data).Token
	st, r = call(t, "POST", "/api/agent/suggestions", advisor, map[string]any{
		"title": "明晚试试这道", "dish_ids": []uint{publicDish.ID}, "meal_type": "dinner", "meal_date": "tomorrow",
	})
	must(t, st, r, http.StatusOK)
	st, r = call(t, "POST", "/api/agent/suggestions", advisor, map[string]any{"dish_ids": []uint{99999999}})
	must(t, st, r, http.StatusBadRequest)

	st, r = call(t, "GET", "/api/suggestions", bobTok, nil)
	must(t, st, r, http.StatusOK)
	if string(r.Data) != "[]" {
		t.Fatalf("bob sees alice's suggestions: %s", r.Data)
	}
	st, r = call(t, "GET", "/api/suggestions", aliceTok, nil)
	must(t, st, r, http.StatusOK)
	suggestions := decode[[]struct {
		ID uint `json:"id"`
	}](t, r.Data)
	if len(suggestions) != 1 {
		t.Fatalf("alice suggestions = %d", len(suggestions))
	}
	st, r = call(t, "POST", fmt.Sprintf("/api/suggestions/%d/resolve", suggestions[0].ID), bobTok, map[string]any{"accept": true})
	must(t, st, r, http.StatusBadRequest)
	st, r = call(t, "POST", fmt.Sprintf("/api/suggestions/%d/resolve", suggestions[0].ID), aliceTok, map[string]any{"accept": true})
	must(t, st, r, http.StatusOK)
	if got := len(decode[struct {
		Created []any `json:"created"`
	}](t, r.Data).Created); got != 1 {
		t.Fatalf("accepting suggestion created %d records", got)
	}

	// ---------- 嵌入式会话令牌 ----------
	st, r = call(t, "POST", "/api/me/agent-session", aliceTok, map[string]any{"scopes": []string{"readonly"}, "actor": "recipe"})
	must(t, st, r, http.StatusOK)
	session := decode[struct {
		Token string `json:"token"`
	}](t, r.Data).Token
	st, r = call(t, "GET", "/api/agent/me", session, nil)
	must(t, st, r, http.StatusOK)
	if !strings.Contains(string(r.Data), `"actor":"recipe"`) {
		t.Fatalf("unexpected session identity: %s", r.Data)
	}
	st, r = call(t, "GET", "/api/me", session, nil)
	must(t, st, r, http.StatusUnauthorized)

	// 审计日志只含 Alice 自己的
	st, r = call(t, "GET", "/api/me/agent-audit", bobTok, nil)
	must(t, st, r, http.StatusOK)
	if decode[page](t, r.Data).Total != 0 {
		t.Fatal("bob sees alice's audit log")
	}
	st, r = call(t, "GET", "/api/me/agent-audit", aliceTok, nil)
	must(t, st, r, http.StatusOK)
	if decode[page](t, r.Data).Total == 0 {
		t.Fatal("alice should see her agents' audit trail")
	}
}

// 未配置大模型时，AI 助手退化为本地推荐引擎，仍以 SSE 返回结果；会话按用户隔离。
func TestAssistantFallbackStreamsAndIsolatesSessions(t *testing.T) {
	_, tok, err := testutil.NewUser("carol@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	_, otherTok, _ := testutil.NewUser("dave@qq.com")

	req := httptest.NewRequest("POST", "/api/assistant/chat", strings.NewReader(`{"message":"今晚想吃辣的"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	body := w.Body.String()
	for _, want := range []string{"event: session", "event: tool_end", "recommend_dishes", "event: done"} {
		if !strings.Contains(body, want) {
			t.Fatalf("SSE missing %q:\n%s", want, body)
		}
	}

	st, r := call(t, "GET", "/api/assistant/sessions", tok, nil)
	must(t, st, r, http.StatusOK)
	sessions := decode[[]struct {
		ID uint `json:"id"`
	}](t, r.Data)
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d", len(sessions))
	}
	st, r = call(t, "GET", fmt.Sprintf("/api/assistant/sessions/%d", sessions[0].ID), otherTok, nil)
	must(t, st, r, http.StatusNotFound)
	st, r = call(t, "GET", fmt.Sprintf("/api/assistant/sessions/%d", sessions[0].ID), tok, nil)
	must(t, st, r, http.StatusOK)
	if !strings.Contains(string(r.Data), `"role":"assistant"`) {
		t.Fatalf("assistant reply not persisted: %s", r.Data)
	}
}

func TestAssistantWithoutModelDoesNotWriteJournal(t *testing.T) {
	_, token, err := testutil.NewUser("fallback-write@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	st, r := call(t, "GET", "/api/assistant/status", token, nil)
	must(t, st, r, http.StatusOK)
	status := decode[struct {
		Enabled     bool     `json:"llm_enabled"`
		Suggestions []string `json:"suggestions"`
	}](t, r.Data)
	if status.Enabled {
		t.Skip("this test requires the local fallback")
	}
	for _, suggestion := range status.Suggestions {
		if strings.Contains(suggestion, "帮我记") {
			t.Fatalf("write suggestion shown without a model: %q", suggestion)
		}
	}

	req := httptest.NewRequest("POST", "/api/assistant/chat", strings.NewReader(`{"message":"帮我记下今天午餐吃了番茄面"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "无法可靠地从对话写入数据") || !strings.Contains(w.Body.String(), "event: done") {
		t.Fatalf("unexpected fallback response: status=%d body=%s", w.Code, w.Body.String())
	}
	st, r = call(t, "GET", "/api/food-journal", token, nil)
	must(t, st, r, http.StatusOK)
	if entries := decode[[]json.RawMessage](t, r.Data); len(entries) != 0 {
		t.Fatalf("fallback wrote %d journal entries", len(entries))
	}
}

func TestPreferencesDriveRecommendations(t *testing.T) {
	_, tok, _ := testutil.NewUser("erin@qq.com")
	st, r := call(t, "PUT", "/api/me/preferences", tok, map[string]any{"allergies": []string{"花生"}, "spice_level": 0})
	must(t, st, r, http.StatusOK)

	for i := 0; i < 5; i++ {
		st, r = call(t, "POST", "/api/pick/smart", tok, map[string]any{"count": 10})
		must(t, st, r, http.StatusOK)
		items := decode[struct {
			Items []struct {
				Dish struct {
					Name        string          `json:"name"`
					Ingredients json.RawMessage `json:"ingredients"`
					Seasonings  json.RawMessage `json:"seasonings"`
				} `json:"dish"`
			} `json:"items"`
		}](t, r.Data).Items
		for _, it := range items {
			text := it.Dish.Name + string(it.Dish.Ingredients) + string(it.Dish.Seasonings)
			if strings.Contains(text, "花生") {
				t.Fatalf("allergen leaked into recommendation: %s", it.Dish.Name)
			}
		}
	}
}

func TestOpenAPIAndHealth(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/agent/openapi.json", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"operationId":"recommend_dishes"`) {
		t.Fatalf("openapi: %d %s", w.Code, w.Body.String()[:200])
	}
	req = httptest.NewRequest("GET", "/healthz", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("healthz = %d", w.Code)
	}
	req = httptest.NewRequest("GET", "/api/does-not-exist", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != 404 || !strings.Contains(w.Body.String(), "40400") {
		t.Fatalf("unknown api = %d %s", w.Code, w.Body.String())
	}
}
