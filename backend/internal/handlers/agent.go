package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"ninimenu/internal/agent"
	"ninimenu/internal/auth"
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
// 所有凭证都归属到某一个用户：个人访问令牌（nm_…，用户在「我的 → AI 连接」创建）、
// 嵌入式会话令牌（App 通过 postMessage 交给内嵌智能体），或用户本人的登录态。
// 数据只落在该用户上，并受令牌 scopes 限制。工具定义见 internal/agent，与 MCP、站内助手共用。

const agentAPIVersion = "2.0"

func principal(c *gin.Context) *auth.Principal { return auth.GetPrincipal(c) }

func toolCtx(c *gin.Context, channel string) *agent.Ctx {
	return &agent.Ctx{Context: c.Request.Context(), Principal: principal(c), Channel: channel}
}

// invokeTool REST 形式调用工具并按统一结构返回。
func invokeTool(c *gin.Context, name string, args any) {
	var raw json.RawMessage
	switch v := args.(type) {
	case nil:
		raw = json.RawMessage("{}")
	case json.RawMessage:
		raw = v
	default:
		b, _ := json.Marshal(v)
		raw = b
	}
	result, err := agent.Invoke(toolCtx(c, "rest"), name, raw)
	if err != nil {
		toolError(c, err)
		return
	}
	utils.Success(c, result)
}

func toolError(c *gin.Context, err error) {
	var forbidden agent.ErrForbidden
	switch {
	case errors.As(err, &forbidden):
		utils.Forbidden(c, err.Error())
	case errors.Is(err, agent.ErrUnknownTool):
		utils.NotFound(c, err.Error())
	case errors.Is(err, services.ErrDishNotFound), errors.Is(err, services.ErrRecordNotFound):
		utils.NotFound(c, err.Error())
	default:
		utils.BadRequest(c, err.Error())
	}
}

func baseURL(c *gin.Context) string {
	if config.C.PublicURL != "" {
		return config.C.PublicURL
	}
	scheme := "http"
	if c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	return scheme + "://" + c.Request.Host
}

// GetAgentCapabilities 能力清单：调用者身份、已授权范围、可用工具、接入方式。
func GetAgentCapabilities(c *gin.Context) {
	p := principal(c)
	base := baseURL(c)
	utils.Success(c, gin.H{
		"app_name":    database.GetSetting("app_name", "NiniMenu"),
		"api_version": agentAPIVersion,
		"user":        gin.H{"id": p.UserID(), "nickname": p.User.DisplayName()},
		"credential":  gin.H{"kind": p.Kind, "actor": p.Actor, "scopes": p.Scopes.List()},
		"auth": gin.H{
			"header": "Authorization: Bearer <个人访问令牌 nm_… 或嵌入会话令牌>",
			"alt":    "X-Agent-Token: <令牌>",
			"note":   "令牌只能访问签发者本人的数据，由用户在 App「我的 → AI 连接」创建与撤销",
		},
		"integrations": gin.H{
			"mcp":              gin.H{"transport": "streamable-http", "url": base + "/mcp"},
			"function_calling": gin.H{"tools": base + "/api/agent/tools", "invoke": base + "/api/agent/tools/{name}"},
			"openapi":          base + "/api/agent/openapi.json",
		},
		"enums": gin.H{
			"categories":   dishes.DefaultCategories(),
			"tastes":       dishes.DefaultTastes(),
			"meal_types":   []string{"lunch", "dinner"},
			"difficulty":   []string{"easy", "medium", "hard"},
			"moods":        []string{"happy", "tired", "lazy", "spicy", "healthy"},
			"record_moods": gin.H{"values": []string{"yum", "ok", "no"}, "desc": "yum=好吃, ok=一般, no=不想再吃"},
			"event_types":  []string{"view", "recommend", "accept", "reject", "search", "chat", "feedback", "custom"},
			"scopes":       auth.ScopeLabels,
		},
		"tools": agent.Catalog(p),
		"rest_endpoints": []gin.H{
			{"method": "GET", "path": "/api/agent/me", "scope": ""},
			{"method": "GET", "path": "/api/agent/dishes", "scope": auth.ScopeDishesRead},
			{"method": "GET", "path": "/api/agent/dishes/:id", "scope": auth.ScopeDishesRead},
			{"method": "GET", "path": "/api/agent/profile", "scope": auth.ScopeProfileRead},
			{"method": "GET|PUT", "path": "/api/agent/preferences", "scope": auth.ScopeProfileRead + " / " + auth.ScopePreferencesWrite},
			{"method": "POST", "path": "/api/agent/recommend", "scope": auth.ScopeDishesRead + " + " + auth.ScopeProfileRead},
			{"method": "GET|POST", "path": "/api/agent/records", "scope": auth.ScopeRecordsRead + " / " + auth.ScopeRecordsWrite},
			{"method": "DELETE", "path": "/api/agent/records/:id", "scope": auth.ScopeRecordsWrite},
			{"method": "GET", "path": "/api/agent/favorites", "scope": auth.ScopeRecordsRead},
			{"method": "POST|DELETE", "path": "/api/agent/favorites/:dishId", "scope": auth.ScopeFavoritesWrite},
			{"method": "GET|POST", "path": "/api/agent/behavior", "scope": auth.ScopeRecordsRead + " / " + auth.ScopeBehaviorWrite},
			{"method": "GET|POST", "path": "/api/agent/suggestions", "scope": auth.ScopeRecordsRead + " / " + auth.ScopeSuggestionsWrite},
			{"method": "GET", "path": "/api/agent/day-ratings", "scope": auth.ScopeRecordsRead},
			{"method": "GET", "path": "/api/agent/stats", "scope": auth.ScopeProfileRead},
			{"method": "GET", "path": "/api/agent/week-plan", "scope": auth.ScopeRecordsRead},
			{"method": "POST", "path": "/api/agent/week-plan/regenerate", "scope": auth.ScopePlanWrite},
			{"method": "GET", "path": "/api/agent/shopping-list", "scope": auth.ScopeRecordsRead},
			{"method": "GET", "path": "/api/agent/export", "scope": auth.ScopeRecordsRead},
		},
	})
}

