package handlers

import (
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"ninimenu/internal/services"
	"ninimenu/internal/utils"
	"time"

	"github.com/gin-gonic/gin"
)

func GetWeekPlan(c *gin.Context) {
	plan := services.GetCachedWeekPlan(uid(c), database.DB.WithContext(c.Request.Context()))
	if plan != nil && len(plan.Days) > 0 {
		services.RecordUniqueAchievementEvent(uid(c), "week_plan", plan.Days[0].Date, database.DB.WithContext(c.Request.Context()))
	}
	utils.Success(c, plan)
}

func RegenerateWeekPlanHandler(c *gin.Context) {
	plan := services.RegenerateWeekPlan(uid(c), database.DB.WithContext(c.Request.Context()))
	services.RecordAchievementEvent(uid(c), "week_plan", "", database.DB.WithContext(c.Request.Context()))
	utils.Success(c, plan)
}

func shoppingDates() []string {
	return []string{time.Now().Format("2006-01-02"), time.Now().AddDate(0, 0, 1).Format("2006-01-02")}
}

func GetShoppingList(c *gin.Context) {
	utils.Success(c, services.BuildShoppingList(uid(c), shoppingDates(), database.DB.WithContext(c.Request.Context())))
}

// GetShoppingOverview 个人 + 家庭两份买菜清单（今明两天），供买菜页分区展示。
func GetShoppingOverview(c *gin.Context) {
	utils.Success(c, services.ShoppingOverview(uid(c), database.DB.WithContext(c.Request.Context())))
}

type ToggleShoppingCheckRequest struct {
	ItemName string `json:"item_name" binding:"required"`
	MealDate string `json:"meal_date"`
	Checked  bool   `json:"checked"`
}

func ToggleShoppingCheckHandler(c *gin.Context) {
	var req ToggleShoppingCheckRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数无效")
		return
	}
	services.ToggleShoppingCheck(uid(c), req.ItemName, shoppingDates(), req.Checked, database.DB.WithContext(c.Request.Context()))
	if req.Checked {
		services.QueueAutoAchievementSync(uid(c))
	}
	utils.SuccessMsg(c, "已更新")
}

type ToggleHomeInventoryRequest struct {
	ItemName string `json:"item_name" binding:"required"`
	InStock  bool   `json:"in_stock"`
}

func ToggleHomeInventoryHandler(c *gin.Context) {
	var req ToggleHomeInventoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数无效")
		return
	}
	services.ToggleHomeInventory(uid(c), req.ItemName, req.InStock, database.DB.WithContext(c.Request.Context()))
	services.QueueAutoAchievementSync(uid(c))
	utils.SuccessMsg(c, "已更新")
}

func GetShoppingCategories(c *gin.Context) {
	utils.Success(c, services.ListShoppingCategoryOverrides(database.DB.WithContext(c.Request.Context())))
}

type UpsertShoppingCategoryRequest struct {
	ItemName string `json:"item_name" binding:"required"`
	Category string `json:"category" binding:"required"`
}

func UpsertShoppingCategoryHandler(c *gin.Context) {
	var req UpsertShoppingCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数无效")
		return
	}
	if !services.UpsertShoppingCategoryOverride(req.ItemName, req.Category, database.DB.WithContext(c.Request.Context())) {
		utils.BadRequest(c, "分类无效")
		return
	}
	utils.SuccessMsg(c, "已更新")
}

func DeleteShoppingCategoryHandler(c *gin.Context) {
	services.DeleteShoppingCategoryOverride(c.Param("itemName"), database.DB.WithContext(c.Request.Context()))
	utils.SuccessMsg(c, "已删除")
}

func GetUpcomingHolidays(c *gin.Context) {
	var holidays []models.Holiday
	database.DB.WithContext(c.Request.Context()).Where("date >= ?", todayStr()).Order("date ASC").Limit(5).Find(&holidays)
	utils.Success(c, holidays)
}
