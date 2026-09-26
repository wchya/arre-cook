package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

// 买菜清单分个人与家庭两份：个人清单来自本人记下 / 确认的餐（ShoppingCheck）；家庭清单来自家庭菜单
// 自动展开的条目（FamilyShoppingCheck）加上成员手动添加的条目（FamilyShoppingItem）。两份分开展示、分开勾选。

const (
	shoppingReminderKey   = "shopping_reminder"    // 用户设置：买菜提醒开关，"0" 关闭
	shoppingRemindedKey   = "shopping_reminded_on" // 用户设置：最近一次每晚提醒的日期
	shoppingPushCountKey  = "shopping_push_count"  // 用户设置：当天智能体推送次数，形如 2026-09-26:3
	shoppingPushDailyMax  = 10
	shoppingCoalesceTTL   = 2 * time.Hour
	shoppingPreviewNames  = 6
	personalShoppingLink  = "/plan?tab=shopping"
	familyShoppingLink    = "/plan?tab=family-shopping"
	defaultReminderHour   = 20
)

type ShoppingMeal struct {
	MealDate    string `json:"meal_date"`
	MealType    string `json:"meal_type"`
	DishID      uint   `json:"dish_id"`
	DishName    string `json:"dish_name"`
	AddedByName string `json:"added_by_name,omitempty"`
}

type PersonalShoppingView struct {
	Meals      []ShoppingMeal     `json:"meals"`
	Categories []ShoppingCategory `json:"categories"`
}

type FamilyShoppingView struct {
	FamilyID   uint                        `json:"family_id"`
	FamilyName string                      `json:"family_name"`
	Meals      []ShoppingMeal              `json:"meals"`
	Categories []ShoppingCategory          `json:"categories"`
	Manual     []models.FamilyShoppingItem `json:"manual"`
}

type ShoppingOverviewView struct {
	Dates           []string             `json:"dates"`
	Personal        PersonalShoppingView `json:"personal"`
	Family          *FamilyShoppingView  `json:"family"`
	ReminderEnabled bool                 `json:"reminder_enabled"`
}

// ShoppingDates 买菜清单覆盖今明两天。
func ShoppingDates() []string {
	now := time.Now()
	return []string{now.Format("2006-01-02"), now.AddDate(0, 0, 1).Format("2006-01-02")}
}

func ShoppingReminderEnabled(uid uint) bool {
	return database.GetUserSetting(uid, shoppingReminderKey, "1") != "0"
}

// ShoppingReminderHour 每晚提醒的整点（服务器本地时区），站点设置 shopping_reminder_hour 可改，默认 20 点。
func ShoppingReminderHour() int {
	if h, err := strconv.Atoi(database.GetSetting("shopping_reminder_hour", "")); err == nil && h >= 0 && h <= 23 {
		return h
	}
	return defaultReminderHour
}

// ShoppingOverview 个人与家庭两份买菜清单（今明两天），并附各自来自哪几餐。
func ShoppingOverview(uid uint) ShoppingOverviewView {
	dates := ShoppingDates()
	view := ShoppingOverviewView{
		Dates:           dates,
		Personal:        PersonalShoppingView{Meals: personalShoppingMeals(uid, dates), Categories: BuildShoppingList(uid, dates)},
		ReminderEnabled: ShoppingReminderEnabled(uid),
	}
	if family, err := FamilyForUser(uid); err == nil && family != nil {
		manual := []models.FamilyShoppingItem{}
		database.DB.Where("family_id = ?", family.ID).Order("checked ASC, created_at DESC").Find(&manual)
		view.Family = &FamilyShoppingView{
			FamilyID:   family.ID,
			FamilyName: family.Name,
			Meals:      familyShoppingMeals(family.ID, dates),
			Categories: BuildFamilyShoppingList(family.ID, dates),
			Manual:     manual,
		}
	}
	return view
}

