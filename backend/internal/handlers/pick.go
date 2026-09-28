package handlers

import (
	"errors"
	"gorm.io/gorm"
	"io"
	"math/rand"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"ninimenu/internal/services"
	"ninimenu/internal/utils"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type PickRequest struct {
	Mood string `json:"mood"`
}

type TomorrowPickRequest struct {
	MealType   string `json:"meal_type"`
	Profile    string `json:"profile"`
	Count      int    `json:"count"`
	ExcludeIDs []uint `json:"exclude_ids"`
}

// recommendForApp 站内推荐统一走推荐引擎（口味画像 + 约束打分），并带一点随机抖动保证“换一个”有变化。
func recommendForApp(u uint, req services.RecommendRequest, dbs ...*gorm.DB) []models.Dish {
	requestDB := database.Handle(dbs...)

	req.Source = "app"
	req.Jitter = true
	result, err := services.RecommendDishes(u, req, requestDB)
	if err != nil || result == nil {
		return nil
	}
	dishes := make([]models.Dish, 0, len(result.Items))
	for _, it := range result.Items {
		dishes = append(dishes, it.Dish)
	}
	return dishes
}

func PickLunch(c *gin.Context) {
	dishes := recommendForApp(uid(c), services.RecommendRequest{MealType: "lunch", Count: getPickCount(c)}, database.DB.WithContext(c.Request.Context()))
	if len(dishes) == 0 {
		utils.NotFound(c, "没有可推荐的菜品")
		return
	}
	services.RecordAchievementEvent(uid(c), "recommend_lunch", "", database.DB.WithContext(c.Request.Context()))
	utils.Success(c, gin.H{"dishes": dishes, "quote": services.GetRandomQuote("lunch", database.DB.WithContext(c.Request.Context()))})
}

func PickDinner(c *gin.Context) {
	dishes := recommendForApp(uid(c), services.RecommendRequest{MealType: "dinner", Count: getPickCount(c)}, database.DB.WithContext(c.Request.Context()))
	if len(dishes) == 0 {
		utils.NotFound(c, "没有可推荐的菜品")
		return
	}
	services.RecordAchievementEvent(uid(c), "recommend_dinner", "", database.DB.WithContext(c.Request.Context()))
	utils.Success(c, gin.H{"dishes": dishes, "quote": services.GetRandomQuote("dinner", database.DB.WithContext(c.Request.Context()))})
}

func PickMood(c *gin.Context) {
	var req PickRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请选择心情")
		return
	}

	rec := services.RecommendRequest{Mood: req.Mood, Count: 4}
	switch req.Mood {
	case "tired", "lazy":
		rec.MaxCookTime = 30
	case "spicy":
		rec.Tastes = []string{"辣"}
	}
	dishes := recommendForApp(uid(c), rec, database.DB.WithContext(c.Request.Context()))
	if len(dishes) == 0 {
		utils.NotFound(c, "没有可推荐的菜品")
		return
	}
	services.RecordAchievementEvent(uid(c), "recommend_mood", "", database.DB.WithContext(c.Request.Context()))

	scene := ""
	switch req.Mood {
	case "tired", "lazy":
		scene = "quick"
	case "spicy":
		scene = "lunch"
	case "healthy":
		scene = "dinner"
	}
	utils.Success(c, gin.H{"dishes": dishes, "quote": services.GetRandomQuote(scene, database.DB.WithContext(c.Request.Context()))})
}

// PickSmart 首页“智能推荐”：直接暴露推荐引擎的完整入参与带理由的结果。
func PickSmart(c *gin.Context) {
	var req services.RecommendRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		utils.BadRequest(c, "请求数据无效")
		return
	}
	req.Source = "app"
	req.Jitter = true
	result, err := services.RecommendDishes(uid(c), req, database.DB.WithContext(c.Request.Context()))
	if err != nil || result == nil || len(result.Items) == 0 {
		utils.NotFound(c, "没有可推荐的菜品")
		return
	}
	if req.Mode != "home_auto" {
		services.RecordAchievementEvent(uid(c), "recommend_mood", req.Mood, database.DB.WithContext(c.Request.Context()))
	}
	utils.Success(c, gin.H{
		"items":           result.Items,
		"profile_summary": result.ProfileSummary,
		"applied":         result.Applied,
		"quote":           services.GetRandomQuote("", database.DB.WithContext(c.Request.Context())),
	})
}

func PickTomorrow(c *gin.Context) {
	var req TomorrowPickRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请求数据无效")
		return
	}

	dishes, err := services.PickTomorrowDishes(uid(c), services.TomorrowPickOptions{
		MealType: req.MealType, Profile: req.Profile, Count: req.Count, ExcludeIDs: req.ExcludeIDs,
	}, database.DB.WithContext(c.Request.Context()))
	if err != nil || len(dishes) == 0 {
		utils.NotFound(c, "没有可推荐的菜品")
		return
	}

	services.RecordAchievementEvent(uid(c), "recommend_mood", req.Profile, database.DB.WithContext(c.Request.Context()))
	utils.Success(c, gin.H{"dishes": dishes, "quote": services.GetRandomQuote(req.Profile, database.DB.WithContext(c.Request.Context()))})
}

func PickBlindBox(c *gin.Context) {
	dishes := services.VisibleEnabledDishes(uid(c), database.DB.WithContext(c.Request.Context()))
	if len(dishes) == 0 {
		utils.NotFound(c, "没有可推荐的菜品")
		return
	}

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	// 自建菜谱被抽中的概率高 30%（与推荐引擎的来源权重一致）
	total := 0.0
	for _, d := range dishes {
		total += services.DishSourceWeight(d)
	}
	x, idx := r.Float64()*total, len(dishes)-1
	for i, d := range dishes {
		if x -= services.DishSourceWeight(d); x < 0 {
			idx = i
			break
		}
	}
	dish := dishes[idx]
	services.MarkFavorite(uid(c), &dish, database.DB.WithContext(c.Request.Context()))

	hint := "点我揭晓今日惊喜~"
	var box models.BlindBox
	if err := database.DB.WithContext(c.Request.Context()).Where("active = ?", true).Limit(1).Find(&box).Error; err == nil && box.Hint != "" {
		hint = box.Hint
	}

	services.RecordAchievementEvent(uid(c), "blind_box", "", database.DB.WithContext(c.Request.Context()))
	services.LogBehavior(uid(c), "recommend", dish.ID, dish.Name, "app", "", map[string]any{"mode": "blind_box"}, database.DB.WithContext(c.Request.Context()))
	utils.Success(c, gin.H{"hint": hint, "dish": dish, "quote": services.GetRandomQuote("", database.DB.WithContext(c.Request.Context()))})
}

func getPickCount(c *gin.Context) int {
	count, err := strconv.Atoi(c.DefaultQuery("count", "3"))
	if err != nil || count < 1 {
		count = 3
	}
	if count > 10 {
		count = 10
	}
	return count
}
