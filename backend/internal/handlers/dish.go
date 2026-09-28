package handlers

import (
	"encoding/json"
	"errors"
	"math/rand"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"ninimenu/internal/services"
	"ninimenu/internal/utils"
	"ninimenu/internal/video"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// 菜品可见性：公共菜谱、本人私房菜，以及当前家庭明确共享的菜谱。

func dishScope(c *gin.Context) *gorm.DB {
	q := database.DB.WithContext(c.Request.Context()).Model(&models.Dish{})
	if c.Query("scope") == "mine" {
		return q.Scopes(database.AuthoredDishes(uid(c)))
	}
	q = q.Scopes(database.VisibleDishes(uid(c)))
	switch c.Query("scope") {
	case "private": // Personal originals that can be copied into a family.
		q = q.Where("owner_id = ? AND family_id = 0", uid(c))
	case "public":
		q = q.Where("owner_id = 0 AND family_id = 0")
	case "family":
		family, err := services.FamilyForUser(uid(c), database.DB.WithContext(c.Request.Context()))
		if err != nil || family == nil {
			q = q.Where("1 = 0")
		} else {
			q = q.Where("family_id = ?", family.ID)
		}
	}
	return q
}

func canEditDish(c *gin.Context, d *models.Dish) bool {
	if d.FamilyID != 0 {
		family, err := services.FamilyForUser(uid(c), database.DB.WithContext(c.Request.Context()))
		return err == nil && family != nil && family.ID == d.FamilyID &&
			(d.OwnerID == uid(c) || family.OwnerID == uid(c))
	}
	if d.OwnerID == 0 {
		return isAdmin(c)
	}
	return d.OwnerID == uid(c)
}

func findEditableDish(c *gin.Context) (*models.Dish, bool) {
	var dish models.Dish
	if err := database.DB.WithContext(c.Request.Context()).Scopes(database.VisibleDishes(uid(c))).Where("id = ?", c.Param("id")).First(&dish).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.NotFound(c, "菜品不存在")
		} else {
			utils.InternalError(c, "菜谱加载失败，请稍后重试")
		}
		return nil, false
	}
	if !canEditDish(c, &dish) {
		utils.Forbidden(c, "只有菜谱作者或家庭创建者可以修改")
		return nil, false
	}
	return &dish, true
}

func GetDishes(c *gin.Context) {
	page, pageSize := pageParams(c, 20)
	query := dishScope(c)

	if category := c.Query("category"); category != "" {
		query = query.Where("category = ?", category)
	}
	if mealType := c.Query("meal_type"); mealType != "" {
		query = query.Where("meal_type IN ?", []string{mealType, "all"})
	}
	if search := strings.TrimSpace(c.Query("search")); search != "" {
		like := "%" + search + "%"
		query = query.Where("name LIKE ? OR ingredients LIKE ? OR tags LIKE ?", like, like, like)
	}
	if enabled := c.Query("enabled"); enabled != "" {
		query = query.Where("enabled = ?", queryBool(enabled))
	}
	if taste := c.Query("taste"); taste != "" {
		query = query.Where("taste LIKE ?", "%"+taste+"%")
	}
	if difficulty := c.Query("difficulty"); difficulty != "" {
		query = query.Where("difficulty = ?", difficulty)
	}
	if queryBool(c.Query("favorite")) {
		query = query.Where("id IN (?)", database.DB.WithContext(c.Request.Context()).Model(&models.Favorite{}).Select("dish_id").Where("user_id = ?", uid(c)))
	}

	sort := c.DefaultQuery("sort", "created_at")
	order := c.DefaultQuery("order", "desc")
	isRandom := sort == "random"
	allowedSorts := map[string]bool{"created_at": true, "name": true, "cook_time": true, "difficulty": true, "sort_order": true, "random": true}
	if !allowedSorts[sort] {
		sort = "created_at"
	}
	if !isRandom {
		if order != "asc" && order != "desc" {
			order = "desc"
		}
		query = query.Order(sort + " " + order).Order("id ASC")
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		utils.InternalError(c, "菜谱加载失败，请稍后重试")
		return
	}

	var dishes []models.Dish
	if isRandom {
		var ids []uint
		if err := query.Pluck("id", &ids).Error; err != nil {
			utils.InternalError(c, "菜谱加载失败")
			return
		}
		rand.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })
		if len(ids) > pageSize {
			ids = ids[:pageSize]
		}
		if len(ids) > 0 {
			if err := database.DB.WithContext(c.Request.Context()).Scopes(database.VisibleDishes(uid(c))).Where("id IN ?", ids).Find(&dishes).Error; err != nil {
				utils.InternalError(c, "菜谱加载失败")
				return
			}
			rand.Shuffle(len(dishes), func(i, j int) { dishes[i], dishes[j] = dishes[j], dishes[i] })
		}

	} else {
		if err := query.Offset((page - 1) * pageSize).Limit(pageSize).Find(&dishes).Error; err != nil {
			utils.InternalError(c, "菜谱加载失败，请稍后重试")
			return
		}
	}
	services.MarkFavorites(uid(c), dishes, database.DB.WithContext(c.Request.Context()))
	utils.SuccessPaginated(c, dishes, total, page, pageSize)
}

