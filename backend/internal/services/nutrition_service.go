package services

import (
	"errors"
	"math"
	"strings"

	"gorm.io/gorm"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
)

const NutritionCalculationVersion = "label-per100-v1"

type NutritionFoodInput struct {
	Name            string                `json:"name"`
	BasisUnit       string                `json:"basis_unit"`
	FoodState       string                `json:"food_state"`
	SourceReference string                `json:"source_reference"`
	Nutrients       models.NutrientValues `json:"nutrients"`
	Version         uint                  `json:"version"`
}

func validFoodState(state string) bool {
	return state == "as_sold" || state == "ready_to_eat" || state == "raw" || state == "cooked"
}

func nutrientFields(v models.NutrientValues) []*float64 {
	return []*float64{v.EnergyKcal, v.ProteinG, v.CarbohydrateG, v.FatG, v.FiberG, v.SodiumMg}
}

func validateNutritionFood(in NutritionFoodInput) error {
	if n := len([]rune(strings.TrimSpace(in.Name))); n == 0 || n > 100 {
		return errors.New("食物名称需为 1–100 字")
	}
	if n := len([]rune(strings.TrimSpace(in.SourceReference))); n == 0 || n > 500 {
		return errors.New("请填写品牌、包装和标签来源，最多 500 字")
	}
	if in.BasisUnit != "g" && in.BasisUnit != "ml" {
		return errors.New("请选择每 100 克或每 100 毫升")
	}
	if !validFoodState(in.FoodState) {
		return errors.New("请选择标签对应的食物状态")
	}
	limits := []float64{1000, 100, 100, 100, 100, 100000}
	known := 0
	for i, value := range nutrientFields(in.Nutrients) {
		if value == nil {
			continue
		}
		known++
		if math.IsNaN(*value) || math.IsInf(*value, 0) || *value < 0 || *value > limits[i] {
			return errors.New("营养数值超出范围，请核对单位和每 100 份量")
		}
	}
	if known == 0 {
		return errors.New("至少填写一项包装标签上的营养数值，缺失项留空")
	}
	if in.BasisUnit == "g" {
		total := 0.0
		for _, v := range []*float64{in.Nutrients.ProteinG, in.Nutrients.CarbohydrateG, in.Nutrients.FatG} {
			if v != nil {
				total += *v
			}
		}
		if total > 105 {
			return errors.New("每 100 克的蛋白质、碳水和脂肪合计不应超过 100 克，请核对标签")
		}
	}
	return nil
}

func ListNutritionFoods(uid uint, dbs ...*gorm.DB) ([]models.NutritionFood, error) {
	out := []models.NutritionFood{}
	err := database.Handle(dbs...).Scopes(database.OwnedBy(uid)).Where("enabled = ?", true).Order("name, id").Find(&out).Error
	return out, err
}

func SaveNutritionFood(uid, id uint, in NutritionFoodInput, dbs ...*gorm.DB) (*models.NutritionFood, error) {
	if err := validateNutritionFood(in); err != nil {
		return nil, err
	}
	var out models.NutritionFood
	err := database.Handle(dbs...).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.User{}).Where("id = ?", uid).UpdateColumn("id", gorm.Expr("id")).Error; err != nil {
			return err
		}
		if id == 0 {
			var n int64
			if err := tx.Model(&models.NutritionFood{}).Where("user_id = ?", uid).Count(&n).Error; err != nil {
				return err
			}
			if n >= 300 {
				return errors.New("个人营养标签最多保存 300 项，请编辑已有条目")
			}
			out = models.NutritionFood{UserID: uid, Enabled: true, Version: 1}
		} else {
			if err := tx.Scopes(database.OwnedBy(uid)).First(&out, id).Error; err != nil {
				return ErrFoodEntryNotFound
			}
			if out.Source != "package_label" {
				return errors.New("标准目录食物不能直接改值；配方请通过配方编辑入口修改")
			}
			if out.Version != in.Version {
				return errors.New("标签已被修改，请刷新后重试")
			}
			out.Version++
		}
		out.Name, out.BasisUnit, out.FoodState = strings.TrimSpace(in.Name), in.BasisUnit, in.FoodState
		out.Source, out.SourceReference = "package_label", strings.TrimSpace(in.SourceReference)
		out.Nutrients = in.Nutrients
		if id == 0 {
			return tx.Create(&out).Error
		}
		return tx.Save(&out).Error
	})
	return &out, err
}

