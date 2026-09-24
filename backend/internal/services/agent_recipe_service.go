package services

import (
	"encoding/json"
	"errors"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"strings"
)

type PrivateRecipePatch struct {
	Name        *string   `json:"name"`
	Category    *string   `json:"category"`
	MealType    *string   `json:"meal_type"`
	Taste       *string   `json:"taste"`
	Ingredients *[]string `json:"ingredients"`
	Seasonings  *[]string `json:"seasonings"`
	Steps       *[]string `json:"steps"`
	CookTime    *int      `json:"cook_time"`
	Difficulty  *string   `json:"difficulty"`
	Remark      *string   `json:"remark"`
}

func applyPrivateRecipePatch(dish *models.Dish, patch PrivateRecipePatch) error {
	if patch.Name != nil {
		name := strings.TrimSpace(*patch.Name)
		if name == "" || len([]rune(name)) > 100 {
			return errors.New("菜名请填写 1-100 个字")
		}
		dish.Name = name
	}
	for _, field := range []struct {
		value  *string
		target *string
		limit  int
	}{{patch.Category, &dish.Category, 40}, {patch.Taste, &dish.Taste, 80}, {patch.Remark, &dish.Remark, 500}} {
		if field.value != nil {
			value := strings.TrimSpace(*field.value)
			if len([]rune(value)) > field.limit {
				return errors.New("菜谱文字过长")
			}
			*field.target = value
		}
	}
	if patch.MealType != nil {
		if *patch.MealType != "all" && *patch.MealType != "lunch" && *patch.MealType != "dinner" {
			return ErrInvalidMealType
		}
		dish.MealType = *patch.MealType
	}
	if patch.Difficulty != nil {
		if *patch.Difficulty != "easy" && *patch.Difficulty != "medium" && *patch.Difficulty != "hard" {
			return errors.New("菜谱难度无效")
		}
		dish.Difficulty = *patch.Difficulty
	}
	if patch.CookTime != nil {
		if *patch.CookTime < 0 || *patch.CookTime > 600 {
			return errors.New("烹饪时间应在 0-600 分钟之间")
		}
		dish.CookTime = *patch.CookTime
	}
	for _, field := range []struct {
		value  *[]string
		target *string
		limit  int
	}{{patch.Ingredients, &dish.Ingredients, 80}, {patch.Seasonings, &dish.Seasonings, 80}, {patch.Steps, &dish.Steps, 500}} {
		if field.value == nil {
			continue
		}
		if len(*field.value) > 100 {
			return errors.New("菜谱条目过多")
		}
		clean := make([]string, 0, len(*field.value))
		for _, raw := range *field.value {
			value := strings.TrimSpace(raw)
			if value == "" || len([]rune(value)) > field.limit {
				return errors.New("菜谱条目为空或过长")
			}
			clean = append(clean, value)
		}
		encoded, _ := json.Marshal(clean)
		*field.target = string(encoded)
	}
	return nil
}

func CreatePrivateRecipe(uid uint, patch PrivateRecipePatch) (*models.Dish, error) {
	if patch.Name == nil {
		return nil, errors.New("请填写菜名")
	}
	var count int64
	if err := database.DB.Model(&models.Dish{}).Where("owner_id = ? AND family_id = 0", uid).Count(&count).Error; err != nil {
		return nil, err
	}
	if count >= 500 {
		return nil, errors.New("私房菜已达 500 道上限")
	}
	dish := &models.Dish{OwnerID: uid, Enabled: true, MealType: "all", Difficulty: "easy", Ingredients: "[]", Seasonings: "[]", Steps: "[]", Images: "[]", Tags: "[]"}
	if err := applyPrivateRecipePatch(dish, patch); err != nil {
		return nil, err
	}
	if err := database.DB.Create(dish).Error; err != nil {
		return nil, err
	}
	QueueAutoAchievementSync(uid)
	return dish, nil
}

func UpdatePrivateRecipe(uid, id uint, patch PrivateRecipePatch) (*models.Dish, error) {
	var dish models.Dish
	if err := database.DB.Where("id = ? AND owner_id = ? AND family_id = 0", id, uid).First(&dish).Error; err != nil {
		return nil, ErrDishNotFound
	}
	if err := applyPrivateRecipePatch(&dish, patch); err != nil {
		return nil, err
	}
	if err := database.DB.Save(&dish).Error; err != nil {
		return nil, err
	}
	InvalidateWeekPlan(uid)
	return &dish, nil
}

func DeletePrivateRecipe(uid, id uint) error {
	result := database.DB.Where("id = ? AND owner_id = ? AND family_id = 0", id, uid).Delete(&models.Dish{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrDishNotFound
	}
	InvalidateWeekPlan(uid)
	QueueAutoAchievementSync(uid)
	return nil
}
