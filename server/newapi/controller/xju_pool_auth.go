package controller

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// xju-api:new — pool-authentication proxy.
//
// xju-api runs a three-tier stack: this new-api instance (L1) sits in front of
// a CLIProxyAPI account pool (L2). Adding an upstream account means dropping a
// codex auth JSON into the pool, which CLIProxyAPI exposes through its
// management API (`/v0/management/auth-files`). That API is bound to the
// internal network and gated by a management secret, so the browser can neither
// reach it nor be trusted with the secret. These handlers are the thin bridge:
// the secret lives only here, in this process's environment. Shared read-only
// responses are sanitized before they reach common users.
//
//   POOL_MGMT_URL     base URL of the pool management API
//                     (default http://cli-proxy-api:8317 — the docker service name)
//   POOL_MGMT_SECRET  the CLIProxyAPI `remote-management.secret-key`
//
// When POOL_MGMT_SECRET is empty the feature is simply off and every handler
// answers 503, so a deployment that doesn't wire it up degrades cleanly.

// poolMgmtProxy forwards a request to the given pool's management API and copies
// the upstream status + body back to the caller under the uniform envelope.
// It reports whether the upstream call succeeded so mutating handlers can write
// their audit entry only for operations that actually happened.
// HTTP 传输走 service.PoolMgmtRoundTrip(round-trip 单一来源,REFACTOR-PLAN §5.2)。
func poolMgmtProxy(c *gin.Context, poolID, method, path string, body io.Reader, contentType string) bool {
	baseURL, secret, ok := common.ResolvePoolMgmt(poolID)
	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"message": "pool management is not configured for pool: " + poolID,
		})
		return false
	}
	status, payload, err := service.PoolMgmtRoundTrip(c.Request.Context(), baseURL, secret, method, path, body, contentType)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"success": false,
			"message": fmt.Sprintf("pool management unreachable: %v", err),
		})
		return false
	}
	if status >= 200 && status < 300 {
		if shouldSanitizePoolResponse(c) && method == http.MethodGet && path == "/v0/management/auth-files" {
			payload, err = sanitizePrivatePoolAuthList(payload)
			if err != nil {
				c.JSON(http.StatusBadGateway, gin.H{
					"success": false,
					"message": "pool management returned an unreadable account list",
				})
				return false
			}
		}
		c.Data(http.StatusOK, "application/json; charset=utf-8", wrapPoolSuccess(payload))
		return true
	}
	c.JSON(status, gin.H{
		"success": false,
		"message": poolErrorMessage(payload, status),
	})
	return false
}

func poolIDFromRequest(c *gin.Context) string {
	if poolID, ok := c.Get(common.ContextKeyPrivatePoolID); ok {
		if value, valid := poolID.(string); valid {
			return value
		}
	}
	return c.Query("pool")
}

func isPrivatePoolRequest(c *gin.Context) bool {
	return c.GetBool(common.ContextKeyPrivatePoolScope)
}

func isPoolReadOnlyRequest(c *gin.Context) bool {
	return c.GetBool(common.ContextKeyPoolReadOnly)
}

func shouldSanitizePoolResponse(c *gin.Context) bool {
	return isPrivatePoolRequest(c) || isPoolReadOnlyRequest(c)
}

func recordPoolAudit(c *gin.Context, action string, params map[string]interface{}) {
	if shouldSanitizePoolResponse(c) {
		recordUserSecurityAudit(c, c.GetInt("id"), action, params)
		return
	}
	recordManageAudit(c, action, params)
}

func sanitizePrivatePoolAuthList(raw []byte) ([]byte, error) {
	var payload any
	if err := common.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	return common.Marshal(sanitizePrivatePoolValue(payload))
}

func sanitizePrivatePoolValue(value any) any {
	switch typed := value.(type) {
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, sanitizePrivatePoolValue(item))
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			switch strings.ToLower(key) {
			case "path", "auth_index", "access_token", "refresh_token", "api_key", "key", "token", "credential", "credentials", "proxy_url":
				continue
			case "id_token":
				if _, ok := item.(map[string]any); !ok {
					continue // keep decoded claims, never return the raw JWT string
				}
			}
			out[key] = sanitizePrivatePoolValue(item)
		}
		return out
	default:
		return value
	}
}

func writePoolSuccessData(c *gin.Context, data any) {
	if !shouldSanitizePoolResponse(c) {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
		return
	}
	raw, err := common.Marshal(data)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	safe, err := sanitizePrivatePoolAuthList(raw)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", wrapPoolSuccess(safe))
}

