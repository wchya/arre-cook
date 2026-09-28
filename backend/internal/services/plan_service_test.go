package services

import (
	"encoding/json"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"ninimenu/internal/testutil"
	"strings"
	"testing"
)

func TestWeekPlanPersistentCacheIsolationAndFavorites(t *testing.T) {
	alice, _, err := testutil.NewUser("plan-memory-alice@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	bob, _, err := testutil.NewUser("plan-memory-bob@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	plan := func(name string) *WeekPlan {
		return &WeekPlan{Days: []WeekDayPlan{{Date: getCurrentWeekKey(), Lunch: []models.Dish{{ID: 1, Name: name, Ingredients: `[{"name":"鸡蛋","amount":"2个"}]`, VideoMeta: `{"title":"做菜视频"}`}}}}}
	}
	saveWeekPlanCache(alice.ID, plan("Alice menu"))
	saveWeekPlanCache(bob.ID, plan("Bob menu"))
	first := GetCachedWeekPlan(alice.ID)
	if len(first.Days) != 1 || first.Days[0].Lunch[0].Name != "Alice menu" {
		t.Fatal("persistent plan was regenerated")
	}
	if dish := first.Days[0].Lunch[0]; dish.Ingredients != plan("").Days[0].Lunch[0].Ingredients || dish.VideoMeta != plan("").Days[0].Lunch[0].VideoMeta {
		t.Fatal("JSON columns or video metadata were lost from the persisted plan")
	}
	first.Days[0].Lunch[0].Name = "client edit"
	if err := database.DB.Create(&models.Favorite{UserID: alice.ID, DishID: 1}).Error; err != nil {
		t.Fatal(err)
	}
	second, other := GetCachedWeekPlan(alice.ID), GetCachedWeekPlan(bob.ID)
	if second.Days[0].Lunch[0].Name != "Alice menu" || !second.Days[0].Lunch[0].Favorite {
		t.Fatalf("menu or current favorite was lost: %+v", second)
	}
	if other.Days[0].Lunch[0].Name != "Bob menu" || other.Days[0].Lunch[0].Favorite {
		t.Fatalf("another user's menu/favorite leaked: %+v", other)
	}
	InvalidateWeekPlan(alice.ID)
	if database.GetUserSetting(alice.ID, "week_plan_cache", "") != "" || database.GetUserSetting(bob.ID, "week_plan_cache", "") == "" {
		t.Fatal("cache invalidation must affect only its owner")
	}
}

func TestWeekPlanCacheReadsLegacyStringAndCurrentArrayFields(t *testing.T) {
	for _, raw := range []string{
		`{"days":[{"date":"2026-09-28","lunch":[{"id":1,"images":"[\"photo.jpg\"]","ingredients":"[{\"name\":\"米饭\"}]","seasonings":"[]","steps":"[]","tags":"[]"}]}]}`,
		`{"days":[{"date":"2026-09-28","lunch":[{"id":1,"images":["photo.jpg"],"ingredients":[{"name":"米饭"}],"seasonings":[],"steps":[],"tags":[],"video_meta":null}]}]}`,
	} {
		plan, err := decodeCachedWeekPlan(raw)
		if err != nil {
			t.Fatal(err)
		}
		dish := plan.Days[0].Lunch[0]
		if dish.Images != `["photo.jpg"]` || dish.Ingredients != `[{"name":"米饭"}]` || dish.VideoMeta != "" {
			t.Fatalf("incompatible persisted JSON: %+v", dish)
		}
		encoded, err := json.Marshal(plan)
		if err != nil || !json.Valid(encoded) {
			t.Fatalf("cached plan cannot be returned to clients: %v", err)
		}
		if !strings.Contains(string(encoded), `"dinner":[]`) {
			t.Fatal("empty meals must remain arrays for existing clients")
		}
	}
}

func TestCompactShoppingAmounts(t *testing.T) {
	tests := []struct {
		name    string
		amounts []string
		want    string
	}{
		{name: "same unit sums", amounts: []string{"1个", "2个"}, want: "3个"},
		{name: "different units stay separate", amounts: []string{"1个", "4颗"}, want: "1个+4颗"},
		{name: "different unit order is stable", amounts: []string{"4颗", "1个"}, want: "1个+4颗"},
		{name: "text amounts dedupe", amounts: []string{"适量", "适量", "适量"}, want: "适量"},
		{name: "mixed text and numeric", amounts: []string{"适量", "1块", "适量"}, want: "适量+1块"},
		{name: "existing plus values compact", amounts: []string{"3个+4个"}, want: "7个"},
		{name: "no unit numeric sums", amounts: []string{"1", "2"}, want: "3"},
		{name: "decimal values sum", amounts: []string{"0.5斤", "1斤"}, want: "1.5斤"},
		{name: "empty defaults to suitable amount", amounts: []string{"", "  "}, want: "适量"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := compactShoppingAmounts(tt.amounts); got != tt.want {
				t.Fatalf("compactShoppingAmounts(%v) = %q, want %q", tt.amounts, got, tt.want)
			}
		})
	}
}

