package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
)

var healthLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

func healthNow() time.Time { return time.Now().In(healthLocation) }
func healthToday() string  { return healthNow().Format("2006-01-02") }

type HealthEvidence struct {
	Nutrition            *models.NutritionSnapshot `json:"nutrition"`
	Source               string                    `json:"source"`
	ID                   uint                      `json:"id"`
	MealType             string                    `json:"meal_type"`
	DishName             string                    `json:"dish_name"`
	Portion              string                    `json:"portion"`
	FoodGroups           []string                  `json:"food_groups"`
	LinkedRecordID       *uint                     `json:"linked_record_id"`
	PossibleDuplicateIDs []uint                    `json:"possible_duplicate_ids"`
}

func healthFingerprint(journal []models.FoodJournalEntry, records []models.MealRecord, omissions ...models.HealthMealOmission) string {
	if journal == nil {
		journal = []models.FoodJournalEntry{}
	}
	if records == nil {
		records = []models.MealRecord{}
	}
	// Deterministic order is essential for confirmations made through separate requests.
	sort.Slice(journal, func(i, j int) bool { return journal[i].ID < journal[j].ID })
	sort.Slice(records, func(i, j int) bool { return records[i].ID < records[j].ID })
	raw, _ := json.Marshal(struct {
		Journal []models.FoodJournalEntry
		Records []models.MealRecord
	}{journal, records})
	if len(omissions) > 0 {
		sort.Slice(omissions, func(i, j int) bool { return omissions[i].MealType < omissions[j].MealType })
		extra, _ := json.Marshal(omissions)
		raw = append(raw, extra...)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// Mutually exclusive counts across de-duplicated food items. Written portions
// are useful evidence but never promoted to measured nutrition inputs.
type HealthPortionCoverage struct {
	MeasuredItems  int `json:"measured_items"`
	StandardItems  int `json:"standard_items"`
	EstimatedItems int `json:"estimated_items"`
	TextOnlyItems  int `json:"text_only_items"`
	UnknownItems   int `json:"unknown_items"`
}

func (c *HealthPortionCoverage) add(j models.FoodJournalEntry) {
	if j.Nutrition != nil {
		switch {
		case j.Nutrition.StandardPortion != nil:
			c.StandardItems++
		case j.Nutrition.PortionSource == "estimated" || j.Nutrition.Recipe != nil:
			c.EstimatedItems++
		case j.Nutrition.PortionSource == "measured":
			c.MeasuredItems++
		default:
			c.UnknownItems++
		}
	} else if strings.TrimSpace(j.Portion) != "" {
		c.TextOnlyItems++
	} else {
		c.UnknownItems++
	}
}

func enrichHealthReport(uid uint, report *HealthReport, journal []models.FoodJournalEntry, records []models.MealRecord, db *gorm.DB) error {
	report.RuleVersion, report.Timezone, report.NutritionStatus = "habits-v1", "Asia/Shanghai", "insufficient_data"
	report.MethodNotes = []string{
		"餐次按日期和主餐归组；有单独标识的加餐分别计算。原始条目保留，确认关联后食物项不重复计数。",
		"完整记录由你确认；补记、编辑或删除后需重新确认。未记录不代表没有吃，未标注不代表没有摄入。",
		"营养根据本人录入的标签或已选标准食物与同单位可食部分食用量计算，混合配方按成品重量比例估算，标签未经平台核验。未知项不计为零，不从菜系推测油盐。食物类别展示出现天数，不是摄入占比。",
		"份量依据按去重食物项统计：称量/量取、标准份量估算、其他估算、仅文字份量和份量未知；称量不表示营养来源已核验。",
		"统计采用北京时间自然日；未来菜单不计实际饮食。推荐依据口味偏好，不表示能补足营养。",
		"明确未吃与漏记分开保存；未吃不计作一餐、食物项或零营养摄入，补记同一餐后自动清除标记。",
	}
	jByDay := map[string][]models.FoodJournalEntry{}
	rByDay := map[string][]models.MealRecord{}
	for _, j := range journal {
		jByDay[j.MealDate] = append(jByDay[j.MealDate], j)
	}
	for _, r := range records {
		rByDay[r.MealDate] = append(rByDay[r.MealDate], r)
	}
	var confirmations []models.HealthDayConfirmation
	if err := db.Scopes(database.OwnedBy(uid)).Where("meal_date >= ? AND meal_date <= ?", report.From, report.To).Find(&confirmations).Error; err != nil {
		return err
	}
	omissions, err := loadMealOmissions(uid, report.From, report.To, db)
	if err != nil {
		return err
	}
	omittedByDay := map[string][]models.HealthMealOmission{}
	for _, row := range omissions {
		omittedByDay[row.MealDate] = append(omittedByDay[row.MealDate], row)
	}
	confirmed := map[string]string{}
	for _, c := range confirmations {
		confirmed[c.MealDate] = c.Fingerprint
	}
	for i := range report.Days {
		day := &report.Days[i]
		js, rs := jByDay[day.Date], rByDay[day.Date]
		day.Fingerprint = healthFingerprint(js, rs, omittedByDay[day.Date]...)
		day.Meals = []HealthMealState{}
		for _, meal := range []string{"breakfast", "lunch", "dinner"} {
			state := "unknown"
			for _, row := range omittedByDay[day.Date] {
				if row.MealType == meal {
					state = "not_eaten"
				}
			}
			for _, j := range js {
				if j.MealType == meal {
					state = "recorded"
				}
			}
			for _, r := range rs {
				if r.MealType == meal {
					state = "recorded"
				}
			}
			if state == "not_eaten" {
				report.NotEatenMeals++
			}
			day.Meals = append(day.Meals, HealthMealState{MealType: meal, Status: state})
		}
		day.Evidence = []HealthEvidence{}
		day.Status = "unknown"
		if len(js)+len(rs)+len(omittedByDay[day.Date]) > 0 {
			day.Status = "partial"
		}
		if confirmed[day.Date] == day.Fingerprint && day.Status != "unknown" {
			day.Status = "complete"
			report.CompleteDays++
		}
		events, linked := map[string]bool{}, map[uint]bool{}
		recordIDs := map[uint]bool{}
		for _, r := range rs {
			recordIDs[r.ID] = true
		}
		for _, j := range js {
			key := j.MealType
			if j.EventKey != "" {
				key += ":" + j.EventKey
			} else if j.MealType == "snack" {
				key += fmt.Sprintf(":journal-%d", j.ID)
			}
			if j.LinkedRecordID != nil && recordIDs[*j.LinkedRecordID] {
				linked[*j.LinkedRecordID] = true
				key = j.MealType
			}
			events[key] = true
			day.ItemCount++
			report.PortionCoverage.add(j)
			if j.Portion != "" {
				report.PortionKnownItems++
			}
			e := HealthEvidence{Source: "journal", ID: j.ID, Nutrition: j.Nutrition, MealType: j.MealType, DishName: j.DishName, Portion: j.Portion, FoodGroups: j.FoodGroups, LinkedRecordID: j.LinkedRecordID, PossibleDuplicateIDs: []uint{}}
			if j.LinkedRecordID != nil && !recordIDs[*j.LinkedRecordID] {
				e.LinkedRecordID = nil
			}
			if j.LinkedRecordID == nil {
				for _, r := range rs {
					if r.MealType == j.MealType && strings.EqualFold(strings.TrimSpace(r.DishName), strings.TrimSpace(j.DishName)) {
						e.PossibleDuplicateIDs = append(e.PossibleDuplicateIDs, r.ID)
					}
				}
			}
			day.Evidence = append(day.Evidence, e)
		}
		for _, r := range rs {
			events[r.MealType] = true
			if !linked[r.ID] {
				report.PortionCoverage.UnknownItems++
				day.ItemCount++
			}
			day.Evidence = append(day.Evidence, HealthEvidence{Source: "record", ID: r.ID, MealType: r.MealType, DishName: r.DishName, FoodGroups: []string{}, PossibleDuplicateIDs: []uint{}})
		}
		day.MealEventCount = len(events)
		report.MealEventCount += day.MealEventCount
		report.ItemCount += day.ItemCount
	}
	enrichNutritionMetrics(report)
	report.Insights = []string{}
	if report.ItemCount == 0 {
		report.Insights = append(report.Insights, "还没有实际饮食记录。从今天吃过的一餐开始，报告会随记录更新。")
	} else {
		report.Insights = append(report.Insights, fmt.Sprintf("本期有 %d 天记录，其中 %d 天已确认完整。", report.LoggedDays, report.CompleteDays))
		if n := report.FoodGroupDays["vegetable"]; n > 0 {
			report.Insights = append(report.Insights, fmt.Sprintf("已记录餐食中，有 %d 天标注了蔬菜。可以点开当天记录查看搭配。", n))
		}
		report.Insights = append(report.Insights, "营养数值只覆盖有标签和食用量的记录；完整日均值按每项营养素分别检查数据覆盖，不评价摄入是否达标。")
	}
	report.PlanActions = []string{"补齐漏记的食物、饮料和加餐，再确认当天记录完整。", "挑一道符合自己忌口的菜，安排到下一餐，并按实际食用情况记录。"}
	return nil
}

func ConfirmHealthDay(uid uint, date, fingerprint string, complete bool, dbs ...*gorm.DB) error {
	db := database.Handle(dbs...)
	parsed, err := time.Parse("2006-01-02", date)
	if err != nil || date > healthToday() || parsed.Before(healthNow().AddDate(0, 0, -366)) {
		return ErrInvalidDate
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := lockHealthProfileUser(uid, tx); err != nil {
			return err
		}
		if !complete {
			return tx.Where("user_id = ? AND meal_date = ?", uid, date).Delete(&models.HealthDayConfirmation{}).Error
		}
		js := []models.FoodJournalEntry{}
		rs := []models.MealRecord{}
		if err := tx.Where("user_id = ? AND meal_date = ?", uid, date).Find(&js).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ? AND meal_date = ?", uid, date).Find(&rs).Error; err != nil {
			return err
		}
		if len(js)+len(rs) == 0 {
			return errors.New("先记录当天吃过的食物，再确认完整")
		}
		omissions, err := loadMealOmissions(uid, date, date, tx)
		if err != nil {
			return err
		}
		if fingerprint != healthFingerprint(js, rs, omissions...) {
			return errors.New("当天记录已变化，请刷新后确认")
		}
		c := models.HealthDayConfirmation{UserID: uid, MealDate: date, Fingerprint: fingerprint, ConfirmedAt: time.Now()}
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "user_id"}, {Name: "meal_date"}}, DoUpdates: clause.AssignmentColumns([]string{"fingerprint", "confirmed_at"})}).Create(&c).Error
	})
}