// ListPools GET /api/pool/pools — the configured pools (default + k12) so the
// frontend can render a pool selector and hide unconfigured pools.
func ListPools(c *gin.Context) {
	pools := common.ListSharedPools()
	if c.GetInt("role") >= common.RoleRootUser {
		pools = common.ListConfiguredPools()
	}
	for i := range pools {
		if pools[i].OwnerUserID <= 0 {
			continue
		}
		if username, err := model.GetUsernameById(pools[i].OwnerUserID, false); err == nil {
			pools[i].OwnerUsername = username
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": pools})
}

var sharedPoolSettingKeys = map[string]bool{
	"PoolAutoCleanEnabled":        true,
	"PoolUsageAutoRefreshEnabled": true,
	"PoolUsageAutoResetEnabled":   true,
}

// UpdateSharedPoolSetting PUT /api/pool/settings — narrow admin surface for
// the three shared-pool automation switches. It deliberately does not expose
// the root-only generic option endpoint to role-10 administrators.
func UpdateSharedPoolSetting(c *gin.Context) {
	var req OptionUpdateRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request body"})
		return
	}
	key := strings.TrimSpace(req.Key)
	if !sharedPoolSettingKeys[key] {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "pool setting is not allowed"})
		return
	}
	value, err := strconv.ParseBool(strings.TrimSpace(fmt.Sprintf("%v", req.Value)))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "setting value must be true or false"})
		return
	}
	if err := model.UpdateOption(key, strconv.FormatBool(value)); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "option.update", map[string]interface{}{"key": key})
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// GetPrivatePool GET /api/private-pool — current user's pool lifecycle state.
// A successful watcher result is finalized here, so the frontend only needs to
// poll this implicit-owner endpoint and never stores or submits a pool id.
func GetPrivatePool(c *gin.Context) {
	userID := c.GetInt("id")
	state, err := service.GetPrivatePoolProvisionState(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}
	data := gin.H{
		"status":            state.Status,
		"provision_enabled": service.ProvisionEnabled(),
	}
	if state.PoolID != "" {
		data["pool_id"] = state.PoolID
	}
	if state.Label != "" {
		data["label"] = state.Label
	}
	if state.Error != "" {
		data["error"] = state.Error
	}
	if state.Status == "ready" {
		pools := common.ListPoolsForActor(userID, c.GetInt("role"))
		if len(pools) == 1 {
			data["pool"] = pools[0]
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

type createPrivatePoolRequest struct {
	Label string `json:"label"`
}

// CreatePrivatePool POST /api/private-pool — create exactly one isolated pool
// for the authenticated user. Label is optional; mode/owner/group are never
// accepted from the client.
func CreatePrivatePool(c *gin.Context) {
	if !service.ProvisionEnabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "pool provisioning is not enabled on this deployment"})
		return
	}
	userID := c.GetInt("id")
	state, err := service.GetPrivatePoolProvisionState(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}
	if state.Status == "ready" || state.Status == "provisioning" {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "private pool already exists", "data": state})
		return
	}

	var reqBody createPrivatePoolRequest
	if c.Request.Body != nil && c.Request.ContentLength != 0 {
		if err := common.DecodeJson(c.Request.Body, &reqBody); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request body"})
			return
		}
	}
	label := strings.TrimSpace(reqBody.Label)
	if label == "" {
		label = strings.TrimSpace(c.GetString("username")) + " 的私人号池"
	}
	if len([]rune(label)) > 80 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "pool name is too long"})
		return
	}
	poolID, err := service.RequestPrivatePoolProvision(label, userID)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": err.Error()})
		return
	}
	recordUserSecurityAudit(c, userID, "private_pool.create", map[string]interface{}{"pool": poolID})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"pool_id": poolID, "status": "provisioning", "label": label}})
}

func wrapPoolSuccess(raw []byte) []byte {
	// Return the upstream body verbatim under `data` without re-marshaling the
	// (already valid) JSON, so list responses keep their exact shape.
	var buf strings.Builder
	buf.WriteString(`{"success":true,"data":`)
	if len(raw) == 0 {
		buf.WriteString("null")
	} else {
		buf.Write(raw)
	}
	buf.WriteString(`}`)
	return []byte(buf.String())
}

func poolErrorMessage(raw []byte, status int) string {
	var parsed struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := common.Unmarshal(raw, &parsed); err == nil {
		if parsed.Error != "" {
			return parsed.Error
		}
		if parsed.Message != "" {
			return parsed.Message
		}
	}
	return fmt.Sprintf("pool management returned HTTP %d", status)
}

// ListPoolAuthFiles GET /api/pool/auth-files — the accounts currently in the pool.
func ListPoolAuthFiles(c *gin.Context) {
	poolMgmtProxy(c, poolIDFromRequest(c), http.MethodGet, "/v0/management/auth-files", nil, "")
}

type addPoolAuthFileRequest struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

// AddPoolAuthFile POST /api/pool/auth-files — paste one auth JSON into the pool.
// CLIProxyAPI hot-reloads the pool on write, so no container restart is needed.
func AddPoolAuthFile(c *gin.Context) {
	if isPrivatePoolRequest(c) {
		// Bound the whole JSON request before decoding it; checking Content after
		// DecodeJson would still let an oversized body occupy memory first.
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, privateMaxImportEntryBytes+(64<<10))
	}
	var reqBody addPoolAuthFileRequest
	if err := common.DecodeJson(c.Request.Body, &reqBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request body"})
		return
	}

	content := strings.TrimSpace(reqBody.Content)
	if content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "content is empty"})
		return
	}
	if isPrivatePoolRequest(c) && len(content) > privateMaxImportEntryBytes {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "credential JSON is too large"})
		return
	}
	// Validate it is real JSON before it ever reaches the pool — a malformed
	// paste should fail here with a clear message, not on the pool side.
	var probe any
	if err := common.UnmarshalJsonStr(content, &probe); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "content is not valid JSON"})
		return
	}

	poolID := poolIDFromRequest(c)
	baseURL, secret, ok := common.ResolvePoolMgmt(poolID)
	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"message": "pool management is not configured for pool: " + poolID,
		})
		return
	}

	// A pasted blob can be one codex object, or an exporter bundle / JSON array
	// holding many accounts; expand it so every account is imported, not just the
	// first. Names come from each account's email.
	items := parsePoolAuthAccounts(content, reqBody.Name)
	if len(items) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "no account found in the JSON"})
		return
	}
	privateSkipped := make([]importSkip, 0)
	if isPrivatePoolRequest(c) {
		items, privateSkipped = filterPrivatePoolCodexItems(items)
		if len(items) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "private pools accept Codex credentials only", "data": gin.H{"skipped": privateSkipped}})
			return
		}
		if err := enforcePrivatePoolAccountLimit(c.Request.Context(), poolID, baseURL, secret, items); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
			return
		}
	}

	imported, failed, skipped, err := forwardPoolAuthItems(c.Request.Context(), baseURL, secret, poolID, items)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": err.Error()})
		return
	}
	skipped = append(privateSkipped, skipped...)

	// xju-api:edit — 审计只记池与计数,凭证内容/文件名永不入审计。
	recordPoolAudit(c, "pool_auth.add", map[string]interface{}{
		"pool":     auditPoolID(poolID),
		"imported": imported,
		"skipped":  len(skipped),
		"failed":   len(failed),
	})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"imported": imported,
			"skipped":  skipped,
			"failed":   failed,
		},
	})
}