func TestSortShoppingItems(t *testing.T) {
	items := []ShoppingItem{
		{Name: "tomato", Amount: "2个"},
		{Name: "egg", Amount: "3个", Checked: true},
		{Name: "egg", Amount: "2个"},
		{Name: "apple", Amount: "1个", InStock: true},
	}

	sortShoppingItems(items)

	got := []string{
		items[0].Name + ":" + items[0].Amount,
		items[1].Name + ":" + items[1].Amount,
		items[2].Name + ":" + items[2].Amount,
		items[3].Name + ":" + items[3].Amount,
	}
	want := []string{"egg:2个", "tomato:2个", "egg:3个", "apple:1个"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sorted[%d] = %q, want %q; full order: %v", i, got[i], want[i], got)
		}
	}
}

func TestShoppingItemPriority(t *testing.T) {
	tests := []struct {
		item ShoppingItem
		want int
	}{
		{item: ShoppingItem{Name: "盐"}, want: 0},
		{item: ShoppingItem{Name: "盐", Checked: true}, want: 1},
		{item: ShoppingItem{Name: "盐", InStock: true}, want: 2},
		{item: ShoppingItem{Name: "盐", Checked: true, InStock: true}, want: 2},
	}

	for _, tt := range tests {
		if got := shoppingItemPriority(tt.item); got != tt.want {
			t.Fatalf("shoppingItemPriority(%+v) = %d, want %d", tt.item, got, tt.want)
		}
	}
}

func TestBuildShoppingCategoryExact(t *testing.T) {
	terms := map[string][]string{
		"蔬菜": {"番茄", "西兰花"},
		"肉类": {"鸡蛋"},
		"无效": {"不会加入"},
	}

	got := buildShoppingCategoryExact(terms)
	if got["番茄"] != "蔬菜" {
		t.Fatalf("番茄 category = %q, want 蔬菜", got["番茄"])
	}
	if got["鸡蛋"] != "肉类" {
		t.Fatalf("鸡蛋 category = %q, want 肉类", got["鸡蛋"])
	}
	if _, exists := got["不会加入"]; exists {
		t.Fatalf("invalid category term should be ignored")
	}
}

func TestClassifyShoppingItem(t *testing.T) {
	overrides := map[string]string{"紫菜": "其他"}
	tests := []struct {
		name string
		want string
	}{
		{name: "番茄", want: "蔬菜"},
		{name: "西红柿", want: "蔬菜"},
		{name: "西兰花", want: "蔬菜"},
		{name: "豆腐", want: "蔬菜"},
		{name: "青椒", want: "蔬菜"},
		{name: "小米辣", want: "蔬菜"},
		{name: "金针菇", want: "蔬菜"},
		{name: "土豆", want: "蔬菜"},
		{name: "玉米", want: "蔬菜"},
		{name: "猪肉末", want: "肉类"},
		{name: "鸡蛋", want: "肉类"},
		{name: "鸡翅", want: "肉类"},
		{name: "牛腩", want: "肉类"},
		{name: "虾仁", want: "肉类"},
		{name: "鱿鱼", want: "肉类"},
		{name: "花椒粉", want: "配料"},
		{name: "豆瓣酱", want: "配料"},
		{name: "辣椒粉", want: "配料"},
		{name: "生抽", want: "配料"},
		{name: "老抽", want: "配料"},
		{name: "料酒", want: "配料"},
		{name: "鸡精", want: "配料"},
		{name: "玉米淀粉", want: "配料"},
		{name: "番茄酱", want: "配料"},
		{name: "蛋黄酱", want: "配料"},
		{name: "面粉", want: "其他"},
		{name: "糯米粉", want: "其他"},
		{name: "粉丝", want: "其他"},
		{name: "鸡蛋面", want: "其他"},
		{name: "可乐", want: "其他"},
		{name: "紫菜", want: "其他"},
		{name: "黄瓜段", want: "蔬菜"},
		{name: "五花肉片", want: "肉类"},
		{name: "黑胡椒碎", want: "配料"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyShoppingItem(tt.name, overrides); got != tt.want {
				t.Fatalf("classifyShoppingItem(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}
