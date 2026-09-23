package dishes

import (
	"encoding/json"
	"fmt"
	"ninimenu/internal/models"
	"strings"
)

type DishPack struct {
	Name     string
	Category string
	Recipes  []RecipeSpec
}

type RecipeSpec struct {
	Name        string
	Image       string
	Category    string
	MealType    string
	Taste       string
	CookTime    int
	Difficulty  string
	Ingredients []item
	Seasonings  []item
	Steps       []stepSpec
	Remark      string
	Tags        []string
}

type item struct {
	Name   string `json:"name"`
	Amount string `json:"amount,omitempty"`
}

type stepSpec struct {
	Text string `json:"text"`
	Time int    `json:"time,omitempty"`
}

// 站内菜谱范围：只保留南方菜系，口味偏辣。
// 按“辣度 / 下饭程度”排序：川菜、湘菜、贵州菜、云南菜、粤菜，共 100 道。
var defaultCategories = []string{
	"川菜",
	"湘菜",
	"贵州菜",
	"云南菜",
	"粤菜",
}

var defaultTastes = []string{
	"酸",
	"甜",
	"辣",
	"鲜",
	"清淡",
	"咸鲜",
	"麻辣",
	"蒜香",
	"葱香",
	"酱香",
	"糖醋",
	"酸辣",
	"香辣",
	"甜辣",
	"豉香",
	"清鲜",
	"咸香",
	"酸甜",
	"鲜辣",
	"浓香",
	"孜然",
}

var defaultPacks = []DishPack{
	sichuanPack,
	hunanPack,
	guizhouPack,
	yunnanPack,
	cantonesePack,
}

// seedImageDirs 是当前菜单种子图片目录；retiredSeedImageDirs 是已下线菜系的图片目录。
// 历史数据库里由旧种子数据写入、如今已从菜单移除的菜品，靠这些前缀识别并清理；
// 用户自己上传的图片落在 /uploads/YYYY/MM/DD/ 下，不会被误伤。
var seedImageDirs = []string{"sichuan", "hunan", "guizhou", "yunnan", "cantonese"}

var retiredSeedImageDirs = []string{"basic_home", "quick_meal", "soup", "staple", "northeast", "xinjiang"}

func DefaultCategories() []string {
	return copyStrings(defaultCategories)
}

func DefaultTastes() []string {
	return copyStrings(defaultTastes)
}

func DefaultPacks() []DishPack {
	packs := make([]DishPack, len(defaultPacks))
	copy(packs, defaultPacks)
	return packs
}

// DefaultDishNameSet 返回当前菜单里全部种子菜名，用于识别已下线的种子菜品。
func DefaultDishNameSet() map[string]bool {
	names := make(map[string]bool, recipeCount(defaultPacks))
	for _, pack := range defaultPacks {
		for _, recipe := range pack.Recipes {
			names[recipe.Name] = true
		}
	}
	return names
}

func SeedImageDirs() []string {
	return copyStrings(seedImageDirs)
}

// IsSeedImage 判断图片路径是否属于当前菜单的种子图片目录。
func IsSeedImage(path string) bool {
	return hasSeedDirPrefix(path, seedImageDirs)
}

// IsRetiredSeedImage 判断图片路径是否属于已下线菜系的种子图片目录。
func IsRetiredSeedImage(path string) bool {
	return hasSeedDirPrefix(path, retiredSeedImageDirs)
}

func hasSeedDirPrefix(path string, dirs []string) bool {
	path = strings.TrimSpace(path)
	for _, dir := range dirs {
		if strings.HasPrefix(path, "/uploads/"+dir+"/") {
			return true
		}
	}
	return false
}

func DefaultDishes() []models.Dish {
	result := make([]models.Dish, 0, recipeCount(defaultPacks))
	sortOrder := 10

	for _, pack := range defaultPacks {
		for _, recipe := range pack.Recipes {
			category := recipe.Category
			if category == "" {
				category = pack.Category
			}
			if category == "" {
				category = defaultCategories[0]
			}

			mealType := recipe.MealType
			if mealType == "" {
				mealType = "all"
			}
			difficulty := recipe.Difficulty
			if difficulty == "" {
				difficulty = "easy"
			}

			tags := uniqueStrings(append([]string{pack.Name, category}, recipe.Tags...))
			result = append(result, models.Dish{
				Name:        recipe.Name,
				Images:      dishImages(recipe.Image),
				Category:    category,
				MealType:    mealType,
				Taste:       recipe.Taste,
				Ingredients: jsonString(recipe.Ingredients),
				Seasonings:  jsonString(recipe.Seasonings),
				Steps:       jsonString(recipe.Steps),
				CookTime:    recipe.CookTime,
				Difficulty:  difficulty,
				Remark:      recipe.Remark,
				Enabled:     true,
				Tags:        jsonString(tags),
				SortOrder:   sortOrder,
			})
			sortOrder += 10
		}
	}

	return result
}

func pack(name, category string, recipes ...RecipeSpec) DishPack {
	return DishPack{Name: name, Category: category, Recipes: recipes}
}

func r(name, image, mealType, difficulty, taste string, cookTime int, ingredients []item, seasonings []item, steps []stepSpec, remark string, tags ...string) RecipeSpec {
	return RecipeSpec{
		Name:        name,
		Image:       image,
		MealType:    mealType,
		Difficulty:  difficulty,
		Taste:       taste,
		CookTime:    cookTime,
		Ingredients: ingredients,
		Seasonings:  seasonings,
		Steps:       steps,
		Remark:      remark,
		Tags:        tags,
	}
}

func items(pairs ...string) []item {
	result := make([]item, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		result = append(result, item{Name: pairs[i], Amount: pairs[i+1]})
	}
	return result
}

func st(text string, minutes int) stepSpec {
	return stepSpec{Text: text, Time: minutes}
}

func jsonString(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func dishImages(image string) string {
	if image == "" {
		return "[]"
	}
	return fmt.Sprintf(`["%s"]`, image)
}

func copyStrings(values []string) []string {
	copied := make([]string, len(values))
	copy(copied, values)
	return copied
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func recipeCount(packs []DishPack) int {
	total := 0
	for _, pack := range packs {
		total += len(pack.Recipes)
	}
	return total
}
