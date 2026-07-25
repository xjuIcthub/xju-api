package controller

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildZip returns an in-memory zip containing the given name->content entries.
func buildZip(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range entries {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

// zipUploadRequest builds a multipart POST carrying the zip under field "file".
func zipUploadRequest(t *testing.T, target string, zipBytes []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", "batch.zip")
	require.NoError(t, err)
	_, err = fw.Write(zipBytes)
	require.NoError(t, err)
	require.NoError(t, mw.Close())
	req := httptest.NewRequest(http.MethodPost, target, &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

func disablePoolTestRedis(t *testing.T) {
	t.Helper()
	old := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = old })
}

func TestImportPoolAuthFiles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	disablePoolTestRedis(t)
	setupModelListControllerTestDB(t)

	// Fake pool: accepts multipart, reports every file part as uploaded.
	var receivedParts int
	pool := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseMultipartForm(32<<20))
		for _, hs := range r.MultipartForm.File {
			receivedParts += len(hs)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","uploaded":` + strconv.Itoa(receivedParts) + `,"files":[],"failed":[]}`))
	}))
	defer pool.Close()

	t.Setenv("POOL_K12_MGMT_URL", pool.URL)
	t.Setenv("POOL_K12_MGMT_SECRET", "k12-secret")

	zipBytes := buildZip(t, map[string]string{
		"alive/a@x.com-k12-1.json": `{"type":"codex","email":"a@x.com","access_token":"t","account_id":"id"}`,
		"alive/note.txt":           `not json`,
		"alive/bad.json":           `{not valid json`,
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = zipUploadRequest(t, "/api/pool/auth-files/import?pool=k12", zipBytes)

	ImportPoolAuthFiles(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Imported int          `json:"imported"`
			Skipped  []importSkip `json:"skipped"`
			Failed   []importFail `json:"failed"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
	assert.Equal(t, 1, resp.Data.Imported, "only the one valid json forwards")
	assert.Equal(t, 1, receivedParts, "pool receives exactly one file part")
	require.Len(t, resp.Data.Skipped, 2, "the .txt and the malformed json are skipped")
	for _, s := range resp.Data.Skipped {
		assert.NotEmpty(t, s.Reason, "each skip carries a reason")
	}
}

func TestImportPoolAuthFilesUnconfiguredPool(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("POOL_K12_MGMT_SECRET", "")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/pool/auth-files/import?pool=k12", nil)
	ImportPoolAuthFiles(c)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestParsePoolAuthAccounts(t *testing.T) {
	t.Run("bare codex object derives name from email", func(t *testing.T) {
		items := parsePoolAuthAccounts(`{"email":"a@b.com","access_token":"tok"}`, "")
		require.Len(t, items, 1)
		assert.Equal(t, "codex-a-b-com.json", items[0].name)
		assert.Contains(t, items[0].content, "a@b.com")
	})
	t.Run("bare object keeps caller-supplied name", func(t *testing.T) {
		items := parsePoolAuthAccounts(`{"email":"a@b.com"}`, "codex-custom.json")
		require.Len(t, items, 1)
		assert.Equal(t, "codex-custom.json", items[0].name)
	})
	t.Run("single OpenAI OAuth wrapper unwraps credentials", func(t *testing.T) {
		blob := `{
			"name":"Wrapped Account",
			"platform":"openai",
			"type":"oauth",
			"priority":2,
			"credentials":{
				"email":"wrapped@example.com",
				"access_token":"access-token",
				"refresh_token":"refresh-token",
				"chatgpt_account_id":"account-123"
			}
		}`
		items := parsePoolAuthAccounts(blob, "")
		require.Len(t, items, 1)
		assert.Equal(t, "codex-wrapped-example-com.json", items[0].name)

		var cred map[string]any
		require.NoError(t, json.Unmarshal([]byte(items[0].content), &cred))
		assert.Equal(t, "codex", cred["type"])
		assert.Equal(t, "account-123", cred["account_id"])
		assert.Equal(t, float64(2), cred["priority"])
		assert.NotContains(t, cred, "credentials")

		accepted, skipped := filterPrivatePoolCodexItems(items)
		require.Len(t, accepted, 1)
		assert.Empty(t, skipped)
	})
	t.Run("non-OpenAI OAuth wrapper is not reclassified", func(t *testing.T) {
		blob := `{"platform":"anthropic","type":"oauth","credentials":{"access_token":"token"}}`
		items := parsePoolAuthAccounts(blob, "claude.json")
		require.Len(t, items, 1)
		assert.JSONEq(t, blob, items[0].content)

		accepted, skipped := filterPrivatePoolCodexItems(items)
		assert.Empty(t, accepted)
		require.Len(t, skipped, 1)
		assert.Contains(t, skipped[0].Reason, "Codex")
	})
	t.Run("exporter bundle expands every account", func(t *testing.T) {
		blob := `{"type":"x","version":1,"accounts":[
			{"name":"one","credentials":{"email":"one@x.com","access_token":"t1"}},
			{"name":"two","credentials":{"email":"two@x.com","access_token":"t2"}},
			{"credentials":{"email":"three@x.com","access_token":"t3"}}
		]}`
		items := parsePoolAuthAccounts(blob, "ignored.json")
		require.Len(t, items, 3)
		assert.ElementsMatch(t,
			[]string{"codex-one-x-com.json", "codex-two-x-com.json", "codex-three-x-com.json"},
			[]string{items[0].name, items[1].name, items[2].name})
		// Each item carries only the inner credential, not the account wrapper.
		assert.Contains(t, items[0].content, "access_token")
	})
	t.Run("exporter bundle with platform tags each account as codex", func(t *testing.T) {
		// go-pool / sub2 exports carry {platform,type:"oauth",credentials} per
		// account. The type marker lives on the wrapper, so the written file must
		// still gain a top-level "type":"codex" or CLIProxyAPI drops it silently.
		blob := `{"accounts":[
			{"name":"Meg","platform":"openai","type":"oauth","priority":1,"credentials":{
				"email":"meg@x.com","access_token":"a","refresh_token":"r","chatgpt_account_id":"acc-9"
			}},
			{"name":"Sol","platform":"codex","type":"oauth","credentials":{
				"email":"sol@x.com","id_token":"i","chatgpt_account_id":"acc-7"
			}}
		]}`
		items := parsePoolAuthAccounts(blob, "")
		require.Len(t, items, 2)
		assert.ElementsMatch(t,
			[]string{"codex-meg-x-com.json", "codex-sol-x-com.json"},
			[]string{items[0].name, items[1].name})
		for _, it := range items {
			var cred map[string]any
			require.NoError(t, json.Unmarshal([]byte(it.content), &cred))
			assert.Equal(t, "codex", cred["type"], "each account must be tagged codex")
			assert.NotContains(t, cred, "credentials")
			assert.NotEmpty(t, cred["account_id"], "account_id mapped from chatgpt_account_id")
		}
		accepted, skipped := filterPrivatePoolCodexItems(items)
		require.Len(t, accepted, 2)
		assert.Empty(t, skipped)
	})
	t.Run("json array of bare objects expands", func(t *testing.T) {
		blob := `[{"email":"x@y.com","access_token":"t"},{"email":"z@y.com","access_token":"t2"}]`
		items := parsePoolAuthAccounts(blob, "")
		require.Len(t, items, 2)
		assert.ElementsMatch(t, []string{"codex-x-y-com.json", "codex-z-y-com.json"},
			[]string{items[0].name, items[1].name})
	})
	t.Run("account without email falls back to account name then index", func(t *testing.T) {
		blob := `{"accounts":[{"name":"Alpha","credentials":{"access_token":"t"}},{"credentials":{"access_token":"t2"}}]}`
		items := parsePoolAuthAccounts(blob, "")
		require.Len(t, items, 2)
		assert.Equal(t, "codex-alpha.json", items[0].name)
		assert.Equal(t, "codex-account-2.json", items[1].name)
	})
	t.Run("non-JSON passes through unchanged for the pool to reject", func(t *testing.T) {
		items := parsePoolAuthAccounts("not json", "raw.json")
		require.Len(t, items, 1)
		assert.Equal(t, "raw.json", items[0].name)
		assert.Equal(t, "not json", items[0].content)
	})
}

func TestPoolIDFromRequestPrefersPrivateScope(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/private-pool/auth-files?pool=forged", nil)
	c.Set("xju_private_pool_id", "1")
	assert.Equal(t, "1", poolIDFromRequest(c))
}

func TestSanitizePrivatePoolAuthList(t *testing.T) {
	raw := []byte(`{"files":[{"name":"alice.json","email":"a@example.com","path":"/root/auths/alice.json","auth_index":"idx","access_token":"secret","refresh_token":"secret2","id_token":{"plan_type":"plus","chatgpt_subscription_active_until":1893456000}},{"name":"raw.json","id_token":"raw-jwt"}]}`)
	safe, err := sanitizePrivatePoolAuthList(raw)
	require.NoError(t, err)
	text := string(safe)
	assert.Contains(t, text, `"email":"a@example.com"`)
	assert.Contains(t, text, `"plan_type":"plus"`)
	assert.NotContains(t, text, "auth_index")
	assert.NotContains(t, text, "access_token")
	assert.NotContains(t, text, "refresh_token")
	assert.NotContains(t, text, "/root/auths")
	assert.NotContains(t, text, "raw-jwt")
}

func TestWritePoolSuccessDataSanitizesSharedReadOnlyResponse(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set(common.ContextKeyPoolReadOnly, true)

	writePoolSuccessData(c, map[string]any{
		"name":         "alice.json",
		"email":        "alice@example.com",
		"auth_index":   "runtime-secret",
		"access_token": "credential-secret",
	})

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "alice@example.com")
	assert.NotContains(t, w.Body.String(), "runtime-secret")
	assert.NotContains(t, w.Body.String(), "credential-secret")
}

func TestReadOnlyPoolUsageRefreshRequiresAccountName(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set(common.ContextKeyPoolReadOnly, true)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/pool/auth-files/usage/refresh?pool=default", strings.NewReader(`{}`))

	RefreshPoolAccountUsage(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "one account at a time")
}

func TestSharedPoolSettingAllowlist(t *testing.T) {
	assert.True(t, sharedPoolSettingKeys["PoolAutoCleanEnabled"])
	assert.True(t, sharedPoolSettingKeys["PoolUsageAutoRefreshEnabled"])
	assert.True(t, sharedPoolSettingKeys["PoolUsageAutoResetEnabled"])
	assert.False(t, sharedPoolSettingKeys["GitHubClientSecret"])
	assert.False(t, sharedPoolSettingKeys["PoolAutoCleanHours"])
}

func TestAdminCannotRenameOrDeletePrivatePoolThroughSharedRoute(t *testing.T) {
	t.Setenv("POOL_REGISTRY_FILE", filepath.Join(t.TempDir(), "pools.json"))
	require.NoError(t, common.AddPoolToRegistry(common.PoolEntry{
		ID: "private-target", MgmtURL: "http://private:8319", MgmtSecret: "secret",
		OwnerUserID: 42, Kind: common.PoolKindPrivate,
	}))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("role", common.RoleAdminUser)
	assert.False(t, canManagePrivatePoolFromSharedRoute(c, "private-target"))
	assert.Equal(t, http.StatusForbidden, w.Code)

	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Set("role", common.RoleRootUser)
	assert.True(t, canManagePrivatePoolFromSharedRoute(c, "private-target"))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestFilterPrivatePoolCodexItems(t *testing.T) {
	accepted, skipped := filterPrivatePoolCodexItems([]poolAuthItem{
		{name: "codex.json", content: `{"type":"codex","email":"a@example.com"}`},
		{name: "legacy.json", content: `{"email":"legacy@example.com"}`},
		{name: "other.json", content: `{"type":"gemini","email":"g@example.com"}`},
	})
	require.Len(t, accepted, 2)
	require.Len(t, skipped, 1)
	assert.Equal(t, "other.json", skipped[0].Name)
	assert.Contains(t, skipped[0].Reason, "Codex")
}

func TestEnforcePrivatePoolAccountLimit(t *testing.T) {
	files := make([]map[string]string, 0, privateMaxAccounts-1)
	for i := 0; i < privateMaxAccounts-1; i++ {
		files = append(files, map[string]string{"name": fmt.Sprintf("account-%d.json", i)})
	}
	payload, err := json.Marshal(map[string]any{"files": files})
	require.NoError(t, err)
	pool := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		_, _ = w.Write(payload)
	}))
	defer pool.Close()

	err = enforcePrivatePoolAccountLimit(t.Context(), "test-pool", pool.URL, "secret", []poolAuthItem{
		{name: "new-a.json", content: `{}`},
		{name: "new-b.json", content: `{}`},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "limit is 20")

	err = enforcePrivatePoolAccountLimit(t.Context(), "test-pool", pool.URL, "secret", []poolAuthItem{
		{name: "account-0.json", content: `{}`},
		{name: "new-a.json", content: `{}`},
	})
	require.NoError(t, err, "replacing one existing file plus one new file stays within capacity")
}

func TestCreatePrivatePoolIgnoresClientOwnershipFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	disablePoolTestRedis(t)
	setupModelListControllerTestDB(t)
	dir := t.TempDir()
	t.Setenv("POOL_PROVISION_DIR", dir)
	t.Setenv("POOL_REGISTRY_FILE", filepath.Join(dir, "registry.json"))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("id", 42)
	c.Set("role", common.RoleCommonUser)
	c.Set("username", "alice")
	c.Request = httptest.NewRequest(http.MethodPost, "/api/private-pool",
		strings.NewReader(`{"label":"Alice Pool","owner_user_id":999,"kind":"admin","group_key":"forged","mode":"gopool"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	CreatePrivatePool(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	requestData, err := os.ReadFile(filepath.Join(dir, "requests", "1.json"))
	require.NoError(t, err)
	var request struct {
		OwnerUserID int    `json:"owner_user_id"`
		Kind        string `json:"kind"`
		GroupKey    string `json:"group_key"`
		Mode        string `json:"mode"`
	}
	require.NoError(t, json.Unmarshal(requestData, &request))
	assert.Equal(t, 42, request.OwnerUserID)
	assert.Equal(t, common.PoolKindPrivate, request.Kind)
	assert.Equal(t, "private-42", request.GroupKey)
	assert.Equal(t, "cliproxy", request.Mode)
}
