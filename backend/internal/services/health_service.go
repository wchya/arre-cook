package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"regexp"
	"sort"
	"strings"
	"time"
)

var ErrFoodEntryNotFound = errors.New("饮食记录不存在")

var allowedFoodGroups = map[string]bool{
	"vegetable": true, "fruit": true, "protein": true, "whole_grain": true, "dairy": true,
}

type FoodJournalInput struct {
	NutritionPortionKey   string   `json:"nutrition_portion_key,omitempty"`
	NutritionPortionCount *float64 `json:"nutrition_portion_count,omitempty"`
	NutritionMode         string   `json:"nutrition_mode"`
	NutritionFoodID       uint     `json:"nutrition_food_id"`
	NutritionAmount       *float64 `json:"nutrition_amount"`
	NutritionUnit         string   `json:"nutrition_unit"`
	FoodState             string   `json:"food_state"`
	PortionSource         string   `json:"portion_source"`
	MealDate              string   `json:"meal_date"`
	MealType              string   `json:"meal_type"`
	DishName              string   `json:"dish_name"`
	Cuisine               string   `json:"cuisine"`
	FoodGroups            []string `json:"food_groups"`
	Notes                 string   `json:"notes"`
	Portion               string   `json:"portion"`
	EventKey              string   `json:"event_key"`
	LinkedRecordID        *uint    `json:"linked_record_id"`
	RequestKey            string   `json:"request_key"`
}

func prepareFoodJournal(uid uint, in FoodJournalInput, requestDB *gorm.DB) (*models.FoodJournalEntry, error) {

	if in.MealDate == "" {
		in.MealDate = healthToday()
	}
	date, err := normalizeMealDate(in.MealDate)
	if err != nil {
		return nil, err
	}
	parsed, _ := time.Parse("2006-01-02", date)
	today, _ := time.Parse("2006-01-02", healthToday())
	if parsed.Before(today.AddDate(0, 0, -365)) || parsed.After(today) {
		return nil, errors.New("只能记录今天及最近一年的实际饮食")
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
	if len([]rune(in.Portion)) > 100 || !validHealthKey(in.EventKey) || !validHealthKey(in.RequestKey) {
		return nil, errors.New("份量或记录标识无效")
	}
	if in.LinkedRecordID != nil {
		var linked models.MealRecord
		if *in.LinkedRecordID == 0 || requestDB.Scopes(database.OwnedBy(uid)).First(&linked, *in.LinkedRecordID).Error != nil || linked.MealDate != date || linked.MealType != in.MealType {
			return nil, errors.New("只能关联本人同日同餐次的菜谱用餐记录")
		}
	}
	snapshot, err := buildNutritionSnapshot(uid, in, requestDB)
	if err != nil {
		return nil, err
	}
	entry := &models.FoodJournalEntry{
		UserID: uid, MealDate: date, MealType: in.MealType, DishName: name, Nutrition: snapshot,
		Cuisine: strings.TrimSpace(in.Cuisine), FoodGroups: groups, Notes: strings.TrimSpace(in.Notes),
		Portion: strings.TrimSpace(in.Portion), EventKey: in.EventKey, LinkedRecordID: in.LinkedRecordID,
	}
	if in.RequestKey != "" {
		entry.RequestKey = &in.RequestKey
		entry.RequestHash = foodJournalRequestHash(in)
	}
	return entry, nil
}

func foodJournalRequestHash(in FoodJournalInput) string {
	if in.MealDate == "" {
		in.MealDate = healthToday()
	}
	raw, _ := json.Marshal(in)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func CreateFoodJournalEntry(uid uint, in FoodJournalInput, dbs ...*gorm.DB) (*models.FoodJournalEntry, error) {
	var out *models.FoodJournalEntry
	err := database.Handle(dbs...).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.User{}).Where("id = ?", uid).UpdateColumn("id", gorm.Expr("id")).Error; err != nil {
			return err
		}
		// Replay a committed request before resolving labels/recipes, which may have been deleted.
		if in.RequestKey != "" && validHealthKey(in.RequestKey) {
			var existing models.FoodJournalEntry
			err := tx.Where("user_id = ? AND request_key = ?", uid, in.RequestKey).First(&existing).Error
			if err == nil {
				if existing.RequestHash != foodJournalRequestHash(in) {
					return errors.New("重试标识已用于另一条记录，请重新打开记餐表单")
				}
				out = &existing
				return nil
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		entry, err := prepareFoodJournal(uid, in, tx)
		if err != nil {
			return err
		}
		if err := validateJournalLinkAvailable(uid, 0, entry.LinkedRecordID, tx); err != nil {
			return err
		}
		if err := clearMealOmission(tx, uid, entry.MealDate, entry.MealType); err != nil {
			return err
		}
		if err := tx.Create(entry).Error; err != nil {
			return err
		}
		out = entry
		return nil
	})
	return out, err
}

func validateJournalLinkAvailable(uid, id uint, linked *uint, db *gorm.DB) error {
	if linked == nil {
		return nil
	}
	var count int64
	if err := db.Model(&models.FoodJournalEntry{}).Where("user_id = ? AND linked_record_id = ? AND id <> ?", uid, *linked, id).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return errors.New("这条菜谱记录已被另一条日记关联")
	}
	return nil
}

