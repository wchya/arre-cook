package handlers

import (
	"errors"
	"fmt"
	"html"
	"net/http"
	"ninimenu/internal/config"
	"ninimenu/internal/database"
	"ninimenu/internal/mailer"
	"ninimenu/internal/models"
	"ninimenu/internal/services"
	"ninimenu/internal/utils"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func familyError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrFamilyRequired):
		utils.NotFound(c, err.Error())
	case errors.Is(err, services.ErrFamilyOwnerOnly), errors.Is(err, services.ErrInviteEmail):
		utils.Forbidden(c, err.Error())
	case errors.Is(err, services.ErrFamilyConflict):
		utils.Error(c, http.StatusConflict, 40900, err.Error())
	case errors.Is(err, services.ErrInviteInvalid), errors.Is(err, services.ErrFamilyDish),
		errors.Is(err, services.ErrFamilyLimit), errors.Is(err, services.ErrInvalidDate),
		errors.Is(err, services.ErrInvalidMealType):
		utils.BadRequest(c, err.Error())
	default:
		utils.BadRequest(c, err.Error())
	}
}

func familyIDParam(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		utils.BadRequest(c, "ID 无效")
		return 0, false
	}
	return uint(id), true
}

func GetFamily(c *gin.Context) {
	info, err := services.FamilyInfo(uid(c))
	if err != nil {
		utils.InternalError(c, "加载家庭失败")
		return
	}
	utils.Success(c, info)
}

func CreateFamily(c *gin.Context) {
	var req struct{ Name string `json:"name" binding:"required"` }
	if c.ShouldBindJSON(&req) != nil {
		utils.BadRequest(c, "请填写家庭名称")
		return
	}
	if _, err := services.CreateFamily(uid(c), req.Name); err != nil {
		familyError(c, err)
		return
	}
	GetFamily(c)
}

func RenameFamily(c *gin.Context) {
	var req struct{ Name string `json:"name" binding:"required"` }
	if c.ShouldBindJSON(&req) != nil {
		utils.BadRequest(c, "请填写家庭名称")
		return
	}
	if err := services.RenameFamily(uid(c), req.Name); err != nil {
		familyError(c, err)
		return
	}
	GetFamily(c)
}

func InviteFamilyMember(c *gin.Context) {
	var req struct{ Email string `json:"email" binding:"required"` }
	if c.ShouldBindJSON(&req) != nil {
		utils.BadRequest(c, "请填写受邀人的 QQ 邮箱")
		return
	}
	invite, token, err := services.CreateFamilyInvitation(uid(c), req.Email)
	if err != nil {
		familyError(c, err)
		return
	}
	base := config.C.PublicURL
	if base == "" {
		base = "http://localhost:5173"
	}
	link := strings.TrimRight(base, "/") + "/family#invite=" + token
	var family models.Family
	database.DB.First(&family, invite.FamilyID)
	appName := database.GetSetting("app_name", "NiniMenu")
	message := fmt.Sprintf(
		"<p>%s 邀请你加入「%s」家庭，一起安排菜单、管理买菜清单。</p><p><a href=\"%s\">接受邀请</a></p><p>邀请 7 天内有效，只能由 %s 登录后接受。</p>",
		html.EscapeString(appName), html.EscapeString(family.Name), html.EscapeString(link), html.EscapeString(invite.Email),
	)
	sent := mailer.Send(invite.Email, "【"+appName+"】家庭邀请", message) == nil
	utils.Success(c, gin.H{"id": invite.ID, "email": invite.Email, "expires_at": invite.ExpiresAt, "link": link, "sent": sent})
}

func RevokeFamilyInvitation(c *gin.Context) {
	id, ok := familyIDParam(c)
	if !ok {
		return
	}
	if err := services.RevokeFamilyInvitation(uid(c), id); err != nil {
		familyError(c, err)
		return
	}
	utils.SuccessMsg(c, "邀请已撤销")
}

func JoinFamily(c *gin.Context) {
	var req struct{ Token string `json:"token" binding:"required"` }
	if c.ShouldBindJSON(&req) != nil {
		utils.BadRequest(c, "邀请链接无效")
		return
	}
	if _, err := services.JoinFamily(uid(c), req.Token); err != nil {
		familyError(c, err)
		return
	}
	GetFamily(c)
}

func TransferFamily(c *gin.Context) {
	var req struct{ UserID uint `json:"user_id" binding:"required"` }
	if c.ShouldBindJSON(&req) != nil {
		utils.BadRequest(c, "请选择家庭成员")
		return
	}
	if err := services.TransferFamily(uid(c), req.UserID); err != nil {
		familyError(c, err)
		return
	}
	GetFamily(c)
}