// poolAuthItem is one single-account auth file ready to upload: a pool-safe
// basename and the exact JSON the pool stores.
type poolAuthItem struct {
	name    string
	content string
}

// parsePoolAuthAccounts expands a pasted or uploaded JSON blob into one or more
// single-account items. It accepts a bare codex auth object, an exporter bundle
// {accounts:[{credentials:{...}}]}, or a JSON array of either — so one file or one
// paste can carry many accounts and every one of them is imported, not just the
// first. Each expanded account's name is derived from its email as
// codex-<slug>.json (mirroring the frontend), so a bundle of N accounts yields N
// distinct files. rawName is the fallback name only for a single bare object with
// no email (the frontend already supplies one). A blob that is not valid JSON
// passes through unchanged as one item so the pool side returns the parse error.
func parsePoolAuthAccounts(content, rawName string) []poolAuthItem {
	trimmed := strings.TrimSpace(content)
	var top any
	if err := common.UnmarshalJsonStr(trimmed, &top); err != nil {
		return []poolAuthItem{{name: sanitizePoolAuthName(rawName), content: trimmed}}
	}
	switch v := top.(type) {
	case map[string]any:
		if accounts, ok := v["accounts"].([]any); ok && len(accounts) > 0 {
			return expandPoolAuthAccounts(accounts)
		}
		return []poolAuthItem{singlePoolAuthItem(v, rawName, trimmed)}
	case []any:
		return expandPoolAuthAccounts(v)
	default:
		return []poolAuthItem{{name: sanitizePoolAuthName(rawName), content: trimmed}}
	}
}

// expandPoolAuthAccounts turns a list of account entries — each either a bundle
// account with a nested `credentials`, or a bare codex object — into items,
// dropping any element that is not an object or cannot be re-marshaled.
func expandPoolAuthAccounts(list []any) []poolAuthItem {
	items := make([]poolAuthItem, 0, len(list))
	for i, el := range list {
		obj, ok := el.(map[string]any)
		if !ok {
			continue
		}
		accountName := stringField(obj, "name")
		cred := obj
		if normalized, ok := singleWrappedCodexCredential(obj); ok {
			// Exporters (go-pool / sub2) wrap each account as
			// {platform:"openai",type:"oauth",credentials:{...}}, carrying the
			// provider marker on the wrapper rather than inside credentials.
			// Normalize so the written file gets the top-level "type":"codex"
			// (and account_id) that CLIProxyAPI requires; without it the account
			// is silently dropped at load time and every model 502s with
			// "unknown provider".
			cred = normalized
		} else if inner, ok := obj["credentials"].(map[string]any); ok {
			cred = inner
		}
		raw, err := common.Marshal(cred)
		if err != nil {
			continue
		}
		items = append(items, poolAuthItem{
			name:    poolAuthAccountName(cred, accountName, i),
			content: string(raw),
		})
	}
	return items
}

// singlePoolAuthItem wraps a bare codex object, keeping the caller-supplied name
// when present (the frontend derives it) and otherwise deriving one from the
// object's email. Some exporters wrap a single OpenAI OAuth account as
// {platform:"openai",type:"oauth",credentials:{...}} instead of placing it in
// an accounts array. Normalize that shape here so it follows the same path as
// the already-supported bundle entries.
func singlePoolAuthItem(obj map[string]any, rawName, content string) poolAuthItem {
	if cred, ok := singleWrappedCodexCredential(obj); ok {
		if raw, err := common.Marshal(cred); err == nil {
			name := strings.TrimSpace(rawName)
			if name == "" {
				name = poolAuthAccountName(cred, stringField(obj, "name"), 0)
			}
			return poolAuthItem{name: sanitizePoolAuthName(name), content: string(raw)}
		}
	}
	name := strings.TrimSpace(rawName)
	if name == "" {
		name = poolAuthAccountName(obj, "", 0)
	}
	return poolAuthItem{name: sanitizePoolAuthName(name), content: content}
}

