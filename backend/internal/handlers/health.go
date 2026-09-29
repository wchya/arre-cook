package handlers

import (
	"ninimenu/internal/database"
	"ninimenu/internal/models"
)

import (
	"errors"
	"net/http"
	"ninimenu/internal/services"
	"ninimenu/internal/utils"
	"strconv"

	"github.com/gin-gonic/gin"
)

func GetFoodJournal(c *gin.Context) {
	items, err := services.ListFoodJournal(uid(c), c.Query("from"), c.Query("to"), database.DB.WithContext(c.Request.Context()))
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
	entry, err := services.CreateFoodJournalEntry(uid(c), input, database.DB.WithContext(c.Request.Context()))
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
	if err := services.DeleteFoodJournalEntry(uid(c), uint(id), database.DB.WithContext(c.Request.Context())); err != nil {
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
	report, err := services.BuildHealthReport(uid(c), period, database.DB.WithContext(c.Request.Context()))
	if err != nil {
		utils.InternalError(c, "生成报告失败")
		return
	}
	utils.Success(c, report)
}

func UpdateFoodJournal(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		utils.BadRequest(c, "记录 ID 无效")
		return
	}
	var in services.FoodJournalInput
	if c.ShouldBindJSON(&in) != nil {
		utils.BadRequest(c, "饮食记录格式无效")
		return
	}
	entry, err := services.UpdateFoodJournalEntry(uid(c), uint(id), in, database.DB.WithContext(c.Request.Context()))
	if errors.Is(err, services.ErrFoodEntryNotFound) {
		utils.NotFound(c, err.Error())
		return
	}
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	utils.Success(c, entry)
}

func ConfirmHealthDay(c *gin.Context) {
	var in struct {
		Fingerprint string `json:"fingerprint"`
		Complete    bool   `json:"complete"`
	}
	if c.ShouldBindJSON(&in) != nil {
		utils.BadRequest(c, "确认格式无效")
		return
	}
	if err := services.ConfirmHealthDay(uid(c), c.Param("date"), in.Fingerprint, in.Complete, database.DB.WithContext(c.Request.Context())); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	utils.SuccessMsg(c, "记录状态已更新")
}

func AcceptHealthPlan(c *gin.Context) {
	var in services.HealthPlanInput
	if c.ShouldBindJSON(&in) != nil {
		utils.BadRequest(c, "菜单格式无效")
		return
	}
	plan, err := services.AcceptHealthPlan(uid(c), in, database.DB.WithContext(c.Request.Context()))
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	utils.Success(c, plan)
}

func CancelHealthPlan(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		utils.BadRequest(c, "计划 ID 无效")
		return
	}
	err = services.CancelHealthPlan(uid(c), uint(id), database.DB.WithContext(c.Request.Context()))
	if errors.Is(err, services.ErrRecordNotFound) {
		utils.NotFound(c, err.Error())
		return
	}
	if err != nil {
		utils.InternalError(c, "撤销失败，请重试")
		return
	}
	utils.SuccessMsg(c, "已撤销计划")
}

func GetFoodJournalEntry(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		utils.BadRequest(c, "记录 ID 无效")
		return
	}
	var entry models.FoodJournalEntry
	if database.DB.WithContext(c.Request.Context()).Scopes(database.OwnedBy(uid(c))).First(&entry, uint(id)).Error != nil {
		utils.NotFound(c, "记录不存在")
		return
	}
	utils.Success(c, entry)
}
