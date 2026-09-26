package handlers

import (
	"context"
	"encoding/json"
	"ninimenu/internal/config"
	"ninimenu/internal/database"
	"ninimenu/internal/llm"
	"ninimenu/internal/models"
	"ninimenu/internal/services"
	"ninimenu/internal/utils"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ---------------- 语录（站点级） ----------------

func GetQuotes(c *gin.Context) {
	var quotes []models.Quote
	database.DB.Order("created_at DESC").Find(&quotes)
	utils.Success(c, quotes)
}

type CreateQuoteRequest struct {
	Content string `json:"content" binding:"required"`
	Scene   string `json:"scene"`
}

func CreateQuote(c *gin.Context) {
	var req CreateQuoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "推荐语内容不能为空")
		return
	}
	quote := models.Quote{Content: req.Content, Scene: req.Scene, Enabled: true}
	database.DB.Create(&quote)
	utils.Success(c, quote)
}

func UpdateQuote(c *gin.Context) {
	var quote models.Quote
	if err := database.DB.First(&quote, c.Param("id")).Error; err != nil {
		utils.NotFound(c, "推荐语不存在")
		return
	}
	var req CreateQuoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请求数据无效")
		return
	}
	quote.Content = req.Content
	quote.Scene = req.Scene
	database.DB.Save(&quote)
	utils.Success(c, quote)
}

func DeleteQuote(c *gin.Context) {
	database.DB.Delete(&models.Quote{}, c.Param("id"))
	utils.SuccessMsg(c, "删除成功")
}

// ---------------- 成就（定义站点级，解锁按用户） ----------------

func GetAchievements(c *gin.Context) {
	u := uid(c)
	services.SyncAutoAchievements(u)

	var achievements []models.Achievement
	database.DB.Order("id ASC").Find(&achievements)

	var unlocked []models.UserAchievement
	database.DB.Scopes(database.OwnedBy(u)).Find(&unlocked)
	unlockedAtMap := make(map[uint]time.Time, len(unlocked))
	for _, ua := range unlocked {
		unlockedAtMap[ua.AchievementID] = ua.UnlockedAt
	}

	type AchievementWithStatus struct {
		models.Achievement
		IsUnlocked bool `json:"is_unlocked"`
	}
	result := make([]AchievementWithStatus, 0, len(achievements))
	for _, a := range achievements {
		at, ok := unlockedAtMap[a.ID]
		a.UnlockedAt = nil
		if ok {
			a.UnlockedAt = &at
		}
		result = append(result, AchievementWithStatus{Achievement: a, IsUnlocked: ok})
	}
	utils.Success(c, result)
}

// UnlockAchievement 手动成就（condition=manual）由用户自己点亮；自动成就不允许手动解锁。
func UnlockAchievement(c *gin.Context) {
	var achievement models.Achievement
	if err := database.DB.First(&achievement, c.Param("id")).Error; err != nil {
		utils.NotFound(c, "成就不存在")
		return
	}
	if achievement.Condition == "auto" && !isAdmin(c) {
		utils.BadRequest(c, "这个成就会在达成条件后自动解锁")
		return
	}
	var n int64
	database.DB.Model(&models.UserAchievement{}).Scopes(database.OwnedBy(uid(c))).Where("achievement_id = ?", achievement.ID).Count(&n)
	if n > 0 {
		utils.SuccessMsg(c, "已解锁")
		return
	}
	database.DB.Create(&models.UserAchievement{UserID: uid(c), AchievementID: achievement.ID, UnlockedAt: time.Now()})
	utils.SuccessMsg(c, "解锁成功")
}

func ToggleAchievement(c *gin.Context) {
	var achievement models.Achievement
	if err := database.DB.First(&achievement, c.Param("id")).Error; err != nil {
		utils.NotFound(c, "成就不存在")
		return
	}
	if achievement.Condition == "auto" && !isAdmin(c) {
		utils.BadRequest(c, "自动成就不能手动切换")
		return
	}
	var ua models.UserAchievement
	if err := database.DB.Scopes(database.OwnedBy(uid(c))).Where("achievement_id = ?", achievement.ID).First(&ua).Error; err == nil {
		database.DB.Delete(&ua)
		utils.SuccessMsg(c, "已关闭")
		return
	}
	database.DB.Create(&models.UserAchievement{UserID: uid(c), AchievementID: achievement.ID, UnlockedAt: time.Now()})
	utils.SuccessMsg(c, "已激活")
}

