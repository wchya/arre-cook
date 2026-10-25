package services

import (
	"gorm.io/gorm"
	"math/rand"
	"ninimenu/internal/config"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"strings"
	"time"
)

type TomorrowPickOptions struct {
	MealType   string
	Profile    string
	Count      int
	ExcludeIDs []uint
}

// PickTomorrowDishes keeps the original dish-only contract while sharing the
// personal scoring engine used by home, MCP and weekly menus.
func PickTomorrowDishes(uid uint, opts TomorrowPickOptions, dbs ...*gorm.DB) ([]models.Dish, error) {
	result, err := PickTomorrowRecommendations(uid, opts, dbs...)
	if err != nil {
		return nil, err
	}
	dishes := make([]models.Dish, 0, len(result.Items))
	for _, item := range result.Items {
		dishes = append(dishes, item.Dish)
	}
	return dishes, nil
}

func PickTomorrowRecommendations(uid uint, opts TomorrowPickOptions, dbs ...*gorm.DB) (*RecommendResponse, error) {
	count := min(max(opts.Count, 1), MaxDishesPerMeal)
	req := RecommendRequest{
		MealType: strings.TrimSpace(opts.MealType),
		MealDate: healthNow().AddDate(0, 0, 1).Format("2006-01-02"),
		Count:    count, ExcludeDishIDs: opts.ExcludeIDs,
		Mode: "tomorrow", Source: "app", Jitter: true,
	}
	switch strings.TrimSpace(opts.Profile) {
	case "quick":
		req.Mood, req.MaxCookTime = "lazy", 30
	case "light":
		req.Mood = "healthy"
	case "spicy":
		req.Mood, req.Tastes = "spicy", []string{"辣"}
	case "favorite":
		// The core exposes favorite preference through its request mode. It
		// remains a soft preference so a small collection can still yield meals.
		req.Mode = "tomorrow_favorite"
	}
	return RecommendDishes(uid, req, dbs...)
}

func recentDishIDMap(uid uint, days int, dbs ...*gorm.DB) map[uint]bool {
	requestDB := database.Handle(dbs...)

	if days <= 0 {
		return map[uint]bool{}
	}
	since := healthNow().AddDate(0, 0, -days).Format("2006-01-02")
	var recentIDs []uint
	requestDB.Model(&models.MealRecord{}).Scopes(database.OwnedBy(uid)).
		Where("meal_date >= ? AND meal_date <= ?", since, healthToday()).
		Pluck("dish_id", &recentIDs)

	result := make(map[uint]bool, len(recentIDs))
	for _, id := range recentIDs {
		result[id] = true
	}
	return result
}

func defaultRepeatDays() int { return config.C.RepeatDays }

func containsTaste(raw string, target string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || target == "" {
		return false
	}
	for _, sep := range []string{"，", "、", "/", "|", ";", "；", " "} {
		raw = strings.ReplaceAll(raw, sep, ",")
	}
	for _, part := range strings.Split(raw, ",") {
		if strings.Contains(strings.TrimSpace(part), target) {
			return true
		}
	}
	return strings.Contains(raw, target)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func GetRandomQuote(scene string, dbs ...*gorm.DB) string {
	requestDB := database.Handle(dbs...)

	var quotes []models.Quote
	query := requestDB.Where("enabled = ?", true)
	if scene != "" {
		query = query.Where("scene = ?", scene)
	}
	query.Find(&quotes)

	if len(quotes) == 0 {
		requestDB.Where("enabled = ?", true).Find(&quotes)
	}

	if len(quotes) == 0 {
		return "今天吃点什么好呢？"
	}

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	return quotes[r.Intn(len(quotes))].Content
}