func GetDish(c *gin.Context) {
	dish, err := services.FindVisibleDish(uid(c), c.Param("id"), database.DB.WithContext(c.Request.Context()))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.NotFound(c, "菜品不存在")
		} else {
			utils.InternalError(c, "菜谱加载失败，请稍后重试")
		}
		return
	}
	services.MarkFavorite(uid(c), &dish, database.DB.WithContext(c.Request.Context()))
	access := services.DishAccessFor(uid(c), isAdmin(c), &dish, database.DB.WithContext(c.Request.Context()))
	dish.Access = &access
	utils.Success(c, dish)
}

type CreateDishRequest struct {
	Name        string `json:"name" binding:"required"`
	ImageURL    string `json:"image_url"`
	Images      string `json:"images"`
	VideoURL    string `json:"video_url"`
	Category    string `json:"category"`
	MealType    string `json:"meal_type"`
	Taste       string `json:"taste"`
	Ingredients string `json:"ingredients"`
	Seasonings  string `json:"seasonings"`
	Steps       string `json:"steps"`
	CookTime    int    `json:"cook_time"`
	Difficulty  string `json:"difficulty"`
	Remark      string `json:"remark"`
	Tags        string `json:"tags"`
	SortOrder   int    `json:"sort_order"`
	// Public 管理员创建公共菜谱；普通用户忽略此字段，一律创建私房菜。
	Public bool `json:"public"`
	Family bool `json:"family"`
}

func jsonOr(raw, def string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || !json.Valid([]byte(raw)) {
		return def
	}
	return raw
}

func (r *CreateDishRequest) apply(d *models.Dish) {
	d.Name = strings.TrimSpace(r.Name)
	d.ImageURL = r.ImageURL
	d.Images = jsonOr(r.Images, "[]")
	d.VideoURL = r.VideoURL
	if in, err := video.Parse(r.VideoURL); err == nil {
		d.VideoURL = in.URL
	}
	d.Category = strings.TrimSpace(r.Category)
	d.MealType = r.MealType
	d.Taste = strings.TrimSpace(r.Taste)
	d.Ingredients = jsonOr(r.Ingredients, "[]")
	d.Seasonings = jsonOr(r.Seasonings, "[]")
	d.Steps = jsonOr(r.Steps, "[]")
	d.CookTime = r.CookTime
	d.Difficulty = r.Difficulty
	d.Remark = r.Remark
	d.Tags = jsonOr(r.Tags, "[]")
	d.SortOrder = r.SortOrder
	if d.MealType == "" {
		d.MealType = "all"
	}
	if d.Difficulty == "" {
		d.Difficulty = "easy"
	}
	if d.CookTime < 0 {
		d.CookTime = 0
	}
}

func CreateDish(c *gin.Context) {
	var req CreateDishRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Name) == "" {
		utils.BadRequest(c, "菜名不能为空")
		return
	}
	if err := req.validate(); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	dish := models.Dish{Enabled: true, OwnerID: uid(c)}
	if req.Public && isAdmin(c) {
		dish.OwnerID = 0
	} else if req.Family {
		family, err := services.RequireFamily(uid(c), database.DB.WithContext(c.Request.Context()))
		if err != nil {
			utils.BadRequest(c, "请先创建或加入家庭")
			return
		}
		dish.FamilyID = family.ID
	}
	req.apply(&dish)
	dish.VideoMeta = services.BuildVideoMeta(dish.VideoURL, c.Request.Context())
	if err := services.InsertDish(database.DB.WithContext(c.Request.Context()), &dish); err != nil {
		if errors.Is(err, services.ErrDishQuota) {
			utils.BadRequest(c, err.Error())
		} else {
			utils.InternalError(c, "创建菜品失败")
		}
		return
	}
	services.QueueAutoAchievementSync(uid(c))
	utils.Success(c, dish)
}

