package common

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// xju-api:new — account-pool management registry.
//
// xju-api addresses each CLIProxyAPI pool by its own management base URL +
// Bearer secret. Two pools are ENV-seeded and permanent: the primary
// ("default") and the K12 pool ("k12"). Any additional pools are DYNAMIC,
// created at runtime (号池验活 Part A / #4 一键开池) and persisted in a JSON
// registry file (POOL_REGISTRY_FILE) on the data volume. Resolving an unknown
// pool, or one whose secret is unset, returns ok=false so callers degrade to
// 503 and the frontend hides that pool — a deployment that wires only the
// default pool keeps working unchanged, and one with no registry file behaves
// exactly as before.

const (
	PoolKindSystem  = "system"
	PoolKindAdmin   = "admin"
	PoolKindPrivate = "private"

	PoolProviderCodex  = "codex"
	PoolProviderClaude = "claude"

	ContextKeyPrivatePoolID    = "xju_private_pool_id"
	ContextKeyPrivatePoolScope = "xju_private_pool_scope"
	ContextKeyPoolReadOnly     = "xju_pool_read_only"
)

// PoolInfo is the safe metadata the frontend uses to render a pool selector.
// Management URLs and secrets intentionally remain exclusive to PoolEntry.
type PoolInfo struct {
	ID                      string `json:"id"`
	Label                   string `json:"label"`
	Provider                string `json:"provider"`
	BuildMode               string `json:"build_mode,omitempty"`
	OwnerUserID             int    `json:"owner_user_id,omitempty"`
	OwnerUsername           string `json:"owner_username,omitempty"`
	Kind                    string `json:"kind"`
	GroupKey                string `json:"group_key,omitempty"`
	CreatedAt               int64  `json:"created_at,omitempty"`
	AutoCleanEnabled        bool   `json:"auto_clean_enabled,omitempty"`
	AutoCleanHours          int    `json:"auto_clean_hours,omitempty"`
	UsageAutoRefreshEnabled bool   `json:"usage_auto_refresh_enabled,omitempty"`
	UsageAutoResetEnabled   bool   `json:"usage_auto_reset_enabled,omitempty"`
}

// PoolEntry is a dynamically-provisioned pool's full record in the registry
// file. The env-seeded default/k12 pools are never stored here.
type PoolEntry struct {
	ID                      string `json:"id"`
	Label                   string `json:"label"`
	Provider                string `json:"provider,omitempty"` // "codex"(legacy/default) | "claude"
	MgmtURL                 string `json:"mgmt_url"`
	MgmtSecret              string `json:"mgmt_secret"`
	Port                    int    `json:"port,omitempty"`
	ChannelID               int    `json:"channel_id,omitempty"`
	BuildMode               string `json:"build_mode,omitempty"` // "cliproxy"(默认) | "gopool";仅 UI 引导,无服务端强制
	OwnerUserID             int    `json:"owner_user_id,omitempty"`
	Kind                    string `json:"kind,omitempty"`      // "admin"(legacy dynamic pool) | "private"
	GroupKey                string `json:"group_key,omitempty"` // immutable routing key; private pools use private-<user id>
	CreatedAt               int64  `json:"created_at,omitempty"`
	AutoCleanEnabled        bool   `json:"auto_clean_enabled,omitempty"`
	AutoCleanHours          int    `json:"auto_clean_hours,omitempty"`
	UsageAutoRefreshEnabled bool   `json:"usage_auto_refresh_enabled,omitempty"`
	UsageAutoResetEnabled   bool   `json:"usage_auto_reset_enabled,omitempty"`
}

type PrivatePoolSettings struct {
	AutoCleanEnabled        bool `json:"auto_clean_enabled"`
	AutoCleanHours          int  `json:"auto_clean_hours"`
	UsageAutoRefreshEnabled bool `json:"usage_auto_refresh_enabled"`
	UsageAutoResetEnabled   bool `json:"usage_auto_reset_enabled"`
}

