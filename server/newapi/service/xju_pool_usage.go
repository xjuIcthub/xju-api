package service

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/google/uuid"
)

// xju-api:new — per-account quota (额度) for the account pool.
//
// ChatGPT accounts meter usage in two rolling windows (5-hour + weekly) and
// Plus/Pro accounts occasionally receive "rate limit reset credits" that renew
// those windows on demand. The channels page already surfaces this for direct
// codex channels; this service brings the same data to pooled accounts by
// pinning wham requests to each account through cliproxy's management api-call
// (`$TOKEN$` substitution), the same vehicle the verify probe uses.
//
// Snapshots live in memory on the master node: the pool page reads the cache,
// a manual refresh (single account or whole pool) updates it, and an opt-in
// hourly task keeps it warm. A second opt-in lets the refresh consume one reset
// credit automatically when it finds an account exhausted — "自动更新额度".

const (
	codexWhamUsageURL        = "https://chatgpt.com/backend-api/wham/usage"
	codexWhamResetConsumeURL = "https://chatgpt.com/backend-api/wham/rate-limit-reset-credits/consume"
)

// PoolAccountUsage is one account's quota snapshot, safe to surface to the
// operator — it never carries the credential, only usage numbers.
type PoolAccountUsage struct {
	Name      string `json:"name"`
	AuthIndex string `json:"auth_index"`
	Plan      string `json:"plan,omitempty"`
	// Window percentages are pointers so "unknown" and "0% used" stay distinct.
	FiveHourUsedPercent *float64 `json:"five_hour_used_percent,omitempty"`
	FiveHourResetAt     int64    `json:"five_hour_reset_at,omitempty"`
	WeeklyUsedPercent   *float64 `json:"weekly_used_percent,omitempty"`
	WeeklyResetAt       int64    `json:"weekly_reset_at,omitempty"`
	LimitReached        bool     `json:"limit_reached"`
	ResetCredits        *int     `json:"reset_credits,omitempty"`
	FetchedAt           int64    `json:"fetched_at"`
	Error               string   `json:"error,omitempty"`
}

// exhausted reports whether the account has no quota left in some window —
// the condition the opt-in auto-reset acts on.
func (u PoolAccountUsage) exhausted() bool {
	if u.LimitReached {
		return true
	}
	for _, p := range []*float64{u.FiveHourUsedPercent, u.WeeklyUsedPercent} {
		if p != nil && *p >= 100 {
			return true
		}
	}
	return false
}

// whamRateLimitWindow mirrors the wham usage payload's window objects.
type whamRateLimitWindow struct {
	UsedPercent        *float64 `json:"used_percent"`
	ResetAt            int64    `json:"reset_at"`
	LimitWindowSeconds int64    `json:"limit_window_seconds"`
}

type whamUsagePayload struct {
	PlanType  string `json:"plan_type"`
	RateLimit struct {
		LimitReached    bool                 `json:"limit_reached"`
		PlanType        string               `json:"plan_type"`
		PrimaryWindow   *whamRateLimitWindow `json:"primary_window"`
		SecondaryWindow *whamRateLimitWindow `json:"secondary_window"`
	} `json:"rate_limit"`
	ResetCredits struct {
		AvailableCount *int `json:"available_count"`
	} `json:"rate_limit_reset_credits"`
}

// fetchPoolAccountUsage pins a wham usage GET to the account and folds the
// response into a snapshot. Any failure lands in .Error so a bad account can't
// abort a whole-pool refresh.
func fetchPoolAccountUsage(baseURL, secret string, e poolAuthEntry) PoolAccountUsage {
	u := PoolAccountUsage{Name: e.Name, AuthIndex: e.AuthIndex, FetchedAt: time.Now().Unix()}
	accountID := strings.TrimSpace(e.IDToken.AccountID)
	if accountID == "" {
		u.Error = "account has no chatgpt_account_id"
		return u
	}
	code, body := poolCredentialAPICall(baseURL, secret, e.AuthIndex, http.MethodGet, codexWhamUsageURL, poolWhamHeaders(accountID), "")
	if code == 0 {
		u.Error = "pool management unreachable"
		return u
	}
	if code < 200 || code >= 300 {
		u.Error = fmt.Sprintf("upstream HTTP %d", code)
		return u
	}
	var parsed whamUsagePayload
	if err := common.UnmarshalJsonStr(body, &parsed); err != nil {
		u.Error = "unparsable usage payload"
		return u
	}
	u.Plan = parsed.PlanType
	if u.Plan == "" {
		u.Plan = parsed.RateLimit.PlanType
	}
	u.LimitReached = parsed.RateLimit.LimitReached
	u.ResetCredits = parsed.ResetCredits.AvailableCount
	for _, w := range []*whamRateLimitWindow{parsed.RateLimit.PrimaryWindow, parsed.RateLimit.SecondaryWindow} {
		if w == nil || w.UsedPercent == nil {
			continue
		}
		// A window spanning a day or more is the weekly meter; shorter is the
		// 5-hour meter (mirrors the channels usage dialog's classification).
		if w.LimitWindowSeconds >= 24*60*60 {
			if u.WeeklyUsedPercent == nil {
				u.WeeklyUsedPercent = w.UsedPercent
				u.WeeklyResetAt = w.ResetAt
			}
		} else if u.FiveHourUsedPercent == nil {
			u.FiveHourUsedPercent = w.UsedPercent
			u.FiveHourResetAt = w.ResetAt
		}
	}
	return u
}