// GetAgentMe 调用者是谁（便于智能体确认身份与权限）。
func GetAgentMe(c *gin.Context) {
	p := principal(c)
	utils.Success(c, gin.H{
		"user_id": p.UserID(), "nickname": p.User.DisplayName(),
		"kind": p.Kind, "actor": p.Actor, "scopes": p.Scopes.List(),
	})
}

// ListAgentTools OpenAI / DeepSeek function calling 格式的工具清单（仅含已授权的）。
func ListAgentTools(c *gin.Context) {
	if c.Query("format") == "mcp" {
		utils.Success(c, agent.MCPTools(principal(c)))
		return
	}
	utils.Success(c, agent.OpenAITools(principal(c)))
}

// InvokeAgentTool POST /api/agent/tools/:name，请求体即工具参数。
func InvokeAgentTool(c *gin.Context) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 64*1024))
	if err != nil {
		utils.BadRequest(c, "读取参数失败")
		return
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		body = []byte("{}")
	}
	if !json.Valid(body) {
		utils.BadRequest(c, "参数必须是 JSON 对象")
		return
	}
	invokeTool(c, c.Param("name"), json.RawMessage(body))
}

// GetAgentOpenAPI 把工具集描述成 OpenAPI 3.1，便于 GPTs Actions / Dify / Coze 等平台直接导入。
func GetAgentOpenAPI(c *gin.Context) {
	paths := gin.H{}
	for _, t := range agent.List(&auth.Principal{Kind: auth.KindUser}) {
		paths["/api/agent/tools/"+t.Name] = gin.H{
			"post": gin.H{
				"operationId": t.Name,
				"summary":     t.Title,
				"description": t.Description + "（需要权限：" + scopeOrNone(t.ScopeDescription()) + "）",
				"requestBody": gin.H{
					"required": false,
					"content":  gin.H{"application/json": gin.H{"schema": t.Schema}},
				},
				"responses": gin.H{
					"200": gin.H{
						"description": "{code: 0, message: success, data: 工具结果}",
						"content": gin.H{"application/json": gin.H{"schema": gin.H{
							"type": "object",
							"properties": gin.H{
								"code":    gin.H{"type": "integer"},
								"message": gin.H{"type": "string"},
								"data":    gin.H{},
							},
						}}},
					},
					"401": gin.H{"description": "令牌无效"},
					"403": gin.H{"description": "令牌缺少权限"},
				},
			},
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"openapi": "3.1.0",
		"info": gin.H{
			"title":       database.GetSetting("app_name", "NiniMenu") + " Agent API",
			"version":     agentAPIVersion,
			"description": "食谱与饮食数据工具集。令牌只能访问签发者本人的数据。",
		},
		"servers":    []gin.H{{"url": baseURL(c)}},
		"paths":      paths,
		"components": gin.H{"securitySchemes": gin.H{"bearerAuth": gin.H{"type": "http", "scheme": "bearer"}}},
		"security":   []gin.H{{"bearerAuth": []string{}}},
	})
}

