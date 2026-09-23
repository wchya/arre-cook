package handlers

import (
	"encoding/json"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"ninimenu/internal/services"
	"ninimenu/internal/utils"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type nameAmount struct {
	Name   string `json:"name"`
	Amount string `json:"amount"`
}

func addShoppingItems(dishID uint, dishName string, mealType string, mealDate string) {
	var dish models.Dish
	if err := database.DB.Unscoped().First(&dish, dishID).Error; err != nil {
		return
	}

	var items []models.ShoppingCheck

	var ingredients []nameAmount
	if err := json.Unmarshal([]byte(dish.Ingredients), &ingredients); err == nil {
		for _, ing := range ingredients {
			if ing.Name != "" {
				items = append(items, models.ShoppingCheck{
					MealDate:   mealDate,
					MealType:   mealType,
					DishID:     dishID,
					DishName:   dishName,
					ItemName:   ing.Name,
					ItemAmount: ing.Amount,
				})
			}
		}
	}

	var seasonings []nameAmount
	if err := json.Unmarshal([]byte(dish.Seasonings), &seasonings); err == nil {
		for _, s := range seasonings {
			if s.Name != "" {
				items = append(items, models.ShoppingCheck{
					MealDate:   mealDate,
					MealType:   mealType,
					DishID:     dishID,
					DishName:   dishName,
					ItemName:   s.Name,
					ItemAmount: s.Amount,
				})
			}
		}
	}

	if len(items) > 0 {
		database.DB.Create(&items)
	}
}

func removeShoppingItems(dishID uint, mealType string, mealDate string) {
	database.DB.Where("dish_id = ? AND meal_type = ? AND meal_date = ?", dishID, mealType, mealDate).
		Delete(&models.ShoppingCheck{})
}

type MealRecordResponse struct {
	ID           uint      `json:"id"`
	DishID       uint      `json:"dish_id"`
	DishName     string    `json:"dish_name"`
	DishImageURL string    `json:"dish_image_url"`
	DishEmoji    string    `json:"dish_emoji"`
	MealType     string    `json:"meal_type"`
	MealDate     string    `json:"meal_date"`
	Rating       int       `json:"rating"`
	Remark       string    `json:"remark"`
	Mood         string    `json:"mood"`
	Photo        string    `json:"photo"`
	CreatedAt    time.Time `json:"created_at"`
}

var categoryEmojis = map[string]string{
	"川菜": "🌶", "湘菜": "🔥", "贵州菜": "🍲", "云南菜": "🍄", "粤菜": "🐟",
}

func isImageURL(url string) bool {
	if url == "" {
		return false
	}
	if strings.HasPrefix(url, "/uploads/") || strings.HasPrefix(url, "http") {
		return true
	}
	lower := strings.ToLower(url)
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".webp", ".gif", ".svg"} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

func getEffectiveDishImageURL(dish models.Dish) string {
	if isImageURL(dish.ImageURL) {
		return dish.ImageURL
	}
	var images []string
	if err := json.Unmarshal([]byte(dish.Images), &images); err == nil {
		for _, img := range images {
			if isImageURL(img) {
				return img
			}
		}
	}
	return ""
}

func getEffectiveDishEmoji(dish models.Dish) string {
	if emoji, ok := categoryEmojis[dish.Category]; ok {
		return emoji
	}
	if dish.ImageURL != "" && !isImageURL(dish.ImageURL) {
		return dish.ImageURL
	}
	return "🍽"
}

func GetRecords(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	query := database.DB.Model(&models.MealRecord{})

	if mealType := c.Query("meal_type"); mealType != "" {
		query = query.Where("meal_type = ?", mealType)
	}
	if dateFrom := c.Query("date_from"); dateFrom != "" {
		query = query.Where("meal_date >= ?", dateFrom)
	}
	if dateTo := c.Query("date_to"); dateTo != "" {
		query = query.Where("meal_date <= ?", dateTo)
	}

	var total int64
	query.Count(&total)

	var records []models.MealRecord
	query.Order("meal_date DESC, created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&records)

	dishIDSet := make(map[uint]bool)
	for _, r := range records {
		dishIDSet[r.DishID] = true
	}
	dishInfoMap := make(map[uint]models.Dish)
	if len(dishIDSet) > 0 {
		ids := make([]uint, 0, len(dishIDSet))
		for id := range dishIDSet {
			ids = append(ids, id)
		}
		var dishes []models.Dish
		database.DB.Unscoped().Where("id IN ?", ids).Find(&dishes)
		for _, d := range dishes {
			dishInfoMap[d.ID] = d
		}
	}

	responses := make([]MealRecordResponse, len(records))
	for i, r := range records {
		responses[i] = MealRecordResponse{
			ID:        r.ID,
			DishID:    r.DishID,
			DishName:  r.DishName,
			MealType:  r.MealType,
			MealDate:  r.MealDate,
			Rating:    r.Rating,
			Remark:    r.Remark,
			Mood:      r.Mood,
			Photo:     r.Photo,
			CreatedAt: r.CreatedAt,
		}
		if dish, ok := dishInfoMap[r.DishID]; ok {
			responses[i].DishImageURL = getEffectiveDishImageURL(dish)
			responses[i].DishEmoji = getEffectiveDishEmoji(dish)
		} else {
			responses[i].DishEmoji = "🍽"
		}
	}

	utils.SuccessPaginated(c, responses, total, page, pageSize)
}

type CreateRecordRequest struct {
	DishID   uint   `json:"dish_id" binding:"required"`
	DishName string `json:"dish_name" binding:"required"`
	MealType string `json:"meal_type" binding:"required"`
	MealDate string `json:"meal_date" binding:"required"`
	Rating   int    `json:"rating"`
	Remark   string `json:"remark"`
	Mood     string `json:"mood"`
	Photo    string `json:"photo"`
}

func CreateRecord(c *gin.Context) {
	var req CreateRecordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请填写完整信息")
		return
	}

	var existing int64
	database.DB.Model(&models.MealRecord{}).
		Where("dish_id = ? AND meal_type = ? AND meal_date = ?", req.DishID, req.MealType, req.MealDate).
		Count(&existing)
	if existing > 0 {
		utils.BadRequest(c, "该菜品已在当日该餐次中记录过")
		return
	}

	record := models.MealRecord{
		DishID:   req.DishID,
		DishName: req.DishName,
		MealType: req.MealType,
		MealDate: req.MealDate,
		Rating:   req.Rating,
		Remark:   req.Remark,
		Mood:     req.Mood,
		Photo:    req.Photo,
	}

	if err := database.DB.Create(&record).Error; err != nil {
		utils.InternalError(c, "创建记录失败")
		return
	}

	addShoppingItems(req.DishID, req.DishName, req.MealType, req.MealDate)
	services.QueueAutoAchievementSync()

	utils.Success(c, record)
}

func BatchCreateRecords(c *gin.Context) {
	type BatchRequest struct {
		Records []CreateRecordRequest `json:"records" binding:"required"`
	}
	var req BatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请求数据无效: "+err.Error())
		return
	}

	var created []models.MealRecord
	var skipped int
	for _, r := range req.Records {
		var existing int64
		database.DB.Model(&models.MealRecord{}).
			Where("dish_id = ? AND meal_type = ? AND meal_date = ?", r.DishID, r.MealType, r.MealDate).
			Count(&existing)
		if existing > 0 {
			skipped++
			continue
		}

		record := models.MealRecord{
			DishID:   r.DishID,
			DishName: r.DishName,
			MealType: r.MealType,
			MealDate: r.MealDate,
			Rating:   r.Rating,
			Remark:   r.Remark,
			Mood:     r.Mood,
			Photo:    r.Photo,
		}

		if err := database.DB.Create(&record).Error; err == nil {
			created = append(created, record)
			addShoppingItems(r.DishID, r.DishName, r.MealType, r.MealDate)
		}
	}
	if len(created) > 0 {
		services.QueueAutoAchievementSync()
	}

	utils.Success(c, gin.H{
		"created": created,
		"skipped": skipped,
		"total":   len(req.Records),
	})
}