func personalShoppingMeals(uid uint, dates []string) []ShoppingMeal {
	meals := []ShoppingMeal{}
	database.DB.Model(&models.ShoppingCheck{}).Scopes(database.OwnedBy(uid)).
		Select("meal_date, meal_type, dish_id, dish_name").
		Where("meal_date IN ?", dates).
		Group("meal_date, meal_type, dish_id, dish_name").
		Scan(&meals)
	sortShoppingMeals(meals)
	return meals
}

func familyShoppingMeals(familyID uint, dates []string) []ShoppingMeal {
	meals := []ShoppingMeal{}
	var items []models.FamilyPlanItem
	database.DB.Where("family_id = ? AND meal_date IN ?", familyID, dates).Find(&items)
	if len(items) == 0 {
		return meals
	}
	dishIDs := make([]uint, 0, len(items))
	userIDs := make([]uint, 0, len(items))
	for _, it := range items {
		dishIDs = append(dishIDs, it.DishID)
		userIDs = append(userIDs, it.AddedBy)
	}
	var dishes []models.Dish
	database.DB.Select("id", "name").Where("id IN ?", dishIDs).Find(&dishes)
	dishNames := make(map[uint]string, len(dishes))
	for _, d := range dishes {
		dishNames[d.ID] = d.Name
	}
	users := userDisplayNames(userIDs)
	for _, it := range items {
		if name, ok := dishNames[it.DishID]; ok {
			meals = append(meals, ShoppingMeal{MealDate: it.MealDate, MealType: it.MealType, DishID: it.DishID, DishName: name, AddedByName: users[it.AddedBy]})
		}
	}
	sortShoppingMeals(meals)
	return meals
}

// sortShoppingMeals 按日期、先午餐后晚餐、菜名排序。
func sortShoppingMeals(meals []ShoppingMeal) {
	order := func(t string) int {
		if t == "lunch" {
			return 0
		}
		return 1
	}
	sort.SliceStable(meals, func(i, j int) bool {
		if meals[i].MealDate != meals[j].MealDate {
			return meals[i].MealDate < meals[j].MealDate
		}
		if order(meals[i].MealType) != order(meals[j].MealType) {
			return order(meals[i].MealType) < order(meals[j].MealType)
		}
		return meals[i].DishName < meals[j].DishName
	})
}

// BuildFamilyShoppingList 家庭菜单自动生成的清单（按分类合并同名食材）；家庭清单不区分家中库存。
func BuildFamilyShoppingList(familyID uint, dates []string) []ShoppingCategory {
	if len(dates) == 0 {
		return []ShoppingCategory{}
	}
	var checks []models.FamilyShoppingCheck
	database.DB.Where("family_id = ? AND meal_date IN ?", familyID, dates).Find(&checks)
	rows := make([]shoppingRow, 0, len(checks))
	for _, ch := range checks {
		rows = append(rows, shoppingRow{name: ch.ItemName, amount: ch.ItemAmount, checked: ch.Checked})
	}
	return groupShoppingRows(rows, nil)
}

// ToggleFamilyShoppingCheck 勾选 / 取消家庭自动清单里的某样食材（今明两天内同名条目一起变）。
func ToggleFamilyShoppingCheck(uid uint, itemName string, checked bool) error {
	family, err := RequireFamily(uid)
	if err != nil {
		return err
	}
	itemName = strings.TrimSpace(itemName)
	if itemName == "" {
		return errors.New("请指定食材")
	}
	return database.DB.Model(&models.FamilyShoppingCheck{}).
		Where("family_id = ? AND item_name = ? AND meal_date IN ?", family.ID, itemName, ShoppingDates()).
		Update("checked", checked).Error
}

// dishShoppingEntries 菜品的食材 + 调料（兼容 [{name,amount}] 与字符串数组两种存法）。
func dishShoppingEntries(dish models.Dish) []nameAmount {
	var out []nameAmount
	for _, raw := range []string{dish.Ingredients, dish.Seasonings} {
		var list []nameAmount
		if err := json.Unmarshal([]byte(raw), &list); err != nil {
			for _, name := range ingredientNames(raw) {
				list = append(list, nameAmount{Name: name})
			}
		}
		for _, it := range list {
			if name := strings.TrimSpace(it.Name); name != "" {
				out = append(out, nameAmount{Name: truncateRunes(name, 80), Amount: strings.TrimSpace(it.Amount)})
			}
		}
	}
	return out
}