// singleWrappedCodexCredential recognizes a single-account exporter wrapper
// and returns the CLIProxyAPI-compatible top-level Codex credential. Requiring
// an explicit OpenAI/Codex platform prevents another provider's OAuth wrapper
// from being silently reclassified as Codex.
func singleWrappedCodexCredential(obj map[string]any) (map[string]any, bool) {
	inner, ok := obj["credentials"].(map[string]any)
	if !ok || len(inner) == 0 {
		return nil, false
	}
	platform := strings.ToLower(strings.TrimSpace(stringField(obj, "platform")))
	if platform != "openai" && platform != "codex" {
		return nil, false
	}
	if stringField(inner, "access_token") == "" &&
		stringField(inner, "refresh_token") == "" &&
		stringField(inner, "id_token") == "" {
		return nil, false
	}

	cred := make(map[string]any, len(inner)+3)
	for key, value := range inner {
		cred[key] = value
	}
	cred["type"] = "codex"
	if stringField(cred, "account_id") == "" {
		if accountID := stringField(cred, "chatgpt_account_id"); accountID != "" {
			cred["account_id"] = accountID
		}
	}
	if _, exists := cred["priority"]; !exists {
		if priority, exists := obj["priority"]; exists {
			cred["priority"] = priority
		}
	}
	return cred, true
}

// poolAuthAccountName builds codex-<slug>.json from the account email, falling
// back to the account's display name, then a 1-based index, so every expanded
// account gets a stable, collision-free basename.
func poolAuthAccountName(cred map[string]any, accountName string, index int) string {
	if slug := poolAuthSlug(stringField(cred, "email")); slug != "" {
		return "codex-" + slug + ".json"
	}
	if slug := poolAuthSlug(accountName); slug != "" {
		return "codex-" + slug + ".json"
	}
	return fmt.Sprintf("codex-account-%d.json", index+1)
}

// poolAuthSlug lowercases a string and collapses non-alphanumeric runs into single
// dashes (capped at 48 chars), matching the frontend deriveAuthFileName so the
// same account resolves to the same file whether added singly or via a bundle.
func poolAuthSlug(s string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastDash = false
		case b.Len() > 0 && !lastDash:
			b.WriteByte('-')
			lastDash = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	if len(slug) > 48 {
		slug = strings.Trim(slug[:48], "-")
	}
	return slug
}

func stringField(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

type patchPoolAuthStatusRequest struct {
	Name     string `json:"name"`
	Disabled *bool  `json:"disabled"`
}

// SetPoolAuthFileStatus PATCH /api/pool/auth-files/status — disable/enable one
// account without deleting it (a depleted account can be re-enabled after top-up).
func SetPoolAuthFileStatus(c *gin.Context) {
	var reqBody patchPoolAuthStatusRequest
	if err := common.DecodeJson(c.Request.Body, &reqBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request body"})
		return
	}
	if strings.TrimSpace(reqBody.Name) == "" || reqBody.Disabled == nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "name and disabled are required"})
		return
	}
	name := sanitizePoolAuthName(reqBody.Name)
	body, err := common.Marshal(map[string]any{
		"name":     name,
		"disabled": *reqBody.Disabled,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	ok := poolMgmtProxy(
		c, poolIDFromRequest(c), http.MethodPatch, "/v0/management/auth-files/status",
		strings.NewReader(string(body)), "application/json",
	)
	if ok {
		recordPoolAudit(c, "pool_auth.status", map[string]interface{}{
			"pool":     auditPoolID(poolIDFromRequest(c)),
			"name":     name,
			"disabled": *reqBody.Disabled,
		})
	}
}

// CleanPoolAuthFilesNow POST /api/pool/auth-files/clean — run the stale-account
// sweep on demand (same logic as the hourly auto-clean), using the current
// PoolAutoCleanHours threshold. Returns how many accounts were disabled.
func CleanPoolAuthFilesNow(c *gin.Context) {
	poolID := poolIDFromRequest(c)
	if _, _, ok := common.ResolvePoolMgmt(poolID); !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"message": "pool management is not configured for pool: " + poolID,
		})
		return
	}
	hours := common.PoolAutoCleanHours
	if isPrivatePoolRequest(c) {
		if entry, ok := common.GetPoolEntry(poolID); ok && entry.AutoCleanHours > 0 {
			hours = entry.AutoCleanHours
		} else {
			hours = 24
		}
	}
	disabled, err := service.SweepPoolOnceForPool(poolID, hours)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": err.Error()})
		return
	}
	recordPoolAudit(c, "pool_auth.clean", map[string]interface{}{
		"pool":     auditPoolID(poolID),
		"disabled": disabled,
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"disabled": disabled}})
}

// DeletePoolAuthFile DELETE /api/pool/auth-files?name=xxx — remove one account.
func DeletePoolAuthFile(c *gin.Context) {
	name := sanitizePoolAuthName(c.Query("name"))
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "name is required"})
		return
	}
	ok := poolMgmtProxy(
		c, poolIDFromRequest(c), http.MethodDelete,
		"/v0/management/auth-files?name="+url.QueryEscape(name),
		nil, "",
	)
	if ok {
		recordPoolAudit(c, "pool_auth.delete", map[string]interface{}{
			"pool": auditPoolID(poolIDFromRequest(c)),
			"name": name,
		})
	}
}

// xju-api:new — active verification (号池验活 Part A). These handlers verify
// whether pool accounts are actually online by pinning a probe request to each
// via cliproxy's api-call, rather than trusting the passively-set `unavailable`
// flag. Single verify only reports; verify-all can opt into auto-disabling the
// accounts it finds genuinely dead.

type verifyPoolAuthFileRequest struct {
	Name  string `json:"name"`
	Heavy bool   `json:"heavy"`
}

