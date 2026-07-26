package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
)

// xju-api:new — one-click pool creation (#4 Phase B, B2 host-helper side).
//
// new-api never touches the docker socket. To create an isolated cliproxy pool
// it drops a request file the host watcher (deploy/provision-poold.sh) picks up;
// the watcher provisions the container and writes a result file that carries the
// new pool's management URL/secret + relay key. new-api polls that result and,
// on success, registers the pool (common.AddPoolToRegistry). The contract:
//
//   POOL_PROVISION_DIR/requests/<id>.json   ← new-api writes owner-aware create metadata
//   POOL_PROVISION_DIR/results/<id>.json     → watcher echoes metadata + runtime secrets
//
// POOL_PROVISION_DIR unset → the feature is off and every call errors cleanly.

func provisionDir() string { return strings.TrimSpace(os.Getenv("POOL_PROVISION_DIR")) }

const MaxPrivatePools = 10

// provisionRequestMu serializes validation + id/port allocation + request
// creation. A single new-api instance is the writer in the supported topology;
// keeping the whole operation under one lock prevents simultaneous creates from
// receiving the same identity or port.
var provisionRequestMu sync.Mutex

// allocatePoolID picks the next numeric pool id, considering both registered
// pools (common.AllocateNextPoolID) and requests still in flight in the provision
// dir (requests/processed/results). Without the dir scan, two creates fired
// before the first finishes registering would both land on the same id.
func allocatePoolID(dir string) string {
	next, _ := strconv.Atoi(common.AllocateNextPoolID()) // registry max + 1 (>= 1)
	for _, sub := range []string{"requests", "processed", "results"} {
		entries, err := os.ReadDir(filepath.Join(dir, sub))
		if err != nil {
			continue
		}
		for _, e := range entries {
			// processed files are named <id>.<unix-ts>.json; take the id segment.
			base := strings.SplitN(strings.TrimSuffix(e.Name(), ".json"), ".", 2)[0]
			if n, err := strconv.Atoi(base); err == nil && n+1 > next {
				next = n + 1
			}
		}
	}
	return strconv.Itoa(next)
}

func allocatePoolPort(dir string) int {
	highest := common.AllocateNextPoolPort() - 1
	for _, sub := range []string{"requests", "processed", "results"} {
		entries, err := os.ReadDir(filepath.Join(dir, sub))
		if err != nil {
			continue
		}
		for _, entry := range entries {
			data, err := os.ReadFile(filepath.Join(dir, sub, entry.Name()))
			if err != nil {
				continue
			}
			var metadata struct {
				Port int `json:"port"`
			}
			if common.Unmarshal(data, &metadata) == nil && metadata.Port > highest {
				highest = metadata.Port
			}
		}
	}
	return highest + 1
}

// ProvisionEnabled reports whether one-click pool creation is wired up.
func ProvisionEnabled() bool { return provisionDir() != "" }

// normalizeBuildMode maps any input to the two supported modes, defaulting
// unknown/empty values to "cliproxy".
func normalizeBuildMode(mode string) string {
	if strings.EqualFold(strings.TrimSpace(mode), "gopool") {
		return "gopool"
	}
	return "cliproxy"
}

type provisionRequest struct {
	Action      string `json:"action"`
	PoolID      string `json:"pool_id"`
	Label       string `json:"label,omitempty"`
	Port        int    `json:"port,omitempty"`
	Provider    string `json:"provider,omitempty"`
	Mode        string `json:"mode,omitempty"`
	OwnerUserID int    `json:"owner_user_id,omitempty"`
	Kind        string `json:"kind,omitempty"`
	GroupKey    string `json:"group_key,omitempty"`
}

type provisionResult struct {
	PoolID      string `json:"pool_id"`
	Label       string `json:"label"`
	Action      string `json:"action"`
	Status      string `json:"status"`
	MgmtURL     string `json:"mgmt_url"`
	MgmtSecret  string `json:"mgmt_secret"`
	Port        int    `json:"port"`
	InternalKey string `json:"internal_key"`
	Error       string `json:"error"`
	Provider    string `json:"provider,omitempty"`
	Mode        string `json:"mode,omitempty"`
	OwnerUserID int    `json:"owner_user_id,omitempty"`
	Kind        string `json:"kind,omitempty"`
	GroupKey    string `json:"group_key,omitempty"`
}

