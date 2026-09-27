package handlers

import (
	"ninimenu/internal/models"
	"testing"
)

func TestRecipeSaveNormalizesVideoShareTextWithoutChangingOtherRecipes(t *testing.T) {
	req := CreateDishRequest{Name: "黄焖辣子鸡", VideoURL: "【【饭店味！！黄焖辣子鸡保姆级教程】-哔哩哔哩】 https://b23.tv/lnFjJz8"}
	dish := models.Dish{OwnerID: 42, FamilyID: 8}
	req.apply(&dish)
	if dish.VideoURL != "https://b23.tv/lnFjJz8" || dish.OwnerID != 42 || dish.FamilyID != 8 {
		t.Fatalf("share text was not normalized safely: %#v", dish)
	}
}