// reservedPoolIDs are env-seeded and can never be created/removed dynamically.
var reservedPoolIDs = map[string]bool{"": true, "default": true, "k12": true}

// IsReservedPoolID reports whether a pool id belongs to the env-seeded pools.
func IsReservedPoolID(id string) bool {
	return reservedPoolIDs[strings.TrimSpace(id)]
}

var (
	poolRegMu         sync.RWMutex
	poolRegMutationMu sync.Mutex
	poolRegEntries    []PoolEntry
	poolRegMtime      time.Time
	poolRegLoaded     bool
)

func poolRegistryFile() string {
	return strings.TrimSpace(os.Getenv("POOL_REGISTRY_FILE"))
}

func normalizePoolKind(kind string, ownerUserID int) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case PoolKindSystem:
		return PoolKindSystem
	case PoolKindPrivate:
		return PoolKindPrivate
	case PoolKindAdmin:
		return PoolKindAdmin
	}
	// Backwards compatibility: old registry entries have neither field and are
	// administrator-created shared pools. Owner-aware entries written during a
	// rolling upgrade are private even if the kind field is absent.
	if ownerUserID > 0 {
		return PoolKindPrivate
	}
	return PoolKindAdmin
}

// NormalizePoolProvider keeps legacy registry entries compatible: every pool
// created before the provider field existed is a Codex/OpenAI account pool.
// Invalid persisted values also degrade to Codex rather than making an existing
// deployment disappear; new mutations are validated strictly by
// AddPoolToRegistry and the provisioning service.
func NormalizePoolProvider(provider string) string {
	if strings.EqualFold(strings.TrimSpace(provider), PoolProviderClaude) {
		return PoolProviderClaude
	}
	return PoolProviderCodex
}

func IsSupportedPoolProvider(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case PoolProviderCodex, PoolProviderClaude:
		return true
	default:
		return false
	}
}

// PrivatePoolGroupKey returns the immutable routing group assigned to a user's
// private pool. It is derived server-side and must never come from a client.
func PrivatePoolGroupKey(ownerUserID int) string {
	return fmt.Sprintf("private-%d", ownerUserID)
}

func normalizePoolEntry(entry PoolEntry) PoolEntry {
	entry.ID = strings.TrimSpace(entry.ID)
	entry.Label = strings.TrimSpace(entry.Label)
	entry.Provider = NormalizePoolProvider(entry.Provider)
	entry.MgmtURL = strings.TrimSpace(entry.MgmtURL)
	entry.MgmtSecret = strings.TrimSpace(entry.MgmtSecret)
	entry.BuildMode = strings.TrimSpace(entry.BuildMode)
	entry.Kind = normalizePoolKind(entry.Kind, entry.OwnerUserID)
	entry.GroupKey = strings.TrimSpace(entry.GroupKey)
	if entry.Kind == PoolKindPrivate && entry.GroupKey == "" && entry.OwnerUserID > 0 {
		entry.GroupKey = PrivatePoolGroupKey(entry.OwnerUserID)
	}
	return entry
}

func poolInfoFromEntry(entry PoolEntry) PoolInfo {
	entry = normalizePoolEntry(entry)
	label := entry.Label
	if label == "" {
		label = entry.ID
	}
	buildMode := entry.BuildMode
	if buildMode == "" {
		buildMode = "cliproxy"
	}
	autoCleanHours := entry.AutoCleanHours
	if autoCleanHours <= 0 {
		autoCleanHours = 24
	}
	return PoolInfo{
		ID:                      entry.ID,
		Label:                   label,
		Provider:                entry.Provider,
		BuildMode:               buildMode,
		OwnerUserID:             entry.OwnerUserID,
		Kind:                    entry.Kind,
		GroupKey:                entry.GroupKey,
		CreatedAt:               entry.CreatedAt,
		AutoCleanEnabled:        entry.AutoCleanEnabled,
		AutoCleanHours:          autoCleanHours,
		UsageAutoRefreshEnabled: entry.UsageAutoRefreshEnabled,
		UsageAutoResetEnabled:   entry.UsageAutoResetEnabled,
	}
}

