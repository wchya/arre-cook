package services

import (
	"errors"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"sort"
	"strings"
	"time"
)

var ErrFoodEntryNotFound = errors.New("饮食记录不存在")

var allowedFoodGroups = map[string]bool{
	"vegetable": true, "fruit": true, "protein": true, "whole_grain": true, "dairy": true,
}

type FoodJournalInput struct {
	MealDate   string   `json:"meal_date"`
	MealType   string   `json:"meal_type"`
	DishName   string   `json:"dish_name"`
	Cuisine    string   `json:"cuisine"`
	FoodGroups []string `json:"food_groups"`
	Notes      string   `json:"notes"`
}

func CreateFoodJournalEntry(uid uint, in FoodJournalInput) (*models.FoodJournalEntry, error) {
	date, err := normalizeMealDate(in.MealDate)
	if err != nil {
		return nil, err
	}
	parsed, _ := time.Parse("2006-01-02", date)
	today, _ := time.Parse("2006-01-02", Today())
	if parsed.Before(today.AddDate(0, 0, -365)) || parsed.After(today.AddDate(0, 0, 1)) {
		return nil, errors.New("只能记录最近一年的饮食")
	}
	if in.MealType != "breakfast" && in.MealType != "lunch" && in.MealType != "dinner" && in.MealType != "snack" {
		return nil, errors.New("请选择早餐、午餐、晚餐或加餐")
	}
	name := strings.TrimSpace(in.DishName)
	if name == "" || len([]rune(name)) > 100 {
		return nil, errors.New("食物名称请填写 1-100 个字")
	}
	if len([]rune(in.Cuisine)) > 40 || len([]rune(in.Notes)) > 500 {
		return nil, errors.New("菜系或备注过长")
	}
	groups := make([]string, 0, len(in.FoodGroups))
	seen := map[string]bool{}
	for _, group := range in.FoodGroups {
		if !allowedFoodGroups[group] {
			return nil, errors.New("食物类别无效")
		}
		if !seen[group] {
			groups = append(groups, group)
			seen[group] = true
		}
	}
	entry := &models.FoodJournalEntry{
		UserID: uid, MealDate: date, MealType: in.MealType, DishName: name,
		Cuisine: strings.TrimSpace(in.Cuisine), FoodGroups: groups, Notes: strings.TrimSpace(in.Notes),
	}
	if err := database.DB.Create(entry).Error; err != nil {
		return nil, err
	}
	return entry, nil
}

func ListFoodJournal(uid uint, from, to string) ([]models.FoodJournalEntry, error) {
	query := database.DB.Scopes(database.OwnedBy(uid))
	if from != "" {
		if _, err := time.Parse("2006-01-02", from); err != nil {
			return nil, ErrInvalidDate
		}
		query = query.Where("meal_date >= ?", from)
	}
	if to != "" {
		if _, err := time.Parse("2006-01-02", to); err != nil {
			return nil, ErrInvalidDate
		}
		query = query.Where("meal_date <= ?", to)
	}
	items := []models.FoodJournalEntry{}
	err := query.Order("meal_date DESC, created_at DESC").Limit(500).Find(&items).Error
	return items, err
}