func UpdateDish(c *gin.Context) {
	dish, ok := findEditableDish(c)
	if !ok {
		return
	}
	var req CreateDishRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Name) == "" {
		utils.BadRequest(c, "请求数据无效")
		return
	}
	if err := req.validate(); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	oldVideoURL := dish.VideoURL
	req.apply(dish)
	if dish.VideoURL != oldVideoURL {
		dish.VideoMeta = services.BuildVideoMeta(dish.VideoURL, c.Request.Context())
	}
	if err := database.DB.WithContext(c.Request.Context()).Save(dish).Error; err != nil {
		utils.InternalError(c, "更新菜品失败")
		return
	}
	services.QueueAutoAchievementSync(uid(c))
	services.MarkFavorite(uid(c), dish, database.DB.WithContext(c.Request.Context()))
	utils.Success(c, dish)
}

func DeleteDish(c *gin.Context) {
	var dish models.Dish
	if err := database.DB.WithContext(c.Request.Context()).Scopes(database.VisibleDishes(uid(c))).Where("id = ?", c.Param("id")).First(&dish).Error; err != nil {
		utils.NotFound(c, "菜品不存在")
		return
	}
	access := services.DishAccessFor(uid(c), isAdmin(c), &dish, database.DB.WithContext(c.Request.Context()))
	switch access.DeleteMode {
	case services.DeleteModeDirect:
		if err := services.DeleteDishCascade(&dish, uid(c), database.DB.WithContext(c.Request.Context())); err != nil {
			utils.InternalError(c, "删除菜品失败")
			return
		}
		services.QueueAutoAchievementSync(uid(c))
		utils.Success(c, gin.H{"deleted": true, "pending": false})
	case services.DeleteModeRequest:
		view, err := services.RequestDishDeletion(uid(c), &dish, database.DB.WithContext(c.Request.Context()))
		if err != nil {
			utils.BadRequest(c, err.Error())
			return
		}
		utils.Success(c, gin.H{"deleted": false, "pending": true, "request": view})
	default:
		utils.Forbidden(c, "无权删除该菜谱")
	}
}

func ToggleDish(c *gin.Context) {
	dish, ok := findEditableDish(c)
	if !ok {
		return
	}
	dish.Enabled = !dish.Enabled
	if err := database.DB.WithContext(c.Request.Context()).Model(dish).Update("enabled", dish.Enabled).Error; err != nil {
		utils.InternalError(c, "更新失败")
		return
	}
	services.QueueAutoAchievementSync(uid(c))
	utils.Success(c, dish)
}

// CloneDish 复制一道菜：普通用户得到一份可自由修改的私房菜；管理员可带 ?public=1 复制为公共菜谱。
func CloneDish(c *gin.Context) {
	src, err := services.FindVisibleDish(uid(c), c.Param("id"), database.DB.WithContext(c.Request.Context()))
	if err != nil {
		utils.NotFound(c, "菜品不存在")
		return
	}
	newDish := src
	newDish.ID = 0
	newDish.CreatedAt, newDish.UpdatedAt = time.Time{}, time.Time{}
	newDish.OwnerID = uid(c)
	newDish.FamilyID = 0
	newDish.Enabled = true
	if isAdmin(c) && queryBool(c.Query("public")) {
		newDish.OwnerID = 0
		newDish.Name = src.Name + " (副本)"
	} else if src.OwnerID == uid(c) {
		newDish.Name = src.Name + " (副本)"
	}
	if err := services.InsertDish(database.DB.WithContext(c.Request.Context()), &newDish); err != nil {
		if errors.Is(err, services.ErrDishQuota) {
			utils.BadRequest(c, err.Error())
		} else {
			utils.InternalError(c, "复制菜品失败")
		}
		return
	}
	services.QueueAutoAchievementSync(uid(c))
	utils.Success(c, newDish)
}

type CategoryCount struct {
	Category string `json:"category"`
	Count    int64  `json:"count"`
}

