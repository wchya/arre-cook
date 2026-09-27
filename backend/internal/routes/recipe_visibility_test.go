package routes_test

import (
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
	if personal.Total != 2 || stats.PrivateDishes != int64(personal.Total) {
		t.Fatalf("personal count %d disagrees with collection %d", stats.PrivateDishes, personal.Total)
	}
	st, response = call(t, "GET", "/api/dishes?scope=family", token, nil)
	must(t, st, response, http.StatusOK)
	shared := decode[page](t, response.Data)
	if shared.Total != 1 || shared.Items[0].Name != "本人家庭菜谱" {
		t.Fatal("family recipe is missing from the shared collection")
	}
}
