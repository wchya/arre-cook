package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"ninimenu/internal/config"
	"ninimenu/internal/database"
	"ninimenu/internal/dishes"
	"ninimenu/internal/models"
	"ninimenu/internal/services"
	"ninimenu/internal/utils"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// 智能体开放接口（/api/agent/*）
//
// 食谱站把菜单、用餐记录、收藏、评价、行为事件、口味画像与推荐引擎全部开放给外部智能体，
// 供其做食谱推荐与用户行为分析。凭证为 X-Agent-Token（见 AgentAuthMiddleware）。
// 返回结构与站内接口一致：{code, message, data}。

const agentAPIVersion = "1.0"

type agentEndpoint struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Desc   string `json:"desc"`
}

// GetAgentCapabilities 能力清单：让智能体自描述可用接口、枚举值与当前数据规模。
func GetAgentCapabilities(c *gin.Context) {
	var dishCount, enabledCount, recordCount, favoriteCount, eventCount int64
	database.DB.Model(&models.Dish{}).Count(&dishCount)
	database.DB.Model(&models.Dish{}).Where("enabled = ?", true).Count(&enabledCount)
	database.DB.Model(&models.MealRecord{}).Count(&recordCount)
	database.DB.Model(&models.Favorite{}).Count(&favoriteCount)
	database.DB.Model(&models.BehaviorEvent{}).Count(&eventCount)

	appName := "NiniMenu"
	var setting models.Setting
	if err := database.DB.Where("`key` = ?", "app_name").First(&setting).Error; err == nil && strings.TrimSpace(setting.Value) != "" {
		appName = strings.TrimSpace(setting.Value)
	}

	utils.Success(c, gin.H{
		"app_name":    appName,
		"api_version": agentAPIVersion,
		"auth":        gin.H{"header": "X-Agent-Token", "alt": "Authorization: Bearer <AGENT_TOKEN 或管理端 JWT>"},
		"categories":  dishes.DefaultCategories(),
		"tastes":      dishes.DefaultTastes(),
		"meal_types":  []string{"lunch", "dinner", "all"},
		"difficulty":  []string{"easy", "medium", "hard"},
		"moods":       []string{"happy", "tired", "lazy", "spicy", "healthy"},
		"record_moods": gin.H{
			"meal": []string{"yum", "ok", "no", "great", "meh"},
			"desc": "yum/great=好吃, ok=一般, no/meh=不想再吃",
		},
		"event_types": []string{"view", "recommend", "accept", "reject", "search", "chat", "feedback", "custom"},
		"repeat_days": config.C.RepeatDays,
		"counts": gin.H{
			"dishes":          dishCount,
			"enabled_dishes":  enabledCount,
			"meal_records":    recordCount,
			"favorites":       favoriteCount,
			"behavior_events": eventCount,
		},
		"endpoints": []agentEndpoint{
			{"GET", "/api/agent/capabilities", "能力清单（本接口）"},
			{"GET", "/api/agent/dishes", "菜品全量/筛选查询：category, taste, difficulty, meal_type, search, ingredient, exclude_ingredient, max_cook_time, exclude_recent=1, enabled, limit(≤500), offset"},
			{"GET", "/api/agent/dishes/:id", "菜品详情 + 该菜的用餐记录与统计"},
			{"GET", "/api/agent/profile?days=90", "口味画像（口味/菜系权重、常吃食材、喜恶、近期已吃、心情、行为统计）"},
			{"POST", "/api/agent/recommend", "推荐引擎：按画像 + 约束（餐段/心情/口味/食材/时长/去重）打分推荐，返回理由"},
			{"GET", "/api/agent/records", "用餐记录（含菜品口味/菜系与当日评价）：date_from, date_to, meal_type, limit"},
			{"POST", "/api/agent/records", "写入用餐记录（单条或 records 数组），可代用户“采纳推荐”"},
			{"DELETE", "/api/agent/records/:id", "删除用餐记录"},
			{"GET", "/api/agent/favorites", "收藏列表"},
			{"POST", "/api/agent/favorites/:dishId", "收藏"},
			{"DELETE", "/api/agent/favorites/:dishId", "取消收藏"},
			{"GET", "/api/agent/behavior", "行为事件流：type, source, dish_id, since, limit"},
			{"POST", "/api/agent/behavior", "写入行为事件（单条或 events 数组）：event_type, dish_id, dish_name, source, actor, meta"},
			{"GET", "/api/agent/day-ratings", "整餐评价与首页心情：date_from, date_to"},
			{"GET", "/api/agent/stats", "整体统计（记录数、热门菜、菜系分布、周趋势）"},
			{"GET", "/api/agent/week-plan", "本周菜单"},
			{"POST", "/api/agent/week-plan/regenerate", "重新生成本周菜单"},
			{"GET", "/api/agent/shopping-list", "今明两日买菜清单"},
			{"GET", "/api/agent/settings", "站点设置（分类、口味、去重天数等）"},
			{"GET", "/api/agent/export?days=365", "一次性导出全部分析所需数据（菜品、记录、收藏、评价、事件、画像）"},
		},
	})
}

