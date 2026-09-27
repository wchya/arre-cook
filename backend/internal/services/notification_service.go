package services

import (
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"ninimenu/internal/database"
	"ninimenu/internal/models"

	"gorm.io/gorm"
)

const deploymentVersionSetting = "notifications_last_deployment_version"

var notificationTypes = map[string]bool{
	"system_update":  true,
	"feature":        true,
	"maintenance":    true,
	"health_tip":     true,
	"agent_security": true,
	"family":         true,
	"shopping":       true,
}

// 合并提醒时最多保留的条目数（按“；”分隔），多出的丢弃最早的。
const coalescedNoticeMaxParts = 8

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

// UpsertRecentNotification 合并短时间内的同类提醒：同一用户、同类型、同标题、同链接、仍未读且在 window 内
// 创建的站内信追加一条内容（用“；”分隔）并置顶，否则新建一条。避免连续确认几餐时刷屏。
func UpsertRecentNotification(uid uint, kind, title, line, link string, window time.Duration) error {
	line = strings.TrimSpace(line)
	if uid == 0 || line == "" {
		return nil
	}
	var n models.Notification
	err := database.DB.Scopes(database.OwnedBy(uid)).
		Where("type = ? AND title = ? AND link = ? AND read_at IS NULL AND created_at > ?", NormalizeNotificationType(kind), strings.TrimSpace(title), strings.TrimSpace(link), time.Now().Add(-window)).
		Order("created_at DESC").First(&n).Error
	if err != nil {
		_, err = CreateNotification(uid, kind, title, line, link)
		return err
	}
	parts := strings.Split(n.Content, "；")
	for _, p := range parts {
		if p == line {
			return nil
		}
	}
	parts = append(parts, line)
	if len(parts) > coalescedNoticeMaxParts {
		parts = parts[len(parts)-coalescedNoticeMaxParts:]
	}
	return database.DB.Model(&n).Updates(map[string]any{"content": strings.Join(parts, "；"), "created_at": time.Now()}).Error
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

const defaultDeploymentNotes = "本次更新了一些内容，并修复了一些 bug。"

// displayDeploymentVersion keeps the internal build marker private while giving
// users a stable version label. Numeric values are rendered as vN; non-numeric
// build markers use the generic v1 label until a numeric APP_VERSION is supplied.
func displayDeploymentVersion(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(strings.ToLower(raw), "v") {
		raw = strings.TrimSpace(raw[1:])
	}
	if matched, _ := regexp.MatchString(`^[0-9]+(?:\.[0-9]+){0,2}$`, raw); matched {
		return "v" + raw
	}
	return "v1"
}

// deploymentNoticeContent turns deployment notes into a short, readable notice. Notes
// are supplied by the deployment environment so the server never exposes commit
// messages or internal file names to end users.
func deploymentNoticeContent(version, rawNotes string) string {
	version = displayDeploymentVersion(version)
	lines := strings.FieldsFunc(strings.TrimSpace(rawNotes), func(r rune) bool {
		return r == '\n' || r == '\r' || r == ';' || r == '；'
	})
	notes := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(strings.TrimLeft(line, "-*• "))
		if line != "" {
			notes = append(notes, line)
		}
		if len(notes) == coalescedNoticeMaxParts {
			break
		}
	}
	if len(notes) == 0 {
		return fmt.Sprintf("系统已更新到版本 %s。\n\n%s", version, defaultDeploymentNotes)
	}
	return fmt.Sprintf("系统已更新到版本 %s。\n\n本次更新：\n- %s", version, strings.Join(notes, "\n- "))
}

// AnnounceDeployment publishes one system notice per build identifier. The version is
// supplied by the container build (APP_VERSION); missing/placeholder values are ignored
// so local development restarts do not spam every account.
func AnnounceDeployment(version string) error {
	return AnnounceDeploymentWithNotes(version, "")
}

// AnnounceDeploymentWithNotes publishes a deployment notice with an optional user-facing
// summary. The marker remains keyed by version, so restarting the same container never
// sends duplicate notices.
func AnnounceDeploymentWithNotes(version, releaseNotes string) error {
	version = strings.TrimSpace(version)
	if version == "" || strings.EqualFold(version, "unknown") || strings.EqualFold(version, "dev") {
		return nil
	}

	var sent int
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		var marker models.Setting
		markerErr := tx.Where("`key` = ?", deploymentVersionSetting).First(&marker).Error
		if markerErr == nil && strings.TrimSpace(marker.Value) == version {
			return nil
		}
		if markerErr != nil && !errors.Is(markerErr, gorm.ErrRecordNotFound) {
			return markerErr
		}

		var ids []uint
		if err := tx.Model(&models.User{}).Where("disabled = ?", false).Pluck("id", &ids).Error; err != nil {
			return err
		}
		content := deploymentNoticeContent(version, releaseNotes)
		rows := make([]models.Notification, 0, len(ids))
		for _, id := range ids {
			rows = append(rows, models.Notification{
				UserID: id, Type: "system_update", Title: "arre食谱推荐小助手 已更新",
				Content: content, Link: "/notifications",
			})
		}
		if len(rows) > 0 {
			if err := tx.Create(&rows).Error; err != nil {
				return err
			}
			sent = len(rows)
		}
		if markerErr == nil {
			return tx.Model(&marker).Update("value", version).Error
		}
		return tx.Create(&models.Setting{Key: deploymentVersionSetting, Value: version}).Error
	})
	if err == nil && sent > 0 {
		log.Printf("已发布版本更新站内信：%s（%d 位用户）", version, sent)
	}
	return err
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
