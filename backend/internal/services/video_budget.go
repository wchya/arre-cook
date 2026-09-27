package services

import (
	"context"
	"errors"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"ninimenu/internal/video"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const videoHourlyRequests = 60
const videoDailyRequests = 200
const videoRequestSpacing = 2 * time.Second

type videoBudgetGate struct{}

func (videoBudgetGate) Acquire(ctx context.Context, platform string) error {
	if database.DB == nil {
		return errors.New("missing database")
	}
	for attempt := 0; attempt < 6; attempt++ {
		delay, err := reserveVideoRequest(database.DB.WithContext(ctx), platform, time.Now())
		if err != nil {
			return err
		}
		if delay <= 0 {
			return nil
		}
		if delay > 3*time.Second {
			return &video.Error{Code: "platform_busy", Message: "视频读取额度暂时用完或平台冷却中，可稍后重试或粘贴字幕", RetryAfter: max(1, int(delay.Seconds())+1)}
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return &video.Error{Code: "platform_busy", Message: "视频读取正忙，请稍后重试或粘贴字幕", RetryAfter: 15}
}

func reserveVideoRequest(db *gorm.DB, platform string, now time.Time) (time.Duration, error) {
	if platform != "bilibili" && platform != "douyin" && platform != "asr" {
		return 0, errors.New("unsupported platform")
	}
	row := models.VideoPlatformBudget{Platform: platform}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		return 0, err
	}
	hour, day, millis := now.Unix()/3600, now.Unix()/86400, now.UnixMilli()
	// Keep the SET order explicit: MySQL evaluates assignments left-to-right.
	// Both buckets and the shared slot are reserved by one atomic SQL statement.
	result := db.Exec("UPDATE video_platform_budgets SET hour_used = CASE WHEN `hour` = ? THEN hour_used + 1 ELSE 1 END, day_used = CASE WHEN `day` = ? THEN day_used + 1 ELSE 1 END, `hour` = ?, `day` = ?, next_at = ? WHERE platform = ? AND next_at <= ? AND blocked_until <= ? AND (`hour` <> ? OR hour_used < ?) AND (`day` <> ? OR day_used < ?)", hour, day, hour, day, now.Add(videoRequestSpacing).UnixMilli(), platform, millis, millis, hour, videoHourlyRequests, day, videoDailyRequests)
	if result.Error != nil {
		return 0, result.Error
	}
	if result.RowsAffected == 1 {
		return 0, nil
	}
	if err := db.Where("platform = ?", platform).Take(&row).Error; err != nil {
		return 0, err
	}
	until := max(row.NextAt, row.BlockedUntil)
	if row.Hour == hour && row.HourUsed >= videoHourlyRequests {
		until = max(until, (hour+1)*3600*1000)
	}
	if row.Day == day && row.DayUsed >= videoDailyRequests {
		until = max(until, (day+1)*86400*1000)
	}
	return max(time.Millisecond, time.Duration(until-millis)*time.Millisecond), nil
}

func (videoBudgetGate) CoolDown(ctx context.Context, platform string, delay time.Duration) error {
	if database.DB == nil {
		return errors.New("missing database")
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	db := database.DB.WithContext(ctx)
	row := models.VideoPlatformBudget{Platform: platform}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		return err
	}
	until := time.Now().Add(delay).UnixMilli()
	return db.Model(&models.VideoPlatformBudget{}).Where("platform = ? AND blocked_until < ?", platform, until).Update("blocked_until", until).Error
}