func agentLimit(c *gin.Context, def, maxLimit int) int {
	limit, err := strconv.Atoi(c.DefaultQuery("limit", strconv.Itoa(def)))
	if err != nil || limit < 1 {
		limit = def
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	return limit
}

func agentBool(v string) bool {
	v = strings.ToLower(strings.TrimSpace(v))
	return v == "1" || v == "true" || v == "yes"
}

// GetAgentDishes 菜品查询：返回完整字段（含食材、调料、步骤），支持按食材筛选与近期去重。
func GetAgentDishes(c *gin.Context) {
	query := database.DB.Model(&models.Dish{})
	if enabled := c.Query("enabled"); enabled != "" {
		query = query.Where("enabled = ?", agentBool(enabled))
	} else {
		query = query.Where("enabled = ?", true)
	}
	if category := c.Query("category"); category != "" {
		query = query.Where("category IN ?", splitParam(category))
	}
	if difficulty := c.Query("difficulty"); difficulty != "" {
		query = query.Where("difficulty = ?", difficulty)
	}
	if mealType := c.Query("meal_type"); mealType != "" {
		query = query.Where("meal_type IN ?", []string{mealType, "all", ""})
	}
	if maxCook, err := strconv.Atoi(c.Query("max_cook_time")); err == nil && maxCook > 0 {
		query = query.Where("cook_time > 0 AND cook_time <= ?", maxCook)
	}
	if ids := c.Query("ids"); ids != "" {
		query = query.Where("id IN ?", splitParam(ids))
	}

	var all []models.Dish
	query.Order("sort_order ASC, id ASC").Find(&all)

	search := strings.ToLower(strings.TrimSpace(c.Query("search")))
	tastes := splitParam(c.Query("taste"))
	include := splitParam(c.Query("ingredient"))
	exclude := splitParam(c.Query("exclude_ingredient"))
	recent := map[uint]bool{}
	if agentBool(c.Query("exclude_recent")) {
		for _, id := range services.RecentDishIDs(config.C.RepeatDays) {
			recent[id] = true
		}
	}

	filtered := make([]models.Dish, 0, len(all))
	for _, d := range all {
		if recent[d.ID] {
			continue
		}
		text := agentDishText(d)
		if search != "" && !strings.Contains(text, search) {
			continue
		}
		if len(tastes) > 0 && !agentTasteMatch(d, tastes) {
			continue
		}
		if !agentContainsAll(text, include) || agentContainsAny(text, exclude) {
			continue
		}
		filtered = append(filtered, d)
	}

	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if offset < 0 {
		offset = 0
	}
	limit := agentLimit(c, 100, 500)
	total := len(filtered)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}

	utils.Success(c, gin.H{
		"items":  filtered[offset:end],
		"total":  total,
		"offset": offset,
		"limit":  limit,
	})
}

