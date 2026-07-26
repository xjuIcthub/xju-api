package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetUserGroupsAddsOnlyOwnedPrivateGroup(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	t.Setenv("POOL_REGISTRY_FILE", filepath.Join(t.TempDir(), "pools.json"))
	t.Setenv("POOL_MGMT_SECRET", "")
	t.Setenv("POOL_K12_MGMT_SECRET", "")
	require.NoError(t, db.Create(&model.User{Id: 42, Username: "alice", Group: "default", Status: common.UserStatusEnabled}).Error)
	require.NoError(t, common.AddPoolToRegistry(common.PoolEntry{
		ID: "codex-team", Label: "Codex Team", Provider: common.PoolProviderCodex,
		MgmtURL: "http://codex-team:8319", MgmtSecret: "secret", ChannelID: 121,
		Kind: common.PoolKindAdmin, GroupKey: "codex-team",
	}))
	require.NoError(t, common.AddPoolToRegistry(common.PoolEntry{
		ID: "claude-team", Label: "Claude Team", Provider: common.PoolProviderClaude,
		MgmtURL: "http://claude-team:8320", MgmtSecret: "secret", ChannelID: 122,
		Kind: common.PoolKindAdmin, GroupKey: "claude-team",
	}))
	require.NoError(t, common.AddPoolToRegistry(common.PoolEntry{
		ID: "1", Label: "Alice Pool", Provider: common.PoolProviderCodex,
		MgmtURL: "http://pool-1:8319", MgmtSecret: "secret", ChannelID: 123,
		OwnerUserID: 42, Kind: common.PoolKindPrivate,
	}))

	oldRatios := ratio_setting.GroupRatio2JSONString()
	ratios := ratio_setting.GetGroupRatioCopy()
	ratios[common.PrivatePoolGroupKey(42)] = 1
	ratios["codex-team"] = 1
	ratios["claude-team"] = 1
	ratios["k12"] = 1
	ratios["vip"] = 1
	rawRatios, err := common.Marshal(ratios)
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(string(rawRatios)))
	t.Cleanup(func() { _ = ratio_setting.UpdateGroupRatioByJSONString(oldRatios) })

	oldUsableGroups := setting.UserUsableGroups2JSONString()
	usableGroups := setting.GetUserUsableGroupsCopy()
	usableGroups["codex-team"] = "Codex Team"
	usableGroups["claude-team"] = "Claude Team"
	usableGroups["k12"] = "K12"
	usableGroups["vip"] = "VIP"
	rawUsableGroups, err := common.Marshal(usableGroups)
	require.NoError(t, err)
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(string(rawUsableGroups)))
	t.Cleanup(func() { _ = setting.UpdateUserUsableGroupsByJSONString(oldUsableGroups) })

	_, globallyVisible := setting.GetUserUsableGroupsCopy()[common.PrivatePoolGroupKey(42)]
	assert.False(t, globallyVisible)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/user/groups", nil)
	c.Set("id", 42)
	GetUserGroups(c)
	require.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Success bool                              `json:"success"`
		Data    map[string]map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.True(t, response.Success)
	privateGroup, ok := response.Data[common.PrivatePoolGroupKey(42)]
	require.True(t, ok)
	assert.Equal(t, "Alice Pool", privateGroup["desc"])
	assert.Equal(t, common.PoolProviderCodex, privateGroup["provider"])
	codexGroup, ok := response.Data["codex-team"]
	require.True(t, ok)
	assert.Equal(t, "Codex Team", codexGroup["desc"])
	assert.Equal(t, common.PoolProviderCodex, codexGroup["provider"])
	claudeGroup, ok := response.Data["claude-team"]
	require.True(t, ok)
	assert.Equal(t, "Claude Team", claudeGroup["desc"])
	assert.Equal(t, common.PoolProviderClaude, claudeGroup["provider"])
	assert.NotContains(t, response.Data, "k12")
	assert.NotContains(t, response.Data, "vip")

	resolved, valid := resolveUserTokenGroup(42, "claude-team")
	assert.True(t, valid)
	assert.Equal(t, "claude-team", resolved)
	_, valid = resolveUserTokenGroup(42, "vip")
	assert.False(t, valid)
}
