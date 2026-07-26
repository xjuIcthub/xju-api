package service

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// xju-api:new — one-click pool creation, new-api side (#4 Phase B). Exercises the
// request/poll/register flow against a fake provision dir + registry file
// (the docker work belongs to the host watcher, which isn't in scope here).

func TestProvisionDisabled(t *testing.T) {
	t.Setenv("POOL_PROVISION_DIR", "")
	assert.False(t, ProvisionEnabled())
	_, err := RequestPoolProvision("x", "cliproxy")
	assert.Error(t, err)
	_, err = PollPoolProvision("x")
	assert.Error(t, err)
}

func TestPoolProvisionFlow(t *testing.T) {
	// Isolate: no env pools, fresh registry + provision dirs.
	t.Setenv("POOL_MGMT_SECRET", "")
	t.Setenv("POOL_K12_MGMT_SECRET", "")
	provDir := t.TempDir()
	t.Setenv("POOL_PROVISION_DIR", provDir)
	t.Setenv("POOL_REGISTRY_FILE", filepath.Join(t.TempDir(), "pools.json"))

	// Request → writes a create request the watcher will pick up. Ids are numeric
	// and independent of the label, so the first pool is "1".
	id, err := RequestPoolProvision("Edu Pool", "cliproxy")
	require.NoError(t, err)
	assert.Equal(t, "1", id)
	reqData, err := os.ReadFile(filepath.Join(provDir, "requests", "1.json"))
	require.NoError(t, err)
	assert.Contains(t, string(reqData), `"action":"create"`)
	assert.Contains(t, string(reqData), `"pool_id":"1"`)
	assert.Contains(t, string(reqData), `"label":"Edu Pool"`)
	assert.Contains(t, string(reqData), `"port":8319`) // first free port above k12 8318

	// An empty label is refused.
	_, err = RequestPoolProvision("   ", "cliproxy")
	assert.Error(t, err)

	// Poll before any result → still provisioning.
	status, err := PollPoolProvision("1")
	require.NoError(t, err)
	assert.Equal(t, "provisioning", status)

	resDir := filepath.Join(provDir, "results")
	require.NoError(t, os.MkdirAll(resDir, 0o755))

	// Watcher reports failure → error, pool not registered.
	require.NoError(t, os.WriteFile(filepath.Join(resDir, "1.json"),
		[]byte(`{"pool_id":"1","status":"error","error":"docker run failed"}`), 0o600))
	status, err = PollPoolProvision("1")
	assert.Equal(t, "error", status)
	assert.Error(t, err)
	_, _, ok := common.ResolvePoolMgmt("1")
	assert.False(t, ok)

	// Watcher reports success → pool registered, status ready.
	require.NoError(t, os.WriteFile(filepath.Join(resDir, "1.json"),
		[]byte(`{"pool_id":"1","label":"Edu Pool","status":"ok",`+
			`"mgmt_url":"http://cli-proxy-api-1:8319","mgmt_secret":"sec","port":8319,"internal_key":"k"}`), 0o600))
	status, err = PollPoolProvision("1")
	require.NoError(t, err)
	assert.Equal(t, "ready", status)

	base, secret, ok := common.ResolvePoolMgmt("1")
	require.True(t, ok)
	assert.Equal(t, "http://cli-proxy-api-1:8319", base)
	assert.Equal(t, "sec", secret)

	// Idempotent: a second poll stays ready without erroring on re-add.
	status, err = PollPoolProvision("1")
	require.NoError(t, err)
	assert.Equal(t, "ready", status)
}

func TestNormalizeBuildMode(t *testing.T) { // T3.3
	for _, in := range []string{"gopool", " GoPool ", "GOPOOL"} {
		assert.Equal(t, "gopool", normalizeBuildMode(in), "in=%q", in)
	}
	for _, in := range []string{"", "cliproxy", "garbage", "xyz"} {
		assert.Equal(t, "cliproxy", normalizeBuildMode(in), "in=%q", in)
	}
}

func TestRequestPoolProvisionMode(t *testing.T) { // T3.4
	dir := t.TempDir()
	t.Setenv("POOL_PROVISION_DIR", dir)
	t.Setenv("POOL_REGISTRY_FILE", filepath.Join(dir, "reg.json"))
	id, err := RequestPoolProvision("Edu Pool", "gopool")
	require.NoError(t, err)
	assert.Equal(t, "1", id)
	data, err := os.ReadFile(filepath.Join(dir, "requests", "1.json"))
	require.NoError(t, err)
	assert.Contains(t, string(data), `"mode":"gopool"`)

	// The first request is still pending (not yet registered), so the id allocator
	// must not hand out "1" again.
	id2, err := RequestPoolProvision("Plain", "")
	require.NoError(t, err)
	assert.Equal(t, "2", id2)
	data2, err := os.ReadFile(filepath.Join(dir, "requests", id2+".json"))
	require.NoError(t, err)
	assert.Contains(t, string(data2), `"mode":"cliproxy"`)
}

