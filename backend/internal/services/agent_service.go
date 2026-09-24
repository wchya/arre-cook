package services

import (
	"encoding/json"
	"errors"
	"ninimenu/internal/auth"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"strings"
	"time"
)

// ---------------- 推荐建议收件箱 ----------------

const maxPendingSuggestions = 30

type SuggestionInput struct {
	Title          string `json:"title"`
	Reason         string `json:"reason"`
	DishIDs        []uint `json:"dish_ids"`
	MealType       string `json:"meal_type"`
	MealDate       string `json:"meal_date"`
	ExpiresInHours int    `json:"expires_in_hours"`
}

type SuggestionView struct {
	models.AgentSuggestion
	Dishes []models.Dish `json:"dishes"`
}

// CreateSuggestion 智能体向用户推送一条建议。只接受该用户可见的菜品。
func CreateSuggestion(uid uint, source string, in SuggestionInput) (*SuggestionView, error) {
	ids := uniqueIDs(in.DishIDs, 6)
	if len(ids) == 0 {
		return nil, errors.New("dish_ids 至少包含一道菜")
	}
	var dishes []models.Dish
	database.DB.Scopes(database.VisibleDishes(uid)).Where("id IN ?", ids).Find(&dishes)
	if len(dishes) != len(ids) {
		return nil, errors.New("包含不存在或不可见的菜品")
	}
	if in.MealType != "" && in.MealType != "lunch" && in.MealType != "dinner" {
		return nil, ErrInvalidMealType
	}
	date := ""
	if strings.TrimSpace(in.MealDate) != "" {
		d, err := normalizeMealDate(in.MealDate)
		if err != nil {
			return nil, err
		}
		date = d
	}

	ExpireSuggestions(uid)
	var pending int64
	database.DB.Model(&models.AgentSuggestion{}).Scopes(database.OwnedBy(uid)).Where("status = ?", "pending").Count(&pending)
	if pending >= maxPendingSuggestions {
		return nil, errors.New("待处理的建议过多，请等用户处理后再推送")
	}

	title := truncateRunes(strings.TrimSpace(in.Title), 60)
	if title == "" {
		title = "为你挑了几道菜"
	}
	hours := in.ExpiresInHours
	if hours <= 0 || hours > 24*14 {
		hours = 72
	}
	exp := time.Now().Add(time.Duration(hours) * time.Hour)
	idsJSON, _ := json.Marshal(ids)
	s := models.AgentSuggestion{
		UserID: uid, Source: truncateRunes(source, 64), Title: title,
		Reason: truncateRunes(strings.TrimSpace(in.Reason), 500), DishIDs: string(idsJSON),
		MealType: in.MealType, MealDate: date, Status: "pending", ExpiresAt: &exp,
	}
	if err := database.DB.Create(&s).Error; err != nil {
		return nil, err
	}
	for _, d := range dishes {
		LogBehavior(uid, "recommend", d.ID, d.Name, source, "", map[string]any{"mode": "suggestion", "suggestion_id": s.ID})
	}
	return &SuggestionView{AgentSuggestion: s, Dishes: orderDishes(dishes, ids)}, nil
}

func ExpireSuggestions(uid uint) {
	database.DB.Model(&models.AgentSuggestion{}).Scopes(database.OwnedBy(uid)).
		Where("status = ? AND expires_at IS NOT NULL AND expires_at < ?", "pending", time.Now()).
		Update("status", "expired")
}

func ListSuggestions(uid uint, status string, limit int) []SuggestionView {
	ExpireSuggestions(uid)
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	q := database.DB.Scopes(database.OwnedBy(uid)).Order("created_at DESC").Limit(limit)
	if status != "" && status != "all" {
		q = q.Where("status = ?", status)
	}
	var rows []models.AgentSuggestion
	q.Find(&rows)

	all := map[uint]bool{}
	for _, r := range rows {
		for _, id := range parseIDs(r.DishIDs) {
			all[id] = true
		}
	}
	ids := make([]uint, 0, len(all))
	for id := range all {
		ids = append(ids, id)
	}
	dishByID := map[uint]models.Dish{}
	if len(ids) > 0 {
		var dishes []models.Dish
		database.DB.Scopes(database.VisibleDishes(uid)).Where("id IN ?", ids).Find(&dishes)
		MarkFavorites(uid, dishes)
		for _, d := range dishes {
			dishByID[d.ID] = d
		}
	}
	out := make([]SuggestionView, 0, len(rows))
	for _, r := range rows {
		v := SuggestionView{AgentSuggestion: r, Dishes: []models.Dish{}}
		for _, id := range parseIDs(r.DishIDs) {
			if d, ok := dishByID[id]; ok {
				v.Dishes = append(v.Dishes, d)
			}
		}
		out = append(out, v)
	}
	return out
}

type ResolveInput struct {
	Accept   bool   `json:"accept"`
	DishIDs  []uint `json:"dish_ids"`  // 只采纳其中几道；空 = 全部
	MealType string `json:"meal_type"` // 覆盖建议里的餐次
	MealDate string `json:"meal_date"`
}