// VerifyPoolAuthFile POST /api/pool/auth-files/verify — verify one account and
// return its verdict. Report-only: it never changes account state.
func VerifyPoolAuthFile(c *gin.Context) {
	var reqBody verifyPoolAuthFileRequest
	if err := common.DecodeJson(c.Request.Body, &reqBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request body"})
		return
	}
	name := sanitizePoolAuthName(reqBody.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "name is required"})
		return
	}
	if isPoolReadOnlyRequest(c) {
		// The deep probe performs a tiny inference. Shared read-only users get the
		// zero-quota light probe even if they forge heavy=true in a direct request.
		reqBody.Heavy = false
	}
	result, err := service.ProbeAuthByName(poolIDFromRequest(c), name, reqBody.Heavy)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": err.Error()})
		return
	}
	recordPoolAudit(c, "pool_auth.verify", map[string]interface{}{
		"pool":    auditPoolID(poolIDFromRequest(c)),
		"name":    name,
		"verdict": string(result.Verdict),
	})
	writePoolSuccessData(c, result)
}

type verifyPoolAllRequest struct {
	Heavy       bool `json:"heavy"`
	AutoDisable bool `json:"auto_disable"`
}

// VerifyPoolAuthFilesNow POST /api/pool/auth-files/verify-all — start a
// background full-pool verification and return the initial job snapshot. The
// frontend polls the progress endpoint. Refuses to start a second run while one
// is already in flight for the pool.
func VerifyPoolAuthFilesNow(c *gin.Context) {
	var reqBody verifyPoolAllRequest
	// Body is optional (both flags default false); ignore decode errors on empty.
	_ = common.DecodeJson(c.Request.Body, &reqBody)

	snap, err := service.StartProbePoolJob(poolIDFromRequest(c), reqBody.Heavy, reqBody.AutoDisable)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": err.Error(), "data": snap})
		return
	}
	recordPoolAudit(c, "pool_auth.verify_all", map[string]interface{}{
		"pool":         auditPoolID(poolIDFromRequest(c)),
		"heavy":        reqBody.Heavy,
		"auto_disable": reqBody.AutoDisable,
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": snap})
}

// GetVerifyPoolProgress GET /api/pool/auth-files/verify-all/progress — the
// latest verify-all job snapshot for the pool (running or finished).
func GetVerifyPoolProgress(c *gin.Context) {
	snap, ok := service.GetProbePoolJob(poolIDFromRequest(c))
	if !ok {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": snap})
}

// xju-api:new — per-account quota (额度). GET returns the cached snapshots +
// refresh-job progress; refresh updates one account synchronously or the whole
// pool in the background; reset consumes one ChatGPT reset credit on demand.

// GetPoolAccountUsage GET /api/pool/auth-files/usage — cached quota snapshots
// (keyed by account file name) plus the latest refresh-job progress.
func GetPoolAccountUsage(c *gin.Context) {
	poolID := poolIDFromRequest(c)
	if _, _, ok := common.ResolvePoolMgmt(poolID); !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"message": "pool management is not configured for pool: " + poolID,
		})
		return
	}
	data := gin.H{"accounts": service.GetPoolUsageSnapshots(poolID)}
	if snap, ok := service.GetPoolUsageJob(poolID); ok {
		data["job"] = snap
	}
	writePoolSuccessData(c, data)
}

type refreshPoolUsageRequest struct {
	Name string `json:"name"`
}

// RefreshPoolAccountUsage POST /api/pool/auth-files/usage/refresh — with a name,
// refresh that account's quota synchronously and return it; without one, start
// a background whole-pool refresh (poll GetPoolAccountUsage for progress).
func RefreshPoolAccountUsage(c *gin.Context) {
	var reqBody refreshPoolUsageRequest
	// Body is optional (empty means whole-pool); ignore decode errors on empty.
	_ = common.DecodeJson(c.Request.Body, &reqBody)

	poolID := poolIDFromRequest(c)
	name := strings.TrimSpace(reqBody.Name)
	if name != "" {
		usage, err := service.RefreshPoolAccountUsageByName(poolID, sanitizePoolAuthName(name))
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": err.Error()})
			return
		}
		writePoolSuccessData(c, usage)
		return
	}
	if isPoolReadOnlyRequest(c) {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "read-only users may refresh one account at a time",
		})
		return
	}

	// The manual whole-pool button is targeted: only exhausted/unknown accounts
	// are re-fetched; accounts with quota left are skipped.
	autoReset := common.PoolUsageAutoResetEnabled
	if isPrivatePoolRequest(c) {
		autoReset = false
		if entry, ok := common.GetPoolEntry(poolID); ok {
			autoReset = entry.UsageAutoResetEnabled
		}
	}
	snap, err := service.StartPoolUsageRefreshJob(poolID, autoReset, true)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": err.Error(), "data": snap})
		return
	}
	recordPoolAudit(c, "pool_auth.usage_refresh", map[string]interface{}{
		"pool": auditPoolID(poolID),
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": snap})
}

type resetPoolQuotaRequest struct {
	Name string `json:"name"`
}

// ResetPoolAccountQuota POST /api/pool/auth-files/usage/reset — consume one
// ChatGPT reset credit on the account and return its renewed quota snapshot.
func ResetPoolAccountQuota(c *gin.Context) {
	var reqBody resetPoolQuotaRequest
	if err := common.DecodeJson(c.Request.Body, &reqBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request body"})
		return
	}
	name := sanitizePoolAuthName(reqBody.Name)
	if strings.TrimSpace(reqBody.Name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "name is required"})
		return
	}
	usage, err := service.ResetPoolAccountQuota(poolIDFromRequest(c), name)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": err.Error()})
		return
	}
	recordPoolAudit(c, "pool_auth.quota_reset", map[string]interface{}{
		"pool": auditPoolID(poolIDFromRequest(c)),
		"name": name,
	})
	writePoolSuccessData(c, usage)
}