func GetDishCategoryCounts(c *gin.Context) {
	var counts []CategoryCount
	query := dishScope(c)
	if c.Query("scope") != "mine" && c.Query("scope") != "private" {
		query = query.Where("enabled = ?", true)
	}
	if err := query.
		Select("category, count(*) as count").
		Group("category").
		Order("count DESC").
		Find(&counts).Error; err != nil {
		utils.InternalError(c, "分类统计加载失败")
		return
	}

	var total, mine int64
	for _, category := range counts {
		total += category.Count
	}
	if err := database.DB.WithContext(c.Request.Context()).Model(&models.Dish{}).Scopes(database.AuthoredDishes(uid(c))).Count(&mine).Error; err != nil {
		utils.InternalError(c, "分类统计加载失败")
		return
	}

	utils.Success(c, gin.H{"total": total, "mine": mine, "categories": counts})
}

// editableIDs 批量操作只作用于调用者有权编辑的菜品。
func editableIDs(c *gin.Context, ids []uint) []uint {
	q := database.DB.WithContext(c.Request.Context()).Model(&models.Dish{}).Where("id IN ?", ids)
	family, err := services.FamilyForUser(uid(c), database.DB.WithContext(c.Request.Context()))
	familyClause := "1 = 0"
	args := []any{}
	if err == nil && family != nil {
		if family.OwnerID == uid(c) {
			familyClause = "family_id = ?"
			args = append(args, family.ID)
		} else {
			familyClause = "family_id = ? AND owner_id = ?"
			args = append(args, family.ID, uid(c))
		}
	}
	if isAdmin(c) {
		q = q.Where("(family_id = 0 AND owner_id IN ?) OR ("+familyClause+")", append([]any{[]uint{0, uid(c)}}, args...)...)
	} else {
		q = q.Where("(family_id = 0 AND owner_id = ?) OR ("+familyClause+")", append([]any{uid(c)}, args...)...)
	}
	var out []uint
	q.Pluck("id", &out)
	return out
}

type BatchToggleRequest struct {
	IDs     []uint `json:"ids" binding:"required"`
	Enabled bool   `json:"enabled"`
}

func BatchToggleDishes(c *gin.Context) {
	var req BatchToggleRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 || len(req.IDs) > 500 {
		utils.BadRequest(c, "请选择菜品")
		return
	}
	if ids := editableIDs(c, req.IDs); len(ids) > 0 {
		database.DB.WithContext(c.Request.Context()).Model(&models.Dish{}).Where("id IN ?", ids).Update("enabled", req.Enabled)
	}
	utils.SuccessMsg(c, "批量操作成功")
}

type BatchDeleteRequest struct {
	IDs []uint `json:"ids" binding:"required"`
}

func BatchDeleteDishes(c *gin.Context) {
	var req BatchDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 || len(req.IDs) > 500 {
		utils.BadRequest(c, "请选择菜品")
		return
	}
	var dishes []models.Dish
	database.DB.WithContext(c.Request.Context()).Scopes(database.VisibleDishes(uid(c))).Where("id IN ?", req.IDs).Find(&dishes)
	deleted, requested := 0, 0
	for i := range dishes {
		switch services.DishAccessFor(uid(c), isAdmin(c), &dishes[i], database.DB.WithContext(c.Request.Context())).DeleteMode {
		case services.DeleteModeDirect:
			if err := services.DeleteDishCascade(&dishes[i], uid(c), database.DB.WithContext(c.Request.Context())); err == nil {
				deleted++
			}
		case services.DeleteModeRequest:
			if _, err := services.RequestDishDeletion(uid(c), &dishes[i], database.DB.WithContext(c.Request.Context())); err == nil {
				requested++
			}
		}
	}
	if deleted > 0 {
		services.QueueAutoAchievementSync(uid(c))
	}
	utils.Success(c, gin.H{"deleted": deleted, "requested": requested})
}

type BatchCategoryRequest struct {
	IDs      []uint `json:"ids" binding:"required"`
	Category string `json:"category" binding:"required"`
}

func BatchUpdateCategory(c *gin.Context) {
	var req BatchCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 || len(req.IDs) > 500 {
		utils.BadRequest(c, "请选择菜品并指定分类")
		return
	}
	if ids := editableIDs(c, req.IDs); len(ids) > 0 {
		database.DB.WithContext(c.Request.Context()).Model(&models.Dish{}).Where("id IN ?", ids).Update("category", req.Category)
	}
	utils.SuccessMsg(c, "批量修改分类成功")
}

type DishRecordWithDay struct {
	ID        uint     `json:"id"`
	MealType  string   `json:"meal_type"`
	MealDate  string   `json:"meal_date"`
	Rating    int      `json:"rating"`
	Remark    string   `json:"remark"`
	Mood      string   `json:"mood"`
	Photo     string   `json:"photo"`
	HomeMood  string   `json:"home_mood"`
	DayMood   string   `json:"day_mood"`
	DayRemark string   `json:"day_remark"`
	Photos    []string `json:"photos"`
}

