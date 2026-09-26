package handlers

import (
	"ninimenu/internal/services"
	"ninimenu/internal/utils"

	"github.com/gin-gonic/gin"
)

// GetLinkPreview 解析视频/网页链接，返回平台、标题、封面与可嵌入播放地址，供添加菜谱时预展示。
func GetLinkPreview(c *gin.Context) {
	preview, err := services.FetchLinkPreview(c.Query("url"))
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	utils.Success(c, preview)
}