func splitParam(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '，' || r == '、' || r == '|' })
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func agentDishText(d models.Dish) string {
	parts := []string{d.Name, d.Category, d.Taste, d.Remark}
	var items []nameAmount
	if json.Unmarshal([]byte(d.Ingredients), &items) == nil {
		for _, it := range items {
			parts = append(parts, it.Name)
		}
	}
	items = nil
	if json.Unmarshal([]byte(d.Seasonings), &items) == nil {
		for _, it := range items {
			parts = append(parts, it.Name)
		}
	}
	var tags []string
	if json.Unmarshal([]byte(d.Tags), &tags) == nil {
		parts = append(parts, tags...)
	}
	return strings.ToLower(strings.Join(parts, " "))
}

func agentTasteMatch(d models.Dish, tastes []string) bool {
	for _, t := range tastes {
		if strings.Contains(d.Taste, t) || strings.Contains(d.Name, t) {
			return true
		}
	}
	return false
}

func agentContainsAll(text string, needles []string) bool {
	for _, n := range needles {
		if !strings.Contains(text, strings.ToLower(n)) {
			return false
		}
	}
	return true
}

func agentContainsAny(text string, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(text, strings.ToLower(n)) {
			return true
		}
	}
	return false
}

// GetAgentDish 菜品详情 + 记录统计。
func GetAgentDish(c *gin.Context) {
	id := c.Param("id")
	var dish models.Dish
	if err := database.DB.First(&dish, id).Error; err != nil {
		utils.NotFound(c, "菜品不存在")
		return
	}

	var records []models.MealRecord
	database.DB.Where("dish_id = ?", dish.ID).Order("meal_date DESC, created_at DESC").Limit(50).Find(&records)

	yum, ok, no, ratingSum, ratingN := 0, 0, 0, 0, 0
	for _, r := range records {
		switch r.Mood {
		case "yum", "great":
			yum++
		case "ok":
			ok++
		case "no", "meh":
			no++
		}
		if r.Rating > 0 {
			ratingSum += r.Rating
			ratingN++
		}
	}
	avg := 0.0
	if ratingN > 0 {
		avg = float64(ratingSum) / float64(ratingN)
	}
	lastDate := ""
	if len(records) > 0 {
		lastDate = records[0].MealDate
	}

	var isFav int64
	database.DB.Model(&models.Favorite{}).Where("dish_id = ?", dish.ID).Count(&isFav)

	var viewCount int64
	database.DB.Model(&models.BehaviorEvent{}).Where("dish_id = ? AND event_type = ?", dish.ID, "view").Count(&viewCount)

	utils.Success(c, gin.H{
		"dish":     dish,
		"records":  records,
		"favorite": isFav > 0,
		"stats": gin.H{
			"total_count": len(records),
			"yum_count":   yum,
			"ok_count":    ok,
			"no_count":    no,
			"avg_rating":  avg,
			"last_date":   lastDate,
			"view_count":  viewCount,
		},
	})
}

// GetAgentProfile 口味画像。
func GetAgentProfile(c *gin.Context) {
	days, _ := strconv.Atoi(c.DefaultQuery("days", "90"))
	utils.Success(c, services.BuildTasteProfile(days))
}

// AgentRecommend 推荐引擎。
func AgentRecommend(c *gin.Context) {
	var req services.RecommendRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		utils.BadRequest(c, "请求数据无效: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Source) == "" {
		req.Source = "agent"
	}
	result, err := services.RecommendDishes(req)
	if err != nil {
		utils.InternalError(c, "推荐失败")
		return
	}
	utils.Success(c, result)
}

type agentRecordItem struct {
	models.MealRecord
	Category string `json:"category"`
	Taste    string `json:"taste"`
	CookTime int    `json:"cook_time"`
	HomeMood string `json:"home_mood"`
	DayMood  string `json:"day_mood"`
}