type CreateAchievementRequest struct {
	Code        string `json:"code" binding:"required"`
	Name        string `json:"name" binding:"required"`
	Description string `json:"description" binding:"required"`
	Icon        string `json:"icon"`
	Condition   string `json:"condition"`
}

func CreateAchievement(c *gin.Context) {
	var req CreateAchievementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "成就名称和描述不能为空")
		return
	}
	if req.Condition == "" {
		req.Condition = "manual"
	}
	achievement := models.Achievement{Code: req.Code, Name: req.Name, Description: req.Description, Icon: req.Icon, Condition: req.Condition}
	if err := database.DB.Create(&achievement).Error; err != nil {
		utils.BadRequest(c, "成就编码已存在")
		return
	}
	utils.Success(c, achievement)
}

func UpdateAchievement(c *gin.Context) {
	var achievement models.Achievement
	if err := database.DB.First(&achievement, c.Param("id")).Error; err != nil {
		utils.NotFound(c, "成就不存在")
		return
	}
	var req CreateAchievementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请求数据无效")
		return
	}
	achievement.Code, achievement.Name, achievement.Description, achievement.Icon = req.Code, req.Name, req.Description, req.Icon
	achievement.Condition = req.Condition
	if achievement.Condition == "" {
		achievement.Condition = "manual"
	}
	database.DB.Save(&achievement)
	utils.Success(c, achievement)
}

func DeleteAchievement(c *gin.Context) {
	id := c.Param("id")
	database.DB.Where("achievement_id = ?", id).Delete(&models.UserAchievement{})
	database.DB.Delete(&models.Achievement{}, id)
	utils.SuccessMsg(c, "删除成功")
}

// ---------------- 设置 ----------------

// 站点级设置（管理员可改）；secret 键永不回显。
var siteSettingKeys = map[string]bool{
	"app_name": true, "categories": true, "tastes": true, "repeat_days": true, "pick_animation": true,
	"lunch_dishes_per_day": true, "dinner_dishes_per_day": true, "agent_embed_url": true,
	"allow_register": true, "llm_base_url": true, "llm_model": true, "llm_api_key": true,
	"announcement": true,
}

var secretSettingKeys = map[string]bool{"llm_api_key": true}

// 用户级设置（每个用户自己的偏好开关）。
var userSettingKeys = map[string]bool{
	"voice_enabled": true, "blind_box_enabled": true, "repeat_days": true,
	"lunch_dishes_per_day": true, "dinner_dishes_per_day": true, "shopping_reminder": true,
}

func GetAppInfo(c *gin.Context) {
	llmOn := llm.Resolve().Enabled()
	utils.Success(c, gin.H{
		"app_name":        database.GetSetting("app_name", "NiniMenu"),
		"agent_embed_url": database.GetSetting("agent_embed_url", ""),
		"announcement":    database.GetSetting("announcement", ""),
		"ai_enabled":      llmOn,
		"wechat_login":    config.C.WechatEnabled(),
		"register_open":   services.RegistrationOpen(),
	})
}

func publicSiteSettings() map[string]any {
	var settings []models.Setting
	database.DB.Where("`key` IN ?", keysOf(siteSettingKeys)).Find(&settings)
	m := make(map[string]any)
	for _, s := range settings {
		if secretSettingKeys[s.Key] {
			m[s.Key+"_set"] = strings.TrimSpace(s.Value) != ""
			continue
		}
		v := strings.TrimSpace(s.Value)
		if (strings.HasPrefix(v, "[") || strings.HasPrefix(v, "{")) && json.Valid([]byte(v)) {
			m[s.Key] = json.RawMessage(v)
		} else {
			m[s.Key] = s.Value
		}
	}
	return m
}