// syncFamilyPlanShopping 家庭菜单格变化时整格替换自动买菜条目；dish 为 nil 表示清空该格。
// 同一道菜重复设置时保留原条目（和勾选状态）。返回该格当前的条目数。
func syncFamilyPlanShopping(familyID uint, date, mealType string, dish *models.Dish) int {
	slot := func() *gorm.DB {
		return database.DB.Where("family_id = ? AND meal_date = ? AND meal_type = ?", familyID, date, mealType)
	}
	if dish != nil {
		var existing []models.FamilyShoppingCheck
		slot().Find(&existing)
		if len(existing) > 0 && existing[0].DishID == dish.ID {
			return len(existing)
		}
	}
	slot().Delete(&models.FamilyShoppingCheck{})
	if dish == nil {
		return 0
	}
	entries := dishShoppingEntries(*dish)
	rows := make([]models.FamilyShoppingCheck, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, models.FamilyShoppingCheck{
			FamilyID: familyID, MealDate: date, MealType: mealType,
			DishID: dish.ID, DishName: dish.Name, ItemName: e.Name, ItemAmount: e.Amount,
		})
	}
	if len(rows) > 0 {
		database.DB.Create(&rows)
	}
	return len(rows)
}

// shoppingWorthReminding 只为还来得及买菜的餐发提醒：明天及以后，或今天下午 4 点前定下的晚餐。
func shoppingWorthReminding(date, mealType string, now time.Time) bool {
	today := now.Format("2006-01-02")
	if date > today {
		return true
	}
	return date == today && mealType == "dinner" && now.Hour() < 16
}

// notifyMealShopping 个人确认一餐后，把新增的食材合并进一条“买菜清单已更新”站内信。
func notifyMealShopping(uid uint, dish models.Dish, mealType, date string, entries []nameAmount) {
	if len(entries) == 0 || !ShoppingReminderEnabled(uid) || !shoppingWorthReminding(date, mealType, time.Now()) {
		return
	}
	line := fmt.Sprintf("%s%s「%s」要买：%s", mealDateLabel(date), mealTypeLabel(mealType), dish.Name, previewEntryNames(entries, shoppingPreviewNames))
	_ = UpsertRecentNotification(uid, "shopping", "买菜清单已更新", line, personalShoppingLink, shoppingCoalesceTTL)
}

// notifyFamilyPlanShopping 家庭菜单定下一餐后通知其他成员，家庭清单已自动同步。
func notifyFamilyPlanShopping(familyID, actor uint, date, mealType string, dish models.Dish, items int) {
	if !shoppingWorthReminding(date, mealType, time.Now()) {
		return
	}
	line := fmt.Sprintf("%s 把%s%s定为「%s」", userDisplayName(actor), mealDateLabel(date), mealTypeLabel(mealType), dish.Name)
	if items > 0 {
		line += fmt.Sprintf("，%d 样食材已加入家庭清单", items)
	}
	for _, uid := range familyMemberUserIDs(familyID) {
		if uid != actor && ShoppingReminderEnabled(uid) {
			_ = UpsertRecentNotification(uid, "shopping", "家庭菜单已更新", line, familyShoppingLink, shoppingCoalesceTTL)
		}
	}
}