func scopeOrNone(s string) string {
	if s == "" {
		return "无"
	}
	return s
}

// ---------------- 兼容旧版 REST 接口（数据按令牌所属用户隔离） ----------------

func intQuery(c *gin.Context, key string, def int) int {
	if n, err := strconv.Atoi(c.Query(key)); err == nil {
		return n
	}
	return def
}

// GetAgentDishes GET /api/agent/dishes
func GetAgentDishes(c *gin.Context) {
	q := services.DishQuery{
		Keyword:           c.Query("search"),
		Categories:        splitParam(c.Query("category")),
		Tastes:            splitParam(c.Query("taste")),
		Ingredients:       splitParam(c.Query("ingredient")),
		ExcludeIngredient: splitParam(c.Query("exclude_ingredient")),
		MaxCookTime:       intQuery(c, "max_cook_time", 0),
		Difficulty:        c.Query("difficulty"),
		MealType:          c.Query("meal_type"),
		OnlyFavorites:     queryBool(c.Query("favorite")),
		OnlyMine:          c.Query("scope") == "mine",
		ExcludeRecent:     queryBool(c.Query("exclude_recent")),
		IncludeDisabled:   c.Query("enabled") != "" && !queryBool(c.Query("enabled")),
		Offset:            intQuery(c, "offset", 0),
		Limit:             intQuery(c, "limit", 100),
	}
	for _, s := range splitParam(c.Query("ids")) {
		if id, err := strconv.Atoi(s); err == nil && id > 0 {
			q.IDs = append(q.IDs, uint(id))
		}
	}
	list, total := services.SearchDishes(uid(c), q)
	if list == nil {
		list = []models.Dish{}
	}
	utils.Success(c, gin.H{"items": list, "total": total, "offset": q.Offset, "limit": q.Limit})
}

// GetAgentDish GET /api/agent/dishes/:id
func GetAgentDish(c *gin.Context) {
	dish, err := services.FindVisibleDish(uid(c), c.Param("id"))
	if err != nil {
		utils.NotFound(c, "菜品不存在")
		return
	}
	stats, records := services.DishStatsForUser(uid(c), dish.ID)
	dish.Favorite = stats.Favorite
	if records == nil {
		records = []models.MealRecord{}
	}
	utils.Success(c, gin.H{"dish": dish, "records": records, "favorite": stats.Favorite, "stats": stats})
}

// GetAgentProfile GET /api/agent/profile 与 App 的 GET /api/profile。
func GetAgentProfile(c *gin.Context) {
	utils.Success(c, services.BuildTasteProfile(uid(c), intQuery(c, "days", 90)))
}

// AgentRecommend POST /api/agent/recommend
func AgentRecommend(c *gin.Context) {
	var req services.RecommendRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		utils.BadRequest(c, "请求数据无效: "+err.Error())
		return
	}
	p := principal(c)
	req.Source = p.Source()
	req.Actor = p.Actor
	req.IgnorePreferences = false
	result, err := services.RecommendDishes(uid(c), req)
	if err != nil {
		utils.InternalError(c, "推荐失败")
		return
	}
	services.WriteAudit(services.AuditEntry{UserID: uid(c), TokenID: p.TokenID, Actor: p.Actor, Channel: "rest", Tool: "recommend", Status: "ok"})
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

