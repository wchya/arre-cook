package handlers

import (
	"ninimenu/internal/auth"
	"ninimenu/internal/config"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"ninimenu/internal/services"
	"ninimenu/internal/storage"
	"ninimenu/internal/utils"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type userView struct {
	ID          uint       `json:"id"`
	Email       string     `json:"email"`
	Username    string     `json:"username"`
	Nickname    string     `json:"nickname"`
	Avatar      string     `json:"avatar"`
	Role        string     `json:"role"`
	WechatBound bool       `json:"wechat_bound"`
	HasPassword bool       `json:"has_password"`
	Disabled    bool       `json:"disabled"`
	LastLoginAt *time.Time `json:"last_login_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

func toUserView(u *models.User) userView {
	v := userView{
		ID: u.ID, Nickname: u.DisplayName(), Avatar: u.Avatar, Role: u.Role,
		WechatBound: u.WechatOpenID != nil && *u.WechatOpenID != "", HasPassword: u.PasswordHash != "",
		Disabled: u.Disabled, LastLoginAt: u.LastLoginAt, CreatedAt: u.CreatedAt,
	}
	if u.Email != nil {
		v.Email = *u.Email
	}
	if u.Username != nil {
		v.Username = *u.Username
	}
	return v
}

func issueLogin(c *gin.Context, u *models.User, extra gin.H) {
	token, exp, err := auth.IssueUserToken(u)
	if err != nil {
		utils.InternalError(c, "生成登录凭证失败")
		return
	}
	data := gin.H{"token": token, "expires_at": exp, "user": toUserView(u)}
	for k, v := range extra {
		data[k] = v
	}
	utils.Success(c, data)
}

// GetAuthOptions 登录页需要知道哪些登录方式可用。
func GetAuthOptions(c *gin.Context) {
	utils.Success(c, gin.H{
		"email":         config.C.SMTPEnabled() || config.C.EmailCodeDevEcho,
		"email_dev":     config.C.EmailCodeDevEcho,
		"email_domains": config.C.EmailDomains,
		"wechat":        config.C.WechatEnabled(),
		"register_open": services.RegistrationOpen(),
	})
}

// SendEmailCode POST /api/auth/email/code
func SendEmailCode(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请输入邮箱")
		return
	}
	wait, err := services.SendEmailCode(req.Email, "login", c.ClientIP())
	if err != nil {
		c.JSON(400, gin.H{"code": 40001, "message": err.Error(), "data": gin.H{"cooldown": wait}})
		return
	}
	utils.Success(c, gin.H{"cooldown": wait, "ttl_minutes": int(config.C.EmailCodeTTL.Minutes())})
}

// EmailLogin POST /api/auth/email/login
func EmailLogin(c *gin.Context) {
	var req struct {
		Email      string `json:"email" binding:"required"`
		Code       string `json:"code" binding:"required"`
		WechatCode string `json:"wechat_code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请输入邮箱和验证码")
		return
	}
	u, created, err := services.LoginWithEmailCode(req.Email, req.Code, req.WechatCode)
	if err != nil {
		utils.Error(c, 400, 40002, err.Error())
		return
	}
	issueLogin(c, u, gin.H{"created": created})
}

// PasswordLogin POST /api/auth/login（兜底：管理员初始账号或设置过密码的用户）
func PasswordLogin(c *gin.Context) {
	var req struct {
		Account  string `json:"account"`
		Username string `json:"username"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请输入账号和密码")
		return
	}
	account := req.Account
	if account == "" {
		account = req.Username
	}
	u, err := services.LoginWithPassword(account, req.Password)
	if err != nil {
		utils.Unauthorized(c, err.Error())
		return
	}
	issueLogin(c, u, nil)
}

// WechatLogin POST /api/auth/wechat —— 小程序 wx.login 的 code。未绑定账号时返回 need_bind。
func WechatLogin(c *gin.Context) {
	var req struct {
		Code string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "缺少 code")
		return
	}
	u, needBind, err := services.LoginWithWechat(req.Code)
	if err != nil {
		utils.Error(c, 400, 40003, err.Error())
		return
	}
	if needBind {
		utils.Success(c, gin.H{"need_bind": true})
		return
	}
	issueLogin(c, u, gin.H{"need_bind": false})
}

// GetMe GET /api/me
func GetMe(c *gin.Context) {
	utils.Success(c, toUserView(auth.CurrentUser(c)))
}

// UpdateMe PUT /api/me
func UpdateMe(c *gin.Context) {
	var req struct {
		Nickname *string `json:"nickname"`
		Avatar   *string `json:"avatar"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请求数据无效")
		return
	}
	u := auth.CurrentUser(c)
	updates := map[string]any{}
	if req.Nickname != nil {
		n := strings.TrimSpace(*req.Nickname)
		if n == "" || len([]rune(n)) > 20 {
			utils.BadRequest(c, "昵称 1-20 个字")
			return
		}
		updates["nickname"] = n
		u.Nickname = n
	}
	if req.Avatar != nil {
		a := strings.TrimSpace(*req.Avatar)
		if a != "" && !storage.IsUploadURL(a) {
			utils.BadRequest(c, "头像地址无效")
			return
		}
		updates["avatar"] = a
		u.Avatar = a
	}
	if len(updates) > 0 {
		database.DB.Model(&models.User{}).Where("id = ?", u.ID).Updates(updates)
	}
	utils.Success(c, toUserView(u))
}

