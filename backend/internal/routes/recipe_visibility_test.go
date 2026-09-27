package routes_test

import (
	"fmt"
	"net/http"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"ninimenu/internal/services"
	"ninimenu/internal/testutil"
	"testing"
)

func TestPersonalRecipeCountMatchesCollectionAfterFamilySharing(t *testing.T) {
	user, token, err := testutil.NewUser("recipe-visibility@example.invalid")
	if err != nil {
		t.Fatal(err)
	}
	family := models.Family{Name: "菜谱范围回归", OwnerID: user.ID}
	if err := database.DB.Create(&family).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.DB.Create(&models.FamilyMember{FamilyID: family.ID, UserID: user.ID}).Error; err != nil {
		t.Fatal(err)
	}
	for _, dish := range []models.Dish{
		{Name: "本人个人菜谱", OwnerID: user.ID, Enabled: true},
		{Name: "本人停用菜谱", OwnerID: user.ID, Enabled: false},
		{Name: "本人家庭菜谱", OwnerID: user.ID, FamilyID: family.ID, Enabled: true},
	} {
		enabled := dish.Enabled
		if err := database.DB.Create(&dish).Error; err != nil {
			t.Fatal(err)
		}
		// GORM applies the true default during Create; explicitly persist false.
		if !enabled {
			if err := database.DB.Model(&dish).Update("enabled", false).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	st, response := call(t, "GET", "/api/dishes?scope=mine", token, nil)
	must(t, st, response, http.StatusOK)
	personal := decode[page](t, response.Data)
	stats := services.BuildUserStats(user.ID)
	if personal.Total != 3 || stats.PrivateDishes != int64(personal.Total) {
		t.Fatalf("personal count %d disagrees with collection %d", stats.PrivateDishes, personal.Total)
	}
	st, response = call(t, "GET", "/api/dishes?scope=family", token, nil)
	must(t, st, response, http.StatusOK)
	shared := decode[page](t, response.Data)
	if shared.Total != 1 || shared.Items[0].Name != "本人家庭菜谱" {
		t.Fatal("family recipe is missing from the shared collection")
	}
	st, response = call(t, "GET", "/api/dishes?scope=private", token, nil)
	must(t, st, response, http.StatusOK)
	private := decode[page](t, response.Data)
	if private.Total != 2 {
		t.Fatal("family sharing candidates must contain only personal originals")
	}
}

func TestMineEntryIncludesOwnedFamilyRecipesWithoutExposingOtherRecipes(t *testing.T) {
	user, token, err := testutil.NewUser("family-only-recipes@example.invalid")
	if err != nil {
		t.Fatal(err)
	}
	other, otherToken, err := testutil.NewUser("family-only-other@example.invalid")
	if err != nil {
		t.Fatal(err)
	}
	family := models.Family{Name: "只有家庭菜谱的账号", OwnerID: user.ID}
	former := models.Family{Name: "已退出的家庭", OwnerID: other.ID}
	for _, row := range []*models.Family{&family, &former} {
		if err := database.DB.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := database.DB.Create(&models.FamilyMember{FamilyID: family.ID, UserID: user.ID}).Error; err != nil {
		t.Fatal(err)
	}
	ownIDs := map[uint]bool{}
	for i, row := range []models.Dish{
		{Name: "本人家庭川菜", OwnerID: user.ID, FamilyID: family.ID, Category: "川菜", Steps: `["翻炒至熟"]`},
		{Name: "本人家庭粤菜", OwnerID: user.ID, FamilyID: family.ID, Category: "粤菜", Steps: `["蒸至熟透"]`},
		{Name: "其他成员的家庭菜谱", OwnerID: other.ID, FamilyID: family.ID, Category: "其他成员"},
		{Name: "他人的私房菜", OwnerID: other.ID, Category: "他人私房"},
		{Name: "已无权访问的家庭菜谱", OwnerID: user.ID, FamilyID: former.ID, Category: "旧家庭"},
	} {
		if err := database.DB.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
		if i < 2 {
			ownIDs[row.ID] = true
		}
	}
	st, response := call(t, "GET", "/api/dishes?scope=mine", token, nil)
	must(t, st, response, http.StatusOK)
	mine := decode[page](t, response.Data)
	if mine.Total != 2 || len(mine.Items) != 2 {
		t.Fatalf("account owns two visible family recipes, mine returned %d", mine.Total)
	}
	for _, row := range mine.Items {
		if !ownIDs[row.ID] {
			t.Fatal("mine contains an unrelated or inaccessible recipe")
		}
		st, response = call(t, "GET", fmt.Sprintf("/api/dishes/%d", row.ID), otherToken, nil)
		must(t, st, response, http.StatusNotFound)
	}
	st, response = call(t, "GET", "/api/dishes/category-counts?scope=mine", token, nil)
	must(t, st, response, http.StatusOK)
	counts := decode[struct {
		Total      int `json:"total"`
		Mine       int `json:"mine"`
		Categories []struct {
			Category string `json:"category"`
			Count    int    `json:"count"`
		} `json:"categories"`
	}](t, response.Data)
	if counts.Total != 2 || counts.Mine != 2 || len(counts.Categories) != 2 {
		t.Fatalf("recipe categories do not match the owned family recipes: %+v", counts)
	}
	for _, category := range counts.Categories {
		if category.Count != 1 || (category.Category != "川菜" && category.Category != "粤菜") {
			t.Fatalf("unrelated category exposed: %+v", category)
		}
	}
	st, response = call(t, "GET", "/api/stats", token, nil)
	must(t, st, response, http.StatusOK)
	if stats := decode[services.UserStats](t, response.Data); stats.PrivateDishes != 2 {
		t.Fatalf("entry count disagrees with the authored list: %d", stats.PrivateDishes)
	}
}
