package handlers

import (
	"errors"
	"github.com/gin-gonic/gin"
	"ninimenu/internal/database"
	"ninimenu/internal/services"
	"ninimenu/internal/utils"
	"strconv"
)

func ListNutritionFoods(c *gin.Context) {
	foods, err := services.ListNutritionFoods(uid(c), database.DB.WithContext(c.Request.Context()))
	if err != nil {
		utils.InternalError(c, "营养标签加载失败")
		return
	}
	utils.Success(c, foods)
}
func CreateNutritionFood(c *gin.Context) { saveNutritionFood(c, 0) }
func UpdateNutritionFood(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		utils.BadRequest(c, "标签 ID 无效")
		return
	}
	saveNutritionFood(c, uint(id))
}
func saveNutritionFood(c *gin.Context, id uint) {
	var in services.NutritionFoodInput
	if c.ShouldBindJSON(&in) != nil {
		utils.BadRequest(c, "营养标签格式无效")
		return
	}
	food, err := services.SaveNutritionFood(uid(c), id, in, database.DB.WithContext(c.Request.Context()))
	if errors.Is(err, services.ErrFoodEntryNotFound) {
		utils.NotFound(c, err.Error())
		return
	}
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	utils.Success(c, food)
}
func DeleteNutritionFood(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		utils.BadRequest(c, "标签 ID 无效")
		return
	}
	err = services.DeleteNutritionFood(uid(c), uint(id), database.DB.WithContext(c.Request.Context()))
	if errors.Is(err, services.ErrFoodEntryNotFound) {
		utils.NotFound(c, err.Error())
		return
	}
	if err != nil {
		utils.InternalError(c, "删除标签失败")
		return
	}
	utils.SuccessMsg(c, "已删除标签，历史摄入快照保留")
}

func CreateNutritionRecipe(c *gin.Context) { saveNutritionRecipe(c, 0) }
func UpdateNutritionRecipe(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		utils.BadRequest(c, "配方 ID 无效")
		return
	}
	saveNutritionRecipe(c, uint(id))
}
func saveNutritionRecipe(c *gin.Context, id uint) {
	var in services.NutritionRecipeInput
	if c.ShouldBindJSON(&in) != nil {
		utils.BadRequest(c, "配方格式无效")
		return
	}
	food, err := services.SaveNutritionRecipe(uid(c), id, in, database.DB.WithContext(c.Request.Context()))
	if errors.Is(err, services.ErrFoodEntryNotFound) {
		utils.NotFound(c, err.Error())
		return
	}
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	utils.Success(c, food)
}

func SearchNutritionCatalog(c *gin.Context) {
	rows, err := services.SearchNutritionCatalog(c.Query("q"), c.Query("state"), database.DB.WithContext(c.Request.Context()))
	if err != nil {
		utils.BadRequest(c, "食物目录查询失败，请检查搜索词和食物状态后重试")
		return
	}
	utils.Success(c, rows)
}
func AdoptNutritionCatalog(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		utils.BadRequest(c, "食物编号无效")
		return
	}
	food, err := services.AdoptNutritionCatalog(uid(c), uint(id), database.DB.WithContext(c.Request.Context()))
	if errors.Is(err, services.ErrFoodEntryNotFound) {
		utils.NotFound(c, "食物不存在或已撤下")
		return
	}
	if err != nil {
		utils.BadRequest(c, "无法添加食物，请检查个人食物数量或稍后重试")
		return
	}
	utils.Success(c, food)
}
