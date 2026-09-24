package models

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

type Dish struct {
	ID          uint           `json:"id" gorm:"primaryKey"`
	Name        string         `json:"name" gorm:"not null"`
	ImageURL    string         `json:"image_url"`
	Images      string         `json:"images" gorm:"default:'[]'"`
	VideoURL    string         `json:"video_url"`
	Category    string         `json:"category" gorm:"index"`
	MealType    string         `json:"meal_type" gorm:"default:'all';index;index:idx_dishes_pick,priority:2"`
	Taste       string         `json:"taste"`
	Ingredients string         `json:"ingredients" gorm:"default:'[]'"`
	Seasonings  string         `json:"seasonings" gorm:"default:'[]'"`
	Steps       string         `json:"steps" gorm:"default:'[]'"`
	CookTime    int            `json:"cook_time" gorm:"default:0"`
	Difficulty  string         `json:"difficulty" gorm:"default:'easy'"`
	Remark      string         `json:"remark"`
	// Favorite 是“当前用户是否收藏”的视图字段，不落库（收藏按用户存在 user_favorites），
	// 由 services.MarkFavorites 在返回前按当前用户填充。
	Favorite bool `json:"favorite" gorm:"-"`
	// OwnerID=0 为公共菜谱（管理员维护，所有人可见）；否则为该用户的私有菜谱，仅本人可见。
	OwnerID     uint           `json:"owner_id" gorm:"not null;default:0;index"`
	Enabled     bool           `json:"enabled" gorm:"default:true;index;index:idx_dishes_pick,priority:1"`
	Tags        string         `json:"tags" gorm:"default:'[]'"`
	SortOrder   int            `json:"sort_order" gorm:"default:0"`
	DeletedAt   gorm.DeletedAt `json:"-" gorm:"index;index:idx_dishes_pick,priority:3"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// jsonField 把数据库里存的 JSON 字符串转成可直接内联的 JSON 片段。
// 合法 JSON 原样输出（数组/对象不再被转义为字符串）；空值或非法内容回退为 []。
func jsonField(s string) json.RawMessage {
	if s == "" {
		return json.RawMessage("[]")
	}
	if json.Valid([]byte(s)) {
		return json.RawMessage(s)
	}
	return json.RawMessage("[]")
}

// MarshalJSON 让 images/ingredients/seasonings/steps/tags 五个字段在 API 响应中
// 输出为真正的 JSON 数组/对象，而不是被转义的字符串。数据库仍以 TEXT 存储，写入路径不变。
func (d Dish) MarshalJSON() ([]byte, error) {
	type alias Dish // 借助别名避免无限递归
	return json.Marshal(&struct {
		alias
		Images      json.RawMessage `json:"images"`
		Ingredients json.RawMessage `json:"ingredients"`
		Seasonings  json.RawMessage `json:"seasonings"`
		Steps       json.RawMessage `json:"steps"`
		Tags        json.RawMessage `json:"tags"`
	}{
		alias:       alias(d),
		Images:      jsonField(d.Images),
		Ingredients: jsonField(d.Ingredients),
		Seasonings:  jsonField(d.Seasonings),
		Steps:       jsonField(d.Steps),
		Tags:        jsonField(d.Tags),
	})
}