// GetAgentRecords 用餐记录（带菜品口味/菜系、当日心情），默认最近 90 天。
func GetAgentRecords(c *gin.Context) {
	query := database.DB.Model(&models.MealRecord{})
	dateFrom := c.Query("date_from")
	if dateFrom == "" && c.Query("date_to") == "" {
		dateFrom = time.Now().AddDate(0, 0, -90).Format("2006-01-02")
	}
	if dateFrom != "" {
		query = query.Where("meal_date >= ?", dateFrom)
	}
	if dateTo := c.Query("date_to"); dateTo != "" {
		query = query.Where("meal_date <= ?", dateTo)
	}
	if mealType := c.Query("meal_type"); mealType != "" {
		query = query.Where("meal_type = ?", mealType)
	}
	if dishID := c.Query("dish_id"); dishID != "" {
		query = query.Where("dish_id = ?", dishID)
	}

	var total int64
	query.Count(&total)
	var records []models.MealRecord
	query.Order("meal_date DESC, created_at DESC").Limit(agentLimit(c, 200, 2000)).Find(&records)

	dishIDs := make([]uint, 0, len(records))
	dates := make([]string, 0, len(records))
	seenID, seenDate := map[uint]bool{}, map[string]bool{}
	for _, r := range records {
		if !seenID[r.DishID] {
			seenID[r.DishID] = true
			dishIDs = append(dishIDs, r.DishID)
		}
		if !seenDate[r.MealDate] {
			seenDate[r.MealDate] = true
			dates = append(dates, r.MealDate)
		}
	}
	dishMap := map[uint]models.Dish{}
	if len(dishIDs) > 0 {
		var list []models.Dish
		database.DB.Unscoped().Select("id", "category", "taste", "cook_time").Where("id IN ?", dishIDs).Find(&list)
		for _, d := range list {
			dishMap[d.ID] = d
		}
	}
	ratingMap := map[string]models.DayRating{}
	if len(dates) > 0 {
		var ratings []models.DayRating
		database.DB.Where("meal_date IN ?", dates).Find(&ratings)
		for _, r := range ratings {
			ratingMap[r.MealDate] = r
		}
	}

	items := make([]agentRecordItem, 0, len(records))
	for _, r := range records {
		it := agentRecordItem{MealRecord: r}
		if d, ok := dishMap[r.DishID]; ok {
			it.Category, it.Taste, it.CookTime = d.Category, d.Taste, d.CookTime
		}
		if dr, ok := ratingMap[r.MealDate]; ok {
			it.HomeMood, it.DayMood = dr.HomeMood, dr.Mood
		}
		items = append(items, it)
	}

	utils.Success(c, gin.H{"items": items, "total": total})
}

type agentRecordInput struct {
	DishID   uint   `json:"dish_id"`
	DishName string `json:"dish_name"`
	MealType string `json:"meal_type"`
	MealDate string `json:"meal_date"`
	Rating   int    `json:"rating"`
	Remark   string `json:"remark"`
	Mood     string `json:"mood"`
	Photo    string `json:"photo"`
}

type agentRecordRequest struct {
	agentRecordInput
	Source  string             `json:"source"`
	Actor   string             `json:"actor"`
	Records []agentRecordInput `json:"records"`
}

