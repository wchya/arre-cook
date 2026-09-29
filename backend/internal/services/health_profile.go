package services

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"gorm.io/gorm"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
)

var ErrHealthProfileConflict = errors.New("档案已在其他页面更新，请重新载入后核对")
var ErrInvalidHealthProfile = errors.New("健康档案内容无效")

type HealthProfileInput struct {
	Version           int      `json:"version"`
	Confirmed         bool     `json:"confirmed"`
	Goal              string   `json:"goal"`
	EatingPattern     string   `json:"eating_pattern"`
	Allergies         []string `json:"allergies"`
	DietaryExclusions []string `json:"dietary_exclusions"`
}

func LoadHealthProfile(uid uint, dbs ...*gorm.DB) (*models.HealthProfile, error) {
	out := models.HealthProfile{UserID: uid, Allergies: []string{}, DietaryExclusions: []string{}}
	err := database.Handle(dbs...).Where("user_id = ?", uid).First(&out).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &out, nil
	}
	return &out, err
}

func validateHealthTerms(values []string) ([]string, error) {
	if len(values) > 20 {
		return nil, fmt.Errorf("%w：每类最多20项食材", ErrInvalidHealthProfile)
	}
	out := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || utf8.RuneCountInString(value) > 40 || strings.IndexFunc(value, unicode.IsControl) >= 0 {
			return nil, fmt.Errorf("%w：每项食材需为1–40字，不含控制字符", ErrInvalidHealthProfile)
		}
		key := strings.ToLower(value)
		if !seen[key] {
			out = append(out, value)
			seen[key] = true
		}
	}
	return out, nil
}

func SaveHealthProfile(uid uint, in HealthProfileInput, dbs ...*gorm.DB) (*models.HealthProfile, error) {
	if !in.Confirmed || in.Version < 0 {
		return nil, fmt.Errorf("%w：请本人核对并确认保存", ErrInvalidHealthProfile)
	}
	switch in.Goal {
	case "", "balanced", "weight_management", "regular_meals":
	default:
		return nil, fmt.Errorf("%w：目标无效", ErrInvalidHealthProfile)
	}
	switch in.EatingPattern {
	case "", "home", "eating_out", "mixed":
	default:
		return nil, fmt.Errorf("%w：就餐方式无效", ErrInvalidHealthProfile)
	}
	allergies, err := validateHealthTerms(in.Allergies)
	if err != nil {
		return nil, err
	}
	exclusions, err := validateHealthTerms(in.DietaryExclusions)
	if err != nil {
		return nil, err
	}
	var result *models.HealthProfile
	err = database.Handle(dbs...).Transaction(func(tx *gorm.DB) error {
		if err := lockHealthProfileUser(uid, tx); err != nil {
			return err
		}
		current, err := LoadHealthProfile(uid, tx)
		if err != nil {
			return err
		}
		if current.Version != in.Version {
			return ErrHealthProfileConflict
		}
		now := time.Now()
		result = &models.HealthProfile{UserID: uid, Version: current.Version + 1, Active: true, Goal: in.Goal, EatingPattern: in.EatingPattern, Allergies: allergies, DietaryExclusions: exclusions, ConfirmedAt: &now}
		if err := tx.Save(result).Error; err != nil {
			return err
		}
		if err := tx.Create(&models.HealthProfileVersion{UserID: uid, Version: result.Version, Snapshot: *result}).Error; err != nil {
			return err
		}
		return tx.Where("user_id = ? AND `key` = ?", uid, "week_plan_cache").Delete(&models.UserSetting{}).Error
	})
	return result, err
}

func ClearHealthProfile(uid uint, version int, dbs ...*gorm.DB) (*models.HealthProfile, error) {
	var result *models.HealthProfile
	err := database.Handle(dbs...).Transaction(func(tx *gorm.DB) error {
		if err := lockHealthProfileUser(uid, tx); err != nil {
			return err
		}
		current, err := LoadHealthProfile(uid, tx)
		if err != nil {
			return err
		}
		if current.Version != version {
			return ErrHealthProfileConflict
		}
		if !current.Active {
			result = current
			return nil
		}
		result = &models.HealthProfile{UserID: uid, Version: version + 1, Allergies: []string{}, DietaryExclusions: []string{}}
		if err := tx.Save(result).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", uid).Delete(&models.HealthProfileVersion{}).Error; err != nil {
			return err
		}
		return tx.Where("user_id = ? AND `key` = ?", uid, "week_plan_cache").Delete(&models.UserSetting{}).Error
	})
	return result, err
}

func lockHealthProfileUser(uid uint, tx *gorm.DB) error {
	result := tx.Model(&models.User{}).Where("id = ?", uid).UpdateColumn("id", gorm.Expr("id"))
	if result.Error != nil {
		return result.Error
	}
	// MySQL may report zero changed rows for a no-op update; check existence explicitly.
	var user models.User
	return tx.Select("id").First(&user, uid).Error
}

// The private fields never enter taste profiles, model prompts or legacy Agent exports.
func healthProfileExclusions(uid uint, db *gorm.DB) ([]string, error) {
	profile, err := LoadHealthProfile(uid, db)
	if err != nil {
		return nil, err
	}
	if !profile.Active {
		return []string{}, nil
	}
	return append(append([]string{}, profile.Allergies...), profile.DietaryExclusions...), nil
}

func loadDietaryExclusions(uid uint, db *gorm.DB) ([]string, error) {
	prefs, err := LoadPreferences(uid, db)
	if err != nil {
		return nil, err
	}
	blocked, err := healthProfileExclusions(uid, db)
	if err != nil {
		return nil, err
	}
	blocked = append(blocked, prefs.Allergies...)
	return append(blocked, prefs.AvoidIngredients...), nil
}