// ResolveSuggestion 用户采纳或忽略建议。采纳时按餐次写入用餐记录；两种结果都回流为行为事件。
func ResolveSuggestion(uid uint, id any, in ResolveInput) (created []models.MealRecord, err error) {
	var s models.AgentSuggestion
	if err := database.DB.Scopes(database.OwnedBy(uid)).First(&s, id).Error; err != nil {
		return nil, errors.New("建议不存在")
	}
	if s.Status != "pending" {
		return nil, errors.New("这条建议已处理过")
	}
	ids := parseIDs(s.DishIDs)
	chosen := map[uint]bool{}
	for _, id := range in.DishIDs {
		chosen[id] = true
	}
	now := time.Now()
	status := "dismissed"
	if in.Accept {
		status = "accepted"
		mealType := firstNonEmpty(in.MealType, s.MealType)
		date := firstNonEmpty(in.MealDate, s.MealDate)
		for _, did := range ids {
			if len(chosen) > 0 && !chosen[did] {
				continue
			}
			if mealType == "" {
				LogBehavior(uid, "accept", did, "", s.Source, "", map[string]any{"suggestion_id": s.ID})
				continue
			}
			rec, err := CreateMealRecord(uid, MealInput{DishID: did, MealType: mealType, MealDate: date}, s.Source, "user")
			if err == nil {
				created = append(created, *rec)
			}
		}
	} else {
		for _, did := range ids {
			LogBehavior(uid, "reject", did, "", s.Source, "", map[string]any{"suggestion_id": s.ID})
		}
	}
	database.DB.Model(&s).Updates(map[string]any{"status": status, "resolved_at": now})
	return created, nil
}

func parseIDs(raw string) []uint {
	var ids []uint
	_ = json.Unmarshal([]byte(raw), &ids)
	return ids
}

func uniqueIDs(in []uint, max int) []uint {
	seen := map[uint]bool{}
	out := []uint{}
	for _, id := range in {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
		if len(out) >= max {
			break
		}
	}
	return out
}

func orderDishes(dishes []models.Dish, ids []uint) []models.Dish {
	byID := map[uint]models.Dish{}
	for _, d := range dishes {
		byID[d.ID] = d
	}
	out := make([]models.Dish, 0, len(ids))
	for _, id := range ids {
		if d, ok := byID[id]; ok {
			out = append(out, d)
		}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// ---------------- 审计 ----------------

type AuditEntry struct {
	UserID     uint
	TokenID    uint
	Actor      string
	Channel    string
	Tool       string
	Args       any
	Status     string
	Error      string
	DurationMs int64
}

func WriteAudit(e AuditEntry) {
	if e.UserID == 0 {
		return
	}
	args := ""
	switch v := e.Args.(type) {
	case nil:
	case string:
		args = v
	case []byte:
		args = string(v)
	default:
		if b, err := json.Marshal(v); err == nil {
			args = string(b)
		}
	}
	database.DB.Create(&models.AgentAuditLog{
		UserID: e.UserID, TokenID: e.TokenID, Actor: truncateRunes(e.Actor, 64), Channel: e.Channel,
		Tool: e.Tool, Args: truncateRunes(args, 2000), Status: e.Status, Error: truncateRunes(e.Error, 500), DurationMs: e.DurationMs,
	})
}

// ---------------- 智能体令牌 ----------------

type AgentTokenView struct {
	models.AgentToken
	ScopeList []string `json:"scopes"`
	Active    bool     `json:"active"`
}

func toTokenView(t models.AgentToken) AgentTokenView {
	var scopes []string
	_ = json.Unmarshal([]byte(t.Scopes), &scopes)
	active := t.RevokedAt == nil && (t.ExpiresAt == nil || time.Now().Before(*t.ExpiresAt))
	return AgentTokenView{AgentToken: t, ScopeList: scopes, Active: active}
}

const maxTokensPerUser = 20

func CreateAgentToken(uid uint, name string, scopes []string, expiresInDays int) (string, *AgentTokenView, error) {
	name = truncateRunes(strings.TrimSpace(name), 32)
	if name == "" {
		return "", nil, errors.New("请给令牌起个名字，例如 DeepSeek、Hermes")
	}
	scopes = auth.NormalizeScopes(scopes)
	if len(scopes) == 0 {
		return "", nil, errors.New("至少选择一项权限")
	}
	var n int64
	database.DB.Model(&models.AgentToken{}).Scopes(database.OwnedBy(uid)).Where("revoked_at IS NULL").Count(&n)
	if n >= maxTokensPerUser {
		return "", nil, errors.New("令牌数量已达上限，请先撤销不用的令牌")
	}
	plain, hash, prefix, err := auth.GeneratePAT()
	if err != nil {
		return "", nil, err
	}
	scopesJSON, _ := json.Marshal(scopes)
	t := models.AgentToken{UserID: uid, Name: name, Prefix: prefix, TokenHash: hash, Scopes: string(scopesJSON)}
	if expiresInDays > 0 {
		exp := time.Now().AddDate(0, 0, expiresInDays)
		t.ExpiresAt = &exp
	}
	if err := database.DB.Create(&t).Error; err != nil {
		return "", nil, err
	}
	v := toTokenView(t)
	return plain, &v, nil
}

func ListAgentTokens(uid uint) []AgentTokenView {
	var rows []models.AgentToken
	database.DB.Scopes(database.OwnedBy(uid)).Order("created_at DESC").Find(&rows)
	out := make([]AgentTokenView, 0, len(rows))
	for _, r := range rows {
		out = append(out, toTokenView(r))
	}
	return out
}

func RevokeAgentToken(uid uint, id any) error {
	res := database.DB.Model(&models.AgentToken{}).Scopes(database.OwnedBy(uid)).
		Where("id = ? AND revoked_at IS NULL", id).Update("revoked_at", time.Now())
	if res.RowsAffected == 0 {
		return errors.New("令牌不存在或已撤销")
	}
	return nil
}
