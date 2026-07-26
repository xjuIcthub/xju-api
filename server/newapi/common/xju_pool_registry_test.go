package common

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resetPoolRegCache clears the mtime-keyed dynamic-registry cache so each test
// starts clean (the cache is package-level, shared across tests).
func resetPoolRegCache() {
	poolRegMu.Lock()
	poolRegLoaded = false
	poolRegEntries = nil
	poolRegMtime = time.Time{}
	poolRegMu.Unlock()
}

func TestResolvePoolMgmt(t *testing.T) {
	t.Setenv("POOL_MGMT_URL", "http://cli-proxy-api:8317")
	t.Setenv("POOL_MGMT_SECRET", "def-secret")
	t.Setenv("POOL_K12_MGMT_URL", "http://cli-proxy-api-k12:8318")
	t.Setenv("POOL_K12_MGMT_SECRET", "k12-secret")

	base, secret, ok := ResolvePoolMgmt("")
	require.True(t, ok)
	assert.Equal(t, "http://cli-proxy-api:8317", base)
	assert.Equal(t, "def-secret", secret)

	base, secret, ok = ResolvePoolMgmt("default")
	require.True(t, ok)
	assert.Equal(t, "def-secret", secret)

	base, secret, ok = ResolvePoolMgmt("k12")
	require.True(t, ok)
	assert.Equal(t, "http://cli-proxy-api-k12:8318", base)
	assert.Equal(t, "k12-secret", secret)

	_, _, ok = ResolvePoolMgmt("nope")
	assert.False(t, ok, "unknown pool must be not-ok")
}

func TestResolvePoolMgmtUnconfiguredSecret(t *testing.T) {
	t.Setenv("POOL_MGMT_SECRET", "")
	t.Setenv("POOL_K12_MGMT_SECRET", "")
	_, _, ok := ResolvePoolMgmt("default")
	assert.False(t, ok, "empty secret means pool is off")
	_, _, ok = ResolvePoolMgmt("k12")
	assert.False(t, ok)
}

func TestListConfiguredPools(t *testing.T) {
	t.Setenv("POOL_MGMT_SECRET", "def-secret")
	t.Setenv("POOL_K12_MGMT_SECRET", "")
	pools := ListConfiguredPools()
	require.Len(t, pools, 1)
	assert.Equal(t, "default", pools[0].ID)

	t.Setenv("POOL_K12_MGMT_SECRET", "k12-secret")
	pools = ListConfiguredPools()
	require.Len(t, pools, 2)
	assert.Equal(t, "k12", pools[1].ID)
}

func TestListSharedPoolsExcludesUserOwnedPools(t *testing.T) {
	file := filepath.Join(t.TempDir(), "pools.json")
	t.Setenv("POOL_REGISTRY_FILE", file)
	t.Setenv("POOL_MGMT_SECRET", "default-secret")
	t.Setenv("POOL_K12_MGMT_SECRET", "")
	resetPoolRegCache()
	t.Cleanup(resetPoolRegCache)

	require.NoError(t, AddPoolToRegistry(PoolEntry{
		ID: "team", Label: "Team", MgmtURL: "http://team:8319", MgmtSecret: "team-secret", Kind: PoolKindAdmin,
	}))
	require.NoError(t, AddPoolToRegistry(PoolEntry{
		ID: "alice", Label: "Alice", MgmtURL: "http://alice:8320", MgmtSecret: "alice-secret",
		OwnerUserID: 42, Kind: PoolKindPrivate,
	}))

	shared := ListSharedPools()
	require.Len(t, shared, 2)
	assert.Equal(t, []string{"default", "team"}, []string{shared[0].ID, shared[1].ID})

	privatePool, ok := FindConfiguredPoolInfo("alice")
	require.True(t, ok)
	assert.Equal(t, PoolKindPrivate, privatePool.Kind)
	defaultPool, ok := FindConfiguredPoolInfo("")
	require.True(t, ok)
	assert.Equal(t, "default", defaultPool.ID)
}

// xju-api:new — dynamic pool registry (号池验活 Part A / #4 Phase A): env-seeded
// default/k12 plus file-backed dynamic pools, with add/remove/port allocation.