func DeleteFoodJournalEntry(uid, id uint) error {
	res := database.DB.Scopes(database.OwnedBy(uid)).Where("id = ?", id).Delete(&models.FoodJournalEntry{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrFoodEntryNotFound
	}
	return nil
}

type CuisineCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type HealthDay struct {
	Date       string   `json:"date"`
	MealCount  int      `json:"meal_count"`
	Cuisines   []string `json:"cuisines"`
	FoodGroups []string `json:"food_groups"`
}

type HealthRecommendation struct {
	Dish   models.Dish `json:"dish"`
	Reason string      `json:"reason"`
}

type HealthReport struct {
	PeriodDays      int                    `json:"period_days"`
	From            string                 `json:"from"`
	To              string                 `json:"to"`
	LoggedDays      int                    `json:"logged_days"`
	MealCount       int                    `json:"meal_count"`
	CuisineCounts   []CuisineCount         `json:"cuisine_counts"`
	FoodGroupDays   map[string]int         `json:"food_group_days"`
	Days            []HealthDay            `json:"days"`
	Insights        []string               `json:"insights"`
	PlanActions     []string               `json:"plan_actions"`
	Recommendations []HealthRecommendation `json:"recommendations"`
}

func BuildHealthReport(uid uint, period int) (*HealthReport, error) {
	if period != 30 {
		period = 7
	}
	now := time.Now()
	from := now.AddDate(0, 0, 1-period).Format("2006-01-02")
	to := now.Format("2006-01-02")
	report := &HealthReport{
		PeriodDays: period, From: from, To: to, CuisineCounts: []CuisineCount{},
		FoodGroupDays: map[string]int{}, Days: []HealthDay{}, Insights: []string{}, PlanActions: []string{},
		Recommendations: []HealthRecommendation{},
	}
	journal, err := ListFoodJournal(uid, from, to)
	if err != nil {
		return nil, err
	}
	var records []models.MealRecord
	if err := database.DB.Scopes(database.OwnedBy(uid)).Where("meal_date >= ? AND meal_date <= ?", from, to).
		Find(&records).Error; err != nil {
		return nil, err
	}
	dishIDs := make([]uint, 0, len(records))
	for _, record := range records {
		dishIDs = append(dishIDs, record.DishID)
	}
	type dishCategory struct {
		ID       uint
		Category string
	}
	var categories []dishCategory
	if len(dishIDs) > 0 {
		if err := database.DB.Unscoped().Model(&models.Dish{}).Select("id, category").Where("id IN ?", dishIDs).Find(&categories).Error; err != nil {
			return nil, err
		}
	}
	categoryByID := map[uint]string{}
	for _, dish := range categories {
		categoryByID[dish.ID] = dish.Category
	}
	type dayData struct {
		count    int
		cuisines map[string]bool
		groups   map[string]bool
	}
	days := map[string]*dayData{}
	getDay := func(date string) *dayData {
		if days[date] == nil {
			days[date] = &dayData{cuisines: map[string]bool{}, groups: map[string]bool{}}
		}
		return days[date]
	}
	cuisineCounts := map[string]int{}
	addCuisine := func(day *dayData, raw string) {
		name := strings.TrimSpace(raw)
		if name != "" {
			day.cuisines[name] = true
			cuisineCounts[name]++
		}
	}
	for _, entry := range journal {
		day := getDay(entry.MealDate)
		day.count++
		addCuisine(day, entry.Cuisine)
		for _, group := range entry.FoodGroups {
			day.groups[group] = true
		}
	}
	for _, record := range records {
		day := getDay(record.MealDate)
		day.count++
		addCuisine(day, categoryByID[record.DishID])
	}
	for i := period - 1; i >= 0; i-- {
		date := now.AddDate(0, 0, -i).Format("2006-01-02")
		view := HealthDay{Date: date, Cuisines: []string{}, FoodGroups: []string{}}
		if day := days[date]; day != nil {
			view.MealCount = day.count
			report.MealCount += day.count
			report.LoggedDays++
			for cuisine := range day.cuisines {
				view.Cuisines = append(view.Cuisines, cuisine)
			}
			for group := range day.groups {
				view.FoodGroups = append(view.FoodGroups, group)
				report.FoodGroupDays[group]++
			}
			sort.Strings(view.Cuisines)
			sort.Strings(view.FoodGroups)
		}
		report.Days = append(report.Days, view)
	}
	for name, count := range cuisineCounts {
		report.CuisineCounts = append(report.CuisineCounts, CuisineCount{Name: name, Count: count})
	}
	sort.Slice(report.CuisineCounts, func(i, j int) bool {
		if report.CuisineCounts[i].Count == report.CuisineCounts[j].Count {
			return report.CuisineCounts[i].Name < report.CuisineCounts[j].Name
		}
		return report.CuisineCounts[i].Count > report.CuisineCounts[j].Count
	})
	if report.MealCount == 0 {
		report.Insights = append(report.Insights, "还没有饮食记录。先记录几餐，报告才能反映你的实际饮食。")
		report.PlanActions = append(report.PlanActions, "从今天的一餐开始记录食物和菜系。")
	} else {
		report.Insights = append(report.Insights, "这段时间有记录的日子："+itoa(report.LoggedDays)+" / "+itoa(period)+" 天。")
		if len(report.CuisineCounts) > 0 {
			report.Insights = append(report.Insights, "记录最多的菜系是「"+report.CuisineCounts[0].Name+"」；换一种菜系可以增加菜单变化。")
		}
		if len(journal) > 0 {
			report.Insights = append(report.Insights, "手动记录中标注蔬菜的日子："+itoa(report.FoodGroupDays["vegetable"])+" 天。未标注不代表没有吃。")
		}
		if report.LoggedDays < period/2 {
			report.PlanActions = append(report.PlanActions, "接下来尝试持续记录几天，报告会更能反映你的实际情况。")
		}
		if len(report.CuisineCounts) > 0 {
			report.PlanActions = append(report.PlanActions, "下次挑一道不同菜系的菜，给菜单增加变化。")
		}
		if len(journal) > 0 {
			report.PlanActions = append(report.PlanActions, "记录下一餐时，可以顺手标注食物类别，方便观察搭配是否多样。")
		}
	}
	if len(report.PlanActions) == 0 {
		report.PlanActions = append(report.PlanActions, "继续记录实际吃的食物，按你的偏好调整下一周菜单。")
	}
	recommended, err := RecommendDishes(uid, RecommendRequest{Count: 8, Source: "health_report"})
	if err != nil {
		return nil, err
	}
	mostCommon := ""
	if len(report.CuisineCounts) > 0 {
		mostCommon = report.CuisineCounts[0].Name
	}
	ordered := make([]int, 0, len(recommended.Items))
	for i, item := range recommended.Items {
		if item.Dish.Category != mostCommon {
			ordered = append(ordered, i)
		}
	}
	for i, item := range recommended.Items {
		if item.Dish.Category == mostCommon {
			ordered = append(ordered, i)
		}
	}
	for _, index := range ordered {
		item := recommended.Items[index]
		reason := "结合你的偏好和近期记录，换一道菜试试"
		if item.Dish.Category != "" && item.Dish.Category != mostCommon {
			reason = "最近较少记录「" + item.Dish.Category + "」，可以换换口味"
		}
		report.Recommendations = append(report.Recommendations, HealthRecommendation{Dish: item.Dish, Reason: reason})
		if len(report.Recommendations) == 3 {
			break
		}
	}
	return report, nil
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
