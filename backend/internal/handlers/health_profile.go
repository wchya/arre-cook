package handlers

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"ninimenu/internal/database"
	"ninimenu/internal/services"
	"ninimenu/internal/utils"
)

func GetHealthProfile(c *gin.Context) {
	profile, err := services.LoadHealthProfile(uid(c), database.DB.WithContext(c.Request.Context()))
	if err != nil {
		healthProfileError(c, err)
		return
	}
	utils.Success(c, profile)
}

func PutHealthProfile(c *gin.Context) {
	var input services.HealthProfileInput
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.BadRequest(c, "档案格式无效")
		return
	}
	profile, err := services.SaveHealthProfile(uid(c), input, database.DB.WithContext(c.Request.Context()))
	if err != nil {
		healthProfileError(c, err)
		return
	}
	utils.Success(c, profile)
}

func DeleteHealthProfile(c *gin.Context) {
	version, err := strconv.Atoi(c.Query("version"))
	if err != nil || version < 0 {
		utils.BadRequest(c, "请提供当前档案版本")
		return
	}
	profile, err := services.ClearHealthProfile(uid(c), version, database.DB.WithContext(c.Request.Context()))
	if err != nil {
		healthProfileError(c, err)
		return
	}
	utils.Success(c, profile)
}

func healthProfileError(c *gin.Context, err error) {
	if errors.Is(err, services.ErrHealthProfileConflict) {
		utils.Error(c, 409, 40900, err.Error())
		return
	}
	if errors.Is(err, services.ErrInvalidHealthProfile) {
		utils.BadRequest(c, err.Error())
		return
	}
	utils.InternalError(c, "健康档案暂时无法保存或读取，请稍后重试")
}