func RemoveFamilyMember(c *gin.Context) {
	id, ok := familyIDParam(c)
	if !ok {
		return
	}
	if err := services.RemoveFamilyMember(uid(c), id); err != nil {
		familyError(c, err)
		return
	}
	utils.SuccessMsg(c, "成员已移除")
}

func LeaveFamily(c *gin.Context) {
	if err := services.LeaveFamily(uid(c)); err != nil {
		familyError(c, err)
		return
	}
	utils.SuccessMsg(c, "已退出家庭")
}

func DeleteFamily(c *gin.Context) {
	var req struct{ Confirm string `json:"confirm"` }
	_ = c.ShouldBindJSON(&req)
	if req.Confirm != "解散家庭" {
		utils.BadRequest(c, "请输入“解散家庭”确认")
		return
	}
	if err := services.DeleteFamily(uid(c)); err != nil {
		familyError(c, err)
		return
	}
	utils.SuccessMsg(c, "家庭已解散")
}

func ShareFamilyDish(c *gin.Context) {
	family, err := services.RequireFamily(uid(c))
	if err != nil {
		familyError(c, err)
		return
	}
	id, ok := familyIDParam(c)
	if !ok {
		return
	}
	var dish models.Dish
	if err := database.DB.Where("id = ? AND owner_id = ? AND family_id = 0", id, uid(c)).First(&dish).Error; err != nil {
		utils.NotFound(c, "只能分享自己的私房菜")
		return
	}
	var count int64
	database.DB.Model(&models.Dish{}).Where("family_id = ?", family.ID).Count(&count)
	if count >= 500 {
		utils.BadRequest(c, "家庭菜谱已达 500 道上限")
		return
	}
	shared := dish
	shared.ID = 0
	shared.FamilyID = family.ID
	shared.Favorite = false
	shared.CreatedAt, shared.UpdatedAt = time.Time{}, time.Time{}
	if err := database.DB.Create(&shared).Error; err != nil {
		utils.InternalError(c, "分享菜谱失败")
		return
	}
	utils.Success(c, shared)
}

func GetFamilyPlan(c *gin.Context) {
	plan, err := services.FamilyPlan(uid(c), c.Query("start"))
	if err != nil {
		familyError(c, err)
		return
	}
	utils.Success(c, plan)
}

func SetFamilyPlan(c *gin.Context) {
	var req struct {
		MealDate string `json:"meal_date" binding:"required"`
		MealType string `json:"meal_type" binding:"required"`
		DishID   uint   `json:"dish_id"`
	}
	if c.ShouldBindJSON(&req) != nil {
		utils.BadRequest(c, "请选择日期和餐次")
		return
	}
	if err := services.SetFamilyPlan(uid(c), req.MealDate, req.MealType, req.DishID); err != nil {
		familyError(c, err)
		return
	}
	utils.SuccessMsg(c, "家庭菜单已更新")
}

func GetFamilyShopping(c *gin.Context) {
	items, err := services.FamilyShopping(uid(c))
	if err != nil {
		familyError(c, err)
		return
	}
	utils.Success(c, items)
}

func AddFamilyShopping(c *gin.Context) {
	var req struct {
		Name   string `json:"name" binding:"required"`
		Amount string `json:"amount"`
	}
	if c.ShouldBindJSON(&req) != nil {
		utils.BadRequest(c, "请填写食材")
		return
	}
	if err := services.AddFamilyShopping(uid(c), req.Name, req.Amount); err != nil {
		familyError(c, err)
		return
	}
	GetFamilyShopping(c)
}

func CheckFamilyShopping(c *gin.Context) {
	id, ok := familyIDParam(c)
	if !ok {
		return
	}
	var req struct{ Checked *bool `json:"checked" binding:"required"` }
	if c.ShouldBindJSON(&req) != nil || req.Checked == nil {
		utils.BadRequest(c, "请指定购买状态")
		return
	}
	if err := services.SetFamilyShoppingChecked(uid(c), id, *req.Checked); err != nil {
		familyError(c, err)
		return
	}
	utils.SuccessMsg(c, "已更新")
}

func DeleteFamilyShopping(c *gin.Context) {
	id, ok := familyIDParam(c)
	if !ok {
		return
	}
	if err := services.DeleteFamilyShopping(uid(c), id); err != nil {
		familyError(c, err)
		return
	}
	utils.SuccessMsg(c, "已删除")
}

func ImportFamilyIngredients(c *gin.Context) {
	var req struct{ DishID uint `json:"dish_id" binding:"required"` }
	if c.ShouldBindJSON(&req) != nil {
		utils.BadRequest(c, "请选择菜谱")
		return
	}
	count, err := services.ImportFamilyIngredients(uid(c), req.DishID)
	if err != nil {
		familyError(c, err)
		return
	}
	utils.Success(c, gin.H{"added": count})
}
