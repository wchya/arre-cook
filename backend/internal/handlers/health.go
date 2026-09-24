package handlers

import (
	"errors"
	"net/http"
	"ninimenu/internal/services"
	"ninimenu/internal/utils"
	"strconv"

	"github.com/gin-gonic/gin"
)

func GetFoodJournal(c *gin.Context) {
	items, err := services.ListFoodJournal(uid(c), c.Query("from"), c.Query("to"))
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	utils.Success(c, items)
}

func CreateFoodJournal(c *gin.Context) {
	var input services.FoodJournalInput
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.BadRequest(c, "饮食记录格式无效")
		return
	}
	entry, err := services.CreateFoodJournalEntry(uid(c), input)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	utils.Success(c, entry)
}

func DeleteFoodJournal(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		utils.BadRequest(c, "记录 ID 无效")
		return
	}
	if err := services.DeleteFoodJournalEntry(uid(c), uint(id)); err != nil {
		if errors.Is(err, services.ErrFoodEntryNotFound) {
			utils.NotFound(c, err.Error())
		} else {
			utils.InternalError(c, "删除记录失败")
		}
		return
	}
	utils.SuccessMsg(c, "已删除")
}

func GetHealthReport(c *gin.Context) {
	period, _ := strconv.Atoi(c.DefaultQuery("days", "7"))
	if period != 7 && period != 30 {
		utils.Error(c, http.StatusBadRequest, 40000, "只支持最近 7 天或 30 天")
		return
	}
	report, err := services.BuildHealthReport(uid(c), period)
	if err != nil {
		utils.InternalError(c, "生成报告失败")
		return
	}
	utils.Success(c, report)
}
