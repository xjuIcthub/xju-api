package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

const privatePoolOAuthDefaultTTL = 30 * time.Minute

var privatePoolOAuthStatePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{16,256}$`)

type privatePoolOAuthStartResponse struct {
	Status    string `json:"status"`
	URL       string `json:"url"`
	State     string `json:"state"`
	ExpiresIn int    `json:"expires_in"`
}

type privatePoolOAuthCallbackRequest struct {
	SessionID   string `json:"session_id"`
	RedirectURL string `json:"redirect_url"`
}

type privatePoolOAuthStatusResponse struct {
	Status string `json:"status"`
	Error  string `json:"error"`
}

type poolOAuthProviderSpec struct {
	PoolProvider       string
	UpstreamProvider   string
	ManagementAuthPath string
	AuthorizationHost  string
	AuthorizationPath  string
	RedirectURL        string
}

func poolOAuthProvider(provider string) (poolOAuthProviderSpec, bool) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case common.PoolProviderCodex:
		return poolOAuthProviderSpec{
			PoolProvider:       common.PoolProviderCodex,
			UpstreamProvider:   "codex",
			ManagementAuthPath: "/v0/management/codex-auth-url",
			AuthorizationHost:  "auth.openai.com",
			AuthorizationPath:  "/oauth/authorize",
			RedirectURL:        "http://localhost:1455/auth/callback",
		}, true
	case common.PoolProviderClaude:
		return poolOAuthProviderSpec{
			PoolProvider:       common.PoolProviderClaude,
			UpstreamProvider:   "anthropic",
			ManagementAuthPath: "/v0/management/anthropic-auth-url",
			AuthorizationHost:  "claude.ai",
			AuthorizationPath:  "/oauth/authorize",
			RedirectURL:        "http://localhost:54545/callback",
		}, true
	default:
		return poolOAuthProviderSpec{}, false
	}
}

func setPrivatePoolOAuthNoStore(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
}

func validCodexAuthorizationURL(raw, state string) bool {
	spec, _ := poolOAuthProvider(common.PoolProviderCodex)
	return validPoolAuthorizationURL(raw, state, spec)
}

func validPoolAuthorizationURL(raw, state string, spec poolOAuthProviderSpec) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	query := u.Query()
	return u.Scheme == "https" &&
		u.User == nil &&
		strings.EqualFold(u.Hostname(), spec.AuthorizationHost) &&
		(u.Port() == "" || u.Port() == "443") &&
		u.Path == spec.AuthorizationPath &&
		u.Fragment == "" &&
		query.Get("state") == strings.TrimSpace(state) &&
		query.Get("redirect_uri") == spec.RedirectURL
}

func parseCodexCallbackURL(raw string) (code, state, errorMessage string, err error) {
	spec, _ := poolOAuthProvider(common.PoolProviderCodex)
	return parsePoolCallbackURL(raw, spec)
}

func parsePoolCallbackURL(raw string, spec poolOAuthProviderSpec) (code, state, errorMessage string, err error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", "", "", err
	}
	expected, _ := url.Parse(spec.RedirectURL)
	if expected == nil || u.Scheme != expected.Scheme || u.User != nil || !strings.EqualFold(u.Hostname(), expected.Hostname()) || u.Port() != expected.Port() || u.Path != expected.Path || u.Fragment != "" {
		return "", "", "", &url.Error{Op: "validate", URL: spec.PoolProvider + " callback", Err: errInvalidCodexCallbackURL}
	}
	query := u.Query()
	code = strings.TrimSpace(query.Get("code"))
	state = strings.TrimSpace(query.Get("state"))
	errorMessage = strings.TrimSpace(query.Get("error"))
	if errorMessage == "" {
		errorMessage = strings.TrimSpace(query.Get("error_description"))
	}
	if state == "" || (code == "" && errorMessage == "") {
		return "", "", "", &url.Error{Op: "validate", URL: spec.PoolProvider + " callback", Err: errInvalidCodexCallbackURL}
	}
	return code, state, errorMessage, nil
}

var errInvalidCodexCallbackURL = &privatePoolOAuthValidationError{"invalid Codex callback URL"}

type privatePoolOAuthValidationError struct{ message string }

func (e *privatePoolOAuthValidationError) Error() string { return e.message }

func poolCodexOAuthAuditAction(c *gin.Context, event string) string {
	prefix := "pool"
	if isPrivatePoolRequest(c) {
		prefix = "private_pool"
	}
	return prefix + ".oauth_" + event
}

func poolCodexOAuthNeedsAccountLimit(poolID string) bool {
	entry, ok := common.GetPoolEntry(poolID)
	return ok && entry.Kind == common.PoolKindPrivate
}

// StartPoolCodexOAuth creates a Codex OAuth flow in the root-selected pool.
func StartPoolCodexOAuth(c *gin.Context) {
	poolID := strings.TrimSpace(poolIDFromRequest(c))
	startPoolOAuth(c, poolID, common.PoolProviderCodex, poolCodexOAuthNeedsAccountLimit(poolID))
}

// StartPoolClaudeOAuth creates the only supported Claude-account import flow.
// It is intentionally exposed only on the root-managed shared-pool routes.
func StartPoolClaudeOAuth(c *gin.Context) {
	poolID := strings.TrimSpace(poolIDFromRequest(c))
	startPoolOAuth(c, poolID, common.PoolProviderClaude, false)
}

// StartPrivatePoolCodexOAuth creates a Codex OAuth flow inside the current
// user's CLIProxyAPI instance. Both entry points intentionally omit
// is_webui=1: the browser returns the localhost callback URL to this API.
func StartPrivatePoolCodexOAuth(c *gin.Context) {
	startPoolOAuth(c, poolIDFromRequest(c), common.PoolProviderCodex, true)
}

func startPoolOAuth(c *gin.Context, poolID, provider string, enforceAccountLimit bool) {
	setPrivatePoolOAuthNoStore(c)
	spec, supported := poolOAuthProvider(provider)
	if !supported {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "unsupported pool OAuth provider"})
		return
	}
	pool, found := common.FindConfiguredPoolInfo(poolID)
	if !found || common.NormalizePoolProvider(pool.Provider) != spec.PoolProvider {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "selected pool does not accept " + spec.PoolProvider + " accounts"})
		return
	}
	if spec.PoolProvider == common.PoolProviderClaude && pool.Kind == common.PoolKindPrivate {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Claude accounts can be added to shared pools only"})
		return
	}
	ownerUserID := c.GetInt("id")
	session, err := service.ReservePrivatePoolOAuthSession(ownerUserID, poolID, spec.PoolProvider, privatePoolOAuthDefaultTTL)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": err.Error()})
		return
	}
	release := true
	defer func() {
		if release {
			service.DeletePrivatePoolOAuthSession(session.ID, ownerUserID)
		}
	}()

	baseURL, secret, ok := common.ResolvePoolMgmt(poolID)
	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "pool is not ready"})
		return
	}
	if enforceAccountLimit {
		existing, err := privatePoolExistingAccountNames(c.Request.Context(), baseURL, secret)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": err.Error()})
			return
		}
		if len(existing)+service.CountPrivatePoolOAuthReservations(poolID) > privateMaxAccounts {
			c.JSON(http.StatusConflict, gin.H{"success": false, "message": "private pool account limit is " + strconv.Itoa(privateMaxAccounts)})
			return
		}
	}

	status, payload, err := service.PoolMgmtRoundTrip(c.Request.Context(), baseURL, secret, http.MethodGet, spec.ManagementAuthPath, nil, "")
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": "pool management unreachable"})
		return
	}
	if status < 200 || status >= 300 {
		c.JSON(status, gin.H{"success": false, "message": poolErrorMessage(payload, status)})
		return
	}
	var upstream privatePoolOAuthStartResponse
	if err := json.Unmarshal(payload, &upstream); err != nil || !privatePoolOAuthStatePattern.MatchString(strings.TrimSpace(upstream.State)) || !validPoolAuthorizationURL(upstream.URL, upstream.State, spec) {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": "pool returned an invalid OAuth session"})
		return
	}
	if upstream.ExpiresIn < 60 || upstream.ExpiresIn > 3600 {
		upstream.ExpiresIn = int(privatePoolOAuthDefaultTTL / time.Second)
	}
	activated, err := service.ActivatePrivatePoolOAuthSession(session.ID, upstream.State, time.Now().Add(time.Duration(upstream.ExpiresIn)*time.Second))
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": err.Error()})
		return
	}
	release = false
	recordPoolAudit(c, poolCodexOAuthAuditAction(c, "start"), map[string]interface{}{"pool": auditPoolID(poolID), "provider": spec.PoolProvider})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"session_id": activated.ID,
		"status":     activated.Phase,
		"url":        upstream.URL,
		"expires_in": upstream.ExpiresIn,
		"expires_at": activated.ExpiresAt.Unix(),
	}})
}

// SubmitPrivatePoolCodexOAuthCallback accepts only the registered localhost
// callback shape, extracts its one-time code, verifies state ownership, and
// immediately forwards it to the owner's pool without persisting the URL/code.
func SubmitPrivatePoolCodexOAuthCallback(c *gin.Context) {
	submitPoolOAuthCallback(c, common.PoolProviderCodex)
}

func SubmitPoolClaudeOAuthCallback(c *gin.Context) {
	submitPoolOAuthCallback(c, common.PoolProviderClaude)
}

func submitPoolOAuthCallback(c *gin.Context, provider string) {
	setPrivatePoolOAuthNoStore(c)
	spec, supported := poolOAuthProvider(provider)
	if !supported {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "unsupported pool OAuth provider"})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	var req privatePoolOAuthCallbackRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request body"})
		return
	}
	ownerUserID := c.GetInt("id")
	session, ok := service.GetPrivatePoolOAuthSession(req.SessionID, ownerUserID)
	if !ok || session.Provider != spec.PoolProvider || session.UpstreamState == "" {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "login session expired"})
		return
	}
	code, state, errorMessage, err := parsePoolCallbackURL(req.RedirectURL, spec)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "paste the complete " + spec.RedirectURL + " callback URL"})
		return
	}
	if state != session.UpstreamState {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "OAuth state does not match this login session"})
		return
	}
	body, _ := json.Marshal(gin.H{"provider": spec.UpstreamProvider, "state": state, "code": code, "error": errorMessage})
	baseURL, secret, ready := common.ResolvePoolMgmt(session.PoolID)
	if !ready {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "pool is not ready"})
		return
	}
	status, payload, err := service.PoolMgmtRoundTrip(c.Request.Context(), baseURL, secret, http.MethodPost, "/v0/management/oauth-callback", bytes.NewReader(body), "application/json")
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": "pool management unreachable"})
		return
	}
	if status < 200 || status >= 300 {
		c.JSON(status, gin.H{"success": false, "message": poolErrorMessage(payload, status)})
		return
	}
	updated, err := service.MarkPrivatePoolOAuthCallbackSubmitted(session.ID, ownerUserID)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": err.Error()})
		return
	}
	recordPoolAudit(c, poolCodexOAuthAuditAction(c, "callback"), map[string]interface{}{"pool": auditPoolID(session.PoolID), "provider": spec.PoolProvider})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"session_id": updated.ID, "status": updated.Phase}})
}

func GetPrivatePoolCodexOAuthStatus(c *gin.Context) {
	getPoolOAuthStatus(c, common.PoolProviderCodex)
}

func GetPoolClaudeOAuthStatus(c *gin.Context) {
	getPoolOAuthStatus(c, common.PoolProviderClaude)
}

func getPoolOAuthStatus(c *gin.Context, provider string) {
	setPrivatePoolOAuthNoStore(c)
	spec, supported := poolOAuthProvider(provider)
	if !supported {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "unsupported pool OAuth provider"})
		return
	}
	ownerUserID := c.GetInt("id")
	sessionID := strings.TrimSpace(c.Query("session_id"))
	session, ok := service.GetPrivatePoolOAuthSession(sessionID, ownerUserID)
	if !ok || session.Provider != spec.PoolProvider {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "login session expired"})
		return
	}
	baseURL, secret, ready := common.ResolvePoolMgmt(session.PoolID)
	if !ready {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "pool is not ready"})
		return
	}
	path := "/v0/management/get-auth-status?state=" + url.QueryEscape(session.UpstreamState)
	status, payload, err := service.PoolMgmtRoundTrip(c.Request.Context(), baseURL, secret, http.MethodGet, path, nil, "")
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": "pool management unreachable"})
		return
	}
	if status < 200 || status >= 300 {
		c.JSON(status, gin.H{"success": false, "message": poolErrorMessage(payload, status)})
		return
	}
	var upstream privatePoolOAuthStatusResponse
	if err := json.Unmarshal(payload, &upstream); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": "pool returned an invalid OAuth status"})
		return
	}
	switch upstream.Status {
	case "ok":
		service.DeletePrivatePoolOAuthSession(session.ID, ownerUserID)
		recordPoolAudit(c, poolCodexOAuthAuditAction(c, "complete"), map[string]interface{}{"pool": auditPoolID(session.PoolID), "provider": spec.PoolProvider})
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"status": "ok"}})
	case "error":
		service.DeletePrivatePoolOAuthSession(session.ID, ownerUserID)
		message := strings.TrimSpace(upstream.Error)
		if message == "" {
			message = "authentication failed"
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"status": "error", "error": message}})
	default:
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"status": session.Phase}})
	}
}

func CancelPrivatePoolCodexOAuth(c *gin.Context) {
	cancelPoolOAuth(c, common.PoolProviderCodex)
}

func CancelPoolClaudeOAuth(c *gin.Context) {
	cancelPoolOAuth(c, common.PoolProviderClaude)
}

func cancelPoolOAuth(c *gin.Context, provider string) {
	setPrivatePoolOAuthNoStore(c)
	spec, supported := poolOAuthProvider(provider)
	if !supported {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "unsupported pool OAuth provider"})
		return
	}
	ownerUserID := c.GetInt("id")
	sessionID := strings.TrimSpace(c.Query("session_id"))
	session, ok := service.GetPrivatePoolOAuthSession(sessionID, ownerUserID)
	if !ok || session.Provider != spec.PoolProvider {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"status": "cancelled"}})
		return
	}
	if baseURL, secret, ready := common.ResolvePoolMgmt(session.PoolID); ready && session.UpstreamState != "" {
		path := "/v0/management/oauth-session?state=" + url.QueryEscape(session.UpstreamState)
		_, _, _ = service.PoolMgmtRoundTrip(c.Request.Context(), baseURL, secret, http.MethodDelete, path, nil, "")
	}
	service.DeletePrivatePoolOAuthSession(session.ID, ownerUserID)
	recordPoolAudit(c, poolCodexOAuthAuditAction(c, "cancel"), map[string]interface{}{"pool": auditPoolID(session.PoolID), "provider": spec.PoolProvider})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"status": "cancelled"}})
}