type UpdateRecordRequest struct {
	Rating int    `json:"rating"`
	Remark string `json:"remark"`
	Mood   string `json:"mood"`
	Photo  string `json:"photo"`
}

func UpdateRecord(c *gin.Context) {
	id := c.Param("id")
	var record models.MealRecord
	if err := database.DB.First(&record, id).Error; err != nil {
		utils.NotFound(c, "记录不存在")
		return
	}

	var req UpdateRecordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请求数据无效")
		return
	}

	record.Rating = req.Rating
	record.Remark = req.Remark
	record.Mood = req.Mood
	record.Photo = req.Photo
	database.DB.Save(&record)
	services.QueueAutoAchievementSync()
	utils.Success(c, record)
}

func DeleteRecord(c *gin.Context) {
	id := c.Param("id")
	var record models.MealRecord
	if err := database.DB.First(&record, id).Error; err != nil {
		utils.NotFound(c, "记录不存在")
		return
	}
	if err := database.DB.Delete(&record).Error; err != nil {
		utils.InternalError(c, "删除记录失败")
		return
	}

	removeShoppingItems(record.DishID, record.MealType, record.MealDate)
	services.QueueAutoAchievementSync()

	utils.Success(c, nil)
}

func GetStats(c *gin.Context) {
	var totalRecords int64
	database.DB.Model(&models.MealRecord{}).Count(&totalRecords)

	var totalDishes int64
	database.DB.Model(&models.Dish{}).Where("enabled = ?", true).Count(&totalDishes)

	type TopDish struct {
		DishName string `json:"dish_name"`
		Count    int64  `json:"count"`
	}
	var topDishes []TopDish
	database.DB.Model(&models.MealRecord{}).
		Select("dish_name, count(*) as count").
		Group("dish_id, dish_name").
		Order("count DESC").
		Limit(5).
		Find(&topDishes)

	var lunchCount int64
	database.DB.Model(&models.MealRecord{}).Where("meal_type = ?", "lunch").Count(&lunchCount)

	var dinnerCount int64
	database.DB.Model(&models.MealRecord{}).Where("meal_type = ?", "dinner").Count(&dinnerCount)

	utils.Success(c, gin.H{
		"total_records": totalRecords,
		"total_dishes":  totalDishes,
		"lunch_count":   lunchCount,
		"dinner_count":  dinnerCount,
		"top_dishes":    topDishes,
	})
}