// GetAgentRecords GET /api/agent/records（带菜品口味/菜系与当日心情，默认最近 90 天）
func GetAgentRecords(c *gin.Context) {
	own := database.OwnedBy(uid(c))
	query := database.DB.Model(&models.MealRecord{}).Scopes(own)
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
	limit := intQuery(c, "limit", 200)
	if limit < 1 || limit > 2000 {
		limit = 200
	}

	var total int64
	query.Count(&total)
	var records []models.MealRecord
	query.Order("meal_date DESC, created_at DESC").Limit(limit).Find(&records)

	dishIDs, dates := []uint{}, []string{}
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
		database.DB.Unscoped().Scopes(database.VisibleDishes(uid(c))).Select("id", "category", "taste", "cook_time").Where("id IN ?", dishIDs).Find(&list)
		for _, d := range list {
			dishMap[d.ID] = d
		}
	}
	ratingMap := map[string]models.DayRating{}
	if len(dates) > 0 {
		var ratings []models.DayRating
		database.DB.Scopes(own).Where("meal_date IN ?", dates).Find(&ratings)
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

// CreateAgentRecords POST /api/agent/records（单条或 records 数组）
func CreateAgentRecords(c *gin.Context) {
	var req struct {
		services.MealInput
		Source  string               `json:"source"`
		Actor   string               `json:"actor"`
		Records []services.MealInput `json:"records"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请求数据无效: "+err.Error())
		return
	}
	list := req.Records
	if len(list) == 0 {
		list = []services.MealInput{req.MealInput}
	}
	if len(list) > 50 {
		utils.BadRequest(c, "一次最多 50 条")
		return
	}
	p := principal(c)
	created := make([]models.MealRecord, 0, len(list))
	errs := make([]string, 0)
	for _, in := range list {
		rec, err := services.CreateMealRecord(uid(c), in, p.Source(), firstNonEmpty(req.Actor, p.Actor))
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		created = append(created, *rec)
	}
	services.WriteAudit(services.AuditEntry{UserID: uid(c), TokenID: p.TokenID, Actor: p.Actor, Channel: "rest", Tool: "log_meal", Args: list, Status: "ok"})
	utils.Success(c, gin.H{"created": created, "skipped": len(list) - len(created), "total": len(list), "errors": errs})
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// DeleteAgentRecord DELETE /api/agent/records/:id
func DeleteAgentRecord(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	invokeTool(c, "delete_meal_record", gin.H{"record_id": id})
}

// GetAgentFavorites GET /api/agent/favorites
func GetAgentFavorites(c *gin.Context) {
	entries, err := loadFavoriteDishEntries(uid(c))
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

func AgentAddFavorite(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("dishId"))
	invokeTool(c, "set_favorite", gin.H{"dish_id": id, "favorite": true})
}

func AgentRemoveFavorite(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("dishId"))
	invokeTool(c, "set_favorite", gin.H{"dish_id": id, "favorite": false})
}

type behaviorInput struct {
	EventType string         `json:"event_type"`
	DishID    uint           `json:"dish_id"`
	DishName  string         `json:"dish_name"`
	Source    string         `json:"source"`
	Actor     string         `json:"actor"`
	Meta      map[string]any `json:"meta"`
}

// CreateAgentBehavior POST /api/agent/behavior（单条或 events 数组）。source 一律按令牌标记，不可伪造成 app。
func CreateAgentBehavior(c *gin.Context) {
	var body struct {
		behaviorInput
		Events []behaviorInput `json:"events"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		utils.BadRequest(c, "请求数据无效")
		return
	}
	list := body.Events
	if len(list) == 0 {
		list = []behaviorInput{body.behaviorInput}
	}
	if len(list) > 100 {
		utils.BadRequest(c, "一次最多 100 条")
		return
	}
	p := principal(c)
	saved := 0
	for _, e := range list {
		if !services.IsValidEventType(e.EventType) {
			continue
		}
		services.LogBehavior(uid(c), e.EventType, e.DishID, e.DishName, p.Source(), firstNonEmpty(e.Actor, p.Actor), e.Meta)
		saved++
	}
	utils.Success(c, gin.H{"saved": saved})
}

// CreateAppBehavior 站内前端埋点：浏览、采纳、拒绝等（source 固定为 app）。
func CreateAppBehavior(c *gin.Context) {
	var e behaviorInput
	if err := c.ShouldBindJSON(&e); err != nil || !services.IsValidEventType(e.EventType) {
		utils.BadRequest(c, "请求数据无效")
		return
	}
	services.LogBehavior(uid(c), e.EventType, e.DishID, e.DishName, "app", "", e.Meta)
	utils.Success(c, nil)
}

// GetAgentBehavior GET /api/agent/behavior
func GetAgentBehavior(c *gin.Context) {
	query := database.DB.Model(&models.BehaviorEvent{}).Scopes(database.OwnedBy(uid(c)))
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
		if t, err := time.ParseInLocation("2006-01-02", since, time.Local); err == nil {
			query = query.Where("created_at >= ?", t)
		} else if t, err := time.Parse(time.RFC3339, since); err == nil {
			query = query.Where("created_at >= ?", t)
		}
	}
	limit := intQuery(c, "limit", 200)
	if limit < 1 || limit > 2000 {
		limit = 200
	}
	var total int64
	query.Count(&total)
	var events []models.BehaviorEvent
	query.Order("created_at DESC, id DESC").Limit(limit).Find(&events)

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

// GetAgentDayRatings GET /api/agent/day-ratings
func GetAgentDayRatings(c *gin.Context) {
	query := database.DB.Model(&models.DayRating{}).Scopes(database.OwnedBy(uid(c)))
	from := c.Query("date_from")
	if from == "" {
		from = time.Now().AddDate(0, 0, -90).Format("2006-01-02")
	}
	query = query.Where("meal_date >= ?", from)
	if to := c.Query("date_to"); to != "" {
		query = query.Where("meal_date <= ?", to)
	}
	var ratings []models.DayRating
	query.Order("meal_date DESC").Limit(1000).Find(&ratings)
	utils.Success(c, ratings)
}

// GetAgentPreferences / UpdateAgentPreferences
func GetAgentPreferences(c *gin.Context) {
	utils.Success(c, services.GetPreferences(uid(c)))
}

func UpdateAgentPreferences(c *gin.Context) {
	body, _ := io.ReadAll(io.LimitReader(c.Request.Body, 16*1024))
	invokeTool(c, "update_preferences", json.RawMessage(body))
}

// Suggestions（智能体侧）
func CreateAgentSuggestion(c *gin.Context) {
	body, _ := io.ReadAll(io.LimitReader(c.Request.Body, 16*1024))
	invokeTool(c, "create_suggestion", json.RawMessage(body))
}

func ListAgentSuggestions(c *gin.Context) {
	utils.Success(c, services.ListSuggestions(uid(c), c.DefaultQuery("status", "all"), intQuery(c, "limit", 50)))
}

// GetAgentExport GET /api/agent/export —— 分析所需数据一次导出（不含对话记录）。
func GetAgentExport(c *gin.Context) {
	days := intQuery(c, "days", 365)
	if days <= 0 || days > 3650 {
		days = 365
	}
	since := time.Now().AddDate(0, 0, -days)
	sinceDate := since.Format("2006-01-02")
	own := database.OwnedBy(uid(c))

	var (
		records   []models.MealRecord
		favorites []models.Favorite
		ratings   []models.DayRating
		events    []models.BehaviorEvent
	)
	database.DB.Scopes(own).Where("meal_date >= ?", sinceDate).Order("meal_date ASC").Find(&records)
	database.DB.Scopes(own).Find(&favorites)
	database.DB.Scopes(own).Where("meal_date >= ?", sinceDate).Order("meal_date ASC").Find(&ratings)
	database.DB.Scopes(own).Where("created_at >= ?", since).Order("created_at ASC").Limit(5000).Find(&events)
	dishList, _ := services.SearchDishes(uid(c), services.DishQuery{Limit: 500})

	utils.Success(c, gin.H{
		"exported_at":     time.Now().Format(time.RFC3339),
		"window_days":     days,
		"dishes":          dishList,
		"meal_records":    records,
		"favorites":       favorites,
		"day_ratings":     ratings,
		"behavior_events": events,
		"preferences":     services.GetPreferences(uid(c)),
		"profile":         services.BuildTasteProfile(uid(c), days),
	})
}

// ---------------- App 侧：AI 连接管理 ----------------

// ListMyAgentTokens GET /api/me/agent-tokens
func ListMyAgentTokens(c *gin.Context) {
	utils.Success(c, gin.H{
		"tokens":  services.ListAgentTokens(uid(c)),
		"scopes":  auth.ScopeLabels,
		"presets": auth.ScopePresets,
		"mcp_url": baseURL(c) + "/mcp",
		"api_url": baseURL(c) + "/api/agent",
	})
}

// CreateMyAgentToken POST /api/me/agent-tokens —— 明文令牌只在这里返回一次。
func CreateMyAgentToken(c *gin.Context) {
	var req struct {
		Name          string   `json:"name"`
		Scopes        []string `json:"scopes"`
		ExpiresInDays int      `json:"expires_in_days"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请求数据无效")
		return
	}
	if req.ExpiresInDays < 0 || req.ExpiresInDays > 3650 {
		utils.BadRequest(c, "有效期无效")
		return
	}
	plain, view, err := services.CreateAgentToken(uid(c), req.Name, req.Scopes, req.ExpiresInDays)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	utils.Success(c, gin.H{"token": plain, "info": view})
}

// RevokeMyAgentToken DELETE /api/me/agent-tokens/:id
func RevokeMyAgentToken(c *gin.Context) {
	if err := services.RevokeAgentToken(uid(c), c.Param("id")); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	utils.SuccessMsg(c, "已撤销")
}

// CreateAgentSession POST /api/me/agent-session —— 给内嵌智能体（iframe）签发短期令牌，
// 前端通过 postMessage 交给嵌入页；令牌只代表当前用户，默认 2 小时过期。
func CreateAgentSession(c *gin.Context) {
	var req struct {
		Scopes []string `json:"scopes"`
		Actor  string   `json:"actor"`
	}
	_ = c.ShouldBindJSON(&req)
	scopes := req.Scopes
	if len(scopes) == 0 {
		scopes = splitParam(database.GetSetting("agent_embed_scopes", "full"))
	}
	actor := strings.TrimSpace(req.Actor)
	if actor == "" {
		actor = "embed"
	}
	if len([]rune(actor)) > 32 {
		actor = string([]rune(actor)[:32])
	}
	token, exp, err := auth.IssueAgentSession(auth.CurrentUser(c), scopes, actor, config.C.AgentSessionTTL)
	if err != nil {
		utils.InternalError(c, "签发失败")
		return
	}
	utils.Success(c, gin.H{
		"token": token, "expires_at": exp, "scopes": auth.NormalizeScopes(scopes),
		"api_base": baseURL(c) + "/api/agent", "mcp_url": baseURL(c) + "/mcp",
	})
}

// ListMyAgentAudit GET /api/me/agent-audit
func ListMyAgentAudit(c *gin.Context) {
	page, pageSize := pageParams(c, 30)
	q := database.DB.Model(&models.AgentAuditLog{}).Scopes(database.OwnedBy(uid(c)))
	if actor := c.Query("actor"); actor != "" {
		q = q.Where("actor = ?", actor)
	}
	var total int64
	q.Count(&total)
	var rows []models.AgentAuditLog
	q.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows)
	utils.SuccessPaginated(c, rows, total, page, pageSize)
}

// ---------------- App 侧：建议收件箱 ----------------

func ListMySuggestions(c *gin.Context) {
	utils.Success(c, services.ListSuggestions(uid(c), c.DefaultQuery("status", "pending"), intQuery(c, "limit", 20)))
}

func ResolveMySuggestion(c *gin.Context) {
	var req services.ResolveInput
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请求数据无效")
		return
	}
	created, err := services.ResolveSuggestion(uid(c), c.Param("id"), req)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	if created == nil {
		created = []models.MealRecord{}
	}
	utils.Success(c, gin.H{"created": created})
}
