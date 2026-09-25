package models

import "time"

// Favorite 用户收藏。多用户改造后换新表 user_favorites（旧表 favorites 的 dish_id 全局唯一约束
// 在 SQLite 上无法原地改成 (user_id, dish_id) 组合唯一），启动时自动把旧数据迁移过来。
type Favorite struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	UserID    uint      `json:"user_id" gorm:"not null;uniqueIndex:idx_user_fav_user_dish,priority:1"`
	DishID    uint      `json:"dish_id" gorm:"not null;uniqueIndex:idx_user_fav_user_dish,priority:2;index"`
	CreatedAt time.Time `json:"created_at"`
}

func (Favorite) TableName() string { return "user_favorites" }

type Quote struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Content   string    `json:"content" gorm:"not null"`
	Scene     string    `json:"scene"`
	Enabled   bool      `json:"enabled" gorm:"default:true"`
	CreatedAt time.Time `json:"created_at"`
}

type Achievement struct {
	ID          uint       `json:"id" gorm:"primaryKey"`
	Code        string     `json:"code" gorm:"not null;unique"`
	Name        string     `json:"name" gorm:"not null"`
	Description string     `json:"description" gorm:"not null"`
	Icon        string     `json:"icon"`
	Condition   string     `json:"condition" gorm:"not null"`
	UnlockedAt  *time.Time `json:"unlocked_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

type UserAchievement struct {
	ID            uint      `json:"id" gorm:"primaryKey"`
	UserID        uint      `json:"user_id" gorm:"not null;default:0;index:idx_user_ach_user_ach,priority:1"`
	AchievementID uint      `json:"achievement_id" gorm:"not null;index;index:idx_user_ach_user_ach,priority:2"`
	UnlockedAt    time.Time `json:"unlocked_at"`
}

type AchievementEvent struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	UserID    uint      `json:"user_id" gorm:"not null;default:0;index:idx_ach_event_user_type,priority:1"`
	EventType string    `json:"event_type" gorm:"not null;size:32;index;index:idx_ach_event_user_type,priority:2"`
	RefKey    string    `json:"ref_key" gorm:"size:128;index"`
	CreatedAt time.Time `json:"created_at" gorm:"index"`
}

type BlindBox struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	DishID    uint      `json:"dish_id" gorm:"not null"`
	Hint      string    `json:"hint"`
	Active    bool      `json:"active" gorm:"default:true"`
	StartDate string    `json:"start_date"`
	EndDate   string    `json:"end_date"`
	CreatedAt time.Time `json:"created_at"`
}

type Holiday struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Name      string    `json:"name" gorm:"not null"`
	Date      string    `json:"date" gorm:"not null"`
	DishIDs   string    `json:"dish_ids" gorm:"default:'[]'"`
	Greeting  string    `json:"greeting"`
	CreatedAt time.Time `json:"created_at"`
}

// Setting 站点级设置（应用名、菜系/口味字典、AI 模型配置等），由管理员维护。
type Setting struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Key       string    `json:"key" gorm:"not null;unique"`
	Value     string    `json:"value"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DayRating 每日整体评价 + 首页心情 + 当日照片。新表 user_day_ratings，唯一键 (user_id, meal_date)。
type DayRating struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	UserID    uint      `json:"-" gorm:"not null;uniqueIndex:idx_user_day_rating_user_date,priority:1"`
	MealDate  string    `json:"meal_date" gorm:"not null;size:10;uniqueIndex:idx_user_day_rating_user_date,priority:2"`
	HomeMood  string    `json:"home_mood" gorm:"size:64;index"`
	Mood      string    `json:"mood" gorm:"size:64;index"`
	Remark    string    `json:"remark"`
	Photos    string    `json:"photos" gorm:"default:'[]'"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (DayRating) TableName() string { return "user_day_ratings" }

type ShoppingCheck struct {
	ID         uint      `json:"id" gorm:"primaryKey"`
	UserID     uint      `json:"-" gorm:"not null;default:0;index:idx_shopping_user_date,priority:1"`
	MealDate   string    `json:"meal_date" gorm:"not null;size:10;index;index:idx_shopping_checks_date_checked,priority:1;index:idx_shopping_user_date,priority:2"`
	MealType   string    `json:"meal_type" gorm:"not null;size:16"`
	DishID     uint      `json:"dish_id" gorm:"not null"`
	DishName   string    `json:"dish_name"`
	ItemName   string    `json:"item_name" gorm:"not null"`
	ItemAmount string    `json:"item_amount"`
	Checked    bool      `json:"checked" gorm:"default:false;index;index:idx_shopping_checks_date_checked,priority:2"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// HomeInventory 家中常备库存。新表 user_home_inventories，唯一键 (user_id, item_name)。
type HomeInventory struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	UserID    uint      `json:"-" gorm:"not null;uniqueIndex:idx_user_inv_user_item,priority:1"`
	ItemName  string    `json:"item_name" gorm:"not null;size:80;uniqueIndex:idx_user_inv_user_item,priority:2"`
	InStock   bool      `json:"in_stock" gorm:"default:true;index"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (HomeInventory) TableName() string { return "user_home_inventories" }

type ShoppingItemCategory struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	ItemName  string    `json:"item_name" gorm:"not null;size:80;uniqueIndex"`
	Category  string    `json:"category" gorm:"not null;size:40"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
