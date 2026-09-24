package services

import (
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"time"
)

type NameCount struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

type TopDishCount struct {
	DishID   uint   `json:"dish_id"`
	DishName string `json:"dish_name"`
	Count    int64  `json:"count"`
}

type DayCount struct {
	Date  string `json:"date"`
	Count int64  `json:"count"`
}

// UserStats 个人统计：用于“我的”页、AI 助手与智能体。
type UserStats struct {
	TotalRecords   int64          `json:"total_records"`
	TotalDishes    int64          `json:"total_dishes"`
	PrivateDishes  int64          `json:"private_dishes"`
	LunchCount     int64          `json:"lunch_count"`
	DinnerCount    int64          `json:"dinner_count"`
	FavoriteCount  int64          `json:"favorite_count"`
	DistinctDishes int64          `json:"distinct_dishes"`
	CookDays       int64          `json:"cook_days"`
	CurrentStreak  int            `json:"current_streak"`
	ThisMonth      int64          `json:"this_month"`
	TopDishes      []TopDishCount `json:"top_dishes"`
	CategoryCounts []NameCount    `json:"category_counts"`
	WeekTrend      []DayCount     `json:"week_trend"`
}

func BuildUserStats(uid uint) UserStats {
	own := database.OwnedBy(uid)
	s := UserStats{TopDishes: []TopDishCount{}, CategoryCounts: []NameCount{}, WeekTrend: []DayCount{}}

	db := database.DB
	db.Model(&models.MealRecord{}).Scopes(own).Count(&s.TotalRecords)
	db.Model(&models.Dish{}).Scopes(database.VisibleDishes(uid)).Where("enabled = ?", true).Count(&s.TotalDishes)
	db.Model(&models.Dish{}).Where("owner_id = ?", uid).Count(&s.PrivateDishes)
	db.Model(&models.MealRecord{}).Scopes(own).Where("meal_type = ?", "lunch").Count(&s.LunchCount)
	db.Model(&models.MealRecord{}).Scopes(own).Where("meal_type = ?", "dinner").Count(&s.DinnerCount)
	db.Model(&models.Favorite{}).Scopes(own).Count(&s.FavoriteCount)
	db.Model(&models.MealRecord{}).Scopes(own).Distinct("dish_id").Count(&s.DistinctDishes)
	db.Model(&models.MealRecord{}).Scopes(own).Distinct("meal_date").Count(&s.CookDays)
	now := time.Now()
	db.Model(&models.MealRecord{}).Scopes(own).Where("meal_date >= ?", now.Format("2006-01")+"-01").Count(&s.ThisMonth)

	db.Model(&models.MealRecord{}).Scopes(own).
		Select("dish_id, dish_name, count(*) as count").
		Group("dish_id, dish_name").Order("count DESC").Limit(8).
		Find(&s.TopDishes)

	db.Table("meal_records AS r").
		Select("d.category AS name, count(*) AS count").
		Joins("JOIN dishes d ON d.id = r.dish_id").
		Where("r.user_id = ?", uid).
		Group("d.category").Order("count DESC").
		Scan(&s.CategoryCounts)

	since := now.AddDate(0, 0, -6).Format("2006-01-02")
	var trend []DayCount
	db.Model(&models.MealRecord{}).Scopes(own).
		Select("meal_date AS date, count(*) AS count").
		Where("meal_date >= ? AND meal_date <= ?", since, now.Format("2006-01-02")).
		Group("meal_date").Scan(&trend)
	byDate := map[string]int64{}
	for _, t := range trend {
		byDate[t.Date] = t.Count
	}
	for i := 6; i >= 0; i-- {
		d := now.AddDate(0, 0, -i).Format("2006-01-02")
		s.WeekTrend = append(s.WeekTrend, DayCount{Date: d, Count: byDate[d]})
	}

	var dates []string
	db.Model(&models.MealRecord{}).Scopes(own).Where("meal_date <= ?", now.Format("2006-01-02")).
		Distinct("meal_date").Order("meal_date DESC").Limit(400).Pluck("meal_date", &dates)
	s.CurrentStreak = currentStreak(dates, now)
	return s
}

// currentStreak 截至今天（或昨天）连续记录的天数。dates 已按倒序。
func currentStreak(dates []string, now time.Time) int {
	if len(dates) == 0 {
		return 0
	}
	expect := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	first, ok := parseMealDate(dates[0])
	if !ok {
		return 0
	}
	first = time.Date(first.Year(), first.Month(), first.Day(), 0, 0, 0, 0, time.Local)
	if expect.Sub(first) > 24*time.Hour {
		return 0
	}
	expect = first
	streak := 0
	for _, raw := range dates {
		d, ok := parseMealDate(raw)
		if !ok {
			continue
		}
		d = time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.Local)
		if !d.Equal(expect) {
			break
		}
		streak++
		expect = expect.AddDate(0, 0, -1)
	}
	return streak
}