// loadPoolRegistry returns the dynamic pool entries, caching by file mtime so a
// concurrent process (or a later write) is picked up without a restart. An
// unset path or a missing/empty file yields no dynamic pools.
func loadPoolRegistry() []PoolEntry {
	path := poolRegistryFile()
	if path == "" {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil // no registry file yet → env-only pools
	}

	poolRegMu.RLock()
	if poolRegLoaded && info.ModTime().Equal(poolRegMtime) {
		entries := poolRegEntries
		poolRegMu.RUnlock()
		return entries
	}
	poolRegMu.RUnlock()

	poolRegMu.Lock()
	defer poolRegMu.Unlock()
	if poolRegLoaded && info.ModTime().Equal(poolRegMtime) {
		return poolRegEntries
	}
	data, err := os.ReadFile(path)
	if err != nil {
		SysError("pool registry read failed: " + err.Error())
		return poolRegEntries
	}
	var parsed []PoolEntry
	if len(strings.TrimSpace(string(data))) > 0 {
		if err := Unmarshal(data, &parsed); err != nil {
			SysError("pool registry parse failed: " + err.Error())
			return poolRegEntries
		}
	}
	// Reserved ids can never come from the file — the env owns them.
	kept := make([]PoolEntry, 0, len(parsed))
	for _, e := range parsed {
		e = normalizePoolEntry(e)
		if !reservedPoolIDs[e.ID] {
			kept = append(kept, e)
		}
	}
	poolRegEntries = kept
	poolRegMtime = info.ModTime()
	poolRegLoaded = true
	return poolRegEntries
}

// ResolvePoolMgmt returns the management base URL and secret for a pool id.
// "" and "default" resolve to the primary pool and "k12" to the K12 pool, both
// from the environment; any other id is looked up in the dynamic registry. ok
// is false when the pool id is unknown or its secret is not configured.
func ResolvePoolMgmt(poolID string) (baseURL string, secret string, ok bool) {
	id := strings.TrimSpace(poolID)
	switch id {
	case "", "default":
		baseURL = GetEnvOrDefaultString("POOL_MGMT_URL", "http://cli-proxy-api:8317")
		secret = strings.TrimSpace(os.Getenv("POOL_MGMT_SECRET"))
	case "k12":
		baseURL = GetEnvOrDefaultString("POOL_K12_MGMT_URL", "http://cli-proxy-api-k12:8318")
		secret = strings.TrimSpace(os.Getenv("POOL_K12_MGMT_SECRET"))
	default:
		for _, e := range loadPoolRegistry() {
			if e.ID != id {
				continue
			}
			url := strings.TrimSpace(e.MgmtURL)
			sec := strings.TrimSpace(e.MgmtSecret)
			if url == "" || sec == "" {
				return "", "", false
			}
			return strings.TrimRight(url, "/"), sec, true
		}
		return "", "", false
	}
	if secret == "" {
		return "", "", false
	}
	return strings.TrimRight(baseURL, "/"), secret, true
}

// ListConfiguredPools returns every pool whose secret is configured, in a stable
// order (default, k12, then dynamic pools in registry order), for the frontend
// pool selector.
func ListConfiguredPools() []PoolInfo {
	pools := make([]PoolInfo, 0, 4)
	if _, _, ok := ResolvePoolMgmt("default"); ok {
		pools = append(pools, PoolInfo{ID: "default", Label: "Default", Provider: PoolProviderCodex, BuildMode: "cliproxy", Kind: PoolKindSystem})
	}
	if _, _, ok := ResolvePoolMgmt("k12"); ok {
		pools = append(pools, PoolInfo{ID: "k12", Label: "K12", Provider: PoolProviderCodex, BuildMode: "cliproxy", Kind: PoolKindSystem})
	}
	for _, e := range loadPoolRegistry() {
		if strings.TrimSpace(e.MgmtSecret) == "" {
			continue
		}
		pools = append(pools, poolInfoFromEntry(e))
	}
	return pools
}

