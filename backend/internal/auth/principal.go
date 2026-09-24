package auth

import (
	"ninimenu/internal/models"

	"github.com/gin-gonic/gin"
)

const principalKey = "ninimenu.principal"

// Principal 当前请求的调用者：一定落在某个用户上；Kind 区分是用户本人还是代表用户的智能体。
type Principal struct {
	User    *models.User
	Kind    string   // user / agent / pat
	Scopes  ScopeSet // user 类型拥有全部权限
	TokenID uint     // PAT 的 ID
	Actor   string   // 智能体名称（PAT 名称 / 嵌入会话 actor），用于审计与行为事件 source
}

func (p *Principal) UserID() uint {
	if p == nil || p.User == nil {
		return 0
	}
	return p.User.ID
}

func (p *Principal) IsAgent() bool { return p != nil && p.Kind != KindUser }

func (p *Principal) Can(scope string) bool {
	if p == nil {
		return false
	}
	if p.Kind == KindUser {
		return true
	}
	return p.Scopes.Has(scope)
}

// Source 行为事件来源标记：站内操作为 app，智能体为 agent:<名称>。
func (p *Principal) Source() string {
	if p == nil || p.Kind == KindUser {
		return "app"
	}
	if p.Actor != "" {
		return "agent:" + p.Actor
	}
	return "agent"
}

func SetPrincipal(c *gin.Context, p *Principal) { c.Set(principalKey, p) }

func GetPrincipal(c *gin.Context) *Principal {
	if v, ok := c.Get(principalKey); ok {
		if p, ok := v.(*Principal); ok {
			return p
		}
	}
	return nil
}

// UID 当前请求所属用户 ID；未认证返回 0（受保护路由不会出现）。
func UID(c *gin.Context) uint { return GetPrincipal(c).UserID() }

func CurrentUser(c *gin.Context) *models.User {
	if p := GetPrincipal(c); p != nil {
		return p.User
	}
	return nil
}