func GetDayRating(c *gin.Context) {
	date := c.Query("date")
	if date == "" {
		utils.BadRequest(c, "请提供日期")
		return
	}
	var rating models.DayRating
	if err := database.DB.Where("meal_date = ?", date).First(&rating).Error; err != nil {
		utils.Success(c, nil)
		return
	}
	utils.Success(c, rating)
}

func GetDayRatings(c *gin.Context) {
	dateFrom := c.Query("date_from")
	dateTo := c.Query("date_to")
	if dateFrom == "" || dateTo == "" {
		utils.BadRequest(c, "请提供日期范围")
		return
	}

	var ratings []models.DayRating
	database.DB.Where("meal_date >= ? AND meal_date <= ?", dateFrom, dateTo).
		Order("meal_date ASC").
		Find(&ratings)

	utils.Success(c, ratings)
}

type CreateDayRatingRequest struct {
	MealDate string `json:"meal_date" binding:"required"`
	Mood     string `json:"mood"`
	Remark   string `json:"remark"`
	Photos   string `json:"photos"`
}

type UpdateHomeMoodRequest struct {
	MealDate string `json:"meal_date" binding:"required"`
	HomeMood string `json:"home_mood" binding:"required"`
}

func CreateOrUpdateHomeMood(c *gin.Context) {
	var req UpdateHomeMoodRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请选择心情")
		return
	}

	var rating models.DayRating
	result := database.DB.Where("meal_date = ?", req.MealDate).First(&rating)
	if result.Error != nil {
		rating = models.DayRating{
			MealDate: req.MealDate,
			HomeMood: req.HomeMood,
			Photos:   "[]",
		}
		if err := database.DB.Create(&rating).Error; err != nil {
			utils.InternalError(c, "保存心情失败")
			return
		}
	} else {
		rating.HomeMood = req.HomeMood
		if rating.Photos == "" {
			rating.Photos = "[]"
		}
		database.DB.Save(&rating)
	}

	services.QueueAutoAchievementSync()
	utils.Success(c, rating)
}

