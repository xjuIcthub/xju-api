package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// xju-api:new — pool routing channel (#4 Phase C). A provisioned pool gets an
// auto-created channel that clones the primary channel's models and routes the
// pool's group to its cliproxy instance; deleting the pool removes it.

func TestPoolChannelCreateDelete(t *testing.T) {
	// Seed the primary channel (id 1) whose model set is cloned.
	require.NoError(t, model.DB.Where("1 = 1").Delete(&model.Channel{}).Error)
	require.NoError(t, model.DB.Create(&model.Channel{
		Id: 1, Type: 1, Name: "cliproxy-pool", Key: "k1",
		Models: "gpt-5,gpt-4", Group: "default", Status: 1,
	}).Error)

	id, err := createPoolChannel("edu", "edu-key", "http://cli-proxy-api-edu:8319/", "", "Edu", "edu", common.PoolProviderCodex, true)
	require.NoError(t, err)
	require.NotZero(t, id)

	var ch model.Channel
	require.NoError(t, model.DB.First(&ch, id).Error)
	assert.Equal(t, constant.ChannelTypeAdvancedCustom, ch.Type)
	assert.Equal(t, "cliproxy-pool-edu", ch.Name)
	assert.Equal(t, "edu-key", ch.Key)
	assert.Equal(t, "edu", ch.Group)
	assert.Equal(t, "gpt-5,gpt-4", ch.Models, "models cloned from channel 1")
	require.NotNil(t, ch.BaseURL)
	assert.Equal(t, "http://cli-proxy-api-edu:8319", *ch.BaseURL, "trailing slash trimmed")
	assert.True(t, ch.GetSetting().PassThroughBodyEnabled)
	assertPoolAdvancedCustomConfig(t, ch.GetOtherSettings().AdvancedCustom)
	assert.Contains(t, ch.GetHeaderOverride(), poolClaudeHeaderPassthroughRule)

	// Idempotent by name — a second create upgrades a legacy row in place and
	// returns the same channel without replacing its routing identity.
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", id).Updates(map[string]any{
		"type":            constant.ChannelTypeOpenAI,
		"setting":         nil,
		"settings":        "",
		"header_override": nil,
	}).Error)
	id2, err := createPoolChannel("edu", "ignored", "ignored", "", "Edu", "edu", common.PoolProviderCodex, true)
	require.NoError(t, err)
	assert.Equal(t, id, id2)
	require.NoError(t, model.DB.First(&ch, id).Error)
	assert.Equal(t, constant.ChannelTypeAdvancedCustom, ch.Type)
	assert.Equal(t, "edu-key", ch.Key)
	assert.Equal(t, "http://cli-proxy-api-edu:8319", *ch.BaseURL)
	assertPoolAdvancedCustomConfig(t, ch.GetOtherSettings().AdvancedCustom)

	// findChannelByName resolves it; a bogus name resolves to 0.
	assert.Equal(t, id, findChannelByName("cliproxy-pool-edu"))
	assert.Zero(t, findChannelByName("cliproxy-pool-nope"))

	// Delete removes the channel row.
	deletePoolChannel("edu", "edu", id)
	var cnt int64
	model.DB.Model(&model.Channel{}).Where("id = ?", id).Count(&cnt)
	assert.Zero(t, cnt, "channel deleted")
}