func ListFoodJournal(uid uint, from, to string, dbs ...*gorm.DB) ([]models.FoodJournalEntry, error) {
	requestDB := database.Handle(dbs...)

	query := requestDB.Scopes(database.OwnedBy(uid))
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

func DeleteFoodJournalEntry(uid, id uint, dbs ...*gorm.DB) error {
	return database.Handle(dbs...).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.User{}).Where("id = ?", uid).UpdateColumn("id", gorm.Expr("id")).Error; err != nil {
			return err
		}
		res := tx.Scopes(database.OwnedBy(uid)).Where("id = ?", id).Delete(&models.FoodJournalEntry{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrFoodEntryNotFound
		}
		return nil
	})
}

type CuisineCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type HealthDay struct {
	Meals          []HealthMealState `json:"meals"`
	MealEventCount int               `json:"meal_event_count"`
	ItemCount      int               `json:"item_count"`
	Status         string            `json:"status"`
	Fingerprint    string            `json:"fingerprint"`
	Evidence       []HealthEvidence  `json:"evidence"`
	Date           string            `json:"date"`
	MealCount      int               `json:"meal_count"`
	Cuisines       []string          `json:"cuisines"`
	FoodGroups     []string          `json:"food_groups"`
}

type HealthRecommendation struct {
	Dish   models.Dish `json:"dish"`
	Reason string      `json:"reason"`
}

type HealthReport struct {
	NotEatenMeals     int                     `json:"not_eaten_meals"`
	PortionCoverage   HealthPortionCoverage   `json:"portion_coverage"`
	Comparison        *HealthPeriodComparison `json:"comparison,omitempty"`
	Nutrients         []HealthNutrientMetric  `json:"nutrients"`
	RuleVersion       string                  `json:"rule_version"`
	Timezone          string                  `json:"timezone"`
	MealEventCount    int                     `json:"meal_event_count"`
	ItemCount         int                     `json:"item_count"`
	CompleteDays      int                     `json:"complete_days"`
	PortionKnownItems int                     `json:"portion_known_items"`
	NutritionStatus   string                  `json:"nutrition_status"`
	MethodNotes       []string                `json:"method_notes"`
	Plans             []HealthPlanView        `json:"plans"`
	PeriodDays        int                     `json:"period_days"`
	From              string                  `json:"from"`
	To                string                  `json:"to"`
	LoggedDays        int                     `json:"logged_days"`
	MealCount         int                     `json:"meal_count"`
	CuisineCounts     []CuisineCount          `json:"cuisine_counts"`
	FoodGroupDays     map[string]int          `json:"food_group_days"`
	Days              []HealthDay             `json:"days"`
	Insights          []string                `json:"insights"`
	PlanActions       []string                `json:"plan_actions"`
	Recommendations   []HealthRecommendation  `json:"recommendations"`
}