func TestDynamicPoolRegistry(t *testing.T) {
	t.Setenv("POOL_MGMT_SECRET", "def")
	t.Setenv("POOL_K12_MGMT_SECRET", "k12")
	file := filepath.Join(t.TempDir(), "pools.json")
	t.Setenv("POOL_REGISTRY_FILE", file)
	resetPoolRegCache()
	t.Cleanup(resetPoolRegCache)

	// No file yet → only the two env pools.
	assert.Len(t, ListConfiguredPools(), 2)
	_, _, ok := ResolvePoolMgmt("edu")
	assert.False(t, ok)

	require.NoError(t, AddPoolToRegistry(PoolEntry{
		ID: "edu", Label: "Edu",
		MgmtURL: "http://cli-proxy-api-edu:8319/", MgmtSecret: "edu-sec", Port: 8319,
	}))

	base, secret, ok := ResolvePoolMgmt("edu")
	require.True(t, ok)
	assert.Equal(t, "http://cli-proxy-api-edu:8319", base) // trailing slash trimmed
	assert.Equal(t, "edu-sec", secret)

	pools := ListConfiguredPools()
	require.Len(t, pools, 3)
	assert.Equal(t, "edu", pools[2].ID)
	assert.Equal(t, "Edu", pools[2].Label)

	// Duplicates and reserved ids are rejected.
	assert.Error(t, AddPoolToRegistry(PoolEntry{ID: "edu", MgmtURL: "x", MgmtSecret: "y"}))
	assert.Error(t, AddPoolToRegistry(PoolEntry{ID: "default", MgmtURL: "x", MgmtSecret: "y"}))
	assert.Error(t, AddPoolToRegistry(PoolEntry{ID: "k12", MgmtURL: "x", MgmtSecret: "y"}))

	// Next port sits above the highest in use (k12 8318 vs edu 8319).
	assert.Equal(t, 8320, AllocateNextPoolPort())

	require.NoError(t, RemovePoolFromRegistry("edu"))
	_, _, ok = ResolvePoolMgmt("edu")
	assert.False(t, ok)
	assert.Len(t, ListConfiguredPools(), 2)
	assert.Error(t, RemovePoolFromRegistry("edu")) // already gone
}

func TestPoolRegistryIgnoresReservedInFile(t *testing.T) {
	t.Setenv("POOL_MGMT_SECRET", "def")
	t.Setenv("POOL_K12_MGMT_SECRET", "")
	file := filepath.Join(t.TempDir(), "pools.json")
	// A hand-written file smuggling a reserved id must not override env.
	require.NoError(t, os.WriteFile(file, []byte(
		`[{"id":"default","mgmt_url":"bogus","mgmt_secret":"bogus"},`+
			`{"id":"edu","label":"Edu","mgmt_url":"http://e:8319","mgmt_secret":"s"}]`), 0o600))
	t.Setenv("POOL_REGISTRY_FILE", file)
	resetPoolRegCache()
	t.Cleanup(resetPoolRegCache)

	_, secret, ok := ResolvePoolMgmt("default")
	require.True(t, ok)
	assert.Equal(t, "def", secret, "reserved id resolves from env, not the file")

	_, _, ok = ResolvePoolMgmt("edu")
	assert.True(t, ok)

	ids := make([]string, 0)
	for _, p := range ListConfiguredPools() {
		ids = append(ids, p.ID)
	}
	assert.Equal(t, []string{"default", "edu"}, ids, "no duplicate default from the file")
}

func TestSavePoolRegistryRequiresPath(t *testing.T) {
	t.Setenv("POOL_REGISTRY_FILE", "")
	resetPoolRegCache()
	assert.Error(t, SavePoolRegistry([]PoolEntry{{ID: "x"}}))
	assert.Error(t, AddPoolToRegistry(PoolEntry{ID: "x", MgmtURL: "u", MgmtSecret: "s"}))
}

func TestPoolEntryBuildModePersists(t *testing.T) { // T3.1
	dir := t.TempDir()
	t.Setenv("POOL_REGISTRY_FILE", filepath.Join(dir, "reg.json"))
	require.NoError(t, AddPoolToRegistry(PoolEntry{
		ID: "edu", Label: "Edu", MgmtURL: "http://x:9", MgmtSecret: "s", BuildMode: "gopool",
	}))
	got, ok := GetPoolEntry("edu")
	require.True(t, ok)
	assert.Equal(t, "gopool", got.BuildMode)
}

func TestPoolEntryProviderPersistsAndLegacyDefaultsToCodex(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("POOL_REGISTRY_FILE", filepath.Join(dir, "reg.json"))
	require.NoError(t, AddPoolToRegistry(PoolEntry{
		ID: "claude", Label: "Claude", MgmtURL: "http://x:9", MgmtSecret: "s",
		Provider: PoolProviderClaude, Kind: PoolKindAdmin, GroupKey: "claude",
	}))
	require.NoError(t, AddPoolToRegistry(PoolEntry{
		ID: "legacy", Label: "Legacy", MgmtURL: "http://y:9", MgmtSecret: "s",
	}))

	claude, ok := GetPoolEntry("claude")
	require.True(t, ok)
	assert.Equal(t, PoolProviderClaude, claude.Provider)
	legacy, ok := GetPoolEntry("legacy")
	require.True(t, ok)
	assert.Equal(t, PoolProviderCodex, legacy.Provider)

	provider, ok := FindPoolProviderByRouting(claude.ChannelID, claude.GroupKey)
	require.True(t, ok)
	assert.Equal(t, PoolProviderClaude, provider)
	assert.Error(t, AddPoolToRegistry(PoolEntry{
		ID: "bad", MgmtURL: "http://z:9", MgmtSecret: "s", Provider: "gemini",
	}))
}

