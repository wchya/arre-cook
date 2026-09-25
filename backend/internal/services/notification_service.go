package services

import (
	"errors"
	"strings"
	"time"

	"ninimenu/internal/database"
	"ninimenu/internal/models"

	"gorm.io/gorm"
)

var notificationTypes = map[string]bool{
	"system_update":  true,
	"feature":        true,
	"maintenance":    true,
	"health_tip":     true,
	"agent_security": true,
	"family":         true,
}

func NormalizeNotificationType(raw string) string {
	t := strings.TrimSpace(raw)
	if notificationTypes[t] {
		return t
	}
	return "system_update"
}

func validateNotification(title, content string) error {
	if strings.TrimSpace(title) == "" || len([]rune(title)) > 128 {
		return errors.New("通知标题不能为空且不超过 128 个字")
	}
	if strings.TrimSpace(content) == "" || len([]rune(content)) > 5000 {
		return errors.New("通知内容不能为空且不超过 5000 个字")
	}
	return nil
}

// CreateNotification 写入一条只属于 uid 的站内信。
func CreateNotification(uid uint, kind, title, content, link string) (*models.Notification, error) {
	if uid == 0 {
		return nil, errors.New("缺少通知接收人")
	}
	title = strings.TrimSpace(title)
	content = strings.TrimSpace(content)
	link = strings.TrimSpace(link)
	if len([]rune(link)) > 255 {
		link = string([]rune(link)[:255])
	}
	if err := validateNotification(title, content); err != nil {
		return nil, err
	}
	n := &models.Notification{UserID: uid, Type: NormalizeNotificationType(kind), Title: title, Content: content, Link: link}
	if err := database.DB.Create(n).Error; err != nil {
		return nil, err
	}
	return n, nil
}

// PublishNotification 发布给指定用户；uid=0 表示向全部正常用户广播。
func PublishNotification(uid uint, kind, title, content, link string) (int64, error) {
	if err := validateNotification(strings.TrimSpace(title), strings.TrimSpace(content)); err != nil {
		return 0, err
	}
	if uid != 0 {
		if _, err := CreateNotification(uid, kind, title, content, link); err != nil {
			return 0, err
		}
		return 1, nil
	}
	var ids []uint
	if err := database.DB.Model(&models.User{}).Where("disabled = ?", false).Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		rows := make([]models.Notification, 0, len(ids))
		for _, id := range ids {
			rows = append(rows, models.Notification{UserID: id, Type: NormalizeNotificationType(kind), Title: strings.TrimSpace(title), Content: strings.TrimSpace(content), Link: strings.TrimSpace(link)})
		}
		return tx.Create(&rows).Error
	})
	return int64(len(ids)), err
}

type NotificationPage struct {
	Items    []models.Notification `json:"items"`
	Total    int64                 `json:"total"`
	Unread   int64                 `json:"unread"`
	Page     int                   `json:"page"`
	PageSize int                   `json:"page_size"`
}

func ListNotifications(uid uint, unreadOnly bool, page, pageSize int) NotificationPage {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	q := database.DB.Model(&models.Notification{}).Scopes(database.OwnedBy(uid))
	var total, unread int64
	q.Count(&total)
	database.DB.Model(&models.Notification{}).Scopes(database.OwnedBy(uid)).Where("read_at IS NULL").Count(&unread)
	if unreadOnly {
		q = q.Where("read_at IS NULL")
	}
	var items []models.Notification
	q.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&items)
	if items == nil {
		items = []models.Notification{}
	}
	return NotificationPage{Items: items, Total: total, Unread: unread, Page: page, PageSize: pageSize}
}

func MarkNotificationRead(uid uint, id any) error {
	now := time.Now()
	res := database.DB.Model(&models.Notification{}).Scopes(database.OwnedBy(uid)).Where("id = ?", id).Where("read_at IS NULL").Update("read_at", now)
	if res.RowsAffected == 0 {
		var exists int64
		database.DB.Model(&models.Notification{}).Scopes(database.OwnedBy(uid)).Where("id = ?", id).Count(&exists)
		if exists == 0 {
			return gorm.ErrRecordNotFound
		}
	}
	return nil
}

func MarkAllNotificationsRead(uid uint) error {
	return database.DB.Model(&models.Notification{}).Scopes(database.OwnedBy(uid)).Where("read_at IS NULL").Update("read_at", time.Now()).Error
}
