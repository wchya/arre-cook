package handlers

import (
	"encoding/json"
	"math/rand"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"ninimenu/internal/services"
	"ninimenu/internal/utils"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// 菜品可见性：公共菜谱（owner_id=0，管理员维护）+ 本人私房菜（owner_id=uid）。
// 编辑权限：私房菜仅本人；公共菜谱仅管理员。

func dishScope(c *gin.Context) *gorm.DB {
	q := database.DB.Model(&models.Dish{}).Scopes(database.VisibleDishes(uid(c)))
	switch c.Query("scope") {
	case "mine":
		q = q.Where("owner_id = ?", uid(c))
	case "public":
		q = q.Where("owner_id = 0")
	}
	return q
}

func canEditDish(c *gin.Context, d *models.Dish) bool {
	if d.OwnerID == 0 {
		return isAdmin(c)
	}
	return d.OwnerID == uid(c)
}

func findEditableDish(c *gin.Context) (*models.Dish, bool) {
	var dish models.Dish
	if err := database.DB.Scopes(database.VisibleDishes(uid(c))).First(&dish, c.Param("id")).Error; err != nil {
		utils.NotFound(c, "菜品不存在")
		return nil, false
	}
	if !canEditDish(c, &dish) {
		utils.Forbidden(c, "公共菜谱只有管理员可以修改，可以先“复制到我的菜谱”再改")
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
		query = query.Where("id IN (?)", database.DB.Model(&models.Favorite{}).Select("dish_id").Where("user_id = ?", uid(c)))
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
	query.Count(&total)

	var dishes []models.Dish
	if isRandom {
		query.Find(&dishes)
		rand.Shuffle(len(dishes), func(i, j int) { dishes[i], dishes[j] = dishes[j], dishes[i] })
		if len(dishes) > pageSize {
			dishes = dishes[:pageSize]
		}
	} else {
		query.Offset((page - 1) * pageSize).Limit(pageSize).Find(&dishes)
	}
	services.MarkFavorites(uid(c), dishes)
	utils.SuccessPaginated(c, dishes, total, page, pageSize)
}

func GetDish(c *gin.Context) {
	dish, err := services.FindVisibleDish(uid(c), c.Param("id"))
	if err != nil {
		utils.NotFound(c, "菜品不存在")
		return
	}
	services.MarkFavorite(uid(c), &dish)
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
	dish := models.Dish{Enabled: true, OwnerID: uid(c)}
	if req.Public && isAdmin(c) {
		dish.OwnerID = 0
	}
	req.apply(&dish)
	if dish.OwnerID != 0 {
		var n int64
		database.DB.Model(&models.Dish{}).Where("owner_id = ?", dish.OwnerID).Count(&n)
		if n >= 500 {
			utils.BadRequest(c, "私房菜已达 500 道上限")
			return
		}
	}
	if err := database.DB.Create(&dish).Error; err != nil {
		utils.InternalError(c, "创建菜品失败")
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
	req.apply(dish)
	if err := database.DB.Save(dish).Error; err != nil {
		utils.InternalError(c, "更新菜品失败")
		return
	}
	services.QueueAutoAchievementSync(uid(c))
	services.MarkFavorite(uid(c), dish)
	utils.Success(c, dish)
}

func DeleteDish(c *gin.Context) {
	dish, ok := findEditableDish(c)
	if !ok {
		return
	}
	database.DB.Delete(dish)
	services.InvalidateWeekPlan(uid(c))
	services.QueueAutoAchievementSync(uid(c))
	utils.SuccessMsg(c, "删除成功")
}

func ToggleDish(c *gin.Context) {
	dish, ok := findEditableDish(c)
	if !ok {
		return
	}
	dish.Enabled = !dish.Enabled
	database.DB.Model(dish).Update("enabled", dish.Enabled)
	services.QueueAutoAchievementSync(uid(c))
	utils.Success(c, dish)
}

// CloneDish 复制一道菜：普通用户得到一份可自由修改的私房菜；管理员可带 ?public=1 复制为公共菜谱。
func CloneDish(c *gin.Context) {
	src, err := services.FindVisibleDish(uid(c), c.Param("id"))
	if err != nil {
		utils.NotFound(c, "菜品不存在")
		return
	}
	newDish := src
	newDish.ID = 0
	newDish.CreatedAt, newDish.UpdatedAt = time.Time{}, time.Time{}
	newDish.OwnerID = uid(c)
	newDish.Enabled = true
	if isAdmin(c) && queryBool(c.Query("public")) {
		newDish.OwnerID = 0
		newDish.Name = src.Name + " (副本)"
	} else if src.OwnerID == uid(c) {
		newDish.Name = src.Name + " (副本)"
	}
	if err := database.DB.Create(&newDish).Error; err != nil {
		utils.InternalError(c, "复制菜品失败")
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
	database.DB.Model(&models.Dish{}).Scopes(database.VisibleDishes(uid(c))).
		Select("category, count(*) as count").
		Where("enabled = ?", true).
		Group("category").
		Order("count DESC").
		Find(&counts)

	var total, mine int64
	database.DB.Model(&models.Dish{}).Scopes(database.VisibleDishes(uid(c))).Where("enabled = ?", true).Count(&total)
	database.DB.Model(&models.Dish{}).Where("owner_id = ?", uid(c)).Count(&mine)

	utils.Success(c, gin.H{"total": total, "mine": mine, "categories": counts})
}

// editableIDs 批量操作只作用于调用者有权编辑的菜品。
func editableIDs(c *gin.Context, ids []uint) []uint {
	q := database.DB.Model(&models.Dish{}).Where("id IN ?", ids)
	if isAdmin(c) {
		q = q.Where("owner_id IN ?", []uint{0, uid(c)})
	} else {
		q = q.Where("owner_id = ?", uid(c))
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
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		utils.BadRequest(c, "请选择菜品")
		return
	}
	if ids := editableIDs(c, req.IDs); len(ids) > 0 {
		database.DB.Model(&models.Dish{}).Where("id IN ?", ids).Update("enabled", req.Enabled)
	}
	utils.SuccessMsg(c, "批量操作成功")
}

type BatchDeleteRequest struct {
	IDs []uint `json:"ids" binding:"required"`
}

func BatchDeleteDishes(c *gin.Context) {
	var req BatchDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		utils.BadRequest(c, "请选择菜品")
		return
	}
	if ids := editableIDs(c, req.IDs); len(ids) > 0 {
		database.DB.Where("id IN ?", ids).Delete(&models.Dish{})
	}
	utils.SuccessMsg(c, "批量删除成功")
}

type BatchCategoryRequest struct {
	IDs      []uint `json:"ids" binding:"required"`
	Category string `json:"category" binding:"required"`
}

func BatchUpdateCategory(c *gin.Context) {
	var req BatchCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		utils.BadRequest(c, "请选择菜品并指定分类")
		return
	}
	if ids := editableIDs(c, req.IDs); len(ids) > 0 {
		database.DB.Model(&models.Dish{}).Where("id IN ?", ids).Update("category", req.Category)
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
	dish, err := services.FindVisibleDish(uid(c), c.Param("id"))
	if err != nil {
		utils.NotFound(c, "菜品不存在")
		return
	}
	own := database.OwnedBy(uid(c))

	var records []models.MealRecord
	database.DB.Scopes(own).Where("dish_id = ?", dish.ID).Order("meal_date DESC, created_at DESC").Find(&records)
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
	database.DB.Scopes(own).Where("meal_date IN ?", dates).Find(&dayRatings)
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