func TestListConfiguredPoolsBuildModeDefault(t *testing.T) { // T3.2
	dir := t.TempDir()
	t.Setenv("POOL_REGISTRY_FILE", filepath.Join(dir, "reg.json"))
	t.Setenv("POOL_MGMT_SECRET", "def")
	require.NoError(t, AddPoolToRegistry(PoolEntry{ID: "legacy", Label: "Legacy", MgmtURL: "http://x:9", MgmtSecret: "s"})) // 无 BuildMode
	require.NoError(t, AddPoolToRegistry(PoolEntry{ID: "go1", Label: "Go1", MgmtURL: "http://x:9", MgmtSecret: "s", BuildMode: "gopool"}))
	byID := map[string]string{}
	for _, p := range ListConfiguredPools() {
		byID[p.ID] = p.BuildMode
	}
	assert.Equal(t, "cliproxy", byID["default"]) // env 池默认 cliproxy
	assert.Equal(t, "cliproxy", byID["legacy"])  // 老条目无字段 → 回填 cliproxy
	assert.Equal(t, "gopool", byID["go1"])       // 透传
}

func TestAllocateNextPoolID(t *testing.T) {
	file := filepath.Join(t.TempDir(), "pools.json")
	t.Setenv("POOL_REGISTRY_FILE", file)
	resetPoolRegCache()
	t.Cleanup(resetPoolRegCache)

	// Empty registry → the first numeric id is 1.
	assert.Equal(t, "1", AllocateNextPoolID())

	// Non-numeric legacy ids are ignored; numeric ids drive max+1.
	require.NoError(t, AddPoolToRegistry(PoolEntry{ID: "main", Label: "Default", MgmtURL: "u", MgmtSecret: "s"}))
	require.NoError(t, AddPoolToRegistry(PoolEntry{ID: "3", Label: "Three", MgmtURL: "u", MgmtSecret: "s"}))
	assert.Equal(t, "4", AllocateNextPoolID(), "max numeric (3) + 1, legacy 'main' ignored")
}

func TestSetPoolLabel(t *testing.T) {
	file := filepath.Join(t.TempDir(), "pools.json")
	t.Setenv("POOL_REGISTRY_FILE", file)
	resetPoolRegCache()
	t.Cleanup(resetPoolRegCache)

	require.NoError(t, AddPoolToRegistry(PoolEntry{ID: "1", Label: "Old", MgmtURL: "u", MgmtSecret: "s"}))
	require.NoError(t, SetPoolLabel("1", "  New Name  "))
	got, ok := GetPoolEntry("1")
	require.True(t, ok)
	assert.Equal(t, "New Name", got.Label, "label updated and trimmed")

	assert.Error(t, SetPoolLabel("nope", "X"), "unknown pool id errors")
}

func TestLegacyPoolEntryDefaultsToAdmin(t *testing.T) {
	file := filepath.Join(t.TempDir(), "pools.json")
	require.NoError(t, os.WriteFile(file, []byte(
		`[{"id":"legacy","label":"Legacy","mgmt_url":"http://x:9","mgmt_secret":"s"}]`), 0o600))
	t.Setenv("POOL_REGISTRY_FILE", file)
	resetPoolRegCache()
	t.Cleanup(resetPoolRegCache)

	entry, ok := GetPoolEntry("legacy")
	require.True(t, ok)
	assert.Equal(t, PoolKindAdmin, entry.Kind)
	assert.Zero(t, entry.OwnerUserID)
	assert.Empty(t, entry.GroupKey)

	pools := ListConfiguredPools()
	require.Len(t, pools, 1)
	assert.Equal(t, PoolKindAdmin, pools[0].Kind)
}

