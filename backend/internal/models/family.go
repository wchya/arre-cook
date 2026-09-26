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

// FamilyShoppingCheck 家庭菜单自动展开的买菜条目：每个菜单格（日期 + 餐次）按菜品食材与调料各占一行，
// 菜单格换菜或清空时整格替换。成员手动添加的条目仍在 FamilyShoppingItem，两者分开展示。
type FamilyShoppingCheck struct {
	ID         uint      `json:"id" gorm:"primaryKey"`
	FamilyID   uint      `json:"family_id" gorm:"not null;index:idx_family_shopping_check_slot,priority:1"`
	MealDate   string    `json:"meal_date" gorm:"not null;size:10;index:idx_family_shopping_check_slot,priority:2"`
	MealType   string    `json:"meal_type" gorm:"not null;size:16;index:idx_family_shopping_check_slot,priority:3"`
	DishID     uint      `json:"dish_id" gorm:"not null;index"`
	DishName   string    `json:"dish_name"`
	ItemName   string    `json:"item_name" gorm:"not null;size:80"`
	ItemAmount string    `json:"item_amount"`
	Checked    bool      `json:"checked" gorm:"not null;default:false"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// DishDeleteRequest 家庭共享菜谱的删除申请：普通成员提交，家庭创建者（管理员）同意后才真正删除。
// 每道菜同一时间最多一条 pending 申请。
type DishDeleteRequest struct {
	ID          uint       `json:"id" gorm:"primaryKey"`
	FamilyID    uint       `json:"family_id" gorm:"not null;index:idx_dish_delete_req_family_status,priority:1"`
	DishID      uint       `json:"dish_id" gorm:"not null;index"`
	DishName    string     `json:"dish_name" gorm:"size:128"`
	RequestedBy uint       `json:"requested_by" gorm:"not null;index"`
	Status      string     `json:"status" gorm:"size:16;not null;default:'pending';index:idx_dish_delete_req_family_status,priority:2"`
	Reason      string     `json:"reason" gorm:"size:255"`
	DecidedBy   uint       `json:"decided_by"`
	DecidedAt   *time.Time `json:"decided_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}
