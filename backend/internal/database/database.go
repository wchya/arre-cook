package database

import (
	"encoding/json"
	"log"
	"ninimenu/internal/achievements"
	"ninimenu/internal/config"
	"ninimenu/internal/dishes"
	"ninimenu/internal/models"
	"os"
	"path/filepath"
	"strings"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// PasswordHasher 由 auth 包注入，避免 database ↔ auth 循环依赖。
var PasswordHasher func(string) (string, error)

func Init() error {
	dir := filepath.Dir(config.C.DBPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	logLevel := logger.Warn
	if config.C.IsProduction() {
		logLevel = logger.Error
	}
	var err error
	DB, err = gorm.Open(sqlite.Open(config.C.DBPath), &gorm.Config{Logger: logger.Default.LogMode(logLevel)})
	if err != nil {
		return err
	}
	if err := configureSQLite(DB); err != nil {
		return err
	}

	if err := DB.AutoMigrate(
		&models.User{},
		&models.EmailCode{},
		&models.UserPreference{},
		&models.UserSetting{},
		&models.AgentToken{},
		&models.AgentAuditLog{},
		&models.AgentSuggestion{},
		&models.ChatSession{},
		&models.ChatMessage{},
		&models.Dish{},
		&models.MealRecord{},
		&models.Favorite{},
		&models.Quote{},
		&models.Achievement{},
		&models.UserAchievement{},
		&models.AchievementEvent{},
		&models.BlindBox{},
		&models.Holiday{},
		&models.Setting{},
		&models.DayRating{},
		&models.ShoppingCheck{},
		&models.HomeInventory{},
		&models.ShoppingItemCategory{},
		&models.BehaviorEvent{},
	); err != nil {
		return err
	}
	// The old global unique index blocks two users from recording the same meal.
	if err := DB.Exec("DROP INDEX IF EXISTS idx_meal_records_unique_day").Error; err != nil {
		return err
	}

	seedData()
	retireRemovedSeedDishes()
	adminID, err := ensureBootstrapAdmin()
	if err != nil {
		return err
	}
	if err := migrateToMultiUser(adminID); err != nil {
		return err
	}
	return nil
}

// OwnedBy 个人数据查询作用域：所有带 user_id 的表都必须经过它，保证用户之间数据隔离。
func OwnedBy(uid uint) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("user_id = ?", uid)
	}
}

// VisibleDishes 菜品可见范围：公共菜谱 + 本人私有菜谱。
func VisibleDishes(uid uint) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if uid == 0 {
			return db.Where("owner_id = 0")
		}
		return db.Where("owner_id IN ?", []uint{0, uid})
	}
}

// ensureBootstrapAdmin 首次启动时创建管理员：用户名 ADMIN_USERNAME + 密码 ADMIN_PASSWORD（兜底登录），
// 若配置了 ADMIN_EMAIL 则同时绑定该邮箱，之后可直接用邮箱验证码登录。
func ensureBootstrapAdmin() (uint, error) {
	adminEmail := strings.ToLower(strings.TrimSpace(config.C.AdminEmail))
	var admin models.User
	if err := DB.Where("role = ?", models.RoleAdmin).Order("id ASC").First(&admin).Error; err == nil {
		if adminEmail != "" && admin.Email == nil {
			var taken int64
			DB.Model(&models.User{}).Where("email = ?", adminEmail).Count(&taken)
			if taken == 0 {
				DB.Model(&admin).Update("email", adminEmail)
			}
		}
		return admin.ID, nil
	}
	if PasswordHasher == nil {
		log.Fatal("database.PasswordHasher 未注入")
	}
	hash, err := PasswordHasher(config.C.AdminPassword)
	if err != nil {
		return 0, err
	}
	username := strings.TrimSpace(config.C.AdminUsername)
	var existing models.User
	if err := DB.Where("username = ?", username).First(&existing).Error; err == nil {
		DB.Model(&existing).Update("role", models.RoleAdmin)
		return existing.ID, nil
	}
	admin = models.User{Username: &username, PasswordHash: hash, Nickname: "管理员", Role: models.RoleAdmin, TokenVersion: 1}
	if adminEmail != "" {
		admin.Email = &adminEmail
	}
	if err := DB.Create(&admin).Error; err != nil {
		return 0, err
	}
	log.Printf("已创建初始管理员账号：%s（密码取自 ADMIN_PASSWORD，请登录后尽快修改）", username)
	return admin.ID, nil
}

const multiUserMigrationKey = "migration_multiuser_v1"