type PrivatePoolProvisionState struct {
	PoolID string `json:"pool_id,omitempty"`
	Label  string `json:"label,omitempty"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

func pendingPrivateProvisions(dir string) map[string]provisionRequest {
	pending := make(map[string]provisionRequest)
	read := func(sub string, onlySuccessfulResults bool) {
		entries, err := os.ReadDir(filepath.Join(dir, sub))
		if err != nil {
			return
		}
		for _, entry := range entries {
			data, err := os.ReadFile(filepath.Join(dir, sub, entry.Name()))
			if err != nil {
				continue
			}
			var raw provisionResult
			if common.Unmarshal(data, &raw) != nil || raw.Action != "create" || raw.Kind != common.PoolKindPrivate || raw.OwnerUserID <= 0 {
				continue
			}
			if onlySuccessfulResults && raw.Status != "ok" {
				continue
			}
			if _, registered := common.GetPoolEntry(raw.PoolID); registered {
				continue
			}
			pending[raw.PoolID] = provisionRequest{
				Action:      raw.Action,
				PoolID:      raw.PoolID,
				Label:       raw.Label,
				Port:        raw.Port,
				Provider:    raw.Provider,
				Mode:        raw.Mode,
				OwnerUserID: raw.OwnerUserID,
				Kind:        raw.Kind,
				GroupKey:    raw.GroupKey,
			}
		}
	}
	read("requests", false)
	// A successful result may be waiting for new-api to poll and register it
	// after the watcher has already moved the original request to processed/.
	read("results", true)
	return pending
}

// GetPrivatePoolProvisionState resolves a user's private pool without accepting
// a client-supplied pool id. It also finalizes a successful watcher result, so
// polling this endpoint is sufficient to move provisioning to ready.
func GetPrivatePoolProvisionState(ownerUserID int) (PrivatePoolProvisionState, error) {
	if ownerUserID <= 0 {
		return PrivatePoolProvisionState{}, fmt.Errorf("private pool owner is required")
	}
	if entry, ok := common.FindPrivatePoolByOwner(ownerUserID); ok {
		return PrivatePoolProvisionState{PoolID: entry.ID, Label: entry.Label, Status: "ready"}, nil
	}
	dir := provisionDir()
	if dir == "" {
		return PrivatePoolProvisionState{Status: "none"}, nil
	}

	candidates := make(map[string]provisionResult)
	read := func(sub string) {
		entries, err := os.ReadDir(filepath.Join(dir, sub))
		if err != nil {
			return
		}
		for _, entry := range entries {
			data, err := os.ReadFile(filepath.Join(dir, sub, entry.Name()))
			if err != nil {
				continue
			}
			var result provisionResult
			if common.Unmarshal(data, &result) != nil || result.Action != "create" || result.Kind != common.PoolKindPrivate || result.OwnerUserID != ownerUserID {
				continue
			}
			candidates[result.PoolID] = result
		}
	}
	read("requests")
	// Results override requests for the same pool id.
	read("results")

	var selected provisionResult
	selectedNumber := -1
	for _, candidate := range candidates {
		number, err := strconv.Atoi(candidate.PoolID)
		if err != nil {
			continue
		}
		if number > selectedNumber {
			selected = candidate
			selectedNumber = number
		}
	}
	if selectedNumber < 0 {
		return PrivatePoolProvisionState{Status: "none"}, nil
	}
	state := PrivatePoolProvisionState{PoolID: selected.PoolID, Label: selected.Label, Status: "provisioning"}
	if selected.Status == "error" {
		state.Status = "error"
		state.Error = selected.Error
		return state, nil
	}
	if selected.Status != "ok" {
		return state, nil
	}
	status, err := PollPrivatePoolProvision(selected.PoolID, ownerUserID)
	if err != nil {
		if status == "error" {
			state.Status = "error"
			state.Error = err.Error()
			return state, nil
		}
		return state, err
	}
	state.Status = status
	return state, nil
}

// RequestPoolProvision allocates a numeric pool id + management port and drops a
// create request for the host watcher. Returns the new pool id. The id is numeric
// and independent of the label, so two pools can share a display name (or be
// renamed freely) without their ids / containers / channels ever colliding.
func RequestPoolProvision(label, mode string) (string, error) {
	return RequestPoolProvisionForProvider(label, common.PoolProviderCodex, mode)
}

// RequestPoolProvisionForProvider starts a shared pool with an explicit
// upstream account provider. Provider and build mode are orthogonal except that
// go-pool currently supports Codex accounts only.
func RequestPoolProvisionForProvider(label, provider, mode string) (string, error) {
	return requestPoolProvision(label, provider, mode, 0, common.PoolKindAdmin)
}

// RequestPrivatePoolProvision starts a user-owned isolated pool. The build mode
// and routing key are server-controlled; ordinary users cannot choose a shared
// routing group or provision more than one pool.
func RequestPrivatePoolProvision(label string, ownerUserID int) (string, error) {
	if ownerUserID <= 0 {
		return "", fmt.Errorf("private pool owner is required")
	}
	return requestPoolProvision(label, common.PoolProviderCodex, "cliproxy", ownerUserID, common.PoolKindPrivate)
}

func validatePoolBuild(provider, mode string) (string, string, error) {
	rawProvider := strings.ToLower(strings.TrimSpace(provider))
	if rawProvider == "" {
		rawProvider = common.PoolProviderCodex
	}
	if !common.IsSupportedPoolProvider(rawProvider) {
		return "", "", fmt.Errorf("unsupported pool provider: %s", rawProvider)
	}
	rawMode := strings.ToLower(strings.TrimSpace(mode))
	if rawMode == "" {
		rawMode = "cliproxy"
	}
	if rawMode != "cliproxy" && rawMode != "gopool" {
		return "", "", fmt.Errorf("unsupported pool build mode: %s", rawMode)
	}
	if rawProvider == common.PoolProviderClaude && rawMode != "cliproxy" {
		return "", "", fmt.Errorf("Claude pools support CPA mode only")
	}
	return rawProvider, rawMode, nil
}

func requestPoolProvision(label, provider, mode string, ownerUserID int, kind string) (string, error) {
	dir := provisionDir()
	if dir == "" {
		return "", fmt.Errorf("pool provisioning is not enabled")
	}
	label = strings.TrimSpace(label)
	if label == "" {
		return "", fmt.Errorf("pool name is required")
	}
	provider, mode, err := validatePoolBuild(provider, mode)
	if err != nil {
		return "", err
	}
	if kind == common.PoolKindPrivate && provider != common.PoolProviderCodex {
		return "", fmt.Errorf("private pools support provider %q only", common.PoolProviderCodex)
	}
	provisionRequestMu.Lock()
	defer provisionRequestMu.Unlock()

	groupKey := ""
	if kind == common.PoolKindPrivate {
		if _, exists := common.FindPrivatePoolByOwner(ownerUserID); exists {
			return "", fmt.Errorf("user %d already has a private pool", ownerUserID)
		}
		pending := pendingPrivateProvisions(dir)
		for _, request := range pending {
			if request.OwnerUserID == ownerUserID {
				return "", fmt.Errorf("user %d already has a private pool provisioning", ownerUserID)
			}
		}
		if common.CountPrivatePools()+len(pending) >= MaxPrivatePools {
			return "", fmt.Errorf("private pool limit reached: %d", MaxPrivatePools)
		}
	}
	id := allocatePoolID(dir)
	if kind == common.PoolKindPrivate {
		groupKey = common.PrivatePoolGroupKey(ownerUserID)
	} else {
		groupKey = id
	}
	req := provisionRequest{
		Action:      "create",
		PoolID:      id,
		Label:       label,
		Port:        allocatePoolPort(dir),
		Provider:    provider,
		Mode:        mode,
		OwnerUserID: ownerUserID,
		Kind:        kind,
		GroupKey:    groupKey,
	}
	if err := writeProvisionRequest(dir, id, req); err != nil {
		return "", err
	}
	return id, nil
}

// RenamePool changes a dynamic pool's display label and channel name. New pools
// keep their immutable GroupKey; only legacy registry entries without one use
// the historical group/card migration behavior.
func RenamePool(poolID, newLabel string) error {
	poolID = strings.TrimSpace(poolID)
	newLabel = strings.TrimSpace(newLabel)
	if newLabel == "" {
		return fmt.Errorf("new pool name is required")
	}
	if common.IsReservedPoolID(poolID) {
		return fmt.Errorf("cannot rename a built-in pool: %s", poolID)
	}
	entry, ok := common.GetPoolEntry(poolID)
	if !ok {
		return fmt.Errorf("pool not found: %s", poolID)
	}
	// Update routing metadata first; only persist the registry label once that
	// succeeds, so a conflict leaves everything consistent.
	if err := renamePoolGroup(
		poolID, entry.ChannelID, newLabel, entry.GroupKey, entry.Kind != common.PoolKindPrivate,
	); err != nil {
		return err
	}
	return common.SetPoolLabel(poolID, newLabel)
}

// PollPoolProvision reports a create's status: "ready" once the pool is
// registered, "provisioning" while the watcher works, or an error. It is
// idempotent — the first poll that sees a successful result registers the pool.
func PollPoolProvision(poolID string) (string, error) {
	return pollPoolProvision(poolID, 0)
}

// PollPrivatePoolProvision is the owner-bound variant used by ordinary-user
// APIs. It refuses results whose echoed ownership metadata does not match.
func PollPrivatePoolProvision(poolID string, ownerUserID int) (string, error) {
	if ownerUserID <= 0 {
		return "", fmt.Errorf("private pool owner is required")
	}
	return pollPoolProvision(poolID, ownerUserID)
}

func pollPoolProvision(poolID string, expectedOwnerUserID int) (string, error) {
	dir := provisionDir()
	if dir == "" {
		return "", fmt.Errorf("pool provisioning is not enabled")
	}
	poolID = strings.TrimSpace(poolID)
	if _, _, ok := common.ResolvePoolMgmt(poolID); ok {
		entry, found := common.GetPoolEntry(poolID)
		if expectedOwnerUserID > 0 {
			if !found || !common.CanManagePool(expectedOwnerUserID, common.RoleCommonUser, entry) {
				return "", fmt.Errorf("private pool ownership mismatch")
			}
		}
		if found && entry.ChannelID != 0 {
			return "ready", nil
		}
		// A previous poll may have registered the pool before channel creation
		// succeeded. Continue through the durable watcher result so the missing
		// channel is retried instead of reporting a permanently unrouted pool.
	}
	data, err := os.ReadFile(filepath.Join(dir, "results", poolID+".json"))
	if err != nil {
		return "provisioning", nil // no result yet
	}
	var r provisionResult
	if err := common.Unmarshal(data, &r); err != nil {
		return "", fmt.Errorf("unreadable provisioning result: %w", err)
	}
	if strings.TrimSpace(r.PoolID) != poolID {
		return "", fmt.Errorf("provisioning result pool id mismatch")
	}
	if r.Action != "" && r.Action != "create" {
		return "", fmt.Errorf("unexpected provisioning result action: %s", r.Action)
	}
	if r.Status != "ok" {
		msg := r.Error
		if msg == "" {
			msg = "provisioning failed"
		}
		return "error", fmt.Errorf("%s", msg)
	}
	kind := strings.ToLower(strings.TrimSpace(r.Kind))
	if kind == "" {
		kind = common.PoolKindAdmin // compatibility with pre-owner watcher results
	}
	if kind != common.PoolKindAdmin && kind != common.PoolKindPrivate {
		return "", fmt.Errorf("invalid provisioning pool kind: %s", kind)
	}
	if expectedOwnerUserID > 0 && (kind != common.PoolKindPrivate || r.OwnerUserID != expectedOwnerUserID) {
		return "", fmt.Errorf("private pool ownership metadata mismatch")
	}
	if kind == common.PoolKindPrivate {
		expectedGroupKey := common.PrivatePoolGroupKey(r.OwnerUserID)
		if r.OwnerUserID <= 0 || strings.TrimSpace(r.GroupKey) != expectedGroupKey {
			return "", fmt.Errorf("invalid private pool ownership metadata")
		}
	} else {
		if strings.TrimSpace(r.GroupKey) == "" {
			r.GroupKey = poolID // compatibility with pre-group-key watcher results
		}
		if r.OwnerUserID != 0 || r.GroupKey != poolID {
			return "", fmt.Errorf("invalid admin pool ownership metadata")
		}
	}
	label := strings.TrimSpace(r.Label)
	if label == "" {
		label = poolID
	}
	provider, mode, err := validatePoolBuild(r.Provider, r.Mode)
	if err != nil {
		return "", fmt.Errorf("invalid provisioning pool configuration: %w", err)
	}
	if kind == common.PoolKindPrivate && provider != common.PoolProviderCodex {
		return "", fmt.Errorf("private pools support provider %q only", common.PoolProviderCodex)
	}
	if err := common.AddPoolToRegistry(common.PoolEntry{
		ID:          r.PoolID,
		Label:       label,
		MgmtURL:     r.MgmtURL,
		MgmtSecret:  r.MgmtSecret,
		Port:        r.Port,
		Provider:    provider,
		BuildMode:   mode,
		OwnerUserID: r.OwnerUserID,
		Kind:        kind,
		GroupKey:    r.GroupKey,
	}); err != nil {
		// A concurrent or earlier poll may have registered it first. Keep going
		// so channel creation can be retried from the durable watcher result.
		if _, _, ok := common.ResolvePoolMgmt(poolID); !ok {
			return "", err
		}
	}
	// Phase C: route the pool's group to its cliproxy instance. The mgmt URL and
	// the relay base URL are the same host:port. A channel failure leaves the
	// pool registered (importable/verifiable) but unrouted — log, don't unwind.
	if chID, err := createPoolChannel(
		r.PoolID, r.InternalKey, r.MgmtURL, r.MgmtSecret, label, r.GroupKey, provider, kind != common.PoolKindPrivate,
	); err != nil {
		common.SysError("pool " + r.PoolID + " registered but channel creation failed: " + err.Error())
	} else {
		_ = common.SetPoolChannelID(r.PoolID, chID)
	}
	return "ready", nil
}

// DeletePoolInstance tears down a dynamic pool: it asks the host watcher to
// remove the container, deletes the routing channel + group options, and drops
// the pool from the registry. Built-in pools (default/k12) are refused.
func DeletePoolInstance(poolID string) error {
	dir := provisionDir()
	if dir == "" {
		return fmt.Errorf("pool provisioning is not enabled")
	}
	poolID = strings.TrimSpace(poolID)
	if common.IsReservedPoolID(poolID) {
		return fmt.Errorf("cannot delete a built-in pool: %s", poolID)
	}
	entry, ok := common.GetPoolEntry(poolID)
	if !ok {
		return fmt.Errorf("pool not found: %s", poolID)
	}
	if err := writeProvisionRequest(dir, poolID, map[string]any{"action": "delete", "pool_id": poolID}); err != nil {
		return err
	}
	deletePoolChannel(poolID, entry.GroupKey, entry.ChannelID)
	return common.RemovePoolFromRegistry(poolID)
}

func writeProvisionRequest(dir, id string, req any) error {
	reqDir := filepath.Join(dir, "requests")
	if err := os.MkdirAll(reqDir, 0o755); err != nil {
		return err
	}
	data, err := common.Marshal(req)
	if err != nil {
		return err
	}
	tmp := filepath.Join(reqDir, id+".json.tmp")
	// 0644: the request carries no secrets and the host watcher (a different
	// user) must be able to read it.
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(reqDir, id+".json"))
}