// GetSettings 合并视图：站点设置 + 当前用户的个人设置（个人覆盖站点）。
func GetSettings(c *gin.Context) {
	m := publicSiteSettings()
	delete(m, "llm_api_key_set")
	delete(m, "llm_base_url")
	delete(m, "llm_model")
	m["voice_enabled"] = "1"
	m["blind_box_enabled"] = "1"
	m["shopping_reminder"] = "1"
	var mine []models.UserSetting
	database.DB.Scopes(database.OwnedBy(uid(c))).Where("`key` IN ?", keysOf(userSettingKeys)).Find(&mine)
	for _, s := range mine {
		if strings.TrimSpace(s.Value) != "" { // 空值表示“跟随站点默认”
			m[s.Key] = s.Value
		}
	}
	if _, ok := m["repeat_days"]; !ok {
		m["repeat_days"] = config.C.RepeatDays
	}
	utils.Success(c, m)
}

type UpdateSettingsRequest struct {
	Settings map[string]string `json:"settings" binding:"required"`
}

// UpdateSettings 修改自己的个人设置。
func UpdateSettings(c *gin.Context) {
	var req UpdateSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请提供设置数据")
		return
	}
	changedPlan := false
	for key, value := range req.Settings {
		if !userSettingKeys[key] {
			continue
		}
		_ = database.SetUserSetting(uid(c), key, strings.TrimSpace(value))
		if key == "lunch_dishes_per_day" || key == "dinner_dishes_per_day" {
			changedPlan = true
		}
	}
	if changedPlan {
		services.InvalidateWeekPlan(uid(c))
	}
	utils.SuccessMsg(c, "更新成功")
}

// GetSiteSettings / UpdateSiteSettings 管理员维护站点设置。
func GetSiteSettings(c *gin.Context) {
	m := publicSiteSettings()
	s := llm.Resolve()
	m["llm_effective"] = gin.H{"base_url": s.BaseURL, "model": s.Model, "enabled": s.Enabled(), "from_env": database.GetSetting("llm_api_key", "") == "" && config.C.LLMAPIKey != ""}
	m["smtp_enabled"] = config.C.SMTPEnabled()
	m["wechat_enabled"] = config.C.WechatEnabled()
	utils.Success(c, m)
}

func UpdateSiteSettings(c *gin.Context) {
	var req UpdateSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请提供设置数据")
		return
	}
	for key, value := range req.Settings {
		if !siteSettingKeys[key] {
			continue
		}
		value = strings.TrimSpace(value)
		// 密钥类字段：前端留空表示“不修改”，传 "-" 表示清除
		if secretSettingKeys[key] {
			if value == "" {
				continue
			}
			if value == "-" {
				value = ""
			}
		}
		if key == "agent_embed_url" && value != "" && !strings.HasPrefix(value, "https://") && !strings.HasPrefix(value, "http://localhost") {
			utils.BadRequest(c, "AI 助手嵌入地址必须是 https 地址")
			return
		}
		if err := database.SetSetting(key, value); err != nil {
			utils.InternalError(c, "保存失败")
			return
		}
	}
	utils.SuccessMsg(c, "更新成功")
}

// TestLLM 管理员测试大模型连接。
func TestLLM(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	s := llm.Resolve()
	if !s.Enabled() {
		utils.BadRequest(c, "请先填写 API Key")
		return
	}
	if err := llm.Ping(ctx, s); err != nil {
		utils.BadRequest(c, "连接失败："+err.Error())
		return
	}
	utils.SuccessMsg(c, "连接成功："+s.Model)
}