func TestRequestPoolProvisionProviderCombinations(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("POOL_PROVISION_DIR", dir)
	t.Setenv("POOL_REGISTRY_FILE", filepath.Join(dir, "reg.json"))

	id, err := RequestPoolProvisionForProvider("Claude Team", common.PoolProviderClaude, "cliproxy")
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(dir, "requests", id+".json"))
	require.NoError(t, err)
	var request provisionRequest
	require.NoError(t, common.Unmarshal(data, &request))
	assert.Equal(t, common.PoolProviderClaude, request.Provider)
	assert.Equal(t, "cliproxy", request.Mode)

	_, err = RequestPoolProvisionForProvider("Invalid Claude", common.PoolProviderClaude, "gopool")
	assert.ErrorContains(t, err, "CPA mode only")
	_, err = RequestPoolProvisionForProvider("Unknown", "gemini", "cliproxy")
	assert.ErrorContains(t, err, "unsupported pool provider")
	_, err = RequestPoolProvisionForProvider("Unknown mode", common.PoolProviderCodex, "custom")
	assert.ErrorContains(t, err, "unsupported pool build mode")
}

func TestPollPoolProvisionRegistersMode(t *testing.T) { // T3.5
	dir := t.TempDir()
	t.Setenv("POOL_PROVISION_DIR", dir)
	t.Setenv("POOL_REGISTRY_FILE", filepath.Join(dir, "reg.json"))
	_, err := RequestPoolProvision("Edu Pool", "gopool")
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "results"), 0o755))
	res := `{"pool_id":"1","label":"Edu Pool","action":"create","status":"ok","mgmt_url":"http://cli-proxy-api-1:8319","mgmt_secret":"sec","port":8319,"internal_key":"k","error":"","mode":"gopool","kind":"admin"}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "results", "1.json"), []byte(res), 0o600))
	status, err := PollPoolProvision("1")
	require.NoError(t, err)
	assert.Equal(t, "ready", status)
	entry, ok := common.GetPoolEntry("1")
	require.True(t, ok)
	assert.Equal(t, "gopool", entry.BuildMode)
}

func TestPollPoolProvisionRegistersClaudeProvider(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("POOL_PROVISION_DIR", dir)
	t.Setenv("POOL_REGISTRY_FILE", filepath.Join(dir, "reg.json"))
	id, err := RequestPoolProvisionForProvider("Claude Team", common.PoolProviderClaude, "cliproxy")
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "results"), 0o755))
	result := provisionResult{
		PoolID: id, Label: "Claude Team", Action: "create", Status: "ok",
		MgmtURL: "http://cli-proxy-api-1:8319", MgmtSecret: "sec", Port: 8319, InternalKey: "key",
		Provider: common.PoolProviderClaude, Mode: "cliproxy", Kind: common.PoolKindAdmin, GroupKey: id,
	}
	data, err := common.Marshal(result)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "results", id+".json"), data, 0o600))

	status, err := PollPoolProvision(id)
	require.NoError(t, err)
	assert.Equal(t, "ready", status)
	entry, ok := common.GetPoolEntry(id)
	require.True(t, ok)
	assert.Equal(t, common.PoolProviderClaude, entry.Provider)
}

func TestPrivatePoolProvisionPersistsOwnership(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("POOL_PROVISION_DIR", dir)
	t.Setenv("POOL_REGISTRY_FILE", filepath.Join(dir, "reg.json"))
	t.Setenv("POOL_MGMT_SECRET", "")
	t.Setenv("POOL_K12_MGMT_SECRET", "")

	id, err := RequestPrivatePoolProvision("Alice Pool", 42)
	require.NoError(t, err)
	assert.Equal(t, "1", id)
	state, err := GetPrivatePoolProvisionState(42)
	require.NoError(t, err)
	assert.Equal(t, "provisioning", state.Status)
	assert.Equal(t, "1", state.PoolID)

	requestData, err := os.ReadFile(filepath.Join(dir, "requests", "1.json"))
	require.NoError(t, err)
	var request provisionRequest
	require.NoError(t, common.Unmarshal(requestData, &request))
	assert.Equal(t, 42, request.OwnerUserID)
	assert.Equal(t, common.PoolKindPrivate, request.Kind)
	assert.Equal(t, "private-42", request.GroupKey)
	assert.Equal(t, "cliproxy", request.Mode)

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "results"), 0o755))
	result := provisionResult{
		PoolID: id, Label: request.Label, Action: "create", Status: "ok",
		MgmtURL: "http://cli-proxy-api-1:8319", MgmtSecret: "sec", Port: request.Port, InternalKey: "key",
		Mode: request.Mode, OwnerUserID: request.OwnerUserID, Kind: request.Kind, GroupKey: request.GroupKey,
	}
	resultData, err := common.Marshal(result)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "results", "1.json"), resultData, 0o600))

	state, err = GetPrivatePoolProvisionState(42)
	require.NoError(t, err)
	assert.Equal(t, "ready", state.Status)
	entry, ok := common.GetPoolEntry(id)
	require.True(t, ok)
	assert.Equal(t, 42, entry.OwnerUserID)
	assert.Equal(t, common.PoolKindPrivate, entry.Kind)
	assert.Equal(t, "private-42", entry.GroupKey)

	_, err = PollPrivatePoolProvision(id, 7)
	assert.Error(t, err, "a different user cannot poll the private pool as their own")
}

func TestPrivatePoolProvisionLimits(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("POOL_PROVISION_DIR", dir)
	t.Setenv("POOL_REGISTRY_FILE", filepath.Join(dir, "reg.json"))

	_, err := RequestPrivatePoolProvision("Alice", 1)
	require.NoError(t, err)
	_, err = RequestPrivatePoolProvision("Alice Again", 1)
	assert.Error(t, err, "one pending private pool per user")

	for owner := 2; owner <= MaxPrivatePools; owner++ {
		_, err = RequestPrivatePoolProvision(fmt.Sprintf("Pool %d", owner), owner)
		require.NoError(t, err)
	}
	_, err = RequestPrivatePoolProvision("Too Many", MaxPrivatePools+1)
	assert.Error(t, err, "private pool count is capped")
}

func TestConcurrentPoolProvisionAllocatesUniqueIDsAndPorts(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("POOL_PROVISION_DIR", dir)
	t.Setenv("POOL_REGISTRY_FILE", filepath.Join(dir, "reg.json"))

	const count = 10
	ids := make(chan string, count)
	errs := make(chan error, count)
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			id, err := RequestPoolProvision(fmt.Sprintf("Pool %d", index), "cliproxy")
			if err != nil {
				errs <- err
				return
			}
			ids <- id
		}(i)
	}
	wg.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	seenIDs := map[string]bool{}
	seenPorts := map[int]bool{}
	for id := range ids {
		assert.False(t, seenIDs[id], "duplicate pool id %s", id)
		seenIDs[id] = true
		data, err := os.ReadFile(filepath.Join(dir, "requests", id+".json"))
		require.NoError(t, err)
		var request provisionRequest
		require.NoError(t, common.Unmarshal(data, &request))
		assert.False(t, seenPorts[request.Port], "duplicate pool port %d", request.Port)
		seenPorts[request.Port] = true
	}
	assert.Len(t, seenIDs, count)
	assert.Len(t, seenPorts, count)
}

func TestRenamePool(t *testing.T) {
	t.Setenv("POOL_MGMT_SECRET", "")
	t.Setenv("POOL_K12_MGMT_SECRET", "")
	t.Setenv("POOL_REGISTRY_FILE", filepath.Join(t.TempDir(), "pools.json"))

	// A registered pool "1" routed by a channel in group "1".
	require.NoError(t, model.DB.Where("1 = 1").Delete(&model.Channel{}).Error)
	require.NoError(t, model.DB.Where("1 = 1").Delete(&model.Token{}).Error)
	ch := &model.Channel{Type: 1, Name: "cliproxy-pool-1", Key: "k", Group: "1", Models: "gpt-5", Status: 1}
	require.NoError(t, ch.Insert())
	// A card already issued into that group.
	card := &model.Token{UserId: 1, Key: "card000000000000000000000000000001", Name: "c1", Group: "1"}
	require.NoError(t, model.DB.Create(card).Error)
	require.NoError(t, common.AddPoolToRegistry(common.PoolEntry{
		ID: "1", Label: "Old Name", MgmtURL: "http://cli-proxy-api-1:8319", MgmtSecret: "s", ChannelID: ch.Id,
	}))

	require.NoError(t, RenamePool("1", "  Campus  "))

	// Registry label, channel name, and channel GROUP all become "Campus".
	entry, ok := common.GetPoolEntry("1")
	require.True(t, ok)
	assert.Equal(t, "Campus", entry.Label)
	got, err := model.GetChannelById(ch.Id, false)
	require.NoError(t, err)
	assert.Equal(t, "Campus", got.Name)
	assert.Equal(t, "Campus", got.Group, "routing group renamed too")

	// The already-issued card was migrated to the new group so it still routes.
	var migrated model.Token
	require.NoError(t, model.DB.First(&migrated, card.Id).Error)
	assert.Equal(t, "Campus", migrated.Group, "issued card migrated to the new group")

	// A name already used by another group is refused (no silent merge of cards).
	other := &model.Channel{Type: 1, Name: "other", Key: "k2", Group: "Taken", Models: "gpt-5", Status: 1}
	require.NoError(t, other.Insert())
	assert.Error(t, RenamePool("1", "Taken"))

	// Guards: empty name, unknown pool, and built-in pools are all refused.
	assert.Error(t, RenamePool("1", "   "))
	assert.Error(t, RenamePool("nope", "X"))
	assert.Error(t, RenamePool("default", "X"))
}
