package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"ninimenu/internal/database"
	"ninimenu/internal/models"

	"gorm.io/gorm"
)

var ErrInvalidRecommendationFeedback = errors.New("推荐反馈无效")

type RecommendationFeedbackInput struct {
	RecommendationID string `json:"recommendation_id"`
	SessionID        string `json:"session_id,omitempty"`
	DishID           uint   `json:"dish_id"`
	EventType        string `json:"event_type"`
	RequestKey       string `json:"request_key,omitempty"`
	Note             string `json:"note,omitempty"`
}

type RecommendationFeedbackResult struct {
	OK      bool `json:"ok"`
	Created bool `json:"created"`
}

func validRecommendationFeedback(kind string) bool {
	switch kind {
	case "impression", "skip", "not_today", "dislike", "undo_dislike", "accept":
		return true
	}
	return false
}

// RecordRecommendationFeedback records a verified action and its event in one
// transaction. A returned candidate is not automatically counted as an impression.
func RecordRecommendationFeedback(uid uint, in RecommendationFeedbackInput, source, actor string, dbs ...*gorm.DB) (*RecommendationFeedbackResult, error) {
	in.EventType = strings.TrimSpace(in.EventType)
	in.RecommendationID = strings.TrimSpace(in.RecommendationID)
	in.SessionID = strings.TrimSpace(in.SessionID)
	in.RequestKey = strings.TrimSpace(in.RequestKey)
	in.Note = strings.TrimSpace(in.Note)
	if uid == 0 || in.DishID == 0 || !validRecommendationFeedback(in.EventType) || len(in.RecommendationID) > 64 || len(in.SessionID) > 64 || len(in.RequestKey) > 128 || len([]rune(in.Note)) > 500 || strings.IndexFunc(in.RequestKey, unicode.IsControl) >= 0 {
		return nil, ErrInvalidRecommendationFeedback
	}
	if in.RecommendationID == "" && in.EventType != "dislike" && in.EventType != "undo_dislike" {
		return nil, fmt.Errorf("%w：请提供 recommendation_id", ErrInvalidRecommendationFeedback)
	}
	// Ignore a caller's session hint; association comes from their own snapshot.
	in.SessionID = ""
	canonical, _ := json.Marshal(struct {
		RecommendationID string
		DishID           uint
		EventType        string
		Note             string
	}{in.RecommendationID, in.DishID, in.EventType, in.Note})
	sum := sha256.Sum256(canonical)
	hash := hex.EncodeToString(sum[:])
	result := &RecommendationFeedbackResult{OK: true}
	err := database.Handle(dbs...).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.User{}).Where("id = ?", uid).UpdateColumn("id", gorm.Expr("id")).Error; err != nil {
			return err
		}
		if in.RequestKey == "" {
			in.RequestKey = "recommendation:" + hash
			if in.EventType == "dislike" || in.EventType == "undo_dislike" {
				var last models.BehaviorEvent
				err := tx.Where("user_id = ? AND dish_id = ? AND event_type IN ?", uid, in.DishID, []string{"dislike", "undo_dislike"}).Order("id DESC").First(&last).Error
				if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
					return err
				}
				if err == nil && last.EventType == in.EventType {
					return nil // Retry of the current state, rather than another vote.
				}
				// A later reversal starts a new action even when old clients omit
				// a request key: dislike -> undo -> dislike must remain possible.
				in.RequestKey += fmt.Sprintf(":%d", last.ID)
			}
		}
		var previous models.RecommendationFeedback
		err := tx.Where("user_id = ? AND request_key = ?", uid, in.RequestKey).First(&previous).Error
		if err == nil {
			if previous.RequestHash != hash {
				return fmt.Errorf("%w：request_key 已用于其他反馈", ErrInvalidRecommendationFeedback)
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var dish models.Dish
		if err := tx.Scopes(database.VisibleDishes(uid)).First(&dish, in.DishID).Error; err != nil {
			return ErrDishNotFound
		}
		if in.RecommendationID != "" {
			if err := ValidateRecommendationItem(uid, in.RecommendationID, in.DishID, tx); err != nil {
				return err
			}
			var err error
			in.SessionID, err = RecommendationSessionID(uid, in.RecommendationID, tx)
			if err != nil {
				return err
			}
		}
		if in.EventType == "accept" {
			// Clients cannot manufacture a plan conversion with a feedback ping.
			var snapshot models.RecommendationSnapshot
			if err := tx.Where("user_id = ? AND id = ?", uid, in.RecommendationID).First(&snapshot).Error; err != nil {
				return ErrInvalidRecommendationFeedback
			}
			var count int64
			query := tx.Model(&models.HealthPlanItem{}).Where("user_id = ? AND dish_id = ? AND recommendation_id = ?", uid, in.DishID, snapshot.ID)
			if err := query.Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				return fmt.Errorf("%w：请先加入菜单", ErrInvalidRecommendationFeedback)
			}
		}
		row := models.RecommendationFeedback{UserID: uid, RequestKey: in.RequestKey, RequestHash: hash, RecommendationID: in.RecommendationID, DishID: in.DishID, EventType: in.EventType}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		meta, _ := json.Marshal(map[string]any{"recommendation_id": in.RecommendationID, "session_id": in.SessionID, "request_key": in.RequestKey, "note": in.Note})
		if err := tx.Create(&models.BehaviorEvent{UserID: uid, EventType: in.EventType, DishID: dish.ID, DishName: dish.Name, Source: truncateRunes(source, 64), Actor: truncateRunes(actor, 120), Meta: string(meta)}).Error; err != nil {
			return err
		}
		result.Created = true
		return nil
	})
	return result, err
}