// CreateAgentRecords 写入用餐记录（单条或 records 数组）。每条成功写入都会记一条 accept 行为事件。
func CreateAgentRecords(c *gin.Context) {
	var req agentRecordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请求数据无效: "+err.Error())
		return
	}

	list := req.Records
	if len(list) == 0 {
		list = []agentRecordInput{req.agentRecordInput}
	}
	source := strings.TrimSpace(req.Source)
	if source == "" {
		source = "agent"
	}

	created := make([]models.MealRecord, 0, len(list))
	skipped := 0
	for _, r := range list {
		if r.DishID == 0 || (r.MealType != "lunch" && r.MealType != "dinner") {
			skipped++
			continue
		}
		if r.MealDate == "" {
			r.MealDate = time.Now().Format("2006-01-02")
		}
		var dish models.Dish
		if err := database.DB.First(&dish, r.DishID).Error; err != nil {
			skipped++
			continue
		}
		if r.DishName == "" {
			r.DishName = dish.Name
		}
		var existing int64
		database.DB.Model(&models.MealRecord{}).
			Where("dish_id = ? AND meal_type = ? AND meal_date = ?", r.DishID, r.MealType, r.MealDate).
			Count(&existing)
		if existing > 0 {
			skipped++
			continue
		}
		record := models.MealRecord{
			DishID: r.DishID, DishName: r.DishName, MealType: r.MealType, MealDate: r.MealDate,
			Rating: r.Rating, Remark: r.Remark, Mood: r.Mood, Photo: r.Photo,
		}
		if err := database.DB.Create(&record).Error; err != nil {
			skipped++
			continue
		}
		addShoppingItems(r.DishID, r.DishName, r.MealType, r.MealDate)
		created = append(created, record)
		logBehavior("accept", r.DishID, r.DishName, source, req.Actor, map[string]any{
			"meal_type": r.MealType, "meal_date": r.MealDate,
		})
	}
	if len(created) > 0 {
		services.QueueAutoAchievementSync()
	}
	utils.Success(c, gin.H{"created": created, "skipped": skipped, "total": len(list)})
}

// DeleteAgentRecord 删除用餐记录。
func DeleteAgentRecord(c *gin.Context) {
	DeleteRecord(c)
}

// GetAgentFavorites 收藏列表。
func GetAgentFavorites(c *gin.Context) {
	entries, err := loadFavoriteDishEntries()
	if err != nil {
		utils.InternalError(c, "获取收藏失败")
		return
	}
	items := make([]gin.H, 0, len(entries))
	for _, e := range entries {
		items = append(items, gin.H{"dish": e.Dish, "favorited_at": e.Favorite.CreatedAt})
	}
	utils.Success(c, items)
}

type agentBehaviorRequest struct {
	EventType string         `json:"event_type"`
	DishID    uint           `json:"dish_id"`
	DishName  string         `json:"dish_name"`
	Source    string         `json:"source"`
	Actor     string         `json:"actor"`
	Meta      map[string]any `json:"meta"`
}

func logBehavior(eventType string, dishID uint, dishName, source, actor string, meta map[string]any) {
	eventType = strings.TrimSpace(eventType)
	if eventType == "" {
		return
	}
	if dishName == "" && dishID > 0 {
		var d models.Dish
		if err := database.DB.Unscoped().Select("name").First(&d, dishID).Error; err == nil {
			dishName = d.Name
		}
	}
	metaJSON := "{}"
	if meta != nil {
		if b, err := json.Marshal(meta); err == nil {
			metaJSON = string(b)
		}
	}
	if source == "" {
		source = "app"
	}
	database.DB.Create(&models.BehaviorEvent{
		EventType: eventType, DishID: dishID, DishName: dishName,
		Source: strings.TrimSpace(source), Actor: strings.TrimSpace(actor), Meta: metaJSON,
	})
}

