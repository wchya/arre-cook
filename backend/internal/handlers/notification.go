package handlers

import (
	"errors"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"ninimenu/internal/services"
	"ninimenu/internal/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ListNotifications GET /api/notifications
func ListNotifications(c *gin.Context) {
	page, pageSize := pageParams(c, 20)
	utils.Success(c, services.ListNotifications(uid(c), queryBool(c.Query("unread")), page, pageSize))
}

func MarkNotificationRead(c *gin.Context) {
	if err := services.MarkNotificationRead(uid(c), c.Param("id")); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.NotFound(c, "通知不存在")
			return
		}
		utils.InternalError(c, "标记通知失败")
		return
	}
	utils.SuccessMsg(c, "已标记为已读")
}

func MarkAllNotificationsRead(c *gin.Context) {
	if err := services.MarkAllNotificationsRead(uid(c)); err != nil {
		utils.InternalError(c, "标记通知失败")
		return
	}
	utils.SuccessMsg(c, "全部已读")
}

// CreateNotification 管理员发布系统通知，可指定 user_id；省略则广播给所有正常用户。
func CreateAdminNotification(c *gin.Context) {
	var req struct {
		UserID  uint   `json:"user_id"`
		Type    string `json:"type"`
		Title   string `json:"title" binding:"required"`
		Content string `json:"content" binding:"required"`
		Link    string `json:"link"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请输入通知标题和内容")
		return
	}
	if req.UserID != 0 {
		var user models.User
		if err := database.DB.First(&user, req.UserID).Error; err != nil {
			utils.NotFound(c, "通知接收人不存在")
			return
		}
	}
	n, err := services.PublishNotification(req.UserID, req.Type, req.Title, req.Content, req.Link)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	utils.Success(c, gin.H{"sent": n, "type": services.NormalizeNotificationType(req.Type)})
}
