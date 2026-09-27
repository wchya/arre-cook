package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"ninimenu/internal/models"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const AssistantRequestTimeout = 90 * time.Second
const assistantLeaseTTL = 2 * time.Minute

var ErrAssistantBusy = errors.New("你已有一个问题正在处理，请等回复结束后再试")
var ErrAssistantServiceBusy = errors.New("助手正在处理较多请求，请稍后再试")

func AcquireAssistantLease(db *gorm.DB, userID uint, now time.Time) (func(), error) {
	entropy := make([]byte, 16)
	if _, err := rand.Read(entropy); err != nil {
		return nil, err
	}
	owner := hex.EncodeToString(entropy)
	keys := []string{}
	release := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		// Also runs after the HTTP context has been canceled.
		db.WithContext(ctx).Where("`key` IN ? AND owner = ?", keys, owner).Delete(&models.AssistantLease{})
	}
	acquire := func(key string) (bool, error) {
		row := models.AssistantLease{Key: key, ExpiresAt: time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)}
		if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return false, err
		}
		result := db.Model(&models.AssistantLease{}).Where("`key` = ? AND expires_at <= ?", key, now).
			Updates(map[string]any{"owner": owner, "expires_at": now.Add(assistantLeaseTTL)})
		if result.Error != nil {
			return false, result.Error
		}
		if result.RowsAffected == 1 {
			keys = append(keys, key)
			return true, nil
		}
		return false, nil
	}
	ok, err := acquire(fmt.Sprintf("user:%d", userID))
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrAssistantBusy
	}
	for slot := 0; slot < 4; slot++ {
		ok, err = acquire(fmt.Sprintf("slot:%d", slot))
		if err != nil {
			release()
			return nil, err
		}
		if ok {
			return release, nil
		}
	}
	release()
	return nil, ErrAssistantServiceBusy
}
