package routes

import (
	"net/http"
	"ninimenu/internal/auth"
	"ninimenu/internal/config"
	"ninimenu/internal/database"
	"ninimenu/internal/handlers"
	mw "ninimenu/internal/middleware"
	"ninimenu/internal/storage"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func Setup(r *gin.Engine) {
	r.MaxMultipartMemory = config.C.MaxUploadSize + 1<<20
	r.Use(mw.SecurityHeaders())
	r.Use(mw.CORSMiddleware())
	r.Use(mw.LoggerMiddleware())

	r.GET("/healthz", healthz)
	r.GET("/uploads/*filepath", gin.WrapF(storage.ServeUploads))
	r.HEAD("/uploads/*filepath", gin.WrapF(storage.ServeUploads))

	// MCP：Hermes / Claude / Cursor 等 MCP 客户端直连（令牌鉴权，数据按令牌所属用户隔离）
	mcp := r.Group("/mcp", mw.AgentAuth())
	mcp.POST("", handlers.MCPPost)
	mcp.GET("", handlers.MCPGet)
	mcp.DELETE("", handlers.MCPDelete)

	api := r.Group("/api")

	// ---------- 公开 ----------
	api.GET("/app-info", handlers.GetAppInfo)
	api.GET("/auth/options", handlers.GetAuthOptions)
	api.GET("/agent/openapi.json", handlers.GetAgentOpenAPI)
	authGroup := api.Group("/auth", mw.AuthRateLimit(30))
	authGroup.POST("/email/code", handlers.SendEmailCode)
	authGroup.POST("/email/login", handlers.EmailLogin)
	authGroup.POST("/login", handlers.PasswordLogin)
	authGroup.POST("/wechat", handlers.WechatLogin)

	// ---------- 用户（App / 小程序 / 管理后台共用登录态） ----------
	app := api.Group("", mw.UserAuth())
	{
		app.GET("/me", handlers.GetMe)
		app.PUT("/me", handlers.UpdateMe)
		app.PUT("/me/password", handlers.ChangePassword)
		app.POST("/me/logout-all", handlers.LogoutAll)
		app.GET("/me/export", handlers.ExportMe)
		app.DELETE("/me", handlers.DeleteMe)
		app.GET("/me/preferences", handlers.GetPreferences)
		app.PUT("/me/preferences", handlers.UpdatePreferences)

		app.GET("/family", handlers.GetFamily)
		app.POST("/family", handlers.CreateFamily)
		app.PATCH("/family", handlers.RenameFamily)
		app.DELETE("/family", handlers.DeleteFamily)
		app.POST("/family/invitations", handlers.InviteFamilyMember)
		app.DELETE("/family/invitations/:id", handlers.RevokeFamilyInvitation)
		app.POST("/family/join", handlers.JoinFamily)
		app.POST("/family/transfer", handlers.TransferFamily)
		app.DELETE("/family/members/:id", handlers.RemoveFamilyMember)
		app.POST("/family/leave", handlers.LeaveFamily)
		app.POST("/family/dishes/:id/share", handlers.ShareFamilyDish)
		app.GET("/family/plan", handlers.GetFamilyPlan)
		app.PUT("/family/plan", handlers.SetFamilyPlan)
		app.GET("/family/shopping", handlers.GetFamilyShopping)
		app.POST("/family/shopping", handlers.AddFamilyShopping)
		app.PATCH("/family/shopping/:id", handlers.CheckFamilyShopping)
		app.DELETE("/family/shopping/:id", handlers.DeleteFamilyShopping)
		app.POST("/family/shopping/import", handlers.ImportFamilyIngredients)
		app.GET("/food-journal", handlers.GetFoodJournal)
		app.POST("/food-journal", handlers.CreateFoodJournal)
		app.DELETE("/food-journal/:id", handlers.DeleteFoodJournal)
		app.GET("/health-report", handlers.GetHealthReport)

		// AI 连接：个人访问令牌、嵌入会话、审计日志
		app.GET("/me/agent-tokens", handlers.ListMyAgentTokens)
		app.POST("/me/agent-tokens", handlers.CreateMyAgentToken)
		app.DELETE("/me/agent-tokens/:id", handlers.RevokeMyAgentToken)
		app.POST("/me/agent-session", handlers.CreateAgentSession)
		app.GET("/me/agent-audit", handlers.ListMyAgentAudit)

		// 智能体推送的建议收件箱
		app.GET("/suggestions", handlers.ListMySuggestions)
		app.POST("/suggestions/:id/resolve", handlers.ResolveMySuggestion)

		// 站内 AI 助手
		app.GET("/assistant/status", handlers.GetAssistantStatus)
		app.POST("/assistant/chat", mw.ChatRateLimit(20), handlers.AssistantChat)
		app.GET("/assistant/sessions", handlers.ListAssistantSessions)
		app.GET("/assistant/sessions/:id", handlers.GetAssistantMessages)
		app.DELETE("/assistant/sessions/:id", handlers.DeleteAssistantSession)

		app.GET("/dishes", handlers.GetDishes)
		app.GET("/dishes/category-counts", handlers.GetDishCategoryCounts)
		app.GET("/dishes/:id", handlers.GetDish)
		app.GET("/dishes/:id/records", handlers.GetDishRecords)
		// 菜品写操作：私房菜本人可改，公共菜谱仅管理员（在 handler 内校验）
		app.POST("/dishes", handlers.CreateDish)
		app.PUT("/dishes/:id", handlers.UpdateDish)
		app.DELETE("/dishes/:id", handlers.DeleteDish)
		app.PUT("/dishes/:id/toggle", handlers.ToggleDish)
		app.POST("/dishes/:id/clone", handlers.CloneDish)
		app.POST("/dishes/batch-toggle", handlers.BatchToggleDishes)
		app.POST("/dishes/batch-delete", handlers.BatchDeleteDishes)
		app.POST("/dishes/batch-category", handlers.BatchUpdateCategory)
		app.POST("/upload/image", handlers.UploadImage)
		app.DELETE("/upload/image", handlers.DeleteImage)

		app.GET("/records", handlers.GetRecords)
		app.POST("/records", handlers.CreateRecord)
		app.POST("/records/batch", handlers.BatchCreateRecords)
		app.PUT("/records/:id", handlers.UpdateRecord)
		app.DELETE("/records/:id", handlers.DeleteRecord)
		app.GET("/favorites", handlers.GetFavorites)
		app.GET("/favorites/overview", handlers.GetFavoritesOverview)
		app.POST("/favorites/:dishId", handlers.AddFavorite)
		app.DELETE("/favorites/:dishId", handlers.RemoveFavorite)
		app.POST("/pick/lunch", handlers.PickLunch)
		app.POST("/pick/dinner", handlers.PickDinner)
		app.POST("/pick/mood", handlers.PickMood)
		app.POST("/pick/tomorrow", handlers.PickTomorrow)
		app.POST("/pick/blind-box", handlers.PickBlindBox)
		app.POST("/pick/smart", handlers.PickSmart)
		app.GET("/profile", handlers.GetAgentProfile)
		app.POST("/behavior", handlers.CreateAppBehavior)
		app.GET("/quotes", handlers.GetQuotes)
		app.GET("/achievements", handlers.GetAchievements)
		app.POST("/achievements/:id/unlock", handlers.UnlockAchievement)
		app.POST("/achievements/:id/toggle", handlers.ToggleAchievement)
		app.GET("/stats", handlers.GetStats)
		app.GET("/day-rating", handlers.GetDayRating)
		app.GET("/day-ratings", handlers.GetDayRatings)
		app.POST("/day-rating/home-mood", handlers.CreateOrUpdateHomeMood)
		app.POST("/day-rating", handlers.CreateOrUpdateDayRating)
		app.GET("/photo-wall", handlers.GetPhotoWall)
		app.GET("/settings", handlers.GetSettings)
		app.PUT("/settings", handlers.UpdateSettings)
		app.GET("/week-plan", handlers.GetWeekPlan)
		app.POST("/week-plan/regenerate", handlers.RegenerateWeekPlanHandler)
		app.GET("/shopping-list", handlers.GetShoppingList)
		app.POST("/shopping-list/toggle", handlers.ToggleShoppingCheckHandler)
		app.POST("/shopping-list/inventory", handlers.ToggleHomeInventoryHandler)
		app.GET("/holidays/upcoming", handlers.GetUpcomingHolidays)
	}

	// ---------- 管理员 ----------
	admin := api.Group("", mw.UserAuth(), mw.AdminOnly())
	{
		admin.GET("/admin/dashboard", handlers.GetDashboard)
		admin.GET("/admin/users", handlers.AdminListUsers)
		admin.PUT("/admin/users/:id", handlers.AdminUpdateUser)
		admin.GET("/admin/settings", handlers.GetSiteSettings)
		admin.PUT("/admin/settings", handlers.UpdateSiteSettings)
		admin.POST("/admin/llm/test", handlers.TestLLM)
		admin.POST("/quotes", handlers.CreateQuote)
		admin.PUT("/quotes/:id", handlers.UpdateQuote)
		admin.DELETE("/quotes/:id", handlers.DeleteQuote)
		admin.GET("/shopping-categories", handlers.GetShoppingCategories)
		admin.POST("/shopping-categories", handlers.UpsertShoppingCategoryHandler)
		admin.DELETE("/shopping-categories/:itemName", handlers.DeleteShoppingCategoryHandler)
		admin.POST("/achievements", handlers.CreateAchievement)
		admin.PUT("/achievements/:id", handlers.UpdateAchievement)
		admin.DELETE("/achievements/:id", handlers.DeleteAchievement)
	}

	// ---------- 智能体开放接口 ----------
	ag := api.Group("/agent", mw.AgentAuth())
	{
		scope := mw.RequireScope
		ag.GET("/capabilities", handlers.GetAgentCapabilities)
		ag.GET("/me", handlers.GetAgentMe)
		ag.GET("/tools", handlers.ListAgentTools)
		ag.POST("/tools/:name", handlers.InvokeAgentTool) // 工具内部按 scope 校验

		ag.GET("/dishes", scope(auth.ScopeDishesRead), handlers.GetAgentDishes)
		ag.GET("/dishes/:id", scope(auth.ScopeDishesRead), handlers.GetAgentDish)
		ag.POST("/recommend", scope(auth.ScopeDishesRead), scope(auth.ScopeProfileRead), handlers.AgentRecommend)
		ag.GET("/profile", scope(auth.ScopeProfileRead), handlers.GetAgentProfile)
		ag.GET("/stats", scope(auth.ScopeProfileRead), handlers.GetStats)
		ag.GET("/settings", scope(auth.ScopeProfileRead), handlers.GetSettings)
		ag.GET("/preferences", scope(auth.ScopeProfileRead), handlers.GetAgentPreferences)
		ag.PUT("/preferences", scope(auth.ScopePreferencesWrite), handlers.UpdateAgentPreferences)
		ag.GET("/records", scope(auth.ScopeRecordsRead), handlers.GetAgentRecords)
		ag.POST("/records", scope(auth.ScopeRecordsWrite), handlers.CreateAgentRecords)
		ag.DELETE("/records/:id", scope(auth.ScopeRecordsWrite), handlers.DeleteAgentRecord)
		ag.GET("/favorites", scope(auth.ScopeRecordsRead), handlers.GetAgentFavorites)
		ag.POST("/favorites/:dishId", scope(auth.ScopeFavoritesWrite), handlers.AgentAddFavorite)
		ag.DELETE("/favorites/:dishId", scope(auth.ScopeFavoritesWrite), handlers.AgentRemoveFavorite)
		ag.GET("/behavior", scope(auth.ScopeRecordsRead), handlers.GetAgentBehavior)
		ag.POST("/behavior", scope(auth.ScopeBehaviorWrite), handlers.CreateAgentBehavior)
		ag.GET("/suggestions", scope(auth.ScopeRecordsRead), handlers.ListAgentSuggestions)
		ag.POST("/suggestions", scope(auth.ScopeSuggestionsWrite), handlers.CreateAgentSuggestion)
		ag.GET("/day-ratings", scope(auth.ScopeRecordsRead), handlers.GetAgentDayRatings)
		ag.GET("/week-plan", scope(auth.ScopeRecordsRead), handlers.GetWeekPlan)
		ag.POST("/week-plan/regenerate", scope(auth.ScopePlanWrite), handlers.RegenerateWeekPlanHandler)
		ag.GET("/shopping-list", scope(auth.ScopeRecordsRead), handlers.GetShoppingList)
		ag.GET("/export", scope(auth.ScopeRecordsRead), scope(auth.ScopeProfileRead), handlers.GetAgentExport)
	}

	spa := spaHandler()
	r.NoRoute(func(c *gin.Context) {
		p := c.Request.URL.Path
		if strings.HasPrefix(p, "/api/") || p == "/api" || p == "/mcp" {
			c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "接口不存在", "data": nil})
			return
		}
		if spa == nil {
			c.Status(http.StatusNotFound)
			return
		}
		spa(c)
	})
}