func poolWhamHeaders(accountID string) map[string]string {
	return map[string]string{
		"chatgpt-account-id": accountID,
		"Accept":             "application/json",
		"originator":         codexProbeOriginator,
		"User-Agent":         codexProbeUserAgent,
	}
}

// consumePoolAccountReset spends one reset credit on the account. Returns the
// upstream status code and body (for error surfacing).
func consumePoolAccountReset(baseURL, secret, authIndex, accountID string) (int, string, error) {
	if strings.TrimSpace(accountID) == "" {
		return 0, "", fmt.Errorf("account has no chatgpt_account_id")
	}
	payload, err := common.Marshal(map[string]string{"redeem_request_id": uuid.NewString()})
	if err != nil {
		return 0, "", err
	}
	headers := poolWhamHeaders(accountID)
	headers["Content-Type"] = "application/json"
	code, body := poolCredentialAPICall(baseURL, secret, authIndex, http.MethodPost, codexWhamResetConsumeURL, headers, string(payload))
	if code == 0 {
		return 0, "", fmt.Errorf("pool management unreachable")
	}
	return code, body, nil
}

// ---------------------------------------------------------------------------
// Snapshot cache — the pool page reads this; refreshes write it.

type poolUsageState struct {
	mu     sync.Mutex
	byName map[string]PoolAccountUsage
}

var poolUsageCache sync.Map // poolID -> *poolUsageState

func poolUsageStateFor(poolID string) *poolUsageState {
	stateAny, _ := poolUsageCache.LoadOrStore(poolID, &poolUsageState{byName: map[string]PoolAccountUsage{}})
	return stateAny.(*poolUsageState)
}

func storePoolAccountUsage(poolID string, u PoolAccountUsage) {
	state := poolUsageStateFor(poolID)
	state.mu.Lock()
	state.byName[u.Name] = u
	state.mu.Unlock()
}

// GetPoolUsageSnapshots returns the cached quota snapshots for a pool, keyed by
// account file name (the same key the auth-files list uses).
func GetPoolUsageSnapshots(poolID string) map[string]PoolAccountUsage {
	state := poolUsageStateFor(poolID)
	state.mu.Lock()
	defer state.mu.Unlock()
	out := make(map[string]PoolAccountUsage, len(state.byName))
	for k, v := range state.byName {
		out[k] = v
	}
	return out
}

// RefreshPoolAccountUsageByName refreshes one account's quota on demand and
// returns the fresh snapshot.
func RefreshPoolAccountUsageByName(poolID, name string) (PoolAccountUsage, error) {
	if err := EnsurePoolProvider(poolID, common.PoolProviderCodex, "ChatGPT account quota"); err != nil {
		return PoolAccountUsage{}, err
	}
	baseURL, secret, ok := common.ResolvePoolMgmt(poolID)
	if !ok {
		return PoolAccountUsage{}, fmt.Errorf("pool management is not configured for pool: %s", poolID)
	}
	entries, err := listPoolEntries(baseURL, secret)
	if err != nil {
		return PoolAccountUsage{}, err
	}
	for _, e := range entries {
		if e.Name == name {
			u := fetchPoolAccountUsage(baseURL, secret, e)
			storePoolAccountUsage(poolID, u)
			return u, nil
		}
	}
	return PoolAccountUsage{}, fmt.Errorf("account not found in pool: %s", name)
}

// ResetPoolAccountQuota consumes one reset credit on the account, then
// refreshes and returns its quota snapshot so the UI shows the renewed windows.
func ResetPoolAccountQuota(poolID, name string) (PoolAccountUsage, error) {
	if err := EnsurePoolProvider(poolID, common.PoolProviderCodex, "ChatGPT quota reset"); err != nil {
		return PoolAccountUsage{}, err
	}
	baseURL, secret, ok := common.ResolvePoolMgmt(poolID)
	if !ok {
		return PoolAccountUsage{}, fmt.Errorf("pool management is not configured for pool: %s", poolID)
	}
	entries, err := listPoolEntries(baseURL, secret)
	if err != nil {
		return PoolAccountUsage{}, err
	}
	for _, e := range entries {
		if e.Name != name {
			continue
		}
		code, body, err := consumePoolAccountReset(baseURL, secret, e.AuthIndex, e.IDToken.AccountID)
		if err != nil {
			return PoolAccountUsage{}, err
		}
		if code < 200 || code >= 300 {
			return PoolAccountUsage{}, fmt.Errorf("reset rejected: %s", poolResetErrorMessage(body, code))
		}
		u := fetchPoolAccountUsage(baseURL, secret, e)
		storePoolAccountUsage(poolID, u)
		return u, nil
	}
	return PoolAccountUsage{}, fmt.Errorf("account not found in pool: %s", name)
}

