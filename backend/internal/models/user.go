package models

import (
	"strings"
	"time"
)

// User 账号。所有“个人数据”（用餐记录、收藏、评价、行为事件、偏好、周菜单、买菜清单、成就解锁、
// 智能体令牌与会话）都以 user_id 归属到账号，互相隔离；菜品库分公共（owner_id=0，管理员维护）与个人私有。
type User struct {
	ID            uint       `json:"id" gorm:"primaryKey"`
	Email         *string    `json:"email" gorm:"uniqueIndex;size:128"`
	Username      *string    `json:"username" gorm:"uniqueIndex;size:64"`
	PasswordHash  string     `json:"-"`
	Nickname      string     `json:"nickname" gorm:"size:64"`
	Avatar        string     `json:"avatar"`
	Role          string     `json:"role" gorm:"size:16;default:'user';index"`
	WechatOpenID  *string    `json:"-" gorm:"uniqueIndex;size:128"`
	WechatUnionID string     `json:"-" gorm:"size:128;index"`
	Disabled      bool       `json:"disabled" gorm:"default:false"`
	TokenVersion  int        `json:"-" gorm:"default:1"` // 改密码/注销时递增，使旧登录态全部失效
	LastLoginAt   *time.Time `json:"last_login_at"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

const (
	RoleUser  = "user"
	RoleAdmin = "admin"
)

func (u *User) IsAdmin() bool { return u != nil && u.Role == RoleAdmin }

// DisplayName 昵称优先，其次用户名。
func (u *User) DisplayName() string {
	if u == nil {
		return ""
	}
	if u.Nickname != "" {
		return u.Nickname
	}
	if u.Username != nil {
		return *u.Username
	}
	if u.Email != nil {
		name, _, _ := strings.Cut(*u.Email, "@")
		return name
	}
	return "微信用户"
}

// EmailCode 邮箱验证码（登录/绑定）。只存 SHA-256 摘要，限时、限次、一次性。
type EmailCode struct {
	ID         uint       `json:"id" gorm:"primaryKey"`
	Email      string     `json:"email" gorm:"size:128;index:idx_email_codes_email_purpose,priority:1"`
	Purpose    string     `json:"purpose" gorm:"size:16;index:idx_email_codes_email_purpose,priority:2"`
	CodeHash   string     `json:"-" gorm:"size:64"`
	Attempts   int        `json:"attempts" gorm:"default:0"`
	IP         string     `json:"ip" gorm:"size:64;index"`
	ExpiresAt  time.Time  `json:"expires_at"`
	ConsumedAt *time.Time `json:"consumed_at"`
	CreatedAt  time.Time  `json:"created_at" gorm:"index"`
}

// UserPreference 用户显式声明的饮食偏好，推荐引擎与 AI 助手都会读取（与行为推断出的口味画像互补）。
type UserPreference struct {
	ID               uint      `json:"-" gorm:"primaryKey"`
	UserID           uint      `json:"-" gorm:"uniqueIndex;not null"`
	AvoidIngredients string    `json:"-" gorm:"default:'[]'"` // 忌口
	Allergies        string    `json:"-" gorm:"default:'[]'"` // 过敏原（硬过滤）
	FavoriteTastes   string    `json:"-" gorm:"default:'[]'"`
	SpiceLevel       int       `json:"spice_level" gorm:"default:-1"` // -1 未设置，0 不吃辣 … 3 特辣
	HouseholdSize    int       `json:"household_size" gorm:"default:0"`
	MaxCookTime      int       `json:"max_cook_time" gorm:"default:0"`
	Goals            string    `json:"goals"` // 如：减脂、增肌、控糖
	Notes            string    `json:"notes"` // 给 AI 的补充说明
	UpdatedAt        time.Time `json:"updated_at"`
}

// UserSetting 用户级设置（语音、盲盒开关、每日道数、周菜单缓存等），与站点级 Setting 分开存。
type UserSetting struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	UserID    uint      `json:"user_id" gorm:"not null;uniqueIndex:idx_user_settings_user_key,priority:1"`
	Key       string    `json:"key" gorm:"not null;size:64;uniqueIndex:idx_user_settings_user_key,priority:2"`
	Value     string    `json:"value"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AgentToken 用户为第三方智能体（DeepSeek / Hermes / 自建 Agent 等）签发的个人访问令牌。
// 令牌只保存 SHA-256 摘要；持有者只能访问签发者本人的数据，且受 scopes 限制。
type AgentToken struct {
	ID         uint       `json:"id" gorm:"primaryKey"`
	UserID     uint       `json:"-" gorm:"not null;index"`
	Name       string     `json:"name" gorm:"size:64"`
	Prefix     string     `json:"prefix" gorm:"size:16"`
	TokenHash  string     `json:"-" gorm:"size:64;uniqueIndex"`
	Scopes     string     `json:"-" gorm:"default:'[]'"`
	LastUsedAt *time.Time `json:"last_used_at"`
	ExpiresAt  *time.Time `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

// AgentAuditLog 智能体（含站内 AI 助手）以用户身份执行的每一次工具调用/写操作，供用户查看与追责。
type AgentAuditLog struct {
	ID         uint      `json:"id" gorm:"primaryKey"`
	UserID     uint      `json:"-" gorm:"not null;index:idx_agent_audit_user_time,priority:1"`
	TokenID    uint      `json:"token_id" gorm:"index"`
	Actor      string    `json:"actor" gorm:"size:64"`   // assistant / token 名 / embed
	Channel    string    `json:"channel" gorm:"size:16"` // rest / mcp / tools / chat
	Tool       string    `json:"tool" gorm:"size:64"`
	Args       string    `json:"args"`
	Status     string    `json:"status" gorm:"size:16"` // ok / error / denied
	Error      string    `json:"error"`
	DurationMs int64     `json:"duration_ms"`
	CreatedAt  time.Time `json:"created_at" gorm:"index:idx_agent_audit_user_time,priority:2"`
}

// AgentSuggestion 智能体推送给用户的推荐建议（收件箱）。外部智能体不直接改菜单，
// 而是提交建议，由用户在 App 里一键采纳或忽略；采纳/忽略会回流为行为事件，形成推荐闭环。
type AgentSuggestion struct {
	ID         uint       `json:"id" gorm:"primaryKey"`
	UserID     uint       `json:"-" gorm:"not null;index:idx_agent_sugg_user_status,priority:1"`
	Source     string     `json:"source" gorm:"size:64"`
	Title      string     `json:"title" gorm:"size:128"`
	Reason     string     `json:"reason"`
	DishIDs    string     `json:"-" gorm:"default:'[]'"`
	MealType   string     `json:"meal_type" gorm:"size:16"`
	MealDate   string     `json:"meal_date" gorm:"size:10"`
	Status     string     `json:"status" gorm:"size:16;default:'pending';index:idx_agent_sugg_user_status,priority:2"`
	ExpiresAt  *time.Time `json:"expires_at"`
	ResolvedAt *time.Time `json:"resolved_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

// ChatSession / ChatMessage 站内 AI 助手的对话历史（按用户隔离）。
type ChatSession struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	UserID    uint      `json:"-" gorm:"not null;index"`
	Title     string    `json:"title" gorm:"size:128"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at" gorm:"index"`
}

type ChatMessage struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	SessionID uint      `json:"session_id" gorm:"not null;index"`
	UserID    uint      `json:"-" gorm:"not null;index"`
	Role      string    `json:"role" gorm:"size:16"` // user / assistant
	Content   string    `json:"content"`
	Cards     string    `json:"-" gorm:"type:longtext;default:'[]'"` // 助手回复附带的菜品卡片、工具轨迹（JSON）
	CreatedAt time.Time `json:"created_at"`
}
