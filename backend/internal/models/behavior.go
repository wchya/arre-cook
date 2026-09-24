package models

import "time"

// BehaviorEvent 用户与智能体的行为事件流：浏览菜品、推荐、采纳、拒绝等。
// 与用餐记录（MealRecord）一起构成推荐分析所需的“用户行为信息”，按 user_id 隔离，
// 通过 /api/agent/behavior 对持有该用户令牌的智能体开放读写。
type BehaviorEvent struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	UserID    uint      `json:"-" gorm:"not null;default:0;index:idx_behavior_user_time,priority:1"`
	EventType string    `json:"event_type" gorm:"not null;index"`
	DishID    uint      `json:"dish_id" gorm:"index"`
	DishName  string    `json:"dish_name"`
	Source    string    `json:"source" gorm:"index"`
	Actor     string    `json:"actor"`
	Meta      string    `json:"meta" gorm:"default:'{}'"`
	CreatedAt time.Time `json:"created_at" gorm:"index;index:idx_behavior_user_time,priority:2"`
}
