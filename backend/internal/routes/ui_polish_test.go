package routes_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"ninimenu/internal/services"
	"ninimenu/internal/testutil"
	"strings"
	"testing"
)

func TestPathIDsCannotBecomeSQLExpressions(t *testing.T) {
	user, token, err := testutil.NewUser("path-ids@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"1 OR 1=1", "0", "-1", "+1", "1.0", "9223372036854775808", "18446744073709551616"} {
		for _, target := range []struct{ method, prefix string }{{"GET", "/api/dishes/"}, {"DELETE", "/api/records/"}, {"POST", "/api/favorites/"}} {
			st, r := call(t, target.method, target.prefix+url.PathEscape(raw), token, nil)
			must(t, st, r, http.StatusBadRequest)
		}
	}
	if dish, err := services.FindVisibleDish(user.ID, "1 OR 1=1"); err == nil || dish.ID != 0 {
		t.Fatal("service must bind a string identifier instead of interpreting it as SQL")
	}
}

func TestMalformedRecipeReplacementPreservesSavedContent(t *testing.T) {
	_, token, err := testutil.NewUser("recipe-validation@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Repeat("小火慢煮并轻轻搅拌，", 40)
	steps, _ := json.Marshal([]map[string]any{{"text": text, "time": 5}})
	st, r := call(t, "POST", "/api/dishes", token, map[string]any{"name": "完整长步骤", "steps": string(steps)})
	must(t, st, r, http.StatusOK)
	id := decode[struct {
		ID uint `json:"id"`
	}](t, r.Data).ID
	path := fmt.Sprintf("/api/dishes/%d", id)
	for _, patch := range []map[string]any{
		{"steps": "[{"}, {"steps": "null"}, {"steps": "{}"}, {"steps": "[null]"},
		{"images": "[{}]"}, {"ingredients": "[42]"}, {"tags": "[null]"},
		{"ingredients": `[{"name":"鸡蛋","amount":12}]`},
		{"steps": `[{"text":"煮熟","time":"稍后"}]`}, {"steps": `[{"text":"煮熟","time":-1}]`},
		{"steps": `[{"text":"煮熟","time":601}]`}, {"steps": `[{"text":"煮熟","image":{}}]`},
		{"meal_type": "unexpected"}, {"difficulty": "unexpected"}, {"cook_time": -1}, {"cook_time": 601},
	} {
		patch["name"] = "不应覆盖原标题"
		st, r = call(t, "PUT", path, token, patch)
		must(t, st, r, http.StatusBadRequest)
		st, r = call(t, "GET", path, token, nil)
		must(t, st, r, http.StatusOK)
		saved := decode[struct {
			Name  string `json:"name"`
			Steps []struct {
				Text string `json:"text"`
			} `json:"steps"`
		}](t, r.Data)
		if saved.Name != "完整长步骤" || len(saved.Steps) != 1 || saved.Steps[0].Text != text {
			t.Fatal("invalid replacement changed the original recipe")
		}
	}
}

func TestMineCategoryCountsExcludePublicAndOtherUsers(t *testing.T) {
	alice, token, err := testutil.NewUser("category-scope@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	bob, _, err := testutil.NewUser("category-other@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	for _, dish := range []models.Dish{
		{Name: "自己的菜", OwnerID: alice.ID, Category: "自定义分类", Enabled: true},
		{Name: "别人的菜", OwnerID: bob.ID, Category: "其他分类", Enabled: true},
	} {
		if err := database.DB.Create(&dish).Error; err != nil {
			t.Fatal(err)
		}
	}
	st, r := call(t, "GET", "/api/dishes/category-counts?scope=mine", token, nil)
	must(t, st, r, http.StatusOK)
	got := decode[struct {
		Total      int `json:"total"`
		Categories []struct {
			Category string `json:"category"`
			Count    int    `json:"count"`
		} `json:"categories"`
	}](t, r.Data)
	if got.Total != 1 || len(got.Categories) != 1 || got.Categories[0].Category != "自定义分类" {
		t.Fatalf("private categories include unrelated recipes: %+v", got)
	}
}