// ChangePassword PUT /api/me/password —— 设置/修改密码（已有密码需校验旧密码）。改密后其他设备登录失效。
func ChangePassword(c *gin.Context) {
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请输入新密码")
		return
	}
	u := auth.CurrentUser(c)
	if u.PasswordHash != "" && !auth.VerifyPassword(u.PasswordHash, req.OldPassword) {
		utils.BadRequest(c, "原密码错误")
		return
	}
	if err := auth.ValidatePassword(req.NewPassword); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		utils.InternalError(c, "设置密码失败")
		return
	}
	u.PasswordHash = hash
	u.TokenVersion++
	database.DB.Model(&models.User{}).Where("id = ?", u.ID).Updates(map[string]any{"password_hash": hash, "token_version": u.TokenVersion})
	issueLogin(c, u, nil)
}

// LogoutAll POST /api/me/logout-all —— 让所有设备上的登录态失效（智能体令牌不受影响，需单独撤销）。
func LogoutAll(c *gin.Context) {
	u := auth.CurrentUser(c)
	database.DB.Model(&models.User{}).Where("id = ?", u.ID).UpdateColumn("token_version", gorm.Expr("token_version + 1"))
	utils.SuccessMsg(c, "已退出所有设备")
}

// userOwnedTables 注销账号时需要清理的个人数据表。
var userOwnedModels = []any{
	&models.MealRecord{}, &models.Favorite{}, &models.DayRating{}, &models.ShoppingCheck{},
	&models.HomeInventory{}, &models.BehaviorEvent{}, &models.AchievementEvent{}, &models.UserAchievement{},
	&models.UserPreference{}, &models.UserSetting{}, &models.AgentToken{}, &models.AgentAuditLog{},
	&models.AgentSuggestion{}, &models.ChatMessage{}, &models.ChatSession{}, &models.FoodJournalEntry{},
}

// DeleteMe DELETE /api/me —— 注销账号并删除全部个人数据（不可恢复）。
func DeleteMe(c *gin.Context) {
	var req struct {
		Confirm string `json:"confirm"`
	}
	_ = c.ShouldBindJSON(&req)
	u := auth.CurrentUser(c)
	if strings.TrimSpace(req.Confirm) != "注销" {
		utils.BadRequest(c, "请输入“注销”确认")
		return
	}
	if u.IsAdmin() {
		var admins int64
		database.DB.Model(&models.User{}).Where("role = ? AND disabled = ?", models.RoleAdmin, false).Count(&admins)
		if admins <= 1 {
			utils.BadRequest(c, "你是唯一的管理员，不能注销")
			return
		}
	}
	family, err := services.FamilyForUser(u.ID)
	if err != nil {
		utils.InternalError(c, "注销失败，请稍后再试")
		return
	}
	if family != nil && family.OwnerID == u.ID {
		utils.BadRequest(c, "请先转让或解散家庭，再注销账号")
		return
	}
	err = database.DB.Transaction(func(tx *gorm.DB) error {
		if family != nil {
			if err := tx.Where("family_id = ? AND user_id = ?", family.ID, u.ID).Delete(&models.FamilyMember{}).Error; err != nil {
				return err
			}
			if err := tx.Model(&models.Dish{}).Where("family_id = ? AND owner_id = ?", family.ID, u.ID).
				Update("owner_id", family.OwnerID).Error; err != nil {
				return err
			}
		}
		for _, m := range userOwnedModels {
			if err := tx.Where("user_id = ?", u.ID).Delete(m).Error; err != nil {
				return err
			}
		}
		if err := tx.Unscoped().Where("owner_id = ? AND family_id = 0", u.ID).Delete(&models.Dish{}).Error; err != nil {
			return err
		}
		return tx.Delete(&models.User{}, u.ID).Error
	})
	if err != nil {
		utils.InternalError(c, "注销失败，请稍后再试")
		return
	}
	services.InvalidateWeekPlan(u.ID)
	utils.SuccessMsg(c, "账号已注销")
}

