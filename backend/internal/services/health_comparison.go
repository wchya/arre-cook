package services

import (
	"math"
	"sort"
	"time"
)

// Paired weekday samples prevent different weekday/weekend coverage from driving the comparison.
type HealthPeriodComparison struct {
	From        string                     `json:"from"`
	To          string                     `json:"to"`
	RuleVersion string                     `json:"rule_version"`
	Method      string                     `json:"method"`
	Nutrients   []HealthNutrientComparison `json:"nutrients"`
}

type HealthNutrientComparison struct {
	Code            string   `json:"code"`
	Label           string   `json:"label"`
	Unit            string   `json:"unit"`
	Status          string   `json:"status"`
	Reason          string   `json:"reason"`
	RequiredDays    int      `json:"required_days"`
	MatchedDays     int      `json:"matched_days"`
	CurrentDates    []string `json:"current_dates"`
	PreviousDates   []string `json:"previous_dates"`
	CurrentAverage  *float64 `json:"current_average"`
	PreviousAverage *float64 `json:"previous_average"`
	Delta           *float64 `json:"delta"`
}

type nutritionDaySample struct {
	date  string
	value float64
}

func comparableNutritionDays(report *HealthReport, field int) [7][]nutritionDaySample {
	var byWeekday [7][]nutritionDaySample
	for _, day := range report.Days {
		if day.Status != "complete" || day.ItemCount == 0 {
			continue
		}
		date, err := time.Parse("2006-01-02", day.Date)
		if err != nil {
			continue
		}
		known, total := 0, 0.0
		for _, e := range day.Evidence {
			if e.Source != "journal" || e.Nutrition == nil || (e.Nutrition.CalculationVersion != NutritionCalculationVersion && e.Nutrition.CalculationVersion != RecipeCalculationVersion && e.Nutrition.CalculationVersion != CatalogCalculationVersion) {
				continue
			}
			value := nutrientFields(e.Nutrition.Consumed)[field]
			if value != nil {
				known++
				total += *value
			}
		}
		if known == day.ItemCount {
			byWeekday[date.Weekday()] = append(byWeekday[date.Weekday()], nutritionDaySample{day.Date, total})
		}
	}
	for i := range byWeekday {
		sort.Slice(byWeekday[i], func(a, b int) bool { return byWeekday[i][a].date > byWeekday[i][b].date })
	}
	return byWeekday
}

func compareHealthPeriods(current, previous *HealthReport) *HealthPeriodComparison {
	out := &HealthPeriodComparison{From: previous.From, To: previous.To, RuleVersion: "weekday-paired-v1", Method: "比较紧邻且不重叠的两个周期；逐营养素选取确认完整、数据齐全且计算版本一致的日期，按相同星期几配对，每类优先取最近日期。周至少 4 对、30 天至少 18 对才展示配对样本日均与差值；可能包含份量估计。差值不代表健康改善或恶化。", Nutrients: []HealthNutrientComparison{}}
	for field, nutrient := range current.Nutrients {
		item := HealthNutrientComparison{Code: nutrient.Code, Label: nutrient.Label, Unit: nutrient.Unit, RequiredDays: nutrient.RequiredDays, Status: "insufficient_data", Reason: "两期按相同星期几匹配的完整且数据齐全日不足，暂不比较。", CurrentDates: []string{}, PreviousDates: []string{}}
		a, b := comparableNutritionDays(current, field), comparableNutritionDays(previous, field)
		currentTotal, previousTotal := 0.0, 0.0
		for weekday := range a {
			count := min(len(a[weekday]), len(b[weekday]))
			for i := 0; i < count; i++ {
				currentTotal += a[weekday][i].value
				previousTotal += b[weekday][i].value
				item.CurrentDates = append(item.CurrentDates, a[weekday][i].date)
				item.PreviousDates = append(item.PreviousDates, b[weekday][i].date)
				item.MatchedDays++
			}
		}
		if item.MatchedDays >= item.RequiredDays && item.MatchedDays > 0 {
			ca := math.Round(currentTotal/float64(item.MatchedDays)*100) / 100
			pa := math.Round(previousTotal/float64(item.MatchedDays)*100) / 100
			delta := math.Round((ca-pa)*100) / 100
			item.CurrentAverage, item.PreviousAverage, item.Delta = &ca, &pa, &delta
			item.Status, item.Reason = "available", "仅比较配对日期，日均可能与本期所有完整日均值不同。"
		}
		out.Nutrients = append(out.Nutrients, item)
	}
	return out
}
