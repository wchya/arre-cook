package handlers

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"ninimenu/internal/auth"
	"ninimenu/internal/database"
	"ninimenu/internal/services"
	"ninimenu/internal/utils"
)

func mealPlanError(c *gin.Context, err error) {
	if errors.Is(err, services.ErrRecordNotFound) || errors.Is(err, services.ErrDishNotFound) {
		utils.NotFound(c, err.Error())
		return
	}
	utils.BadRequest(c, err.Error())
}

func GetMealPlans(c *gin.Context) {
	plans, err := services.ListMealPlans(uid(c), c.Query("date_from"), c.Query("date_to"), database.DB.WithContext(c.Request.Context()))
	if err != nil {
		mealPlanError(c, err)
		return
	}
	utils.Success(c, plans)
}

func CreateMealPlan(c *gin.Context) {
	var in services.MealPlanInput
	if c.ShouldBindJSON(&in) != nil {
		utils.BadRequest(c, "菜单计划格式无效")
		return
	}
	p := auth.GetPrincipal(c)
	in.Source, in.Actor = p.Source(), p.Actor
	plan, err := services.AcceptMealPlan(uid(c), in, database.DB.WithContext(c.Request.Context()))
	if err != nil {
		mealPlanError(c, err)
		return
	}
	utils.Success(c, plan)
}

func ReplaceMealPlan(c *gin.Context) {
	var body struct {
		MealDate          string          `json:"meal_date"`
		MealType          string          `json:"meal_type"`
		DishIDs           *[]uint         `json:"dish_ids"`
		RecommendationIDs map[uint]string `json:"recommendation_ids"`
	}
	if c.ShouldBindJSON(&body) != nil || body.DishIDs == nil {
		utils.BadRequest(c, "请提供整餐菜品；清空时传空数组")
		return
	}
	p := auth.GetPrincipal(c)
	in := services.MealPlanReplaceInput{MealDate: body.MealDate, MealType: body.MealType, DishIDs: *body.DishIDs, RecommendationIDs: body.RecommendationIDs, Source: p.Source(), Actor: p.Actor}
	plans, err := services.ReplaceMealPlan(uid(c), in, database.DB.WithContext(c.Request.Context()))
	if err != nil {
		mealPlanError(c, err)
		return
	}
	utils.Success(c, plans)
}

func CancelMealPlan(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		utils.BadRequest(c, "计划 ID 无效")
		return
	}
	if err := services.CancelMealPlan(uid(c), uint(id), database.DB.WithContext(c.Request.Context())); err != nil {
		mealPlanError(c, err)
		return
	}
	utils.SuccessMsg(c, "已撤销计划")
}
