package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSharedPoolScopeRoleMatrix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("POOL_REGISTRY_FILE", filepath.Join(t.TempDir(), "pools.json"))
	t.Setenv("POOL_MGMT_SECRET", "default-secret")
	t.Setenv("POOL_K12_MGMT_SECRET", "")
	require.NoError(t, common.AddPoolToRegistry(common.PoolEntry{
		ID: "team", Label: "Team", MgmtURL: "http://team:8319", MgmtSecret: "team-secret", Kind: common.PoolKindAdmin,
	}))
	require.NoError(t, common.AddPoolToRegistry(common.PoolEntry{
		ID: "alice", Label: "Alice", MgmtURL: "http://alice:8320", MgmtSecret: "alice-secret",
		OwnerUserID: 42, Kind: common.PoolKindPrivate,
	}))

	tests := []struct {
		name     string
		role     int
		pool     string
		status   int
		readOnly bool
	}{
		{name: "common may inspect default read-only", role: common.RoleCommonUser, pool: "default", status: http.StatusOK, readOnly: true},
		{name: "common may inspect dynamic shared pool read-only", role: common.RoleCommonUser, pool: "team", status: http.StatusOK, readOnly: true},
		{name: "common cannot target private pool", role: common.RoleCommonUser, pool: "alice", status: http.StatusForbidden},
		{name: "common cannot target unknown pool", role: common.RoleCommonUser, pool: "missing", status: http.StatusForbidden},
		{name: "admin manages shared pool", role: common.RoleAdminUser, pool: "team", status: http.StatusOK},
		{name: "admin cannot cross into private pool", role: common.RoleAdminUser, pool: "alice", status: http.StatusForbidden},
		{name: "root keeps global access", role: common.RoleRootUser, pool: "alice", status: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set("role", tt.role)
				c.Next()
			})
			router.Use(SharedPoolScope())
			router.GET("/test", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"read_only": c.GetBool(common.ContextKeyPoolReadOnly)})
			})

			w := httptest.NewRecorder()
			target := fmt.Sprintf("/test?pool=%s", tt.pool)
			router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
			assert.Equal(t, tt.status, w.Code, w.Body.String())
			if tt.status == http.StatusOK {
				assert.Contains(t, w.Body.String(), fmt.Sprintf(`"read_only":%t`, tt.readOnly))
			}
		})
	}
}