func BuildHealthReport(uid uint, period int, dbs ...*gorm.DB) (*HealthReport, error) {
	var report *HealthReport
	now := healthNow()
	if period != 30 {
		period = 7
	}
	err := database.Handle(dbs...).Transaction(func(tx *gorm.DB) error {
		var err error
		report, err = buildHealthReportWindow(uid, period, now, true, tx)
		if err != nil {
			return err
		}
		previous, err := buildHealthReportWindow(uid, period, now.AddDate(0, 0, -period), false, tx)
		if err != nil {
			return err
		}
		report.Comparison = compareHealthPeriods(report, previous)
		report.MethodNotes = append(report.MethodNotes, report.Comparison.Method)
		return nil
	})
	return report, err
}

func buildHealthReportWindow(uid uint, period int, now time.Time, includeActions bool, requestDB *gorm.DB) (*HealthReport, error) {

	if period != 30 {
		period = 7
	}
	from := now.AddDate(0, 0, 1-period).Format("2006-01-02")
	to := now.Format("2006-01-02")
	report := &HealthReport{
		PeriodDays: period, From: from, To: to, CuisineCounts: []CuisineCount{},
		FoodGroupDays: map[string]int{}, Days: []HealthDay{}, Insights: []string{}, PlanActions: []string{},
		Recommendations: []HealthRecommendation{},
	}
	journal := []models.FoodJournalEntry{}
	err := requestDB.Scopes(database.OwnedBy(uid)).Where("meal_date >= ? AND meal_date <= ?", from, to).Order("id ASC").Find(&journal).Error
	if err != nil {
		return nil, err
	}
	var records []models.MealRecord
	if err := requestDB.Scopes(database.OwnedBy(uid)).Where("meal_date >= ? AND meal_date <= ?", from, to).
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
		if err := requestDB.Unscoped().Model(&models.Dish{}).Select("id, category").Where("id IN ?", dishIDs).Find(&categories).Error; err != nil {
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
	if err := enrichHealthReport(uid, report, journal, records, requestDB); err != nil {
		return nil, err
	}
	if !includeActions {
		return report, nil
	}
	report.Plans, err = ListHealthPlans(uid, report.From, now.AddDate(0, 0, 6).Format("2006-01-02"), requestDB)
	if err != nil {
		return nil, err
	}
	recommended, err := RecommendDishes(uid, RecommendRequest{Count: 8, Source: "health_report"}, requestDB)
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
	blocked, err := loadDietaryExclusions(uid, requestDB)
	if err != nil {
		return nil, err
	}
	for _, index := range ordered {
		item := recommended.Items[index]
		if containsAny(dishSearchText(item.Dish), blocked) {
			continue
		}
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

var healthKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func validHealthKey(key string) bool { return key == "" || healthKeyPattern.MatchString(key) }

func UpdateFoodJournalEntry(uid, id uint, in FoodJournalInput, dbs ...*gorm.DB) (*models.FoodJournalEntry, error) {
	var out *models.FoodJournalEntry
	err := database.Handle(dbs...).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.User{}).Where("id = ?", uid).UpdateColumn("id", gorm.Expr("id")).Error; err != nil {
			return err
		}
		var old models.FoodJournalEntry
		if err := tx.Scopes(database.OwnedBy(uid)).First(&old, id).Error; err != nil {
			return ErrFoodEntryNotFound
		}
		in.RequestKey = ""
		next, err := prepareFoodJournal(uid, in, tx)
		if err != nil {
			return err
		}
		if err := validateJournalLinkAvailable(uid, id, next.LinkedRecordID, tx); err != nil {
			return err
		}
		if in.NutritionMode == "" || in.NutritionMode == "keep" {
			if next.DishName == old.DishName && next.Portion == old.Portion {
				next.Nutrition = old.Nutrition
			}
		}
		next.ID, next.CreatedAt, next.RequestKey, next.RequestHash = old.ID, old.CreatedAt, old.RequestKey, old.RequestHash
		if err := clearMealOmission(tx, uid, next.MealDate, next.MealType); err != nil {
			return err
		}
		if err := tx.Save(next).Error; err != nil {
			return err
		}
		out = next
		return nil
	})
	return out, err
}