// ListSharedPools returns the system and administrator-created pools that are
// safe to expose through the shared pool workbench. User-owned private pools
// stay on their implicit-owner /api/private-pool surface.
func ListSharedPools() []PoolInfo {
	configured := ListConfiguredPools()
	shared := make([]PoolInfo, 0, len(configured))
	for _, pool := range configured {
		if pool.Kind != PoolKindPrivate {
			shared = append(shared, pool)
		}
	}
	return shared
}

// FindConfiguredPoolInfo resolves safe pool metadata by id. An empty id is the
// same primary pool as ResolvePoolMgmt's empty/default aliases.
func FindConfiguredPoolInfo(id string) (PoolInfo, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		id = "default"
	}
	for _, pool := range ListConfiguredPools() {
		if pool.ID == id {
			return pool, true
		}
	}
	return PoolInfo{}, false
}

// ListPoolsForActor returns only pools the caller may manage. Root can manage
// every configured pool; all other roles are limited to their own private pool.
func ListPoolsForActor(actorUserID, actorRole int) []PoolInfo {
	if actorRole >= RoleRootUser {
		return ListConfiguredPools()
	}
	entry, ok := FindPrivatePoolByOwner(actorUserID)
	if !ok || strings.TrimSpace(entry.MgmtSecret) == "" {
		return nil
	}
	return []PoolInfo{poolInfoFromEntry(entry)}
}

// ---------------------------------------------------------------------------
// Registry mutation (used by #4 一键开池 provisioning, Phase B/C). Writes are
// atomic (temp + rename) and invalidate the mtime cache so the next read
// reflects the change.

// SavePoolRegistry persists the full dynamic pool list.
func SavePoolRegistry(entries []PoolEntry) error {
	path := poolRegistryFile()
	if path == "" {
		return fmt.Errorf("POOL_REGISTRY_FILE is not configured")
	}
	data, err := Marshal(entries)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	poolRegMu.Lock()
	poolRegLoaded = false
	poolRegMu.Unlock()
	return nil
}

// AddPoolToRegistry appends a new dynamic pool. It rejects reserved ids and
// duplicates.
func AddPoolToRegistry(entry PoolEntry) error {
	poolRegMutationMu.Lock()
	defer poolRegMutationMu.Unlock()

	rawProvider := strings.ToLower(strings.TrimSpace(entry.Provider))
	if rawProvider != "" && !IsSupportedPoolProvider(rawProvider) {
		return fmt.Errorf("unsupported pool provider: %q", entry.Provider)
	}
	entry = normalizePoolEntry(entry)
	id := entry.ID
	if id == "" || reservedPoolIDs[id] {
		return fmt.Errorf("invalid or reserved pool id: %q", id)
	}
	if entry.Kind == PoolKindSystem {
		return fmt.Errorf("dynamic pool cannot use kind %q", PoolKindSystem)
	}
	if entry.Kind == PoolKindPrivate && entry.OwnerUserID <= 0 {
		return fmt.Errorf("private pool owner_user_id must be positive")
	}
	if entry.Kind == PoolKindPrivate && entry.Provider != PoolProviderCodex {
		return fmt.Errorf("private pools support provider %q only", PoolProviderCodex)
	}
	if entry.Kind == PoolKindPrivate && entry.GroupKey != PrivatePoolGroupKey(entry.OwnerUserID) {
		return fmt.Errorf("private pool group_key must be %q", PrivatePoolGroupKey(entry.OwnerUserID))
	}
	if entry.CreatedAt == 0 {
		entry.CreatedAt = time.Now().Unix()
	}
	entries := append([]PoolEntry(nil), loadPoolRegistry()...)
	for _, e := range entries {
		if e.ID == id {
			return fmt.Errorf("pool already exists: %s", id)
		}
		if entry.Kind == PoolKindPrivate && e.Kind == PoolKindPrivate && e.OwnerUserID == entry.OwnerUserID {
			return fmt.Errorf("user %d already owns private pool %s", entry.OwnerUserID, e.ID)
		}
		if entry.GroupKey != "" && e.GroupKey == entry.GroupKey {
			return fmt.Errorf("pool group_key already exists: %s", entry.GroupKey)
		}
	}
	entries = append(entries, entry)
	return SavePoolRegistry(entries)
}

