package handlers

import (
	"encoding/json"
	"errors"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"ninimenu/internal/services"
	"ninimenu/internal/storage"
	"ninimenu/internal/utils"
	"time"

	"github.com/gin-gonic/gin"
)

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

func getEffectiveDishEmoji(dish models.Dish) string {
	if emoji, ok := categoryEmojis[dish.Category]; ok {
		return emoji
	}
	if dish.ImageURL != "" && services.DishImageURL(models.Dish{ImageURL: dish.ImageURL}) == "" {
		return dish.ImageURL
	}
	return "🍽"
}

func GetRecords(c *gin.Context) {
	page, pageSize := pageParams(c, 20)
	query := database.DB.Model(&models.MealRecord{}).Scopes(database.OwnedBy(uid(c)))

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
	query.Order("meal_date DESC, created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&records)

	ids := make([]uint, 0, len(records))
	seen := map[uint]bool{}
	for _, r := range records {
		if !seen[r.DishID] {
			seen[r.DishID] = true
			ids = append(ids, r.DishID)
		}
	}
	dishInfoMap := make(map[uint]models.Dish)
	if len(ids) > 0 {
		var dishes []models.Dish
		database.DB.Unscoped().Scopes(database.VisibleDishes(uid(c))).Where("id IN ?", ids).Find(&dishes)
		for _, d := range dishes {
			dishInfoMap[d.ID] = d
		}
	}

	responses := make([]MealRecordResponse, len(records))
	for i, r := range records {
		responses[i] = MealRecordResponse{
			ID: r.ID, DishID: r.DishID, DishName: r.DishName, MealType: r.MealType, MealDate: r.MealDate,
			Rating: r.Rating, Remark: r.Remark, Mood: r.Mood, Photo: r.Photo, CreatedAt: r.CreatedAt, DishEmoji: "🍽",
		}
		if dish, ok := dishInfoMap[r.DishID]; ok {
			responses[i].DishImageURL = services.DishImageURL(dish)
			responses[i].DishEmoji = getEffectiveDishEmoji(dish)
		}
	}
	utils.SuccessPaginated(c, responses, total, page, pageSize)
}

type CreateRecordRequest struct {
	DishID   uint   `json:"dish_id" binding:"required"`
	DishName string `json:"dish_name"`
	MealType string `json:"meal_type" binding:"required"`
	MealDate string `json:"meal_date"`
	Rating   int    `json:"rating"`
	Remark   string `json:"remark"`
	Mood     string `json:"mood"`
	Photo    string `json:"photo"`
}

func (r CreateRecordRequest) input() services.MealInput {
	return services.MealInput{DishID: r.DishID, MealType: r.MealType, MealDate: r.MealDate, Rating: r.Rating, Remark: r.Remark, Mood: r.Mood, Photo: r.Photo}
}

func recordError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrDishNotFound), errors.Is(err, services.ErrRecordNotFound):
		utils.NotFound(c, err.Error())
	case errors.Is(err, services.ErrDuplicateMeal), errors.Is(err, services.ErrInvalidMealType), errors.Is(err, services.ErrInvalidDate):
		utils.BadRequest(c, err.Error())
	default:
		utils.InternalError(c, "操作失败")
	}
}

func CreateRecord(c *gin.Context) {
	var req CreateRecordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请填写完整信息")
		return
	}
	record, err := services.CreateMealRecord(uid(c), req.input(), "app", "")
	if err != nil {
		recordError(c, err)
		return
	}
	utils.Success(c, record)
}

func BatchCreateRecords(c *gin.Context) {
	var req struct {
		Records []CreateRecordRequest `json:"records" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Records) > 50 {
		utils.BadRequest(c, "请求数据无效")
		return
	}
	created := make([]models.MealRecord, 0, len(req.Records))
	skipped := 0
	for _, r := range req.Records {
		record, err := services.CreateMealRecord(uid(c), r.input(), "app", "")
		if err != nil {
			skipped++
			continue
		}
		created = append(created, *record)
	}
	utils.Success(c, gin.H{"created": created, "skipped": skipped, "total": len(req.Records)})
}

func UpdateRecord(c *gin.Context) {
	var req struct {
		Rating int    `json:"rating"`
		Remark string `json:"remark"`
		Mood   string `json:"mood"`
		Photo  string `json:"photo"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请求数据无效")
		return
	}
	record, err := services.UpdateMealRecord(uid(c), c.Param("id"), services.MealPatch{Rating: &req.Rating, Remark: &req.Remark, Mood: &req.Mood, Photo: &req.Photo})
	if err != nil {
		recordError(c, err)
		return
	}
	utils.Success(c, record)
}

func DeleteRecord(c *gin.Context) {
	if _, err := services.DeleteMealRecord(uid(c), c.Param("id")); err != nil {
		recordError(c, err)
		return
	}
	utils.Success(c, nil)
}