// xju-api:new — one-click pool creation (#4 Phase B). CreatePoolInstance drops a
// provisioning request for the host watcher; the frontend then polls
// GetPoolCreateStatus until the new pool is registered. Both are root-only.

type createPoolRequest struct {
	Label string `json:"label"`
	Mode  string `json:"mode"`
}

// CreatePoolInstance POST /api/pool/create — start provisioning a new isolated
// pool from a display label. Returns the derived pool id; the actual container
// is brought up asynchronously by the host watcher.
func CreatePoolInstance(c *gin.Context) {
	if !service.ProvisionEnabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "pool provisioning is not enabled on this deployment"})
		return
	}
	var reqBody createPoolRequest
	if err := common.DecodeJson(c.Request.Body, &reqBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request body"})
		return
	}
	poolID, err := service.RequestPoolProvision(reqBody.Label, reqBody.Mode)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	recordManageAudit(c, "pool_auth.create", map[string]interface{}{
		"pool":  poolID,
		"label": strings.TrimSpace(reqBody.Label),
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"pool_id": poolID, "status": "provisioning"}})
}

type deletePoolInstanceRequest struct {
	PoolID string `json:"pool_id"`
}

// DeletePoolInstance POST /api/pool/delete — tear down a dynamically-created
// pool (container + routing channel + registry). Built-in pools are refused.
func DeletePoolInstance(c *gin.Context) {
	if !service.ProvisionEnabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "pool provisioning is not enabled on this deployment"})
		return
	}
	var reqBody deletePoolInstanceRequest
	if err := common.DecodeJson(c.Request.Body, &reqBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request body"})
		return
	}
	poolID := strings.TrimSpace(reqBody.PoolID)
	if !canManagePrivatePoolFromSharedRoute(c, poolID) {
		return
	}
	if err := service.DeletePoolInstance(poolID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	recordManageAudit(c, "pool_auth.delete_pool", map[string]interface{}{"pool": poolID})
	c.JSON(http.StatusOK, gin.H{"success": true})
}

type renamePoolInstanceRequest struct {
	PoolID string `json:"pool_id"`
	Label  string `json:"label"`
}

// RenamePoolInstance POST /api/pool/rename — change a dynamic pool's display
// label and its routing channel's display name. It touches only operator-facing
// names, never the numeric id / container / group / card routing. Built-in pools
// are refused.
func RenamePoolInstance(c *gin.Context) {
	var reqBody renamePoolInstanceRequest
	if err := common.DecodeJson(c.Request.Body, &reqBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request body"})
		return
	}
	poolID := strings.TrimSpace(reqBody.PoolID)
	if !canManagePrivatePoolFromSharedRoute(c, poolID) {
		return
	}
	if err := service.RenamePool(poolID, reqBody.Label); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	recordManageAudit(c, "pool_auth.rename_pool", map[string]interface{}{
		"pool":  poolID,
		"label": strings.TrimSpace(reqBody.Label),
	})
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// Admins manage shared pools; root retains the existing global recovery path.
// This closes the body-id gap for rename/delete, which cannot use the query-
// scoped middleware that protects the rest of /api/pool.
func canManagePrivatePoolFromSharedRoute(c *gin.Context, poolID string) bool {
	if c.GetInt("role") >= common.RoleRootUser {
		return true
	}
	entry, ok := common.GetPoolEntry(poolID)
	if !ok || entry.Kind != common.PoolKindPrivate {
		return true
	}
	c.JSON(http.StatusForbidden, gin.H{
		"success": false,
		"message": "private pools can only be managed by their owner or root",
	})
	return false
}

// GetPoolCreateStatus GET /api/pool/create/status?id=xxx — poll provisioning
// progress. On success the pool is registered and status becomes "ready".
func GetPoolCreateStatus(c *gin.Context) {
	poolID := strings.TrimSpace(c.Query("id"))
	if poolID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "id is required"})
		return
	}
	status, err := service.PollPoolProvision(poolID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"pool_id": poolID, "status": "error", "error": err.Error()}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"pool_id": poolID, "status": status}})
}

// auditPoolID normalizes the pool query param for audit entries ("" resolves
// to the default pool, mirroring common.ResolvePoolMgmt).
func auditPoolID(poolID string) string {
	if strings.TrimSpace(poolID) == "" {
		return "default"
	}
	return poolID
}

// xju-api:new — cross-pool import guard. The batch importer stamps a pool's
// files with "-<poolID>-" in their name (e.g. "alice@x.com-k12-<hash>.json"), so
// a file whose name carries a *different* configured pool's marker almost
// certainly belongs to that other pool. This catches the real incident where an
// operator imported the k12 zip into the default pool by forgetting to switch
// the pool selector, silently polluting it with 500 accounts. Returns the
// foreign pool id when the name looks misrouted.
func foreignPoolMarker(name, targetPool string) (string, bool) {
	target := auditPoolID(targetPool)
	lower := strings.ToLower(name)
	for _, p := range common.ListConfiguredPools() {
		if p.ID == target {
			continue
		}
		if strings.Contains(lower, "-"+strings.ToLower(p.ID)+"-") {
			return p.ID, true
		}
	}
	return "", false
}

// sanitizePoolAuthName keeps the name a plain `*.json` basename. The pool side
// has its own guard, but stripping path separators here means a bad name can
// never be composed into a traversal against the management API.
func sanitizePoolAuthName(raw string) string {
	name := strings.TrimSpace(raw)
	name = strings.ReplaceAll(name, "\\", "/")
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		name = name[idx+1:]
	}
	if name == "" {
		name = fmt.Sprintf("codex-%d.json", time.Now().Unix())
	}
	if !strings.HasSuffix(strings.ToLower(name), ".json") {
		name += ".json"
	}
	return name
}

