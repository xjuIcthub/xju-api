package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func privatePoolOAuthContext(t *testing.T, method, target, body string, owner int) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("id", owner)
	c.Set("role", common.RoleCommonUser)
	c.Set(common.ContextKeyPrivatePoolID, "oauth-pool")
	c.Set(common.ContextKeyPrivatePoolScope, true)
	c.Request = httptest.NewRequest(method, target, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, w
}

func TestPrivatePoolCodexOAuthFlow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	disablePoolTestRedis(t)
	setupModelListControllerTestDB(t)
	owner := 78101
	state := "0123456789abcdef0123456789abcdef"
	var callbackCode string
	pool := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v0/management/auth-files":
			_, _ = w.Write([]byte(`{"files":[]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v0/management/codex-auth-url":
			authURL := "https://auth.openai.com/oauth/authorize?state=" + state + "&redirect_uri=" + url.QueryEscape("http://localhost:1455/auth/callback")
			_, _ = fmt.Fprintf(w, `{"status":"ok","url":%q,"state":%q,"expires_in":1800}`, authURL, state)
		case r.Method == http.MethodPost && r.URL.Path == "/v0/management/oauth-callback":
			var payload map[string]string
			require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
			callbackCode = payload["code"]
			assert.Equal(t, state, payload["state"])
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v0/management/get-auth-status":
			assert.Equal(t, state, r.URL.Query().Get("state"))
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer pool.Close()

	dir := t.TempDir()
	t.Setenv("POOL_REGISTRY_FILE", filepath.Join(dir, "registry.json"))
	require.NoError(t, common.SavePoolRegistry([]common.PoolEntry{{
		ID: "oauth-pool", Label: "OAuth", MgmtURL: pool.URL, MgmtSecret: "secret",
		OwnerUserID: owner, Kind: common.PoolKindPrivate, GroupKey: common.PrivatePoolGroupKey(owner),
	}}))

	startContext, startRecorder := privatePoolOAuthContext(t, http.MethodPost, "/api/private-pool/oauth/codex/start", "", owner)
	StartPrivatePoolCodexOAuth(startContext)
	require.Equal(t, http.StatusOK, startRecorder.Code, startRecorder.Body.String())
	var startResponse struct {
		Data struct {
			SessionID string `json:"session_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(startRecorder.Body.Bytes(), &startResponse))
	require.NotEmpty(t, startResponse.Data.SessionID)

	callbackURL := "http://localhost:1455/auth/callback?code=one-time-code&state=" + state
	callbackBody, _ := json.Marshal(map[string]string{"session_id": startResponse.Data.SessionID, "redirect_url": callbackURL})
	callbackContext, callbackRecorder := privatePoolOAuthContext(t, http.MethodPost, "/api/private-pool/oauth/codex/callback", string(callbackBody), owner)
	SubmitPrivatePoolCodexOAuthCallback(callbackContext)
	require.Equal(t, http.StatusOK, callbackRecorder.Code, callbackRecorder.Body.String())
	assert.Equal(t, "one-time-code", callbackCode)

	statusContext, statusRecorder := privatePoolOAuthContext(t, http.MethodGet, "/api/private-pool/oauth/codex/status?session_id="+url.QueryEscape(startResponse.Data.SessionID), "", owner)
	statusContext.Request.URL.RawQuery = "session_id=" + url.QueryEscape(startResponse.Data.SessionID)
	GetPrivatePoolCodexOAuthStatus(statusContext)
	require.Equal(t, http.StatusOK, statusRecorder.Code, statusRecorder.Body.String())
	assert.Contains(t, statusRecorder.Body.String(), `"status":"ok"`)
}

func TestParseCodexCallbackURLRejectsNonLocalTarget(t *testing.T) {
	invalid := []string{
		"https://evil.example/callback?code=x&state=y",
		"http://user@localhost:1455/auth/callback?code=x&state=y",
		"http://localhost:1455/auth/callback/extra?code=x&state=y",
	}
	for _, raw := range invalid {
		_, _, _, err := parseCodexCallbackURL(raw)
		require.Error(t, err, raw)
	}
}

func TestRootPoolCodexOAuthStartsSelectedPool(t *testing.T) {
	gin.SetMode(gin.TestMode)
	disablePoolTestRedis(t)
	setupModelListControllerTestDB(t)
	owner := 78102
	state := "abcdef0123456789abcdef0123456789"
	pool := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v0/management/codex-auth-url", r.URL.Path)
		authURL := "https://auth.openai.com/oauth/authorize?state=" + state + "&redirect_uri=" + url.QueryEscape("http://localhost:1455/auth/callback")
		_, _ = fmt.Fprintf(w, `{"status":"ok","url":%q,"state":%q,"expires_in":1800}`, authURL, state)
	}))
	defer pool.Close()

	dir := t.TempDir()
	t.Setenv("POOL_REGISTRY_FILE", filepath.Join(dir, "registry.json"))
	require.NoError(t, common.SavePoolRegistry([]common.PoolEntry{{
		ID: "admin-oauth", Label: "Admin OAuth", MgmtURL: pool.URL, MgmtSecret: "secret", Kind: common.PoolKindAdmin,
	}}))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("id", owner)
	c.Set("role", common.RoleRootUser)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/pool/oauth/codex/start?pool=admin-oauth", nil)
	StartPoolCodexOAuth(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var response struct {
		Data struct {
			SessionID string `json:"session_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.NotEmpty(t, response.Data.SessionID)
	service.DeletePrivatePoolOAuthSession(response.Data.SessionID, owner)
}

func TestRootPoolClaudeOAuthFlow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	disablePoolTestRedis(t)
	setupModelListControllerTestDB(t)
	owner := 78103
	state := "claude0123456789claude0123456789"
	var callbackProvider string
	pool := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v0/management/anthropic-auth-url":
			authURL := "https://claude.ai/oauth/authorize?state=" + state + "&redirect_uri=" + url.QueryEscape("http://localhost:54545/callback")
			_, _ = fmt.Fprintf(w, `{"status":"ok","url":%q,"state":%q}`, authURL, state)
		case r.Method == http.MethodPost && r.URL.Path == "/v0/management/oauth-callback":
			var payload map[string]string
			require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
			callbackProvider = payload["provider"]
			assert.Equal(t, state, payload["state"])
			assert.Equal(t, "claude-code", payload["code"])
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v0/management/get-auth-status":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer pool.Close()

	dir := t.TempDir()
	t.Setenv("POOL_REGISTRY_FILE", filepath.Join(dir, "registry.json"))
	require.NoError(t, common.SavePoolRegistry([]common.PoolEntry{{
		ID: "claude-oauth", Label: "Claude OAuth", Provider: common.PoolProviderClaude,
		MgmtURL: pool.URL, MgmtSecret: "secret", Kind: common.PoolKindAdmin, GroupKey: "claude-oauth",
	}}))

	startRecorder := httptest.NewRecorder()
	startContext, _ := gin.CreateTestContext(startRecorder)
	startContext.Set("id", owner)
	startContext.Set("role", common.RoleRootUser)
	startContext.Request = httptest.NewRequest(http.MethodPost, "/api/pool/oauth/claude/start?pool=claude-oauth", nil)
	StartPoolClaudeOAuth(startContext)
	require.Equal(t, http.StatusOK, startRecorder.Code, startRecorder.Body.String())
	var startResponse struct {
		Data struct {
			SessionID string `json:"session_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(startRecorder.Body.Bytes(), &startResponse))
	require.NotEmpty(t, startResponse.Data.SessionID)

	callbackBody, _ := json.Marshal(map[string]string{
		"session_id":   startResponse.Data.SessionID,
		"redirect_url": "http://localhost:54545/callback?code=claude-code&state=" + state,
	})
	callbackRecorder := httptest.NewRecorder()
	callbackContext, _ := gin.CreateTestContext(callbackRecorder)
	callbackContext.Set("id", owner)
	callbackContext.Request = httptest.NewRequest(http.MethodPost, "/api/pool/oauth/claude/callback", strings.NewReader(string(callbackBody)))
	callbackContext.Request.Header.Set("Content-Type", "application/json")
	SubmitPoolClaudeOAuthCallback(callbackContext)
	require.Equal(t, http.StatusOK, callbackRecorder.Code, callbackRecorder.Body.String())
	assert.Equal(t, "anthropic", callbackProvider)

	statusRecorder := httptest.NewRecorder()
	statusContext, _ := gin.CreateTestContext(statusRecorder)
	statusContext.Set("id", owner)
	statusContext.Request = httptest.NewRequest(http.MethodGet, "/api/pool/oauth/claude/status?session_id="+url.QueryEscape(startResponse.Data.SessionID), nil)
	GetPoolClaudeOAuthStatus(statusContext)
	require.Equal(t, http.StatusOK, statusRecorder.Code, statusRecorder.Body.String())
	assert.Contains(t, statusRecorder.Body.String(), `"status":"ok"`)
}