// FindPoolProviderByRouting resolves the upstream account provider from the
// durable routing identity stored in quota/log rows. Dynamic pools prefer their
// channel id and fall back to the immutable group key. The two environment-
// seeded pools predate registry channel ids and are always Codex pools.
func FindPoolProviderByRouting(channelID int, groupKey string) (string, bool) {
	groupKey = strings.TrimSpace(groupKey)
	if groupKey == "default" || groupKey == "k12" {
		return PoolProviderCodex, true
	}
	for _, entry := range loadPoolRegistry() {
		entry = normalizePoolEntry(entry)
		if channelID > 0 && entry.ChannelID == channelID {
			return entry.Provider, true
		}
		if groupKey != "" && entry.GroupKey == groupKey {
			return entry.Provider, true
		}
	}
	return "", false
}

// GetPoolEntry returns a dynamic pool's full record (incl. channel id) by id.
func GetPoolEntry(id string) (PoolEntry, bool) {
	id = strings.TrimSpace(id)
	for _, e := range loadPoolRegistry() {
		if e.ID == id {
			return e, true
		}
	}
	return PoolEntry{}, false
}

// ListRegisteredPoolChannelIDs returns only routing-channel identifiers from
// the dynamic registry. It intentionally includes temporarily unconfigured
// entries (for example, a missing management secret during a rolling restart)
// and never exposes registry credentials to callers.
func ListRegisteredPoolChannelIDs() []int {
	entries := loadPoolRegistry()
	ids := make([]int, 0, len(entries))
	seen := make(map[int]struct{})
	for _, entry := range entries {
		if entry.ChannelID <= 0 {
			continue
		}
		if _, ok := seen[entry.ChannelID]; ok {
			continue
		}
		seen[entry.ChannelID] = struct{}{}
		ids = append(ids, entry.ChannelID)
	}
	return ids
}

// FindPrivatePoolByOwner returns the private pool owned by a user. The registry
// enforces one private pool per owner, so the first match is definitive.
func FindPrivatePoolByOwner(ownerUserID int) (PoolEntry, bool) {
	if ownerUserID <= 0 {
		return PoolEntry{}, false
	}
	for _, entry := range loadPoolRegistry() {
		if entry.Kind == PoolKindPrivate && entry.OwnerUserID == ownerUserID {
			return entry, true
		}
	}
	return PoolEntry{}, false
}

// FindPrivatePoolByGroupKey resolves the immutable routing key used by API
// tokens back to its owner-aware registry entry.
func FindPrivatePoolByGroupKey(groupKey string) (PoolEntry, bool) {
	groupKey = strings.TrimSpace(groupKey)
	if groupKey == "" {
		return PoolEntry{}, false
	}
	for _, entry := range loadPoolRegistry() {
		if entry.Kind == PoolKindPrivate && entry.GroupKey == groupKey {
			return entry, true
		}
	}
	return PoolEntry{}, false
}

// CanManagePool is the shared authorization rule for pool APIs. Admin is not a
// global-pool role: only root can manage every pool; other users can manage
// exactly the private pool they own.
func CanManagePool(actorUserID, actorRole int, entry PoolEntry) bool {
	if actorRole >= RoleRootUser {
		return true
	}
	entry = normalizePoolEntry(entry)
	return actorUserID > 0 && entry.Kind == PoolKindPrivate && entry.OwnerUserID == actorUserID
}

// CountPrivatePools reports how many user-owned pools exist, including entries
// whose management service is temporarily unavailable.
func CountPrivatePools() int {
	count := 0
	for _, entry := range loadPoolRegistry() {
		if entry.Kind == PoolKindPrivate {
			count++
		}
	}
	return count
}

