package routes_test

import (
	"fmt"
	"net/http"
	"ninimenu/internal/services"
	"ninimenu/internal/testutil"
	"strings"
	"testing"
)

func TestFamilyInvitationAndIsolation(t *testing.T) {
	alice, aliceToken, err := testutil.NewUser("family-alice@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	_, bobToken, err := testutil.NewUser("family-bob@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	_, strangerToken, err := testutil.NewUser("family-stranger@qq.com")
	if err != nil {
		t.Fatal(err)
	}

	status, response := call(t, "POST", "/api/family", aliceToken, map[string]any{"name": "测试家庭"})
	must(t, status, response, http.StatusOK)
	status, response = call(t, "POST", "/api/family/invitations", aliceToken, map[string]any{"email": "family-bob@qq.com"})
	must(t, status, response, http.StatusOK)
	invite := decode[struct {
		Link string `json:"link"`
	}](t, response.Data)
	parts := strings.Split(invite.Link, "#invite=")
	if len(parts) != 2 || len(parts[1]) != 32 {
		t.Fatalf("invalid invitation link: %q", invite.Link)
	}
	status, response = call(t, "POST", "/api/family/join", strangerToken, map[string]any{"token": parts[1]})
	must(t, status, response, http.StatusForbidden)
	status, response = call(t, "POST", "/api/family/join", bobToken, map[string]any{"token": parts[1]})
	must(t, status, response, http.StatusOK)
	status, response = call(t, "POST", "/api/family/join", bobToken, map[string]any{"token": parts[1]})
	must(t, status, response, http.StatusBadRequest)

	status, response = call(t, "POST", "/api/dishes", aliceToken, map[string]any{"name": "家传汤", "category": "家常"})
	must(t, status, response, http.StatusOK)
	private := decode[struct {
		ID uint `json:"id"`
	}](t, response.Data)
	status, response = call(t, "POST", fmt.Sprintf("/api/family/dishes/%d/share", private.ID), aliceToken, nil)
	must(t, status, response, http.StatusOK)
	shared := decode[struct {
		ID       uint `json:"id"`
		FamilyID uint `json:"family_id"`
	}](t, response.Data)
	if shared.FamilyID == 0 {
		t.Fatal("shared dish has no family")
	}
	status, response = call(t, "GET", fmt.Sprintf("/api/dishes/%d", shared.ID), bobToken, nil)
	must(t, status, response, http.StatusOK)
	status, response = call(t, "GET", fmt.Sprintf("/api/dishes/%d", shared.ID), strangerToken, nil)
	must(t, status, response, http.StatusNotFound)
	status, response = call(t, "PUT", "/api/family/plan", bobToken, map[string]any{"meal_date": services.Today(), "meal_type": "dinner", "dish_id": shared.ID})
	must(t, status, response, http.StatusOK)
	status, response = call(t, "GET", "/api/family/plan", strangerToken, nil)
	must(t, status, response, http.StatusNotFound)
	status, response = call(t, "POST", "/api/family/leave", aliceToken, nil)
	must(t, status, response, http.StatusBadRequest)
	_ = alice
}

func TestHealthJournalReportAndAgentScope(t *testing.T) {
	_, aliceToken, err := testutil.NewUser("health-alice@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	_, bobToken, err := testutil.NewUser("health-bob@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	today := services.Today()
	status, response := call(t, "POST", "/api/food-journal", aliceToken, map[string]any{
		"meal_date": today, "meal_type": "lunch", "dish_name": "青椒肉丝", "cuisine": "川菜", "food_groups": []string{"vegetable", "protein"},
	})
	must(t, status, response, http.StatusOK)
	entry := decode[struct {
		ID uint `json:"id"`
	}](t, response.Data)
	status, response = call(t, "GET", "/api/food-journal", bobToken, nil)
	must(t, status, response, http.StatusOK)
	if got := decode[[]any](t, response.Data); len(got) != 0 {
		t.Fatalf("bob sees alice journal: %v", got)
	}
	status, response = call(t, "GET", "/api/health-report?days=7", bobToken, nil)
	must(t, status, response, http.StatusOK)
	if got := decode[struct {
		MealCount int `json:"meal_count"`
	}](t, response.Data).MealCount; got != 0 {
		t.Fatalf("bob report has %d entries", got)
	}
	status, response = call(t, "DELETE", fmt.Sprintf("/api/food-journal/%d", entry.ID), bobToken, nil)
	must(t, status, response, http.StatusNotFound)
	status, response = call(t, "GET", "/api/health-report?days=7", aliceToken, nil)
	must(t, status, response, http.StatusOK)
	if got := decode[struct {
		MealCount int `json:"meal_count"`
	}](t, response.Data).MealCount; got != 1 {
		t.Fatalf("alice report has %d entries", got)
	}

	status, response = call(t, "POST", "/api/me/agent-tokens", bobToken, map[string]any{"name": "Diet Reader", "scopes": []string{"readonly"}})
	must(t, status, response, http.StatusOK)
	readToken := decode[struct {
		Token string `json:"token"`
	}](t, response.Data).Token
	status, response = call(t, "POST", "/api/agent/tools/get_health_report", readToken, map[string]any{"days": 7})
	must(t, status, response, http.StatusOK)
	if got := decode[struct {
		MealCount int `json:"meal_count"`
	}](t, response.Data).MealCount; got != 0 {
		t.Fatalf("agent sees another user's %d entries", got)
	}
	status, response = call(t, "POST", "/api/agent/tools/log_food_journal", readToken, map[string]any{"meal_type": "lunch", "dish_name": "越权"})
	must(t, status, response, http.StatusForbidden)

	status, response = call(t, "POST", "/api/me/agent-tokens", bobToken, map[string]any{"name": "Diet Writer", "scopes": []string{"full"}})
	must(t, status, response, http.StatusOK)
	writeToken := decode[struct {
		Token string `json:"token"`
	}](t, response.Data).Token
	status, response = call(t, "POST", "/api/agent/tools/log_food_journal", writeToken, map[string]any{"meal_type": "dinner", "dish_name": "杂粮饭"})
	must(t, status, response, http.StatusOK)
	status, response = call(t, "POST", "/api/agent/tools/list_food_journal", writeToken, map[string]any{})
	must(t, status, response, http.StatusOK)
	if got := decode[struct {
		Total int `json:"total"`
	}](t, response.Data).Total; got != 1 {
		t.Fatalf("agent journal count = %d", got)
	}
}

func TestAgentPrivateRecipeManagement(t *testing.T) {
	_, aliceLogin, err := testutil.NewUser("recipe-alice@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	_, bobLogin, err := testutil.NewUser("recipe-bob@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	makeToken := func(login, name, preset string) string {
		status, response := call(t, "POST", "/api/me/agent-tokens", login, map[string]any{"name": name, "scopes": []string{preset}})
		must(t, status, response, http.StatusOK)
		return decode[struct {
			Token string `json:"token"`
		}](t, response.Data).Token
	}
	aliceRead := makeToken(aliceLogin, "Recipe Reader", "readonly")
	aliceWrite := makeToken(aliceLogin, "Recipe Writer", "full")
	bobWrite := makeToken(bobLogin, "Recipe Stranger", "full")
	status, response := call(t, "POST", "/api/agent/tools/create_private_recipe", aliceRead, map[string]any{"name": "越权菜"})
	must(t, status, response, http.StatusForbidden)
	status, response = call(t, "POST", "/api/agent/tools/create_private_recipe", aliceWrite, map[string]any{
		"name": "番茄面", "category": "家常", "ingredients": []string{"番茄 2 个", "面条 100 克"}, "steps": []string{"番茄切块", "煮熟面条"},
	})
	must(t, status, response, http.StatusOK)
	created := decode[struct {
		ID          uint     `json:"id"`
		OwnerID     uint     `json:"owner_id"`
		FamilyID    uint     `json:"family_id"`
		Ingredients []string `json:"ingredients"`
	}](t, response.Data)
	if created.OwnerID == 0 || created.FamilyID != 0 || len(created.Ingredients) != 2 {
		t.Fatalf("invalid private recipe: %+v", created)
	}
	status, response = call(t, "POST", "/api/agent/tools/update_private_recipe", bobWrite, map[string]any{"dish_id": created.ID, "name": "越权修改"})
	must(t, status, response, http.StatusNotFound)
	status, response = call(t, "POST", "/api/agent/tools/delete_private_recipe", bobWrite, map[string]any{"dish_id": created.ID})
	must(t, status, response, http.StatusNotFound)
	status, response = call(t, "POST", "/api/agent/tools/update_private_recipe", aliceWrite, map[string]any{"dish_id": created.ID, "name": "番茄鸡蛋面"})
	must(t, status, response, http.StatusOK)
	updated := decode[struct {
		Name        string   `json:"name"`
		Ingredients []string `json:"ingredients"`
	}](t, response.Data)
	if updated.Name != "番茄鸡蛋面" || len(updated.Ingredients) != 2 {
		t.Fatalf("unexpected updated recipe: %+v", updated)
	}
	status, response = call(t, "POST", "/api/agent/tools/delete_private_recipe", aliceWrite, map[string]any{"dish_id": created.ID})
	must(t, status, response, http.StatusOK)
	status, response = call(t, "GET", fmt.Sprintf("/api/dishes/%d", created.ID), aliceLogin, nil)
	must(t, status, response, http.StatusNotFound)
}