// SendShoppingReminders 每晚提醒一次：明天的个人或家庭清单里还有没买的食材，就给相关用户发一条站内信。
// 同一用户同一天只发一次，关闭了买菜提醒的用户跳过。返回发送条数。
func SendShoppingReminders(now time.Time) int {
	today := now.Format("2006-01-02")
	tomorrow := now.AddDate(0, 0, 1).Format("2006-01-02")

	personal := map[uint][]string{}
	var checks []models.ShoppingCheck
	database.DB.Where("meal_date = ? AND checked = ?", tomorrow, false).Find(&checks)
	for _, ch := range checks {
		personal[ch.UserID] = appendUniqueName(personal[ch.UserID], ch.ItemName)
	}
	familyItems := map[uint][]string{}
	var familyChecks []models.FamilyShoppingCheck
	database.DB.Where("meal_date = ? AND checked = ?", tomorrow, false).Find(&familyChecks)
	for _, ch := range familyChecks {
		familyItems[ch.FamilyID] = appendUniqueName(familyItems[ch.FamilyID], ch.ItemName)
	}
	familyOf := map[uint]uint{}
	if len(familyItems) > 0 {
		ids := make([]uint, 0, len(familyItems))
		for id := range familyItems {
			ids = append(ids, id)
		}
		var members []models.FamilyMember
		database.DB.Where("family_id IN ?", ids).Find(&members)
		for _, m := range members {
			familyOf[m.UserID] = m.FamilyID
		}
	}

	recipients := make([]uint, 0, len(personal)+len(familyOf))
	seen := map[uint]bool{}
	for uid := range personal {
		recipients, seen[uid] = append(recipients, uid), true
	}
	for uid := range familyOf {
		if !seen[uid] {
			recipients = append(recipients, uid)
		}
	}
	sort.Slice(recipients, func(i, j int) bool { return recipients[i] < recipients[j] })

	sent := 0
	for _, uid := range recipients {
		if uid == 0 || !ShoppingReminderEnabled(uid) || database.GetUserSetting(uid, shoppingRemindedKey, "") == today {
			continue
		}
		parts := []string{}
		link := personalShoppingLink
		if names := personal[uid]; len(names) > 0 {
			parts = append(parts, fmt.Sprintf("我的清单还差 %d 样：%s", len(names), previewNames(names, shoppingPreviewNames)))
		}
		if names := familyItems[familyOf[uid]]; familyOf[uid] != 0 && len(names) > 0 {
			parts = append(parts, fmt.Sprintf("家庭清单还差 %d 样：%s", len(names), previewNames(names, shoppingPreviewNames)))
			if len(personal[uid]) == 0 {
				link = familyShoppingLink
			}
		}
		if len(parts) == 0 {
			continue
		}
		if _, err := CreateNotification(uid, "shopping", "明天要买的菜", strings.Join(parts, "；")+"。", link); err == nil {
			_ = database.SetUserSetting(uid, shoppingRemindedKey, today)
			sent++
		}
	}
	return sent
}

type ShoppingReminderResult struct {
	OK      bool   `json:"ok"`
	Sent    int    `json:"sent"`
	Items   int    `json:"items"`
	Message string `json:"message"`
}