type HealthPlanView struct {
	models.HealthPlanItem
	Status    string `json:"status"`
	Available bool   `json:"available"`
}

func ListHealthPlans(uid uint, from, to string, db *gorm.DB) ([]HealthPlanView, error) {
	plans := []models.HealthPlanItem{}
	if err := db.Scopes(database.OwnedBy(uid)).Where("meal_date >= ? AND meal_date <= ?", from, to).Order("meal_date, meal_type, id").Find(&plans).Error; err != nil {
		return nil, err
	}
	out := []HealthPlanView{}
	blocked, err := loadDietaryExclusions(uid, db)
	if err != nil {
		return nil, err
	}
	for _, p := range plans {
		var n int64
		if err := db.Model(&models.MealRecord{}).Where("user_id = ? AND meal_date = ? AND meal_type = ? AND dish_id = ?", uid, p.MealDate, p.MealType, p.DishID).Count(&n).Error; err != nil {
			return nil, err
		}
		status := "planned"
		if n > 0 {
			status = "recorded"
		} else if p.MealDate < healthToday() {
			status = "unconfirmed"
		}
		var dish models.Dish
		err := db.Scopes(database.VisibleDishes(uid)).First(&dish, p.DishID).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		out = append(out, HealthPlanView{HealthPlanItem: p, Status: status, Available: err == nil && dish.Enabled && !containsAny(dishSearchText(dish), blocked)})
	}
	return out, nil
}