func healthz(c *gin.Context) {
	sqlDB, err := database.DB.DB()
	status, code := "UP", http.StatusOK
	if err != nil || sqlDB.Ping() != nil {
		status, code = "DOWN", http.StatusServiceUnavailable
	}
	c.JSON(code, gin.H{"status": status, "storage": storage.Backend(), "time": time.Now().Format(time.RFC3339)})
}

// spaHandler 前端构建产物：带 hash 的资源强缓存，index.html 不缓存，其余路径回落到 index.html。
func spaHandler() gin.HandlerFunc {
	staticDir := "static"
	if info, err := os.Stat(staticDir); err != nil || !info.IsDir() {
		return nil
	}
	root, _ := filepath.Abs(staticDir)
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.Status(http.StatusNotFound)
			return
		}
		p := filepath.Join(root, filepath.Clean("/"+c.Request.URL.Path))
		if rel, err := filepath.Rel(root, p); err == nil && !strings.HasPrefix(rel, "..") {
			if info, err := os.Stat(p); err == nil && !info.IsDir() {
				if strings.HasPrefix(c.Request.URL.Path, "/assets/") {
					c.Header("Cache-Control", "public, max-age=31536000, immutable")
				} else {
					c.Header("Cache-Control", "public, max-age=3600")
				}
				c.File(p)
				return
			}
		}
		if strings.HasPrefix(c.Request.URL.Path, "/assets/") {
			c.Status(http.StatusNotFound)
			return
		}
		c.Header("Cache-Control", "no-cache")
		c.File(filepath.Join(root, "index.html"))
	}
}