// PushShoppingReminder 智能体推送买菜提醒：audience=me 发给本人（个人 + 家庭清单），
// audience=family 把家庭清单发给全体家庭成员。只统计未勾选的食材；每位用户每天最多推送 10 次。
func PushShoppingReminder(uid uint, audience, rawDate, note, actor string) (*ShoppingReminderResult, error) {
	if strings.TrimSpace(rawDate) == "" {
		rawDate = "tomorrow"
	}
	date, err := normalizeMealDate(rawDate)
	if err != nil {
		return nil, err
	}
	note = truncateRunes(strings.TrimSpace(note), 100)
	actor = strings.TrimSpace(actor)
	if actor == "" {
		actor = "AI 助手"
	}

	var family *models.Family
	if f, err := FamilyForUser(uid); err == nil {
		family = f
	}
	var personalNames, familyNames []string
	if audience != "family" {
		var checks []models.ShoppingCheck
		database.DB.Scopes(database.OwnedBy(uid)).Where("meal_date = ? AND checked = ?", date, false).Find(&checks)
		for _, ch := range checks {
			personalNames = appendUniqueName(personalNames, ch.ItemName)
		}
	} else if family == nil {
		return nil, ErrFamilyRequired
	}
	if family != nil {
		var checks []models.FamilyShoppingCheck
		database.DB.Where("family_id = ? AND meal_date = ? AND checked = ?", family.ID, date, false).Find(&checks)
		for _, ch := range checks {
			familyNames = appendUniqueName(familyNames, ch.ItemName)
		}
		var manual []models.FamilyShoppingItem
		database.DB.Where("family_id = ? AND checked = ?", family.ID, false).Find(&manual)
		for _, it := range manual {
			familyNames = appendUniqueName(familyNames, it.Name)
		}
	}

	total := len(personalNames) + len(familyNames)
	if total == 0 {
		return &ShoppingReminderResult{OK: true, Message: mealDateLabel(date) + "的清单里没有待买的食材，没有发送提醒"}, nil
	}
	if !allowShoppingPush(uid) {
		return nil, errors.New("今天的买菜提醒次数已达上限")
	}

	suffix := ""
	if note != "" {
		suffix = "（" + note + "）"
	}
	result := &ShoppingReminderResult{OK: true, Items: total}
	if audience == "family" {
		content := fmt.Sprintf("%s 提醒大家：%s家庭清单还差 %d 样：%s。%s", actor, mealDateLabel(date), len(familyNames), previewNames(familyNames, 10), suffix)
		for _, member := range familyMemberUserIDs(family.ID) {
			if _, err := CreateNotification(member, "shopping", "买菜提醒", content, familyShoppingLink); err == nil {
				result.Sent++
			}
		}
	} else {
		parts := []string{}
		link := personalShoppingLink
		if len(personalNames) > 0 {
			parts = append(parts, fmt.Sprintf("我的清单还差 %d 样：%s", len(personalNames), previewNames(personalNames, 10)))
		}
		if len(familyNames) > 0 {
			parts = append(parts, fmt.Sprintf("家庭清单还差 %d 样：%s", len(familyNames), previewNames(familyNames, 10)))
			if len(personalNames) == 0 {
				link = familyShoppingLink
			}
		}
		content := fmt.Sprintf("%s 提醒你%s要买菜：%s。%s", actor, mealDateLabel(date), strings.Join(parts, "；"), suffix)
		if _, err := CreateNotification(uid, "shopping", "买菜提醒", content, link); err == nil {
			result.Sent++
		}
	}
	result.Message = fmt.Sprintf("已推送 %d 条提醒", result.Sent)
	return result, nil
}

func allowShoppingPush(uid uint) bool {
	today := Today()
	count := 0
	if raw := database.GetUserSetting(uid, shoppingPushCountKey, ""); strings.HasPrefix(raw, today+":") {
		count, _ = strconv.Atoi(strings.TrimPrefix(raw, today+":"))
	}
	if count >= shoppingPushDailyMax {
		return false
	}
	_ = database.SetUserSetting(uid, shoppingPushCountKey, fmt.Sprintf("%s:%d", today, count+1))
	return true
}

func mealDateLabel(date string) string {
	now := time.Now()
	switch date {
	case now.Format("2006-01-02"):
		return "今天"
	case now.AddDate(0, 0, 1).Format("2006-01-02"):
		return "明天"
	case now.AddDate(0, 0, 2).Format("2006-01-02"):
		return "后天"
	}
	if t, err := time.Parse("2006-01-02", date); err == nil {
		return fmt.Sprintf("%d月%d日", t.Month(), t.Day())
	}
	return date
}

func mealTypeLabel(mealType string) string {
	if mealType == "lunch" {
		return "午餐"
	}
	return "晚餐"
}

func appendUniqueName(list []string, name string) []string {
	name = strings.TrimSpace(name)
	if name == "" {
		return list
	}
	for _, v := range list {
		if v == name {
			return list
		}
	}
	return append(list, name)
}

func previewNames(names []string, max int) string {
	if len(names) <= max {
		return strings.Join(names, "、")
	}
	return strings.Join(names[:max], "、") + fmt.Sprintf(" 等 %d 样", len(names))
}

func previewEntryNames(entries []nameAmount, max int) string {
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = appendUniqueName(names, e.Name)
	}
	return previewNames(names, max)
}