type DishRecordsStats struct {
	TotalCount  int     `json:"total_count"`
	LunchCount  int     `json:"lunch_count"`
	DinnerCount int     `json:"dinner_count"`
	YumPercent  int     `json:"yum_percent"`
	OkPercent   int     `json:"ok_percent"`
	NoPercent   int     `json:"no_percent"`
	AvgRating   float64 `json:"avg_rating"`
	LastDate    string  `json:"last_date"`
	AvgInterval int     `json:"avg_interval"`
}

type DishRecordsResponse struct {
	Records []DishRecordWithDay `json:"records"`
	Stats   DishRecordsStats    `json:"stats"`
}

// GetDishRecords 当前用户吃这道菜的历史。
func GetDishRecords(c *gin.Context) {
	dish, err := services.FindVisibleDish(uid(c), c.Param("id"), database.DB.WithContext(c.Request.Context()))
	if err != nil {
		utils.NotFound(c, "菜品不存在")
		return
	}
	own := database.OwnedBy(uid(c))

	var records []models.MealRecord
	database.DB.WithContext(c.Request.Context()).Scopes(own).Where("dish_id = ?", dish.ID).Order("meal_date DESC, created_at DESC").Find(&records)
	if len(records) == 0 {
		utils.Success(c, DishRecordsResponse{Records: []DishRecordWithDay{}, Stats: DishRecordsStats{}})
		return
	}

	dates := make([]string, 0, len(records))
	for _, r := range records {
		dates = append(dates, r.MealDate)
	}
	dayRatingMap := make(map[string]models.DayRating)
	var dayRatings []models.DayRating
	database.DB.WithContext(c.Request.Context()).Scopes(own).Where("meal_date IN ?", dates).Find(&dayRatings)
	for _, dr := range dayRatings {
		dayRatingMap[dr.MealDate] = dr
	}

	result := make([]DishRecordWithDay, 0, len(records))
	yumCount, okCount, noCount, totalRating, ratingCount, lunchCount := 0, 0, 0, 0, 0, 0
	for _, r := range records {
		var photos []string
		var homeMood, dayMood, dayRemark string
		if dr, ok := dayRatingMap[r.MealDate]; ok {
			homeMood, dayMood, dayRemark = dr.HomeMood, dr.Mood, dr.Remark
			if dr.Photos != "" {
				_ = json.Unmarshal([]byte(dr.Photos), &photos)
			}
		}
		if photos == nil {
			photos = []string{}
		}
		switch r.Mood {
		case "yum", "great":
			yumCount++
		case "ok":
			okCount++
		case "no", "meh":
			noCount++
		}
		if r.Rating > 0 {
			totalRating += r.Rating
			ratingCount++
		}
		if r.MealType == "lunch" {
			lunchCount++
		}
		result = append(result, DishRecordWithDay{
			ID: r.ID, MealType: r.MealType, MealDate: r.MealDate, Rating: r.Rating, Remark: r.Remark,
			Mood: r.Mood, Photo: r.Photo, HomeMood: homeMood, DayMood: dayMood, DayRemark: dayRemark, Photos: photos,
		})
	}

	total := len(records)
	stats := DishRecordsStats{TotalCount: total, LunchCount: lunchCount, DinnerCount: total - lunchCount, LastDate: records[0].MealDate}
	if moodTotal := yumCount + okCount + noCount; moodTotal > 0 {
		stats.YumPercent = yumCount * 100 / moodTotal
		stats.OkPercent = okCount * 100 / moodTotal
		stats.NoPercent = noCount * 100 / moodTotal
	}
	if ratingCount > 0 {
		stats.AvgRating = float64(totalRating) / float64(ratingCount)
	}
	if total >= 2 {
		totalDays := 0
		for i := 0; i < total-1; i++ {
			d1, e1 := time.Parse("2006-01-02", records[i].MealDate)
			d2, e2 := time.Parse("2006-01-02", records[i+1].MealDate)
			if e1 == nil && e2 == nil {
				if diff := int(d1.Sub(d2).Hours() / 24); diff > 0 {
					totalDays += diff
				}
			}
		}
		stats.AvgInterval = totalDays / (total - 1)
	}
	utils.Success(c, DishRecordsResponse{Records: result, Stats: stats})
}