// ExportMe GET /api/me/export —— 导出本人全部数据（JSON）。
func ExportMe(c *gin.Context) {
	uid := auth.UID(c)
	own := database.OwnedBy(uid)
	var (
		records     []models.MealRecord
		favorites   []models.Favorite
		ratings     []models.DayRating
		events      []models.BehaviorEvent
		dishes      []models.Dish
		suggestions []models.AgentSuggestion
		sessions    []models.ChatSession
		messages    []models.ChatMessage
		journal     []models.FoodJournalEntry
	)
	database.DB.Scopes(own).Order("meal_date ASC").Find(&records)
	database.DB.Scopes(own).Find(&favorites)
	database.DB.Scopes(own).Order("meal_date ASC").Find(&ratings)
	database.DB.Scopes(own).Order("created_at DESC").Limit(10000).Find(&events)
	database.DB.Where("owner_id = ?", uid).Find(&dishes)
	database.DB.Scopes(own).Find(&suggestions)
	database.DB.Scopes(own).Find(&sessions)
	database.DB.Scopes(own).Order("id ASC").Find(&messages)
	database.DB.Scopes(own).Order("meal_date ASC").Find(&journal)
	c.Header("Content-Disposition", `attachment; filename="ninimenu-export.json"`)
	utils.Success(c, gin.H{
		"exported_at":     time.Now().Format(time.RFC3339),
		"user":            toUserView(auth.CurrentUser(c)),
		"preferences":     services.GetPreferences(uid),
		"meal_records":    records,
		"favorites":       favorites,
		"day_ratings":     ratings,
		"behavior_events": events,
		"my_dishes":       dishes,
		"suggestions":     suggestions,
		"chat_sessions":   sessions,
		"chat_messages":   messages,
		"food_journal":    journal,
	})
}

// GetPreferences / UpdatePreferences /api/me/preferences
func GetPreferences(c *gin.Context) {
	utils.Success(c, services.GetPreferences(auth.UID(c)))
}

func UpdatePreferences(c *gin.Context) {
	var patch services.PreferencesPatch
	if err := c.ShouldBindJSON(&patch); err != nil {
		utils.BadRequest(c, "请求数据无效")
		return
	}
	prefs, err := services.SavePreferences(auth.UID(c), patch)
	if err != nil {
		utils.InternalError(c, "保存失败")
		return
	}
	services.InvalidateWeekPlan(auth.UID(c))
	utils.Success(c, prefs)
}

// ---------------- 管理员：用户管理 ----------------

func AdminListUsers(c *gin.Context) {
	page, pageSize := pageParams(c, 20)
	q := database.DB.Model(&models.User{})
	if s := strings.TrimSpace(c.Query("search")); s != "" {
		like := "%" + s + "%"
		q = q.Where("email LIKE ? OR nickname LIKE ? OR username LIKE ?", like, like, like)
	}
	var total int64
	q.Count(&total)
	var users []models.User
	q.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&users)

	ids := make([]uint, 0, len(users))
	for _, u := range users {
		ids = append(ids, u.ID)
	}
	type countRow struct {
		UserID uint
		N      int64
	}
	var rows []countRow
	if len(ids) > 0 {
		database.DB.Model(&models.MealRecord{}).Select("user_id, count(*) as n").Where("user_id IN ?", ids).Group("user_id").Scan(&rows)
	}
	counts := map[uint]int64{}
	for _, r := range rows {
		counts[r.UserID] = r.N
	}
	type item struct {
		userView
		RecordCount int64 `json:"record_count"`
	}
	items := make([]item, 0, len(users))
	for i := range users {
		items = append(items, item{userView: toUserView(&users[i]), RecordCount: counts[users[i].ID]})
	}
	utils.SuccessPaginated(c, items, total, page, pageSize)
}

func AdminUpdateUser(c *gin.Context) {
	var req struct {
		Role     *string `json:"role"`
		Disabled *bool   `json:"disabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请求数据无效")
		return
	}
	var u models.User
	if err := database.DB.First(&u, c.Param("id")).Error; err != nil {
		utils.NotFound(c, "用户不存在")
		return
	}
	if u.ID == auth.UID(c) {
		utils.BadRequest(c, "不能修改自己的角色或状态")
		return
	}
	updates := map[string]any{}
	if req.Role != nil {
		if *req.Role != models.RoleAdmin && *req.Role != models.RoleUser {
			utils.BadRequest(c, "角色无效")
			return
		}
		updates["role"] = *req.Role
	}
	if req.Disabled != nil {
		updates["disabled"] = *req.Disabled
		if *req.Disabled {
			updates["token_version"] = u.TokenVersion + 1
		}
	}
	if len(updates) > 0 {
		database.DB.Model(&u).Updates(updates)
	}
	database.DB.First(&u, u.ID)
	utils.Success(c, toUserView(&u))
}