// migrateToMultiUser 把单用户时代的数据（全局收藏、评价、记录、事件、库存…）归属到初始管理员账号。
// 只执行一次（settings 里记标记），旧表保留不删，便于回滚核对。
func migrateToMultiUser(adminID uint) error {
	var flag int64
	DB.Model(&models.Setting{}).Where("`key` = ?", multiUserMigrationKey).Count(&flag)
	if flag > 0 {
		return nil
	}
	m := DB.Migrator()
	err := DB.Transaction(func(tx *gorm.DB) error {
		if m.HasTable("favorites") {
			if err := tx.Exec(`INSERT OR IGNORE INTO user_favorites (user_id, dish_id, created_at)
				SELECT ?, dish_id, created_at FROM favorites`, adminID).Error; err != nil {
				return err
			}
		}
		if m.HasTable("day_ratings") {
			if err := tx.Exec(`INSERT OR IGNORE INTO user_day_ratings (user_id, meal_date, home_mood, mood, remark, photos, created_at, updated_at)
				SELECT ?, meal_date, home_mood, mood, remark, photos, created_at, updated_at FROM day_ratings`, adminID).Error; err != nil {
				return err
			}
		}
		if m.HasTable("home_inventories") {
			if err := tx.Exec(`INSERT OR IGNORE INTO user_home_inventories (user_id, item_name, in_stock, created_at, updated_at)
				SELECT ?, item_name, in_stock, created_at, updated_at FROM home_inventories`, adminID).Error; err != nil {
				return err
			}
		}
		for _, table := range []string{"meal_records", "behavior_events", "shopping_checks", "achievement_events", "user_achievements"} {
			if err := tx.Exec("UPDATE "+table+" SET user_id = ? WHERE user_id = 0 OR user_id IS NULL", adminID).Error; err != nil {
				return err
			}
		}
		// 原全局开关迁移为管理员的个人设置；周菜单缓存改为按用户缓存
		var legacy []models.Setting
		tx.Where("`key` IN ?", []string{"voice_enabled", "blind_box_enabled"}).Find(&legacy)
		for _, s := range legacy {
			tx.Create(&models.UserSetting{UserID: adminID, Key: s.Key, Value: s.Value})
		}
		tx.Where("`key` = ?", "week_plan_cache").Delete(&models.Setting{})
		return tx.Create(&models.Setting{Key: multiUserMigrationKey, Value: "done"}).Error
	})
	if err == nil {
		log.Printf("多用户迁移完成：历史数据已归属到管理员账号 #%d", adminID)
	}
	return err
}

// retireRemovedSeedDishes 把历史数据库里由旧种子写入、如今已从菜单移除的公共菜品下线：
// 软删除并取消收藏。用餐记录保留（记录自带菜名）。用户私有菜谱与自传图片的菜品不受影响。
func retireRemovedSeedDishes() {
	keep := dishes.DefaultDishNameSet()

	var all []models.Dish
	DB.Select("id", "name", "image_url", "images").Where("owner_id = 0").Find(&all)

	var retired []uint
	for _, d := range all {
		if keep[d.Name] {
			continue
		}
		image := d.ImageURL
		var images []string
		if json.Unmarshal([]byte(d.Images), &images) == nil && len(images) > 0 && image == "" {
			image = images[0]
		}
		switch {
		case dishes.IsRetiredSeedImage(image):
			retired = append(retired, d.ID)
		case dishes.IsSeedImage(image) && !strings.Contains(d.Name, "副本"):
			retired = append(retired, d.ID)
		}
	}
	if len(retired) == 0 {
		return
	}
	DB.Where("dish_id IN ?", retired).Delete(&models.Favorite{})
	DB.Where("id IN ?", retired).Delete(&models.Dish{})
	DB.Where("`key` = ?", "week_plan_cache").Delete(&models.UserSetting{})
	log.Printf("已下线 %d 道不在当前菜单中的旧种子菜品", len(retired))
}

func configureSQLite(db *gorm.DB) error {
	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA temp_store = MEMORY",
	}
	for _, pragma := range pragmas {
		if err := db.Exec(pragma).Error; err != nil {
			return err
		}
	}
	return nil
}