// CreateAgentBehavior 写入行为事件（单条或 events 数组）。
func CreateAgentBehavior(c *gin.Context) {
	var body struct {
		agentBehaviorRequest
		Events []agentBehaviorRequest `json:"events"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		utils.BadRequest(c, "请求数据无效")
		return
	}
	list := body.Events
	if len(list) == 0 {
		list = []agentBehaviorRequest{body.agentBehaviorRequest}
	}
	saved := 0
	for _, e := range list {
		if strings.TrimSpace(e.EventType) == "" {
			continue
		}
		source := e.Source
		if source == "" {
			source = "agent"
		}
		logBehavior(e.EventType, e.DishID, e.DishName, source, e.Actor, e.Meta)
		saved++
	}
	utils.Success(c, gin.H{"saved": saved})
}

// CreateAppBehavior 站内前端埋点（应用密码即可）：浏览菜品、采纳/拒绝推荐等。
func CreateAppBehavior(c *gin.Context) {
	var e agentBehaviorRequest
	if err := c.ShouldBindJSON(&e); err != nil || strings.TrimSpace(e.EventType) == "" {
		utils.BadRequest(c, "请求数据无效")
		return
	}
	if e.Source == "" {
		e.Source = "app"
	}
	logBehavior(e.EventType, e.DishID, e.DishName, e.Source, e.Actor, e.Meta)
	utils.Success(c, nil)
}

// GetAgentBehavior 行为事件流。
func GetAgentBehavior(c *gin.Context) {
	query := database.DB.Model(&models.BehaviorEvent{})
	if t := c.Query("type"); t != "" {
		query = query.Where("event_type IN ?", splitParam(t))
	}
	if s := c.Query("source"); s != "" {
		query = query.Where("source = ?", s)
	}
	if id := c.Query("dish_id"); id != "" {
		query = query.Where("dish_id = ?", id)
	}
	if since := c.Query("since"); since != "" {
		if t, err := time.Parse("2006-01-02", since); err == nil {
			query = query.Where("created_at >= ?", t)
		} else if t, err := time.Parse(time.RFC3339, since); err == nil {
			query = query.Where("created_at >= ?", t)
		}
	}
	var total int64
	query.Count(&total)
	var events []models.BehaviorEvent
	query.Order("created_at DESC, id DESC").Limit(agentLimit(c, 200, 2000)).Find(&events)

	type eventOut struct {
		models.BehaviorEvent
		MetaJSON json.RawMessage `json:"meta"`
	}
	items := make([]eventOut, 0, len(events))
	for _, e := range events {
		meta := json.RawMessage(e.Meta)
		if !json.Valid(meta) {
			meta = json.RawMessage("{}")
		}
		items = append(items, eventOut{BehaviorEvent: e, MetaJSON: meta})
	}
	utils.Success(c, gin.H{"items": items, "total": total})
}

// GetAgentDayRatings 整餐评价与首页心情。
func GetAgentDayRatings(c *gin.Context) {
	query := database.DB.Model(&models.DayRating{})
	if from := c.Query("date_from"); from != "" {
		query = query.Where("meal_date >= ?", from)
	} else {
		query = query.Where("meal_date >= ?", time.Now().AddDate(0, 0, -90).Format("2006-01-02"))
	}
	if to := c.Query("date_to"); to != "" {
		query = query.Where("meal_date <= ?", to)
	}
	var ratings []models.DayRating
	query.Order("meal_date DESC").Limit(agentLimit(c, 200, 1000)).Find(&ratings)
	utils.Success(c, ratings)
}

// GetAgentStats 整体统计，复用管理端仪表盘。
func GetAgentStats(c *gin.Context) {
	GetDashboard(c)
}

// GetAgentSettings 站点设置（复用应用端设置接口，已过滤密码类键）。
func GetAgentSettings(c *gin.Context) {
	GetSettings(c)
}

// GetAgentExport 一次性导出全部分析数据。
func GetAgentExport(c *gin.Context) {
	days, _ := strconv.Atoi(c.DefaultQuery("days", "365"))
	if days <= 0 {
		days = 365
	}
	since := time.Now().AddDate(0, 0, -days)
	sinceDate := since.Format("2006-01-02")

	var dishList []models.Dish
	database.DB.Order("sort_order ASC, id ASC").Find(&dishList)
	var records []models.MealRecord
	database.DB.Where("meal_date >= ?", sinceDate).Order("meal_date ASC").Find(&records)
	var favorites []models.Favorite
	database.DB.Find(&favorites)
	var ratings []models.DayRating
	database.DB.Where("meal_date >= ?", sinceDate).Order("meal_date ASC").Find(&ratings)
	var events []models.BehaviorEvent
	database.DB.Where("created_at >= ?", since).Order("created_at ASC").Limit(5000).Find(&events)

	utils.Success(c, gin.H{
		"exported_at":     time.Now().Format(time.RFC3339),
		"window_days":     days,
		"dishes":          dishList,
		"meal_records":    records,
		"favorites":       favorites,
		"day_ratings":     ratings,
		"behavior_events": events,
		"profile":         services.BuildTasteProfile(days),
	})
}