func TestPoolProviderModelsUsesClaudeCatalogWithoutGPTFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v0/management/model-definitions/claude", r.URL.Path)
		assert.Equal(t, "Bearer secret", r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"channel":"claude","models":[{"id":"claude-sonnet-4-5"},{"id":"claude-opus-4-1"},{"id":"claude-sonnet-4-5"}]}`))
	}))
	defer server.Close()

	models, err := poolProviderModels(server.URL, "secret", common.PoolProviderClaude)
	require.NoError(t, err)
	assert.Equal(t, "claude-sonnet-4-5,claude-opus-4-1", models)
	assert.NotContains(t, models, "gpt")
}

func TestEnsurePoolChannelCompatibilityPreservesRoutingIdentityAndIsIdempotent(t *testing.T) {
	require.NoError(t, model.DB.Where("1 = 1").Delete(&model.Channel{}).Error)

	baseURL := "http://cliproxy.internal:8317"
	settingJSON := `{"system_prompt":"keep me"}`
	headerJSON := `{"X-Custom":"still-here"}`
	channel := &model.Channel{
		Type:           constant.ChannelTypeOpenAI,
		Name:           "Renamed default pool",
		Key:            "internal-key",
		BaseURL:        &baseURL,
		Models:         "gpt-5,gpt-5-codex",
		Group:          "default",
		Status:         common.ChannelStatusEnabled,
		Setting:        &settingJSON,
		HeaderOverride: &headerJSON,
		OtherSettings:  `{"allow_speed":true,"advanced_custom":{"advanced_routes":[{"incoming_path":"/custom-health","upstream_path":"/health","converter":"none"},{"incoming_path":"/v1/messages","upstream_path":"/legacy-messages","converter":"response"}]}}`,
	}
	require.NoError(t, channel.Insert())

	changed, err := ensurePoolChannelCompatibility(channel)
	require.NoError(t, err)
	assert.True(t, changed)

	var got model.Channel
	require.NoError(t, model.DB.First(&got, channel.Id).Error)
	assert.Equal(t, constant.ChannelTypeAdvancedCustom, got.Type)
	assert.Equal(t, "Renamed default pool", got.Name)
	assert.Equal(t, "internal-key", got.Key)
	assert.Equal(t, "default", got.Group)
	assert.Equal(t, "gpt-5,gpt-5-codex", got.Models)
	require.NotNil(t, got.BaseURL)
	assert.Equal(t, baseURL, *got.BaseURL)
	assert.Equal(t, common.ChannelStatusEnabled, got.Status)
	assert.Equal(t, "keep me", got.GetSetting().SystemPrompt)
	assert.True(t, got.GetSetting().PassThroughBodyEnabled)
	assert.True(t, got.GetOtherSettings().AllowSpeed)
	config := got.GetOtherSettings().AdvancedCustom
	require.NotNil(t, config)
	require.Len(t, config.Routes, len(poolAdvancedCustomPaths)+1)
	assert.Equal(t, "/custom-health", config.Routes[0].IncomingPath)
	assert.Equal(t, "/health", config.Routes[0].UpstreamPath)
	assert.Equal(t, "none", config.Routes[0].Converter)
	assertPoolRequiredAdvancedCustomRoutes(t, config)
	assert.Equal(t, "still-here", got.GetHeaderOverride()["X-Custom"])
	assert.Contains(t, got.GetHeaderOverride(), poolClaudeHeaderPassthroughRule)

	changed, err = ensurePoolChannelCompatibility(&got)
	require.NoError(t, err)
	assert.False(t, changed)
}

func TestRegisteredPoolChannelsFindsRenamedRegistryChannel(t *testing.T) {
	require.NoError(t, model.DB.Where("1 = 1").Delete(&model.Channel{}).Error)
	registryPath := filepath.Join(t.TempDir(), "pools.json")
	t.Setenv("POOL_REGISTRY_FILE", registryPath)
	require.NoError(t, os.WriteFile(registryPath, []byte("[]"), 0o600))

	channel := &model.Channel{
		Type: constant.ChannelTypeOpenAI, Name: "Alice's pool", Key: "internal",
		Models: "gpt-5", Group: common.PrivatePoolGroupKey(42), Status: common.ChannelStatusEnabled,
	}
	require.NoError(t, channel.Insert())
	require.NoError(t, common.SavePoolRegistry([]common.PoolEntry{{
		ID: "private-42", Label: "Alice", MgmtURL: "http://pool:8319",
		ChannelID: channel.Id, OwnerUserID: 42, Kind: common.PoolKindPrivate,
		GroupKey: common.PrivatePoolGroupKey(42),
	}}))

	channels, err := registeredPoolChannels()
	require.NoError(t, err)
	require.Len(t, channels, 1)
	assert.Equal(t, channel.Id, channels[0].Id)
}

func assertPoolAdvancedCustomConfig(t *testing.T, config *dto.AdvancedCustomConfig) {
	t.Helper()
	require.NotNil(t, config)
	require.Len(t, config.Routes, len(poolAdvancedCustomPaths))
	assertPoolRequiredAdvancedCustomRoutes(t, config)
	encoded, err := json.Marshal(config)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "claude-sonnet")
}

func assertPoolRequiredAdvancedCustomRoutes(t *testing.T, config *dto.AdvancedCustomConfig) {
	t.Helper()
	for _, path := range poolAdvancedCustomPaths {
		matches := 0
		for _, route := range config.Routes {
			if route.IncomingPath != path || len(route.Models) != 0 {
				continue
			}
			matches++
			assert.Equal(t, path, route.UpstreamPath)
			assert.Equal(t, "none", route.Converter)
		}
		assert.Equal(t, 1, matches, "required path %s must have one catch-all route", path)
	}
}

func TestPrivatePoolChannelUsesHiddenImmutableGroup(t *testing.T) {
	require.NoError(t, model.DB.Where("1 = 1").Delete(&model.Channel{}).Error)
	require.NoError(t, model.DB.Where("1 = 1").Delete(&model.Token{}).Error)
	require.NoError(t, model.DB.Create(&model.Channel{
		Id: 1, Type: 1, Name: "cliproxy-pool", Key: "k1",
		Models: "gpt-5", Group: "default", Status: 1,
	}).Error)

	groupKey := common.PrivatePoolGroupKey(4242)
	id, err := createPoolChannel("private-case", "private-key", "http://private:9000", "", "Alice Pool", groupKey, common.PoolProviderCodex, false)
	require.NoError(t, err)
	t.Cleanup(func() { deletePoolChannel("private-case", groupKey, id) })

	var ch model.Channel
	require.NoError(t, model.DB.First(&ch, id).Error)
	assert.Equal(t, groupKey, ch.Group)
	assert.True(t, ratio_setting.ContainsGroupRatio(groupKey), "private group still needs a billing ratio")
	_, globallyVisible := setting.GetUserUsableGroupsCopy()[groupKey]
	assert.False(t, globallyVisible, "private group must not be exposed globally")

	card := &model.Token{UserId: 4242, Key: "privategroupkey000000000000000000000001", Name: "private", Group: groupKey}
	require.NoError(t, model.DB.Create(card).Error)
	require.NoError(t, renamePoolGroup("private-case", id, "Renamed Pool", groupKey, false))

	require.NoError(t, model.DB.First(&ch, id).Error)
	assert.Equal(t, "Renamed Pool", ch.Name)
	assert.Equal(t, groupKey, ch.Group, "renaming display label cannot change private routing")
	var cardAfter model.Token
	require.NoError(t, model.DB.First(&cardAfter, card.Id).Error)
	assert.Equal(t, groupKey, cardAfter.Group)
	_, globallyVisible = setting.GetUserUsableGroupsCopy()[groupKey]
	assert.False(t, globallyVisible)
}