func poolResetErrorMessage(body string, code int) string {
	var parsed struct {
		Detail string `json:"detail"`
		Error  struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := common.UnmarshalJsonStr(body, &parsed); err == nil {
		if parsed.Error.Message != "" {
			return parsed.Error.Message
		}
		if parsed.Detail != "" {
			return parsed.Detail
		}
	}
	return fmt.Sprintf("upstream HTTP %d", code)
}

// ---------------------------------------------------------------------------
// Whole-pool refresh job (single-node, IsMasterNode-guarded), same shape as the
// verify-all job: too slow to hold an HTTP request on a 500-account pool, so it
// runs in the background and the frontend polls.

type PoolUsageJobSnapshot struct {
	Running    bool  `json:"running"`
	Total      int   `json:"total"`
	Done       int   `json:"done"`
	StartedAt  int64 `json:"started_at"`
	FinishedAt int64 `json:"finished_at"`
	AutoReset  bool  `json:"auto_reset"`
	// OnlyExhausted marks a targeted run: accounts whose cached snapshot still
	// shows quota left were skipped (counted in Skipped) instead of re-fetched.
	OnlyExhausted bool   `json:"only_exhausted"`
	Skipped       int    `json:"skipped"`
	Resets        int    `json:"resets"`
	Errors        int    `json:"errors"`
	Error         string `json:"error"`
}

type poolUsageJob struct {
	mu   sync.Mutex
	snap PoolUsageJobSnapshot
}

var poolUsageJobs sync.Map // poolID -> *poolUsageJob

// StartPoolUsageRefreshJob refreshes pool accounts' quota in the background.
// With onlyExhausted (the manual "refresh all" button), accounts whose cached
// snapshot still shows quota left are skipped — the button exists to find out
// which depleted accounts recovered, not to re-poll healthy ones. Accounts
// with no snapshot yet, a fetch error, or an active cooldown are always
// fetched. With autoReset, an exhausted account holding a reset credit gets
// one consumed and its windows re-fetched.
func StartPoolUsageRefreshJob(poolID string, autoReset, onlyExhausted bool) (PoolUsageJobSnapshot, error) {
	if !common.IsMasterNode {
		return PoolUsageJobSnapshot{}, fmt.Errorf("quota refresh runs on the master node only")
	}
	if err := EnsurePoolProvider(poolID, common.PoolProviderCodex, "ChatGPT account quota"); err != nil {
		return PoolUsageJobSnapshot{}, err
	}
	baseURL, secret, ok := common.ResolvePoolMgmt(poolID)
	if !ok {
		return PoolUsageJobSnapshot{}, fmt.Errorf("pool management is not configured for pool: %s", poolID)
	}

	jobAny, _ := poolUsageJobs.LoadOrStore(poolID, &poolUsageJob{})
	job := jobAny.(*poolUsageJob)

	job.mu.Lock()
	if job.snap.Running {
		snap := job.snap
		job.mu.Unlock()
		return snap, fmt.Errorf("a quota refresh is already running for pool: %s", poolID)
	}
	job.snap = PoolUsageJobSnapshot{Running: true, AutoReset: autoReset, OnlyExhausted: onlyExhausted, StartedAt: time.Now().Unix()}
	job.mu.Unlock()

	gopool.Go(func() { runPoolUsageRefreshJob(job, poolID, baseURL, secret, autoReset, onlyExhausted) })
	return job.Snapshot(), nil
}

// needsQuotaFetch decides whether a targeted (onlyExhausted) run must re-fetch
// this account: unknown or failed snapshots and cooldown-flagged entries are
// fetched; a snapshot that still shows quota left is skipped.
func needsQuotaFetch(e poolAuthEntry, cached PoolAccountUsage, hasCached bool) bool {
	if !hasCached || cached.Error != "" {
		return true
	}
	// An active cooldown usually means live traffic just hit a 429 — the cached
	// percentages predate that, so re-check regardless of what they say.
	if e.Unavailable {
		return true
	}
	return cached.exhausted()
}

func runPoolUsageRefreshJob(job *poolUsageJob, poolID, baseURL, secret string, autoReset, onlyExhausted bool) {
	entries, err := listPoolEntries(baseURL, secret)
	if err != nil {
		job.finish(err)
		return
	}
	state := poolUsageStateFor(poolID)
	targets := make([]poolAuthEntry, 0, len(entries))
	skipped := 0
	for _, e := range entries {
		if e.Disabled {
			continue
		}
		if onlyExhausted {
			state.mu.Lock()
			cached, hasCached := state.byName[e.Name]
			state.mu.Unlock()
			if !needsQuotaFetch(e, cached, hasCached) {
				skipped++
				continue
			}
		}
		targets = append(targets, e)
	}

	job.mu.Lock()
	job.snap.Total = len(targets)
	job.snap.Skipped = skipped
	job.mu.Unlock()

	sem := make(chan struct{}, ProbePoolConcurrency)
	var wg sync.WaitGroup
	for _, entry := range targets {
		wg.Add(1)
		sem <- struct{}{}
		go func(e poolAuthEntry) {
			defer wg.Done()
			defer func() { <-sem }()
			u := fetchPoolAccountUsage(baseURL, secret, e)
			didReset := false
			if autoReset && u.Error == "" && u.exhausted() && u.ResetCredits != nil && *u.ResetCredits > 0 {
				code, body, err := consumePoolAccountReset(baseURL, secret, e.AuthIndex, e.IDToken.AccountID)
				if err == nil && code >= 200 && code < 300 {
					didReset = true
					u = fetchPoolAccountUsage(baseURL, secret, e)
					common.SysLog(fmt.Sprintf("pool quota auto-reset: consumed a reset credit for %s in pool %s", e.Name, poolID))
				} else if err != nil {
					common.SysError("pool quota auto-reset failed for " + e.Name + ": " + err.Error())
				} else {
					common.SysError(fmt.Sprintf("pool quota auto-reset rejected for %s: %s", e.Name, poolResetErrorMessage(body, code)))
				}
			}
			storePoolAccountUsage(poolID, u)
			job.record(u.Error != "", didReset)
		}(entry)
	}
	wg.Wait()
	job.finish(nil)
}

func (j *poolUsageJob) record(failed, didReset bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.snap.Done++
	if failed {
		j.snap.Errors++
	}
	if didReset {
		j.snap.Resets++
	}
}

func (j *poolUsageJob) finish(err error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.snap.Running = false
	j.snap.FinishedAt = time.Now().Unix()
	if err != nil {
		j.snap.Error = err.Error()
	}
}

func (j *poolUsageJob) Snapshot() PoolUsageJobSnapshot {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.snap
}

// GetPoolUsageJob returns the latest quota-refresh snapshot for a pool, or
// ok=false if none has ever run.
func GetPoolUsageJob(poolID string) (PoolUsageJobSnapshot, bool) {
	jobAny, ok := poolUsageJobs.Load(poolID)
	if !ok {
		return PoolUsageJobSnapshot{}, false
	}
	return jobAny.(*poolUsageJob).Snapshot(), true
}

// ---------------------------------------------------------------------------
// Hourly auto-refresh (opt-in via PoolUsageAutoRefreshEnabled). Each tick
// re-checks the toggles so they take effect without a restart, mirroring the
// auto-clean task.

var poolUsageAutoRefreshOnce sync.Once

const poolUsageRefreshInterval = time.Hour

func StartPoolUsageAutoRefreshTask() {
	poolUsageAutoRefreshOnce.Do(func() {
		gopool.Go(func() {
			ticker := time.NewTicker(poolUsageRefreshInterval)
			defer ticker.Stop()
			for range ticker.C {
				runPoolUsageAutoRefreshOnce()
			}
		})
	})
}

func runPoolUsageAutoRefreshOnce() {
	if !common.IsMasterNode {
		return
	}
	for _, pool := range common.ListConfiguredPools() {
		if common.NormalizePoolProvider(pool.Provider) != common.PoolProviderCodex {
			continue
		}
		autoReset := common.PoolUsageAutoResetEnabled
		if pool.Kind == common.PoolKindPrivate {
			if !pool.UsageAutoRefreshEnabled {
				continue
			}
			autoReset = pool.UsageAutoResetEnabled
		} else if !common.PoolUsageAutoRefreshEnabled {
			continue
		}
		// The hourly pass is a full sweep — it is what keeps every account's
		// percentages fresh; only the manual button narrows to exhausted accounts.
		if _, err := StartPoolUsageRefreshJob(pool.ID, autoReset, false); err != nil {
			// An in-flight manual refresh is fine — skip quietly; real failures log.
			if !strings.Contains(err.Error(), "already running") {
				common.SysError("pool quota auto-refresh failed for pool " + pool.ID + ": " + err.Error())
			}
		}
	}
}