type HealthPlanInput struct {
	DishID     uint   `json:"dish_id"`
	MealDate   string `json:"meal_date"`
	MealType   string `json:"meal_type"`
	PeriodDays int    `json:"period_days"`
}

func AcceptHealthPlan(uid uint, in HealthPlanInput, dbs ...*gorm.DB) (*models.HealthPlanItem, error) {
	db := database.Handle(dbs...)
	if _, err := time.Parse("2006-01-02", in.MealDate); err != nil || in.MealDate < healthToday() || in.MealDate > healthNow().AddDate(0, 0, 6).Format("2006-01-02") {
		return nil, errors.New("请选择今天起七天内的日期")
	}
	if !isMainMeal(in.MealType) {
		return nil, ErrInvalidMealType
	}
	if in.PeriodDays != 7 && in.PeriodDays != 30 {
		return nil, errors.New("报告周期无效")
	}
	var result models.HealthPlanItem
	err := db.Transaction(func(tx *gorm.DB) error {
		// Serialize explicit plan changes per account, including retries/cancellation.
		if err := tx.Model(&models.User{}).Where("id = ?", uid).UpdateColumn("id", gorm.Expr("id")).Error; err != nil {
			return err
		}
		var dish models.Dish
		if err := tx.Scopes(database.VisibleDishes(uid)).Where("enabled = ?", true).First(&dish, in.DishID).Error; err != nil {
			return ErrDishNotFound
		}
		blocked, err := loadDietaryExclusions(uid, tx)
		if err != nil {
			return err
		}
		if containsAny(dishSearchText(dish), blocked) {
			return errors.New("这道菜与当前忌口冲突，请换一道")
		}
		q := tx.Where("user_id = ? AND meal_date = ? AND meal_type = ? AND dish_id = ?", uid, in.MealDate, in.MealType, in.DishID)
		if err := tx.Where("user_id = ? AND meal_date = ? AND meal_type = ? AND dish_id = ?", uid, in.MealDate, in.MealType, in.DishID).First(&result).Error; err == nil {
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var count int64
		if err := tx.Model(&models.HealthPlanItem{}).Where("user_id = ? AND meal_date = ? AND meal_type = ?", uid, in.MealDate, in.MealType).Count(&count).Error; err != nil {
			return err
		}
		if count >= int64(MaxDishesPerMeal) {
			return errors.New("这餐已安排足够菜品，请先撤销再添加")
		}
		result = models.HealthPlanItem{UserID: uid, MealDate: in.MealDate, MealType: in.MealType, DishID: dish.ID, DishName: dish.Name, ReportFrom: healthNow().AddDate(0, 0, 1-in.PeriodDays).Format("2006-01-02"), ReportTo: healthToday()}
		if err := tx.Create(&result).Error; err != nil {
			return err
		}
		var existing int64
		if err := q.Model(&models.ShoppingCheck{}).Count(&existing).Error; err != nil {
			return err
		}
		if existing == 0 {
			_, err := addShoppingItems(tx, uid, dish, in.MealType, in.MealDate)
			return err
		}
		return nil
	})
	return &result, err
}

func CancelHealthPlan(uid, id uint, dbs ...*gorm.DB) error {
	return database.Handle(dbs...).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.User{}).Where("id = ?", uid).UpdateColumn("id", gorm.Expr("id")).Error; err != nil {
			return err
		}
		var p models.HealthPlanItem
		if err := tx.Scopes(database.OwnedBy(uid)).First(&p, id).Error; err != nil {
			return ErrRecordNotFound
		}
		if err := tx.Delete(&p).Error; err != nil {
			return err
		}
		var records int64
		q := tx.Where("user_id = ? AND meal_date = ? AND meal_type = ? AND dish_id = ?", uid, p.MealDate, p.MealType, p.DishID)
		if err := q.Model(&models.MealRecord{}).Count(&records).Error; err != nil {
			return err
		}
		if records == 0 {
			return tx.Where("user_id = ? AND meal_date = ? AND meal_type = ? AND dish_id = ?", uid, p.MealDate, p.MealType, p.DishID).Delete(&models.ShoppingCheck{}).Error
		}
		return nil
	})
}