func TestPrivatePoolOwnershipMetadataAndQueries(t *testing.T) {
	file := filepath.Join(t.TempDir(), "pools.json")
	t.Setenv("POOL_REGISTRY_FILE", file)
	t.Setenv("POOL_MGMT_SECRET", "def")
	resetPoolRegCache()
	t.Cleanup(resetPoolRegCache)

	require.NoError(t, AddPoolToRegistry(PoolEntry{
		ID: "1", Label: "Alice Pool", MgmtURL: "http://pool-1:8319", MgmtSecret: "secret",
		OwnerUserID: 42, Kind: PoolKindPrivate,
	}))

	entry, ok := FindPrivatePoolByOwner(42)
	require.True(t, ok)
	assert.Equal(t, "private-42", entry.GroupKey)
	assert.Positive(t, entry.CreatedAt)
	assert.Equal(t, 1, CountPrivatePools())

	byGroup, ok := FindPrivatePoolByGroupKey("private-42")
	require.True(t, ok)
	assert.Equal(t, "1", byGroup.ID)

	assert.True(t, CanManagePool(42, RoleCommonUser, entry))
	assert.True(t, CanManagePool(42, RoleAdminUser, entry))
	assert.False(t, CanManagePool(7, RoleAdminUser, entry), "admin cannot manage another user's pool")
	assert.True(t, CanManagePool(7, RoleRootUser, entry))

	userPools := ListPoolsForActor(42, RoleCommonUser)
	require.Len(t, userPools, 1)
	assert.Equal(t, 42, userPools[0].OwnerUserID)
	assert.Equal(t, "private-42", userPools[0].GroupKey)
	assert.Len(t, ListPoolsForActor(7, RoleCommonUser), 0)
	assert.Len(t, ListPoolsForActor(7, RoleRootUser), 2, "root sees system and private pools")
}

func TestPrivatePoolRegistryRejectsInvalidOwnership(t *testing.T) {
	file := filepath.Join(t.TempDir(), "pools.json")
	t.Setenv("POOL_REGISTRY_FILE", file)
	resetPoolRegCache()
	t.Cleanup(resetPoolRegCache)

	assert.Error(t, AddPoolToRegistry(PoolEntry{
		ID: "1", MgmtURL: "u", MgmtSecret: "s", Kind: PoolKindPrivate,
	}), "private pool requires an owner")
	assert.Error(t, AddPoolToRegistry(PoolEntry{
		ID: "1", MgmtURL: "u", MgmtSecret: "s", Kind: PoolKindPrivate, OwnerUserID: 42, GroupKey: "forged",
	}), "private pool group key is derived from owner")

	require.NoError(t, AddPoolToRegistry(PoolEntry{
		ID: "1", MgmtURL: "u", MgmtSecret: "s", Kind: PoolKindPrivate, OwnerUserID: 42,
	}))
	assert.Error(t, AddPoolToRegistry(PoolEntry{
		ID: "2", MgmtURL: "u2", MgmtSecret: "s2", Kind: PoolKindPrivate, OwnerUserID: 42,
	}), "one private pool per user")
	assert.Error(t, AddPoolToRegistry(PoolEntry{
		ID: "3", MgmtURL: "u3", MgmtSecret: "s3", Kind: PoolKindSystem,
	}), "system pools are environment-seeded only")
}

func TestSetPrivatePoolSettingsIsOwnerScoped(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("POOL_REGISTRY_FILE", filepath.Join(dir, "registry.json"))
	require.NoError(t, SavePoolRegistry([]PoolEntry{{
		ID: "private-settings", Label: "Settings", MgmtURL: "http://pool", MgmtSecret: "secret",
		Kind: PoolKindPrivate, OwnerUserID: 42, GroupKey: PrivatePoolGroupKey(42),
	}}))

	err := SetPrivatePoolSettings("private-settings", 7, PrivatePoolSettings{AutoCleanEnabled: true, AutoCleanHours: 12})
	require.Error(t, err)
	require.NoError(t, SetPrivatePoolSettings("private-settings", 42, PrivatePoolSettings{
		AutoCleanEnabled: true, AutoCleanHours: 12, UsageAutoRefreshEnabled: true, UsageAutoResetEnabled: true,
	}))
	entry, ok := GetPoolEntry("private-settings")
	require.True(t, ok)
	assert.True(t, entry.AutoCleanEnabled)
	assert.Equal(t, 12, entry.AutoCleanHours)
	assert.True(t, entry.UsageAutoRefreshEnabled)
	assert.True(t, entry.UsageAutoResetEnabled)
}

func TestConcurrentPoolRegistryAddsDoNotLoseEntries(t *testing.T) {
	file := filepath.Join(t.TempDir(), "pools.json")
	t.Setenv("POOL_REGISTRY_FILE", file)
	resetPoolRegCache()
	t.Cleanup(resetPoolRegCache)

	const count = 10
	errs := make(chan error, count)
	var wg sync.WaitGroup
	for i := 1; i <= count; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			errs <- AddPoolToRegistry(PoolEntry{
				ID: fmt.Sprint(id), MgmtURL: fmt.Sprintf("http://pool-%d", id), MgmtSecret: "s",
			})
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	for i := 1; i <= count; i++ {
		_, ok := GetPoolEntry(fmt.Sprint(i))
		assert.True(t, ok, "pool %d was lost during concurrent registry writes", i)
	}
}
