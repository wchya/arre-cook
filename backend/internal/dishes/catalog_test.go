package dishes

import (
	"encoding/json"
	"strings"
	"testing"
)

// 站内菜单约定：只保留南方菜系，共 100 道，以辣味为主。
const (
	expectedDishCount = 100
	minSpicyRatio     = 0.7
)

var southernCategories = map[string]bool{"川菜": true, "湘菜": true, "贵州菜": true, "云南菜": true, "粤菜": true}

func TestDefaultDishesAreComplete(t *testing.T) {
	categorySet := toSet(DefaultCategories())
	tasteSet := toSet(DefaultTastes())
	seenNames := map[string]bool{}

	packs := DefaultPacks()
	if len(packs) != len(DefaultCategories()) {
		t.Fatalf("DefaultPacks() length = %d, want %d", len(packs), len(DefaultCategories()))
	}
	for _, pack := range packs {
		if len(pack.Recipes) < 3 {
			t.Fatalf("%s recipe count = %d, want at least 3", pack.Name, len(pack.Recipes))
		}
		if !categorySet[pack.Category] {
			t.Fatalf("%s category %q is not in defaults", pack.Name, pack.Category)
		}
	}

	dishes := DefaultDishes()
	expectedDishCountFromPacks := recipeCount(packs)
	if len(dishes) != expectedDishCountFromPacks {
		t.Fatalf("DefaultDishes() length = %d, want %d", len(dishes), expectedDishCountFromPacks)
	}

	for _, dish := range dishes {
		if strings.TrimSpace(dish.Name) == "" {
			t.Fatalf("dish has empty name: %+v", dish)
		}
		if seenNames[dish.Name] {
			t.Fatalf("duplicate dish name: %s", dish.Name)
		}
		seenNames[dish.Name] = true

		if !categorySet[dish.Category] {
			t.Fatalf("%s category %q is not in defaults", dish.Name, dish.Category)
		}
		for _, taste := range splitTaste(dish.Taste) {
			if !tasteSet[taste] {
				t.Fatalf("%s taste %q is not in defaults", dish.Name, taste)
			}
		}
		if dish.MealType != "all" && dish.MealType != "lunch" && dish.MealType != "dinner" {
			t.Fatalf("%s meal_type = %q", dish.Name, dish.MealType)
		}
		if dish.Difficulty != "easy" && dish.Difficulty != "medium" && dish.Difficulty != "hard" {
			t.Fatalf("%s difficulty = %q", dish.Name, dish.Difficulty)
		}
		if dish.CookTime <= 0 {
			t.Fatalf("%s cook_time should be positive", dish.Name)
		}
		if jsonArrayLen(dish.Ingredients) == 0 {
			t.Fatalf("%s has no ingredients", dish.Name)
		}
		if jsonArrayLen(dish.Seasonings) == 0 {
			t.Fatalf("%s has no seasonings", dish.Name)
		}
		if jsonArrayLen(dish.Steps) == 0 {
			t.Fatalf("%s has no steps", dish.Name)
		}
		if jsonArrayLen(dish.Tags) == 0 {
			t.Fatalf("%s has no tags", dish.Name)
		}
		if !IsSeedImage(firstImage(dish.Images)) {
			t.Fatalf("%s image %q is not under a seed image dir", dish.Name, dish.Images)
		}
	}
}

func TestMenuIsSouthernAndSpicy(t *testing.T) {
	dishes := DefaultDishes()
	if len(dishes) != expectedDishCount {
		t.Fatalf("menu has %d dishes, want exactly %d", len(dishes), expectedDishCount)
	}

	spicy := 0
	for _, dish := range dishes {
		if !southernCategories[dish.Category] {
			t.Fatalf("%s category %q is not a southern cuisine", dish.Name, dish.Category)
		}
		if strings.Contains(dish.Taste, "辣") || strings.Contains(dish.Name, "辣") || strings.Contains(dish.Name, "椒") {
			spicy++
		}
	}
	ratio := float64(spicy) / float64(len(dishes))
	if ratio < minSpicyRatio {
		t.Fatalf("spicy ratio = %.2f (%d/%d), want at least %.2f", ratio, spicy, len(dishes), minSpicyRatio)
	}

	names := DefaultDishNameSet()
	if len(names) != len(dishes) {
		t.Fatalf("DefaultDishNameSet() size = %d, want %d", len(names), len(dishes))
	}
}

func firstImage(raw string) string {
	var images []string
	if err := json.Unmarshal([]byte(raw), &images); err != nil || len(images) == 0 {
		return ""
	}
	return images[0]
}

func toSet(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}

func splitTaste(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}
	}
	for _, sep := range []string{"，", "、", "/", "|", ";", "；", " "} {
		raw = strings.ReplaceAll(raw, sep, ",")
	}
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func jsonArrayLen(raw string) int {
	var values []any
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return 0
	}
	return len(values)
}
