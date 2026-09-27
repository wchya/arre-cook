package middleware

import (
	"ninimenu/internal/utils"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// NumericPathIDs rejects expressions before an identifier reaches the ORM.
// Names and itemName are intentionally excluded: those routes use text keys.
func NumericPathIDs() gin.HandlerFunc {
	return func(c *gin.Context) {
		for _, key := range []string{"id", "dishId"} {
			raw := c.Param(key)
			if raw == "" {
				continue
			}
			id, err := strconv.ParseUint(raw, 10, strconv.IntSize-1)
			if err != nil || id == 0 || strings.IndexFunc(raw, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
				utils.BadRequest(c, "无效的记录编号")
				c.Abort()
				return
			}
		}
		c.Next()
	}
}