// Explicit choices override the corresponding random slot; cache remains untouched.
func applyHealthPlans(uid uint, plan *WeekPlan, db *gorm.DB) {
	if plan == nil || len(plan.Days) == 0 {
		return
	}
	var items []models.HealthPlanItem
	if db.Where("user_id = ? AND meal_date >= ? AND meal_date <= ?", uid, plan.Days[0].Date, plan.Days[len(plan.Days)-1].Date).Order("id").Find(&items).Error != nil {
		return
	}
	blocked, err := loadDietaryExclusions(uid, db)
	if err != nil {
		return
	}
	slots := map[string][]models.Dish{}
	for _, p := range items {
		var d models.Dish
		if db.Scopes(database.VisibleDishes(uid)).Where("enabled = ?", true).First(&d, p.DishID).Error == nil && !containsAny(dishSearchText(d), blocked) {
			slots[p.MealDate+":"+p.MealType] = append(slots[p.MealDate+":"+p.MealType], d)
		}
	}
	for i := range plan.Days {
		day := &plan.Days[i]
		day.Breakfast = slots[day.Date+":breakfast"]
		if ds := slots[day.Date+":lunch"]; len(ds) > 0 {
			day.Lunch = ds
		}
		if ds := slots[day.Date+":dinner"]; len(ds) > 0 {
			day.Dinner = ds
		}
	}
}
