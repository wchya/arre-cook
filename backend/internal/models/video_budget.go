package models

// VideoPlatformBudget keeps request spacing, budgets and platform cooldowns
// across restarts and instances. It stores no URLs or user content.
type VideoPlatformBudget struct {
	Platform     string `gorm:"primaryKey;size:16"`
	Hour         int64  `gorm:"not null;default:0"`
	HourUsed     int    `gorm:"not null;default:0"`
	Day          int64  `gorm:"not null;default:0"`
	DayUsed      int    `gorm:"not null;default:0"`
	NextAt       int64  `gorm:"not null;default:0"`
	BlockedUntil int64  `gorm:"not null;default:0"`
}
