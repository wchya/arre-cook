package services

import (
	"errors"
	"math"
	"strings"

	"gorm.io/gorm"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
)

const RecipeCalculationVersion = "retained-mixture-v1"

type NutritionRecipeIngredientInput struct {
	FoodID      uint    `json:"food_id"`
	FoodVersion uint    `json:"food_version"`
	Amount      float64 `json:"amount"`
	Unit        string  `json:"unit"`
	FoodState   string  `json:"food_state"`
}

type NutritionRecipeInput struct {
	Name            string                           `json:"name"`
	SourceReference string                           `json:"source_reference"`
	Version         uint                             `json:"version"`
	YieldG          float64                          `json:"yield_g"`
	Method          string                           `json:"method"`
	Ingredients     []NutritionRecipeIngredientInput `json:"ingredients"`
}

func SaveNutritionRecipe(uid, id uint, in NutritionRecipeInput, dbs ...*gorm.DB) (*models.NutritionFood, error) {
	name, reference := strings.TrimSpace(in.Name), strings.TrimSpace(in.SourceReference)
	if len([]rune(name)) < 1 || len([]rune(name)) > 100 || len([]rune(reference)) < 1 || len([]rune(reference)) > 500 {
		return nil, errors.New("请填写配方名称（最多100字）和来源说明（最多500字）")
	}
	if in.Method != "unheated_all_retained" {
		return nil, errors.New("仅支持无需加热、全部原料及液体保留的混合配方，请确认适用条件")
	}
	if math.IsNaN(in.YieldG) || math.IsInf(in.YieldG, 0) || in.YieldG < 0.01 || in.YieldG > 10000 {
		return nil, errors.New("成品重量需为0.01至10000克")
	}
	if len(in.Ingredients) < 2 || len(in.Ingredients) > 20 {
		return nil, errors.New("配方需包含2至20项原料，所有原料和调料都需录入")
	}
	var out models.NutritionFood
	err := database.Handle(dbs...).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.User{}).Where("id = ?", uid).UpdateColumn("id", gorm.Expr("id")).Error; err != nil {
			return err
		}
		if id == 0 {
			var count int64
			if err := tx.Model(&models.NutritionFood{}).Where("user_id = ?", uid).Count(&count).Error; err != nil {
				return err
			}
			if count >= 300 {
				return errors.New("个人标签和配方最多共300项")
			}
			out = models.NutritionFood{UserID: uid, Version: 1, Enabled: true}
		} else {
			if err := tx.Scopes(database.OwnedBy(uid)).First(&out, id).Error; err != nil {
				return ErrFoodEntryNotFound
			}
			if out.Source != "retained_mixture" || out.Version != in.Version {
				return errors.New("配方类型或版本已变化，请刷新后重试")
			}
			out.Version++
		}
		recipe := &models.NutritionRecipe{YieldG: in.YieldG, Method: in.Method, Ingredients: []models.NutritionSnapshot{}}
		totals := [6]float64{}
		known := [6]int{}
		allGrams, grams := true, 0.0
		seen := map[uint]bool{}
		for _, item := range in.Ingredients {
			if item.FoodID == 0 || seen[item.FoodID] {
				return errors.New("请选择原料；同一种标签请合并用量，不要重复添加")
			}
			seen[item.FoodID] = true
			snap, err := buildNutritionSnapshot(uid, FoodJournalInput{NutritionMode: "replace", NutritionFoodID: item.FoodID, NutritionAmount: &item.Amount, NutritionUnit: item.Unit, FoodState: item.FoodState, PortionSource: "estimated"}, tx)
			if err != nil {
				return err
			}
			if (snap.Source != "package_label" && snap.Source != "standard_food") || snap.Recipe != nil {
				return errors.New("原料只能选择本人标签或已添加的标准食物，暂不支持嵌套配方")
			}
			if snap.FoodVersion != item.FoodVersion {
				return errors.New("原料标签已修改，请刷新并重新确认配方")
			}
			recipe.Ingredients = append(recipe.Ingredients, *snap)
			if item.Unit == "g" {
				grams += item.Amount
			} else {
				allGrams = false
			}
			for i, value := range nutrientFields(snap.Per100) {
				if value != nil {
					totals[i] += *value * item.Amount / 100
					known[i]++
				}
			}
		}
		if (allGrams && math.Abs(grams-in.YieldG) > math.Max(1, grams*0.05)) || grams-in.YieldG > math.Max(1, grams*0.05) {
			return errors.New("全部原料保留时，成品重量应接近原料克重合计，请核对用量（允许5%称量误差）")
		}
		values := make([]*float64, 6)
		for i, total := range totals {
			if known[i] == len(in.Ingredients) {
				value := math.Round(total*100) / 100
				values[i] = &value
			}
		}
		recipe.Total = models.NutrientValues{EnergyKcal: values[0], ProteinG: values[1], CarbohydrateG: values[2], FatG: values[3], FiberG: values[4], SodiumMg: values[5]}
		out.Name, out.SourceReference, out.BasisUnit, out.FoodState, out.Source = name, reference, "g", "ready_to_eat", "retained_mixture"
		out.Recipe, out.Nutrients = recipe, scaleNutrients(recipe.Total, 100/in.YieldG)
		if id == 0 {
			return tx.Create(&out).Error
		}
		return tx.Save(&out).Error
	})
	return &out, err
}