// SetPoolChannelID records the new-api channel that routes a dynamic pool, so a
// later delete can remove it. No-op-safe: errors if the pool is unknown.
func SetPoolChannelID(id string, channelID int) error {
	poolRegMutationMu.Lock()
	defer poolRegMutationMu.Unlock()

	id = strings.TrimSpace(id)
	entries := append([]PoolEntry(nil), loadPoolRegistry()...)
	for i := range entries {
		if entries[i].ID == id {
			entries[i].ChannelID = channelID
			return SavePoolRegistry(entries)
		}
	}
	return fmt.Errorf("pool not found: %s", id)
}

// RemovePoolFromRegistry drops a dynamic pool by id.
func RemovePoolFromRegistry(id string) error {
	poolRegMutationMu.Lock()
	defer poolRegMutationMu.Unlock()

	id = strings.TrimSpace(id)
	entries := loadPoolRegistry()
	out := make([]PoolEntry, 0, len(entries))
	found := false
	for _, e := range entries {
		if e.ID == id {
			found = true
			continue
		}
		out = append(out, e)
	}
	if !found {
		return fmt.Errorf("pool not found: %s", id)
	}
	return SavePoolRegistry(out)
}

// AllocateNextPoolPort returns the next free management port above the highest
// currently in use — env default 8317 / k12 8318, plus any dynamic entries.
func AllocateNextPoolPort() int {
	highest := 8318 // k12
	for _, e := range loadPoolRegistry() {
		if e.Port > highest {
			highest = e.Port
		}
	}
	return highest + 1
}

// AllocateNextPoolID returns the next numeric pool id as a string. Pool ids are
// numeric and independent of the display label, so two pools may share a label
// (or be renamed freely) without their ids / containers / channels ever
// colliding. It scans existing numeric ids and returns max+1, starting at 1;
// non-numeric ids (the legacy main/k12-pool/test) are ignored.
func AllocateNextPoolID() string {
	highest := 0
	for _, e := range loadPoolRegistry() {
		if n, err := strconv.Atoi(strings.TrimSpace(e.ID)); err == nil && n > highest {
			highest = n
		}
	}
	return strconv.Itoa(highest + 1)
}

// SetPoolLabel updates a dynamic pool's display label in the registry. It errors
// if the pool id is not a registered dynamic pool.
func SetPoolLabel(id, label string) error {
	poolRegMutationMu.Lock()
	defer poolRegMutationMu.Unlock()

	id = strings.TrimSpace(id)
	entries := append([]PoolEntry(nil), loadPoolRegistry()...)
	for i := range entries {
		if entries[i].ID == id {
			entries[i].Label = strings.TrimSpace(label)
			return SavePoolRegistry(entries)
		}
	}
	return fmt.Errorf("pool not found: %s", id)
}

// SetPrivatePoolSettings updates owner-scoped automation settings without
// exposing or accepting management connection fields.
func SetPrivatePoolSettings(id string, ownerUserID int, settings PrivatePoolSettings) error {
	poolRegMutationMu.Lock()
	defer poolRegMutationMu.Unlock()

	id = strings.TrimSpace(id)
	if settings.AutoCleanHours <= 0 {
		settings.AutoCleanHours = 24
	}
	entries := append([]PoolEntry(nil), loadPoolRegistry()...)
	for i := range entries {
		entry := normalizePoolEntry(entries[i])
		if entry.ID != id || entry.Kind != PoolKindPrivate || entry.OwnerUserID != ownerUserID {
			continue
		}
		entries[i].AutoCleanEnabled = settings.AutoCleanEnabled
		entries[i].AutoCleanHours = settings.AutoCleanHours
		entries[i].UsageAutoRefreshEnabled = settings.UsageAutoRefreshEnabled
		entries[i].UsageAutoResetEnabled = settings.UsageAutoResetEnabled
		return SavePoolRegistry(entries)
	}
	return fmt.Errorf("private pool not found")
}
