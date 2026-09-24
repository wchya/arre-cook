package auth

import (
	"sort"
	"strings"
)

// 智能体权限范围。第三方智能体（DeepSeek 函数调用、Hermes MCP、自建 Agent）拿到的令牌
// 只能访问签发者本人的数据，并且只能调用 scopes 覆盖的工具。
const (
	ScopeProfileRead       = "profile:read"      // 口味画像、偏好、统计
	ScopeDishesRead        = "dishes:read"       // 菜品库（公共 + 本人私有）
	ScopeRecordsRead       = "records:read"      // 用餐记录、评价、收藏、行为、周菜单、买菜清单
	ScopeRecordsWrite      = "records:write"     // 记一餐 / 删记录
	ScopeFavoritesWrite    = "favorites:write"   // 收藏 / 取消收藏
	ScopePlanWrite         = "plan:write"        // 重新生成周菜单
	ScopePreferencesWrite  = "preferences:write" // 修改饮食偏好
	ScopeSuggestionsWrite  = "suggestions:write" // 向用户推送推荐建议
	ScopeBehaviorWrite     = "behavior:write"    // 写入行为事件
)

var AllScopes = []string{
	ScopeProfileRead, ScopeDishesRead, ScopeRecordsRead,
	ScopeRecordsWrite, ScopeFavoritesWrite, ScopePlanWrite,
	ScopePreferencesWrite, ScopeSuggestionsWrite, ScopeBehaviorWrite,
}

var ScopeLabels = map[string]string{
	ScopeProfileRead:      "读取口味画像与偏好",
	ScopeDishesRead:       "读取菜品库",
	ScopeRecordsRead:      "读取用餐记录、收藏、周菜单",
	ScopeRecordsWrite:     "代我记录/删除用餐",
	ScopeFavoritesWrite:   "代我收藏菜品",
	ScopePlanWrite:        "重新生成周菜单",
	ScopePreferencesWrite: "修改我的饮食偏好",
	ScopeSuggestionsWrite: "向我推送推荐建议",
	ScopeBehaviorWrite:    "写入行为反馈",
}

// ScopePresets 常用授权组合，前端“连接智能体”时一键选择。
var ScopePresets = map[string][]string{
	"readonly": {ScopeProfileRead, ScopeDishesRead, ScopeRecordsRead},
	"advisor":  {ScopeProfileRead, ScopeDishesRead, ScopeRecordsRead, ScopeSuggestionsWrite, ScopeBehaviorWrite},
	"full":     AllScopes,
}

func NormalizeScopes(in []string) []string {
	valid := map[string]bool{}
	for _, s := range AllScopes {
		valid[s] = true
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if preset, ok := ScopePresets[s]; ok {
			for _, p := range preset {
				if !seen[p] {
					seen[p] = true
					out = append(out, p)
				}
			}
			continue
		}
		if valid[s] && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

type ScopeSet map[string]bool

func NewScopeSet(scopes []string) ScopeSet {
	set := ScopeSet{}
	for _, s := range scopes {
		set[s] = true
	}
	return set
}

func (s ScopeSet) Has(scope string) bool {
	return scope == "" || s[scope]
}

func (s ScopeSet) List() []string {
	out := make([]string, 0, len(s))
	for k := range s {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