func DeleteNutritionFood(uid, id uint, dbs ...*gorm.DB) error {
	return database.Handle(dbs...).Transaction(func(tx *gorm.DB) error {
		// Serialize with label updates and snapshot creation; Save must not resurrect a deleted label.
		if err := tx.Model(&models.User{}).Where("id = ?", uid).UpdateColumn("id", gorm.Expr("id")).Error; err != nil {
			return err
		}
		result := tx.Scopes(database.OwnedBy(uid)).Where("id = ?", id).Delete(&models.NutritionFood{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrFoodEntryNotFound
		}
		return nil
	})
}

func scaleNutrients(v models.NutrientValues, factor float64) models.NutrientValues {
	scale := func(n *float64) *float64 {
		if n == nil {
			return nil
		}
		value := math.Round(*n*factor*100) / 100
		return &value
	}
	return models.NutrientValues{EnergyKcal: scale(v.EnergyKcal), ProteinG: scale(v.ProteinG), CarbohydrateG: scale(v.CarbohydrateG), FatG: scale(v.FatG), FiberG: scale(v.FiberG), SodiumMg: scale(v.SodiumMg)}
}

func buildNutritionSnapshot(uid uint, in FoodJournalInput, db *gorm.DB) (*models.NutritionSnapshot, error) {
	if in.NutritionMode != "" && in.NutritionMode != "keep" && in.NutritionMode != "replace" && in.NutritionMode != "clear" {
		return nil, errors.New("营养记录方式无效")
	}
	if in.NutritionMode != "replace" {
		return nil, nil
	}
	var food models.NutritionFood
	if err := db.Scopes(database.OwnedBy(uid)).Where("enabled = ?", true).First(&food, in.NutritionFoodID).Error; err != nil {
		return nil, errors.New("营养标签不存在，请重新选择")
	}
	var portion *models.PortionSnapshot
	if in.NutritionPortionKey != "" || in.NutritionPortionCount != nil {
		if food.Catalog == nil || food.Recipe != nil || in.NutritionAmount != nil || in.NutritionPortionKey == "" || in.NutritionPortionCount == nil {
			return nil, errors.New("请选择一个标准份量和数量，不要同时提交克重或毫升数")
		}
		count := *in.NutritionPortionCount
		if math.IsNaN(count) || math.IsInf(count, 0) || count <= 0 || count > 100 {
			return nil, errors.New("标准份量数量需大于0且不超过100")
		}
		for _, candidate := range food.Portions {
			if candidate.Key == in.NutritionPortionKey {
				portion = &models.PortionSnapshot{CatalogPortion: candidate, Count: count}
				break
			}
		}
		if portion == nil {
			return nil, errors.New("标准份量不存在，请重新选择")
		}
		amount := portion.Amount * count
		in.NutritionAmount = &amount
		in.PortionSource = "estimated"
	}
	if in.NutritionAmount == nil || math.IsNaN(*in.NutritionAmount) || math.IsInf(*in.NutritionAmount, 0) || *in.NutritionAmount <= 0 || *in.NutritionAmount > 10000 {
		return nil, errors.New("请填写个人实际食用量，范围为 0–10000（不含零）")
	}
	if in.PortionSource != "measured" && in.PortionSource != "estimated" {
		return nil, errors.New("请选择份量是称量还是估计")
	}
	if in.NutritionFoodID == 0 || food.BasisUnit != in.NutritionUnit || food.FoodState != in.FoodState {
		return nil, errors.New("食用量单位和生熟状态必须与标签一致，不能自动换算")
	}
	if food.Recipe != nil {
		if *in.NutritionAmount > food.Recipe.YieldG {
			return nil, errors.New("食用量不能超过这份配方的成品重量，请按实际批次调整配方")
		}
		return &models.NutritionSnapshot{FoodID: food.ID, FoodVersion: food.Version, FoodName: food.Name, Amount: *in.NutritionAmount, Unit: "g", FoodState: food.FoodState, PortionSource: "estimated", Source: food.Source, SourceReference: food.SourceReference, CalculationVersion: RecipeCalculationVersion, Per100: food.Nutrients, Consumed: scaleNutrients(food.Recipe.Total, *in.NutritionAmount/food.Recipe.YieldG), Recipe: food.Recipe}, nil
	}
	version := NutritionCalculationVersion
	if food.Catalog != nil {
		version = CatalogCalculationVersion
	}
	return &models.NutritionSnapshot{StandardPortion: portion, Catalog: food.Catalog, FoodID: food.ID, FoodVersion: food.Version, FoodName: food.Name, Amount: *in.NutritionAmount, Unit: food.BasisUnit, FoodState: food.FoodState, PortionSource: in.PortionSource, Source: food.Source, SourceReference: food.SourceReference, CalculationVersion: version, Per100: food.Nutrients, Consumed: scaleNutrients(food.Nutrients, *in.NutritionAmount/100)}, nil
}

type HealthNutrientMetric struct {
	Code         string   `json:"code"`
	Label        string   `json:"label"`
	Unit         string   `json:"unit"`
	KnownTotal   *float64 `json:"known_total"`
	DailyAverage *float64 `json:"daily_average"`
	CoveredItems int      `json:"covered_items"`
	TotalItems   int      `json:"total_items"`
	CompleteDays int      `json:"complete_days"`
	RequiredDays int      `json:"required_days"`
	Status       string   `json:"status"`
}

// Each nutrient has its own completeness gate. Missing days/items never become zero.
func enrichNutritionMetrics(report *HealthReport) {
	defs := [][3]string{{"energy_kcal", "能量", "kcal"}, {"protein_g", "蛋白质", "g"}, {"carbohydrate_g", "碳水化合物", "g"}, {"fat_g", "脂肪", "g"}, {"fiber_g", "膳食纤维", "g"}, {"sodium_mg", "钠", "mg"}}
	report.Nutrients = []HealthNutrientMetric{}
	required := int(math.Ceil(float64(report.PeriodDays) * 4 / 7))
	for field, def := range defs {
		metric := HealthNutrientMetric{Code: def[0], Label: def[1], Unit: def[2], TotalItems: report.ItemCount, RequiredDays: required, Status: "insufficient_data"}
		total, completeTotal := 0.0, 0.0
		for _, day := range report.Days {
			known, subtotal := 0, 0.0
			for _, e := range day.Evidence {
				if e.Source != "journal" || e.Nutrition == nil {
					continue
				}
				value := nutrientFields(e.Nutrition.Consumed)[field]
				if value != nil {
					known++
					subtotal += *value
				}
			}
			metric.CoveredItems += known
			total += subtotal
			if day.Status == "complete" && day.ItemCount > 0 && known == day.ItemCount {
				metric.CompleteDays++
				completeTotal += subtotal
			}
		}
		if metric.CoveredItems > 0 {
			v := math.Round(total*100) / 100
			metric.KnownTotal = &v
			metric.Status = "partial"
		}
		if metric.CompleteDays >= required {
			v := math.Round(completeTotal/float64(metric.CompleteDays)*100) / 100
			metric.DailyAverage = &v
			metric.Status = "available"
		}
		report.Nutrients = append(report.Nutrients, metric)
	}
	report.NutritionStatus = "insufficient_data"
	for _, n := range report.Nutrients {
		if n.Status == "partial" {
			report.NutritionStatus = "partial"
		}
	}
	for _, n := range report.Nutrients {
		if n.Status == "available" {
			report.NutritionStatus = "available"
			break
		}
	}
}