// Batch-import limits: keep a hostile upload from exhausting memory. The real
// K12 zip is <1MB / 500 files, so these ceilings are generous.
const (
	maxImportZipBytes   = 64 << 20 // 64 MiB uploaded zip
	maxImportEntries    = 2000     // process at most this many entries
	maxImportEntryBytes = 1 << 20  // 1 MiB per JSON entry

	privateMaxAccounts         = 20
	privateMaxImportZipBytes   = 8 << 20   // 8 MiB uploaded zip
	privateMaxImportEntries    = 50        // private pools are intentionally small
	privateMaxImportEntryBytes = 512 << 10 // 512 KiB per JSON entry
)

type poolImportLimits struct {
	zipBytes   int64
	entries    int
	entryBytes uint64
}

func importLimitsForRequest(c *gin.Context) poolImportLimits {
	if isPrivatePoolRequest(c) {
		return poolImportLimits{zipBytes: privateMaxImportZipBytes, entries: privateMaxImportEntries, entryBytes: privateMaxImportEntryBytes}
	}
	return poolImportLimits{zipBytes: maxImportZipBytes, entries: maxImportEntries, entryBytes: maxImportEntryBytes}
}

// filterPrivatePoolCodexItems rejects credentials that explicitly identify a
// different provider. Older valid Codex exports sometimes omit `type`, so an
// absent discriminator remains accepted for backward compatibility.
func filterPrivatePoolCodexItems(items []poolAuthItem) ([]poolAuthItem, []importSkip) {
	accepted := make([]poolAuthItem, 0, len(items))
	skipped := make([]importSkip, 0)
	for _, item := range items {
		var obj map[string]any
		if err := common.UnmarshalJsonStr(item.content, &obj); err != nil {
			skipped = append(skipped, importSkip{Name: item.name, Reason: "not valid JSON"})
			continue
		}
		kind := strings.ToLower(strings.TrimSpace(stringField(obj, "type")))
		if kind != "" && kind != "codex" {
			skipped = append(skipped, importSkip{Name: item.name, Reason: "private pools accept Codex credentials only"})
			continue
		}
		accepted = append(accepted, item)
	}
	return accepted, skipped
}

func privatePoolExistingAccountNames(ctx context.Context, baseURL, secret string) (map[string]struct{}, error) {
	status, payload, err := service.PoolMgmtRoundTrip(ctx, baseURL, secret, http.MethodGet, "/v0/management/auth-files", nil, "")
	if err != nil {
		return nil, fmt.Errorf("cannot check private pool capacity: %v", err)
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("cannot check private pool capacity: %s", poolErrorMessage(payload, status))
	}
	var parsed struct {
		Files []struct {
			Name string `json:"name"`
		} `json:"files"`
	}
	if err := common.Unmarshal(payload, &parsed); err != nil {
		return nil, fmt.Errorf("cannot check private pool capacity: invalid pool response")
	}
	names := make(map[string]struct{}, len(parsed.Files))
	for _, file := range parsed.Files {
		if name := strings.TrimSpace(file.Name); name != "" {
			names[name] = struct{}{}
		}
	}
	return names, nil
}

func enforcePrivatePoolAccountLimit(ctx context.Context, poolID, baseURL, secret string, items []poolAuthItem) error {
	existing, err := privatePoolExistingAccountNames(ctx, baseURL, secret)
	if err != nil {
		return err
	}
	newNames := make(map[string]struct{})
	for _, item := range items {
		if _, ok := existing[item.name]; ok {
			continue // replacing an existing auth file does not consume capacity
		}
		newNames[item.name] = struct{}{}
	}
	reserved := service.CountPrivatePoolOAuthReservations(poolID)
	if len(existing)+len(newNames)+reserved > privateMaxAccounts {
		return fmt.Errorf("private pool account limit is %d (currently %d)", privateMaxAccounts, len(existing))
	}
	return nil
}

type importSkip struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

type importFail struct {
	Name  string `json:"name"`
	Error string `json:"error"`
}

// forwardPoolAuthItems uploads one or more single-account items to the pool's
// bulk auth-files endpoint in a single multipart request and returns the merged
// {imported, failed} plus any items skipped locally (foreign-pool markers). It is
// shared by AddPoolAuthFile (paste / single upload) and ImportPoolAuthFiles (zip),
// so all three entry points import multi-account blobs identically.
func forwardPoolAuthItems(ctx context.Context, baseURL, secret, poolID string, items []poolAuthItem) (imported int, failed []importFail, skipped []importSkip, err error) {
	failed = make([]importFail, 0)
	skipped = make([]importSkip, 0)
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	forwarded := 0
	for _, it := range items {
		if fp, misrouted := foreignPoolMarker(it.name, poolID); misrouted {
			skipped = append(skipped, importSkip{Name: it.name, Reason: "belongs to pool " + fp + " (name contains -" + fp + "-)"})
			continue
		}
		part, cErr := mw.CreateFormFile("files", it.name)
		if cErr != nil {
			skipped = append(skipped, importSkip{Name: it.name, Reason: "internal error"})
			continue
		}
		if _, wErr := part.Write([]byte(it.content)); wErr != nil {
			skipped = append(skipped, importSkip{Name: it.name, Reason: "internal error"})
			continue
		}
		forwarded++
	}
	if cErr := mw.Close(); cErr != nil {
		return 0, failed, skipped, cErr
	}
	if forwarded == 0 {
		return 0, failed, skipped, nil
	}
	status, payload, rErr := service.PoolMgmtRoundTrip(
		ctx, baseURL, secret,
		http.MethodPost, "/v0/management/auth-files", &body, mw.FormDataContentType(),
	)
	if rErr != nil {
		return 0, failed, skipped, fmt.Errorf("pool management unreachable: %v", rErr)
	}
	if status < 200 || status >= 300 {
		return 0, failed, skipped, fmt.Errorf("%s", poolErrorMessage(payload, status))
	}
	var parsed struct {
		Uploaded int          `json:"uploaded"`
		Files    []string     `json:"files"`
		Failed   []importFail `json:"failed"`
	}
	if uErr := common.Unmarshal(payload, &parsed); uErr == nil {
		failed = append(failed, parsed.Failed...)
		imported = parsed.Uploaded
		if imported == 0 && len(parsed.Files) > 0 {
			imported = len(parsed.Files)
		}
		if imported == 0 && len(parsed.Failed) == 0 {
			imported = forwarded
		}
	} else {
		imported = forwarded
	}
	return imported, failed, skipped, nil
}

