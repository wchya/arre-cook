package models

import "time"

// Family is an opt-in shared space. Personal data continues to belong to UserID.
type Family struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Name      string    `json:"name" gorm:"not null;size:64"`
	OwnerID   uint      `json:"owner_id" gorm:"not null;index"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type FamilyMember struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	FamilyID  uint      `json:"family_id" gorm:"not null;index"`
	UserID    uint      `json:"user_id" gorm:"not null;uniqueIndex"`
	JoinedAt  time.Time `json:"joined_at"`
}

type FamilyInvitation struct {
	ID         uint       `json:"id" gorm:"primaryKey"`
	FamilyID   uint       `json:"family_id" gorm:"not null;index"`
	Email      string     `json:"email" gorm:"not null;size:128;index"`
	TokenHash  string     `json:"-" gorm:"not null;size:64;uniqueIndex"`
	InvitedBy  uint       `json:"invited_by" gorm:"not null"`
	ExpiresAt  time.Time  `json:"expires_at"`
	AcceptedAt *time.Time `json:"accepted_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

type FamilyPlanItem struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	FamilyID  uint      `json:"family_id" gorm:"not null;uniqueIndex:idx_family_plan_slot,priority:1"`
	MealDate  string    `json:"meal_date" gorm:"not null;size:10;uniqueIndex:idx_family_plan_slot,priority:2"`
	MealType  string    `json:"meal_type" gorm:"not null;size:16;uniqueIndex:idx_family_plan_slot,priority:3"`
	DishID    uint      `json:"dish_id" gorm:"not null;index"`
	AddedBy   uint      `json:"added_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type FamilyShoppingItem struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	FamilyID  uint      `json:"family_id" gorm:"not null;uniqueIndex:idx_family_shopping_name,priority:1"`
	Name      string    `json:"name" gorm:"not null;size:80;uniqueIndex:idx_family_shopping_name,priority:2"`
	Amount    string    `json:"amount" gorm:"size:80"`
	Checked   bool      `json:"checked" gorm:"not null;default:false"`
	AddedBy   uint      `json:"added_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
