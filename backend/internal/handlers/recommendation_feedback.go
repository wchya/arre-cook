package handlers

import (
	"ninimenu/internal/database"
	"ninimenu/internal/services"
	"ninimenu/internal/utils"

	"github.com/gin-gonic/gin"
)

func CreateRecommendationFeedback(c *gin.Context) {
	var in services.RecommendationFeedbackInput
	if c.ShouldBindJSON(&in) != nil {
		utils.BadRequest(c, "推荐反馈格式无效")
		return
	}
	source, actor := "app", ""
	if p := principal(c); p != nil && p.IsAgent() {
		source, actor = p.Source(), p.Actor
	}
	result, err := services.RecordRecommendationFeedback(uid(c), in, source, actor, database.DB.WithContext(c.Request.Context()))
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	utils.Success(c, result)
}
