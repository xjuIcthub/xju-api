package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func poolPermissionRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	previousRateLimit := common.GlobalApiRateLimitEnable
	common.GlobalApiRateLimitEnable = false
	t.Cleanup(func() { common.GlobalApiRateLimitEnable = previousRateLimit })

	router := gin.New()
	router.Use(sessions.Sessions("session", cookie.NewStore([]byte("pool-permission-test"))))
	router.GET("/test-login/:role", func(c *gin.Context) {
		role, err := strconv.Atoi(c.Param("role"))
		if err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		session := sessions.Default(c)
		session.Set("username", "pool-tester")
		session.Set("role", role)
		session.Set("id", 77)
		session.Set("status", common.UserStatusEnabled)
		session.Set("group", "default")
		require.NoError(t, session.Save())
		c.Status(http.StatusNoContent)
	})
	SetApiRouter(router)
	return router
}

func poolPermissionCookies(t *testing.T, router *gin.Engine, role int) []*http.Cookie {
	t.Helper()
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/test-login/"+strconv.Itoa(role), nil))
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	return w.Result().Cookies()
}

func performPoolPermissionRequest(router *gin.Engine, cookies []*http.Cookie, method, target string) *httptest.ResponseRecorder {
	return performPoolPermissionRequestWithBody(router, cookies, method, target, `{}`)
}

func performPoolPermissionRequestWithBody(router *gin.Engine, cookies []*http.Cookie, method, target, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("New-Api-User", "77")
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	router.ServeHTTP(w, req)
	return w
}

func TestSharedPoolRoutePermissionMatrix(t *testing.T) {
	poolServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v0/management/auth-files" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"files":[{"name":"alice.json","email":"alice@example.com","auth_index":"runtime-secret","access_token":"credential-secret"}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(poolServer.Close)
	t.Setenv("POOL_REGISTRY_FILE", filepath.Join(t.TempDir(), "pools.json"))
	t.Setenv("POOL_MGMT_URL", poolServer.URL)
	t.Setenv("POOL_MGMT_SECRET", "default-secret")
	t.Setenv("POOL_K12_MGMT_SECRET", "")
	require.NoError(t, common.AddPoolToRegistry(common.PoolEntry{
		ID: "team", Label: "Team", MgmtURL: "http://team:8319", MgmtSecret: "team-secret", Kind: common.PoolKindAdmin,
	}))
	require.NoError(t, common.AddPoolToRegistry(common.PoolEntry{
		ID: "private-42-pool", Label: "Private", MgmtURL: "http://private:8320", MgmtSecret: "private-secret",
		OwnerUserID: 42, Kind: common.PoolKindPrivate,
	}))

	router := poolPermissionRouter(t)
	commonCookies := poolPermissionCookies(t, router, common.RoleCommonUser)

	listResponse := performPoolPermissionRequest(router, commonCookies, http.MethodGet, "/api/pool/pools")
	require.Equal(t, http.StatusOK, listResponse.Code, listResponse.Body.String())
	assert.Contains(t, listResponse.Body.String(), `"id":"default"`)
	assert.Contains(t, listResponse.Body.String(), `"id":"team"`)
	assert.NotContains(t, listResponse.Body.String(), "private-42-pool")

	authFilesResponse := performPoolPermissionRequest(router, commonCookies, http.MethodGet, "/api/pool/auth-files?pool=default")
	require.Equal(t, http.StatusOK, authFilesResponse.Code, authFilesResponse.Body.String())
	assert.Contains(t, authFilesResponse.Body.String(), "alice@example.com")
	assert.NotContains(t, authFilesResponse.Body.String(), "runtime-secret")
	assert.NotContains(t, authFilesResponse.Body.String(), "credential-secret")

	usageResponse := performPoolPermissionRequest(router, commonCookies, http.MethodGet, "/api/pool/auth-files/usage?pool=default")
	require.Equal(t, http.StatusOK, usageResponse.Code, usageResponse.Body.String())
	assert.Contains(t, usageResponse.Body.String(), `"success":true`)

	verifyResponse := performPoolPermissionRequestWithBody(router, commonCookies, http.MethodPost, "/api/pool/auth-files/verify?pool=default", `{"name":"missing.json","heavy":true}`)
	assert.Equal(t, http.StatusBadGateway, verifyResponse.Code, verifyResponse.Body.String())
	assert.Contains(t, verifyResponse.Body.String(), "account not found")

	quotaRefreshResponse := performPoolPermissionRequestWithBody(router, commonCookies, http.MethodPost, "/api/pool/auth-files/usage/refresh?pool=default", `{"name":""}`)
	assert.Equal(t, http.StatusForbidden, quotaRefreshResponse.Code, quotaRefreshResponse.Body.String())
	assert.Contains(t, quotaRefreshResponse.Body.String(), "one account at a time")

	mutations := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/pool/create"},
		{http.MethodPost, "/api/pool/delete"},
		{http.MethodPost, "/api/pool/rename"},
		{http.MethodPost, "/api/pool/auth-files?pool=default"},
		{http.MethodPost, "/api/pool/auth-files/import?pool=default"},
		{http.MethodPost, "/api/pool/auth-files/clean?pool=default"},
		{http.MethodPost, "/api/pool/auth-files/verify-all?pool=default"},
		{http.MethodPost, "/api/pool/auth-files/usage/reset?pool=default"},
		{http.MethodPatch, "/api/pool/auth-files/status?pool=default"},
		{http.MethodDelete, "/api/pool/auth-files?pool=default"},
		{http.MethodPost, "/api/pool/oauth/codex/start?pool=default"},
		{http.MethodPut, "/api/pool/settings"},
		{http.MethodPut, "/api/pool/default-pricing"},
	}
	for _, mutation := range mutations {
		t.Run(mutation.method+" "+mutation.path, func(t *testing.T) {
			response := performPoolPermissionRequest(router, commonCookies, mutation.method, mutation.path)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			var envelope struct {
				Success bool `json:"success"`
			}
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
			assert.False(t, envelope.Success)
		})
	}

	adminCookies := poolPermissionCookies(t, router, common.RoleAdminUser)
	pricingResponse := performPoolPermissionRequest(router, adminCookies, http.MethodGet, "/api/pool/default-pricing")
	require.Equal(t, http.StatusOK, pricingResponse.Code, pricingResponse.Body.String())
	assert.Contains(t, pricingResponse.Body.String(), `"success":true`)

	privateResponse := performPoolPermissionRequest(router, adminCookies, http.MethodGet, "/api/pool/auth-files?pool=private-42-pool")
	assert.Equal(t, http.StatusForbidden, privateResponse.Code, privateResponse.Body.String())
}