func seedData() {
	var count int64
	DB.Model(&models.Setting{}).Count(&count)
	if count == 0 {
		DB.Create(&models.Setting{Key: "repeat_days", Value: "3"})
		DB.Create(&models.Setting{Key: "pick_animation", Value: "wheel"})
	}

	DB.Model(&models.Quote{}).Count(&count)
	if count == 0 {
		quotes := []models.Quote{
			{Content: "中午来点硬菜，下午才有力气搬砖", Scene: "lunch"},
			{Content: "今晚吃顿好的，犒劳一下辛苦的你", Scene: "dinner"},
			{Content: "15分钟搞定，比外卖还快！", Scene: "quick"},
			{Content: "喝碗热汤，暖暖的，很贴心", Scene: "soup"},
			{Content: "你的心头好又来啦~", Scene: "favorite"},
			{Content: "今天辛苦了，吃点好的犒劳自己", Scene: "lunch"},
			{Content: "简单做做，好吃就行", Scene: "quick"},
			{Content: "换个口味，换种心情", Scene: "dinner"},
			{Content: "这道菜，吃了会开心的", Scene: "lunch"},
			{Content: "美食是最好的治愈", Scene: "dinner"},
			{Content: "尝尝这道，保准你满意", Scene: "lunch"},
			{Content: "下班了，来顿丰盛的", Scene: "dinner"},
			{Content: "好久没吃这个了吧？来一个！", Scene: "favorite"},
			{Content: "今天有点累？来道简单的", Scene: "quick"},
			{Content: "周末就该吃点好的", Scene: "dinner"},
		}
		DB.Create(&quotes)
	}

	for _, a := range achievements.DefaultAchievements() {
		var existing models.Achievement
		if err := DB.Where("code = ?", a.Code).First(&existing).Error; err == nil {
			if existing.Condition == "" || existing.Condition == "manual" {
				DB.Model(&existing).Update("condition", a.Condition)
			}
			continue
		}
		DB.Create(&a)
	}
	// 已随菜系裁剪下线的旧成就（家常菜/汤品/主食/小食等）连同解锁记录一并移除
	if retired := achievements.RetiredCodes(); len(retired) > 0 {
		var ids []uint
		DB.Model(&models.Achievement{}).Where("code IN ? AND `condition` = ?", retired, "auto").Pluck("id", &ids)
		if len(ids) > 0 {
			DB.Where("achievement_id IN ?", ids).Delete(&models.UserAchievement{})
			DB.Where("id IN ?", ids).Delete(&models.Achievement{})
		}
	}

	for _, dish := range dishes.DefaultDishes() {
		var existing models.Dish
		if err := DB.Unscoped().Where("name = ? AND owner_id = 0", dish.Name).First(&existing).Error; err == nil {
			// 曾被下线的种子菜品重新回到菜单：恢复而不是重复创建
			if existing.DeletedAt.Valid {
				DB.Unscoped().Model(&existing).Update("deleted_at", nil)
			}
			continue
		}
		DB.Create(&dish)
	}

	ensureSetting("lunch_dishes_per_day", "1")
	ensureSetting("dinner_dishes_per_day", "1")
	ensureSetting("allow_register", "1")

	upsertManagedStringArraySetting("categories", dishes.DefaultCategories())
	upsertStringArraySetting("tastes", dishes.DefaultTastes())
}

func ensureSetting(key, value string) {
	var n int64
	DB.Model(&models.Setting{}).Where("`key` = ?", key).Count(&n)
	if n == 0 {
		DB.Create(&models.Setting{Key: key, Value: value})
	}
}

// GetSetting 读取站点级设置，不存在返回 fallback。
func GetSetting(key, fallback string) string {
	var s models.Setting
	if err := DB.Where("`key` = ?", key).First(&s).Error; err != nil {
		return fallback
	}
	if strings.TrimSpace(s.Value) == "" {
		return fallback
	}
	return s.Value
}

func SetSetting(key, value string) error {
	var s models.Setting
	if err := DB.Where("`key` = ?", key).First(&s).Error; err == nil {
		return DB.Model(&models.Setting{}).Where("`key` = ?", key).Update("value", value).Error
	}
	return DB.Create(&models.Setting{Key: key, Value: value}).Error
}

// GetUserSetting / SetUserSetting 用户级设置。
func GetUserSetting(uid uint, key, fallback string) string {
	var s models.UserSetting
	if err := DB.Where("user_id = ? AND `key` = ?", uid, key).First(&s).Error; err != nil {
		return fallback
	}
	return s.Value
}

func SetUserSetting(uid uint, key, value string) error {
	res := DB.Model(&models.UserSetting{}).Where("user_id = ? AND `key` = ?", uid, key).Update("value", value)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return DB.Create(&models.UserSetting{UserID: uid, Key: key, Value: value}).Error
	}
	return nil
}

func jsonArrayString(values []string) string {
	data, err := json.Marshal(values)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func upsertStringArraySetting(key string, defaults []string) {
	var setting models.Setting
	if err := DB.Where("`key` = ?", key).First(&setting).Error; err != nil {
		DB.Create(&models.Setting{Key: key, Value: jsonArrayString(defaults)})
		return
	}

	existing := parseStringArray(setting.Value)
	merged := appendMissingStringValues(existing, defaults)
	if !sameStringValues(existing, merged) {
		DB.Model(&models.Setting{}).Where("`key` = ?", key).Update("value", jsonArrayString(merged))
	}
}

func upsertManagedStringArraySetting(key string, defaults []string) {
	var setting models.Setting
	if err := DB.Where("`key` = ?", key).First(&setting).Error; err != nil {
		DB.Create(&models.Setting{Key: key, Value: jsonArrayString(defaults)})
		return
	}

	allowed := stringSet(defaults)
	existing := parseStringArray(setting.Value)
	managed := make([]string, 0, len(defaults))
	for _, value := range existing {
		if allowed[value] {
			managed = append(managed, value)
		}
	}
	managed = appendMissingStringValues(managed, defaults)
	if !sameStringValues(existing, managed) {
		DB.Model(&models.Setting{}).Where("`key` = ?", key).Update("value", jsonArrayString(managed))
	}
}

func parseStringArray(raw string) []string {
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return []string{}
	}
	return values
}

func appendMissingStringValues(existing []string, defaults []string) []string {
	seen := make(map[string]bool, len(existing)+len(defaults))
	result := make([]string, 0, len(existing)+len(defaults))
	for _, value := range existing {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	for _, value := range defaults {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func stringSet(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		if value != "" {
			result[value] = true
		}
	}
	return result
}

func sameStringValues(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
