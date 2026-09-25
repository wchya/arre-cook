package models

import "time"

// Notification 用户站内信。每一行只属于一个用户；系统广播在发布时展开成用户行，
// 因此读取、已读状态和删除都能沿用 user_id 隔离规则。
type Notification struct {
	ID        uint       `json:"id" gorm:"primaryKey"`
	UserID    uint       `json:"-" gorm:"not null;index:idx_notifications_user_read,priority:1"`
	Type      string     `json:"type" gorm:"size:32;not null;index"`
	Title     string     `json:"title" gorm:"size:128;not null"`
	Content   string     `json:"content" gorm:"not null"`
	Link      string     `json:"link" gorm:"size:255"`
	ReadAt    *time.Time `json:"read_at" gorm:"index:idx_notifications_user_read,priority:2"`
	CreatedAt time.Time  `json:"created_at" gorm:"index"`
}
