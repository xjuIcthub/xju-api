package middleware

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"

	"github.com/gin-gonic/gin"
)

// SharedPoolScope protects the pool selected through ?pool=. Common users and
// administrators may only target shared system/admin pools from /api/pool;
// user-owned pools stay behind the implicit-owner /api/private-pool routes.
// Root keeps its existing global troubleshooting access. Common users are
// additionally marked read-only so handlers can remove sensitive fields and
// reject maintenance variants of otherwise-safe probe endpoints.
func SharedPoolScope() gin.HandlerFunc {
	return func(c *gin.Context) {
		role := c.GetInt("role")
		if role < common.RoleRootUser {
			poolID := strings.TrimSpace(c.Query("pool"))
			pool, ok := common.FindConfiguredPoolInfo(poolID)
			if !ok || pool.Kind == common.PoolKindPrivate {
				c.JSON(http.StatusForbidden, gin.H{
					"success": false,
					"message": "shared pool access is not allowed for this pool",
				})
				c.Abort()
				return
			}
		}
		if role < common.RoleAdminUser {
			c.Set(common.ContextKeyPoolReadOnly, true)
		}
		c.Next()
	}
}