// GetStats 个人统计。
func GetStats(c *gin.Context) {
	utils.Success(c, services.BuildUserStats(uid(c)))
}

func GetDayRating(c *gin.Context) {
	date := c.Query("date")
	if date == "" {
		utils.BadRequest(c, "请提供日期")
		return
	}
	var rating models.DayRating
	if err := database.DB.Scopes(database.OwnedBy(uid(c))).Where("meal_date = ?", date).First(&rating).Error; err != nil {
		utils.Success(c, nil)
		return
	}
	utils.Success(c, rating)
}

func GetDayRatings(c *gin.Context) {
	dateFrom, dateTo := c.Query("date_from"), c.Query("date_to")
	if dateFrom == "" || dateTo == "" {
		utils.BadRequest(c, "请提供日期范围")
		return
	}
	var ratings []models.DayRating
	database.DB.Scopes(database.OwnedBy(uid(c))).Where("meal_date >= ? AND meal_date <= ?", dateFrom, dateTo).
		Order("meal_date ASC").Find(&ratings)
	utils.Success(c, ratings)
}

func findOrNewDayRating(u uint, date string) models.DayRating {
	var rating models.DayRating
	if err := database.DB.Scopes(database.OwnedBy(u)).Where("meal_date = ?", date).First(&rating).Error; err != nil {
		return models.DayRating{UserID: u, MealDate: date, Photos: "[]"}
	}
	if rating.Photos == "" {
		rating.Photos = "[]"
	}
	return rating
}

func CreateOrUpdateHomeMood(c *gin.Context) {
	var req struct {
		MealDate string `json:"meal_date" binding:"required"`
		HomeMood string `json:"home_mood" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请选择心情")
		return
	}
	rating := findOrNewDayRating(uid(c), req.MealDate)
	rating.HomeMood = req.HomeMood
	if err := database.DB.Save(&rating).Error; err != nil {
		utils.InternalError(c, "保存心情失败")
		return
	}
	services.QueueAutoAchievementSync(uid(c))
	utils.Success(c, rating)
}

func CreateOrUpdateDayRating(c *gin.Context) {
	var req struct {
		MealDate string `json:"meal_date" binding:"required"`
		Mood     string `json:"mood"`
		Remark   string `json:"remark"`
		Photos   string `json:"photos"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请填写完整信息")
		return
	}
	photos := jsonOr(req.Photos, "[]")
	var list []string
	if json.Unmarshal([]byte(photos), &list) != nil || len(list) > 18 {
		utils.BadRequest(c, "照片最多 18 张")
		return
	}
	for _, p := range list {
		if !storage.IsUploadURL(p) {
			utils.BadRequest(c, "照片地址无效")
			return
		}
	}
	rating := findOrNewDayRating(uid(c), req.MealDate)
	if req.Mood != "" {
		rating.Mood = req.Mood
	}
	rating.Remark = req.Remark
	rating.Photos = photos
	if err := database.DB.Save(&rating).Error; err != nil {
		utils.InternalError(c, "保存评价失败")
		return
	}
	services.QueueAutoAchievementSync(uid(c))
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

// GetPhotoWall 当前用户所有留下照片的日子，按日期倒序。
func GetPhotoWall(c *gin.Context) {
	own := database.OwnedBy(uid(c))
	var ratings []models.DayRating
	database.DB.Scopes(own).Where("photos IS NOT NULL AND photos != '' AND photos != '[]'").
		Order("meal_date DESC").Find(&ratings)

	dates := make([]string, 0, len(ratings))
	for _, r := range ratings {
		dates = append(dates, r.MealDate)
	}
	recordsByDate := make(map[string][]PhotoWallRecord)
	if len(dates) > 0 {
		var records []models.MealRecord
		database.DB.Scopes(own).Where("meal_date IN ?", dates).Order("meal_type ASC, created_at ASC").Find(&records)
		for _, rec := range records {
			recordsByDate[rec.MealDate] = append(recordsByDate[rec.MealDate], PhotoWallRecord{
				DishID: rec.DishID, DishName: rec.DishName, MealType: rec.MealType, Mood: rec.Mood, Remark: rec.Remark,
			})
		}
	}

	days := make([]PhotoWallDay, 0, len(ratings))
	totalPhotos := 0
	for _, r := range ratings {
		var photos []string
		if err := json.Unmarshal([]byte(r.Photos), &photos); err != nil || len(photos) == 0 {
			continue
		}
		recs := recordsByDate[r.MealDate]
		if recs == nil {
			recs = []PhotoWallRecord{}
		}
		totalPhotos += len(photos)
		days = append(days, PhotoWallDay{
			MealDate: r.MealDate, Photos: photos, HomeMood: r.HomeMood, DayMood: r.Mood,
			DayRemark: r.Remark, Records: recs, PhotoCount: len(photos),
		})
	}
	utils.Success(c, gin.H{"days": days, "total_days": len(days), "total_photos": totalPhotos})
}