func keysOf(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// ---------------- 管理员仪表盘（全站） ----------------

func GetDashboard(c *gin.Context) {
	db := database.DB
	var totalDishes, enabledDishes, disabledDishes, privateDishes, totalRecords, todayRecords, favoriteCount int64
	var userCount, activeUsers7d, newUsers7d, agentTokens, chatMessages7d, pendingSuggestions int64
	db.Model(&models.Dish{}).Where("owner_id = 0").Count(&totalDishes)
	db.Model(&models.Dish{}).Where("owner_id = 0 AND enabled = ?", true).Count(&enabledDishes)
	db.Model(&models.Dish{}).Where("owner_id = 0 AND enabled = ?", false).Count(&disabledDishes)
	db.Model(&models.Dish{}).Where("owner_id <> 0").Count(&privateDishes)
	db.Model(&models.MealRecord{}).Count(&totalRecords)
	db.Model(&models.MealRecord{}).Where("meal_date = ?", todayStr()).Count(&todayRecords)
	db.Model(&models.Favorite{}).Count(&favoriteCount)
	weekAgo := time.Now().AddDate(0, 0, -7)
	db.Model(&models.User{}).Count(&userCount)
	db.Model(&models.User{}).Where("last_login_at >= ?", weekAgo).Count(&activeUsers7d)
	db.Model(&models.User{}).Where("created_at >= ?", weekAgo).Count(&newUsers7d)
	db.Model(&models.AgentToken{}).Where("revoked_at IS NULL").Count(&agentTokens)
	db.Model(&models.ChatMessage{}).Where("role = ? AND created_at >= ?", "user", weekAgo).Count(&chatMessages7d)
	db.Model(&models.AgentSuggestion{}).Where("status = ?", "pending").Count(&pendingSuggestions)

	var categoryCounts []CategoryCount
	db.Model(&models.Dish{}).Select("category, count(*) as count").Where("owner_id = 0 AND enabled = ?", true).Group("category").Order("count DESC").Find(&categoryCounts)

	var topDishes []services.TopDishCount
	db.Model(&models.MealRecord{}).Select("dish_id, dish_name, count(*) as count").Group("dish_id, dish_name").Order("count DESC").Limit(8).Find(&topDishes)

	type RecentRecord struct {
		ID       uint   `json:"id"`
		DishID   uint   `json:"dish_id"`
		DishName string `json:"dish_name"`
		MealType string `json:"meal_type"`
		MealDate string `json:"meal_date"`
		Mood     string `json:"mood"`
		Rating   int    `json:"rating"`
		UserName string `json:"user_name"`
	}
	var recentRecords []RecentRecord
	db.Table("meal_records AS r").
		Select("r.id, r.dish_id, r.dish_name, r.meal_type, r.meal_date, r.mood, r.rating, COALESCE(NULLIF(u.nickname, ''), u.email, u.username) AS user_name").
		Joins("LEFT JOIN users u ON u.id = r.user_id").
		Order("r.meal_date DESC, r.created_at DESC").Limit(10).Scan(&recentRecords)

	since := time.Now().AddDate(0, 0, -6).Format("2006-01-02")
	var trendRows []services.DayCount
	db.Model(&models.MealRecord{}).Select("meal_date AS date, count(*) AS count").Where("meal_date >= ?", since).Group("meal_date").Scan(&trendRows)
	byDate := map[string]int64{}
	for _, t := range trendRows {
		byDate[t.Date] = t.Count
	}
	weekTrend := make([]services.DayCount, 0, 7)
	for i := 6; i >= 0; i-- {
		d := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
		weekTrend = append(weekTrend, services.DayCount{Date: d, Count: byDate[d]})
	}

	type DifficultyCount struct {
		Difficulty string `json:"difficulty"`
		Count      int64  `json:"count"`
	}
	var difficultyCounts []DifficultyCount
	db.Model(&models.Dish{}).Select("difficulty, count(*) as count").Where("owner_id = 0 AND enabled = ?", true).Group("difficulty").Find(&difficultyCounts)

	utils.Success(c, gin.H{
		"total_dishes":        totalDishes,
		"enabled_dishes":      enabledDishes,
		"disabled_dishes":     disabledDishes,
		"private_dishes":      privateDishes,
		"total_records":       totalRecords,
		"today_records":       todayRecords,
		"favorite_count":      favoriteCount,
		"user_count":          userCount,
		"active_users_7d":     activeUsers7d,
		"new_users_7d":        newUsers7d,
		"agent_tokens":        agentTokens,
		"chat_messages_7d":    chatMessages7d,
		"pending_suggestions": pendingSuggestions,
		"category_counts":     categoryCounts,
		"top_dishes":          topDishes,
		"recent_records":      recentRecords,
		"week_trend":          weekTrend,
		"difficulty_counts":   difficultyCounts,
		"ai_enabled":          llm.Resolve().Enabled(),
		"smtp_enabled":        config.C.SMTPEnabled(),
	})
}