func CreateOrUpdateDayRating(c *gin.Context) {
	var req CreateDayRatingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请填写完整信息")
		return
	}

	photos := req.Photos
	if photos == "" {
		photos = "[]"
	}

	var rating models.DayRating
	result := database.DB.Where("meal_date = ?", req.MealDate).First(&rating)
	if result.Error != nil {
		rating = models.DayRating{
			MealDate: req.MealDate,
			Mood:     req.Mood,
			Remark:   req.Remark,
			Photos:   photos,
		}
		if err := database.DB.Create(&rating).Error; err != nil {
			utils.InternalError(c, "保存评价失败")
			return
		}
	} else {
		if req.Mood != "" {
			rating.Mood = req.Mood
		}
		rating.Remark = req.Remark
		rating.Photos = photos
		database.DB.Save(&rating)
	}

	services.QueueAutoAchievementSync()
	utils.Success(c, rating)
}

// PhotoWallDay 照片墙的一天：当日照片 + 当日整体评价 + 当日各菜品评价
type PhotoWallDay struct {
	MealDate   string            `json:"meal_date"`
	Photos     []string          `json:"photos"`
	HomeMood   string            `json:"home_mood"`
	DayMood    string            `json:"day_mood"`
	DayRemark  string            `json:"day_remark"`
	Records    []PhotoWallRecord `json:"records"`
	PhotoCount int               `json:"photo_count"`
}

type PhotoWallRecord struct {
	DishID   uint   `json:"dish_id"`
	DishName string `json:"dish_name"`
	MealType string `json:"meal_type"`
	Mood     string `json:"mood"`
	Remark   string `json:"remark"`
}

// GetPhotoWall 返回所有留下照片的日子，按日期倒序，组成"回忆照片墙"
func GetPhotoWall(c *gin.Context) {
	var ratings []models.DayRating
	database.DB.Where("photos IS NOT NULL AND photos != '' AND photos != '[]'").
		Order("meal_date DESC").
		Find(&ratings)

	// 一次性取出所有相关日期的菜品记录，避免 N+1 查询
	dates := make([]string, 0, len(ratings))
	for _, r := range ratings {
		dates = append(dates, r.MealDate)
	}

	recordsByDate := make(map[string][]PhotoWallRecord)
	if len(dates) > 0 {
		var records []models.MealRecord
		database.DB.Where("meal_date IN ?", dates).
			Order("meal_type ASC, created_at ASC").
			Find(&records)
		for _, rec := range records {
			recordsByDate[rec.MealDate] = append(recordsByDate[rec.MealDate], PhotoWallRecord{
				DishID:   rec.DishID,
				DishName: rec.DishName,
				MealType: rec.MealType,
				Mood:     rec.Mood,
				Remark:   rec.Remark,
			})
		}
	}

	days := make([]PhotoWallDay, 0, len(ratings))
	for _, r := range ratings {
		var photos []string
		if err := json.Unmarshal([]byte(r.Photos), &photos); err != nil {
			continue
		}
		if len(photos) == 0 {
			continue
		}
		recs := recordsByDate[r.MealDate]
		if recs == nil {
			recs = []PhotoWallRecord{}
		}
		days = append(days, PhotoWallDay{
			MealDate:   r.MealDate,
			Photos:     photos,
			HomeMood:   r.HomeMood,
			DayMood:    r.Mood,
			DayRemark:  r.Remark,
			Records:    recs,
			PhotoCount: len(photos),
		})
	}

	var totalPhotos int
	for _, d := range days {
		totalPhotos += d.PhotoCount
	}

	utils.Success(c, gin.H{
		"days":         days,
		"total_days":   len(days),
		"total_photos": totalPhotos,
	})
}