// ImportPoolAuthFiles POST /api/pool/auth-files/import?pool=xxx — accept a .zip
// of codex auth JSON files, expand it server-side, and forward every valid entry
// as one multipart batch to the target pool's management API. Locally-skipped
// entries (non-json, malformed, oversize) are merged with the pool's per-file
// failures into a single {imported, skipped, failed} report. No file is written
// to disk here and only the entry's base name is used, so there is no zip-slip
// surface; token contents are never logged.
func ImportPoolAuthFiles(c *gin.Context) {
	poolID := poolIDFromRequest(c)
	baseURL, secret, ok := common.ResolvePoolMgmt(poolID)
	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"message": "pool management is not configured for pool: " + poolID,
		})
		return
	}

	limits := importLimitsForRequest(c)
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limits.zipBytes+(1<<20))
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "no zip uploaded (field 'file')"})
		return
	}
	if fileHeader.Size > limits.zipBytes {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "zip too large"})
		return
	}
	f, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "cannot read upload"})
		return
	}
	defer f.Close()
	zipBytes, err := io.ReadAll(io.LimitReader(f, limits.zipBytes+1))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "cannot read upload"})
		return
	}
	if int64(len(zipBytes)) > limits.zipBytes {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "zip too large"})
		return
	}

	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid zip"})
		return
	}

	items := make([]poolAuthItem, 0)
	skipped := make([]importSkip, 0)
	seen := 0

	for _, entry := range zr.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		base := poolZipEntryBase(entry.Name)
		if strings.HasPrefix(base, ".") || strings.Contains(entry.Name, "__MACOSX/") {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(base), ".json") {
			skipped = append(skipped, importSkip{Name: base, Reason: "not a .json file"})
			continue
		}
		seen++
		if seen > limits.entries {
			skipped = append(skipped, importSkip{Name: base, Reason: "entry limit reached, skipped"})
			common.SysLog("pool import: entry limit " + strconv.Itoa(limits.entries) + " reached, extra entries skipped")
			continue
		}
		if entry.UncompressedSize64 > limits.entryBytes {
			skipped = append(skipped, importSkip{Name: base, Reason: "file too large"})
			continue
		}
		rc, err := entry.Open()
		if err != nil {
			skipped = append(skipped, importSkip{Name: base, Reason: "cannot read entry"})
			continue
		}
		raw, err := io.ReadAll(io.LimitReader(rc, int64(limits.entryBytes)+1))
		rc.Close()
		if err != nil {
			skipped = append(skipped, importSkip{Name: base, Reason: "cannot read entry"})
			continue
		}
		if uint64(len(raw)) > limits.entryBytes {
			skipped = append(skipped, importSkip{Name: base, Reason: "file too large"})
			continue
		}
		content := strings.TrimSpace(string(raw))
		var probe any
		if err := common.UnmarshalJsonStr(content, &probe); err != nil {
			skipped = append(skipped, importSkip{Name: base, Reason: "not valid JSON"})
			continue
		}
		// A single zip entry can itself be a multi-account bundle/array; expand it
		// so every account inside is imported, not just the first.
		items = append(items, parsePoolAuthAccounts(content, base)...)
	}
	if isPrivatePoolRequest(c) {
		var privateSkipped []importSkip
		items, privateSkipped = filterPrivatePoolCodexItems(items)
		skipped = append(skipped, privateSkipped...)
		if len(items) > 0 {
			if err := enforcePrivatePoolAccountLimit(c.Request.Context(), poolID, baseURL, secret, items); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error(), "data": gin.H{"skipped": skipped}})
				return
			}
		}
	}

	imported, failed, forwardSkipped, err := forwardPoolAuthItems(c.Request.Context(), baseURL, secret, poolID, items)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": err.Error()})
		return
	}
	skipped = append(skipped, forwardSkipped...)

	recordPoolAudit(c, "pool_auth.import", map[string]interface{}{
		"pool":     auditPoolID(poolID),
		"imported": imported,
		"skipped":  len(skipped),
		"failed":   len(failed),
	})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"imported": imported,
			"skipped":  skipped,
			"failed":   failed,
		},
	})
}

// poolZipEntryBase returns the final path element of a zip entry name, treating
// both '/' and '\\' as separators (zip entries can carry either).
func poolZipEntryBase(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		name = name[idx+1:]
	}
	return name
}
