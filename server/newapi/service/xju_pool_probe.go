package service

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/bytedance/gopkg/util/gopool"
)

// xju-api:new — active account-pool verification (号池验活 Part A).
//
// cliproxy has no active health check: `unavailable` only means "a cooldown is
// running", set passively when live traffic happens to hit a bad account. This
// service verifies each account on demand by pinning a probe request to it via
// cliproxy's management api-call, which injects that account's own credential
// (and proxy) and returns the upstream status.
//
// Probe endpoint (empirically confirmed against production):
//   - light: GET https://chatgpt.com/backend-api/codex/responses
//     GET on a POST-only endpoint is method-checked AFTER auth, so a valid
//     token returns 405 (Method Not Allowed) while a dead token returns 401 —
//     a zero-quota, Cloudflare-immune liveness signal.
//   - heavy: POST the same endpoint with a minimal 1-token inference to confirm
//     the account can actually run (catches quota exhaustion the light GET can't
//     see). Opt-in "补测" — only run on light-verdict online accounts.

const (
	codexProbeURL = "https://chatgpt.com/backend-api/codex/responses"
	// The upstream gates codex on the tui client fingerprint; mirror it so a
	// probe is treated like real codex traffic.
	codexProbeUserAgent  = "codex-tui/0.135.0 (Mac OS 26.5.0; arm64) iTerm.app/3.6.10 (codex-tui; 0.135.0)"
	codexProbeOriginator = "codex-tui"
)

// ProbeVerdict is the classified health of one account.
type ProbeVerdict string

const (
	VerdictOnline              ProbeVerdict = "online"
	VerdictCredentialDead      ProbeVerdict = "credential_dead"
	VerdictRateLimited         ProbeVerdict = "rate_limited"
	VerdictQuotaExhausted      ProbeVerdict = "quota_exhausted"
	VerdictSubscriptionExpired ProbeVerdict = "subscription_expired"
	VerdictUnknown             ProbeVerdict = "unknown"
)

// dead reports whether a verdict means the account is genuinely unusable (as
// opposed to transiently limited or merely unknown). Only these verdicts are
// eligible for opt-in auto-disable during verify-all.
func (v ProbeVerdict) dead() bool {
	return v == VerdictCredentialDead || v == VerdictSubscriptionExpired
}

// ProbeResult is one account's verdict, safe to surface to the operator — it
// never carries the credential itself, only the outcome.
type ProbeResult struct {
	Name      string       `json:"name"`
	AuthIndex string       `json:"auth_index"`
	Verdict   ProbeVerdict `json:"verdict"`
	HTTPCode  int          `json:"http_code"` // upstream status; 0 when unreached
	Detail    string       `json:"detail"`
	At        int64        `json:"at"` // unix seconds
}

// classifyProbe maps a probe outcome to a verdict. Pure and total so the
// classification table is unit-tested without any network. `subExpired` is the
// local subscription-window check (short-circuits before any request);
// `upstreamCode`/`upstreamBody` come from the pinned api-call.
func classifyProbe(subExpired bool, upstreamCode int, upstreamBody string) (ProbeVerdict, string) {
	if subExpired {
		return VerdictSubscriptionExpired, "subscription window closed"
	}
	switch {
	case upstreamCode == 405:
		// GET on the POST-only endpoint passed auth → token is live.
		return VerdictOnline, ""
	case upstreamCode >= 200 && upstreamCode < 300:
		// Heavy POST succeeded → account can actually run inference.
		return VerdictOnline, ""
	case upstreamCode == 401:
		return VerdictCredentialDead, "token rejected (401)"
	case upstreamCode == 402, upstreamCode == 403:
		// 402 deactivated_workspace, 403 forbidden — account-level death.
		return VerdictCredentialDead, fmt.Sprintf("account unavailable (%d)", upstreamCode)
	case upstreamCode == 429:
		if isCodexUsageLimitBody(upstreamBody) {
			return VerdictQuotaExhausted, "usage limit reached"
		}
		return VerdictRateLimited, "rate limited (429)"
	default:
		return VerdictUnknown, fmt.Sprintf("upstream HTTP %d", upstreamCode)
	}
}

// isCodexUsageLimitBody detects codex quota exhaustion, which arrives as a 429
// whose error.type is "usage_limit_reached" (vs a transient rate limit that
// should be retried, not treated as death). Mirrors cliproxy's own check.
func isCodexUsageLimitBody(body string) bool {
	return strings.Contains(body, "usage_limit_reached")
}

// probeTarget is the minimal per-account input a probe needs.
type probeTarget struct {
	Name                    string
	AuthIndex               string
	SubscriptionActiveUntil string
}

// ProbeAuthByName verifies a single account, resolved by its pool file name.
// Stage 0 is the local subscription check; stage 1 is the light GET probe;
// `heavy` adds a stage-2 minimal inference only when the light probe says
// online (to confirm quota). The pool is listed once to resolve the account's
// auth_index + subscription window.
func ProbeAuthByName(poolID, name string, heavy bool) (ProbeResult, error) {
	if err := EnsurePoolProvider(poolID, common.PoolProviderCodex, "Codex account verification"); err != nil {
		return ProbeResult{}, err
	}
	baseURL, secret, ok := common.ResolvePoolMgmt(poolID)
	if !ok {
		return ProbeResult{}, fmt.Errorf("pool management is not configured for pool: %s", poolID)
	}
	entries, err := listPoolEntries(baseURL, secret)
	if err != nil {
		return ProbeResult{}, err
	}
	for _, e := range entries {
		if e.Name == name {
			target := probeTarget{
				Name: e.Name, AuthIndex: e.AuthIndex,
				SubscriptionActiveUntil: e.IDToken.SubscriptionActiveUntil,
			}
			return probeWithMgmt(baseURL, secret, target, heavy), nil
		}
	}
	return ProbeResult{}, fmt.Errorf("account not found in pool: %s", name)
}

func probeWithMgmt(baseURL, secret string, target probeTarget, heavy bool) ProbeResult {
	now := time.Now()
	res := ProbeResult{Name: target.Name, AuthIndex: target.AuthIndex, At: now.Unix()}

	subExpired := subscriptionExpiredAt(target.SubscriptionActiveUntil, now)
	if subExpired {
		res.Verdict, res.Detail = classifyProbe(true, 0, "")
		return res
	}

	code, body := codexApiCall(baseURL, secret, target.AuthIndex, "GET", codexProbeURL, nil, "")
	res.HTTPCode = code
	res.Verdict, res.Detail = classifyProbe(false, code, body)

	// Stage 2 (opt-in): the light GET can't see quota — a token can be live (405)
	// yet out of quota. Only when asked and only for online accounts, confirm
	// with a minimal inference so quota_exhausted surfaces.
	if heavy && res.Verdict == VerdictOnline {
		if hc, hb, ok := codexHeavyProbe(baseURL, secret, target.AuthIndex); ok {
			res.HTTPCode = hc
			res.Verdict, res.Detail = classifyProbe(false, hc, hb)
		}
	}
	return res
}

func subscriptionExpiredAt(until string, now time.Time) bool {
	t := parsePoolTimestamp(until)
	return !t.IsZero() && t.Before(now)
}

// codexApiCall pins one request to `authIndex` via cliproxy's management
// api-call and returns the UPSTREAM status code + body. `$TOKEN$` in the auth
// header is substituted with the account's real credential by cliproxy. A
// transport failure returns code 0 so the caller classifies it as unknown.
func codexApiCall(baseURL, secret, authIndex, method, targetURL string, extraHeaders map[string]string, data string) (int, string) {
	header := map[string]string{"Authorization": "Bearer $TOKEN$"}
	for k, v := range extraHeaders {
		header[k] = v
	}
	reqBody := map[string]any{
		"auth_index": authIndex,
		"method":     method,
		"url":        targetURL,
		"header":     header,
	}
	if data != "" {
		reqBody["data"] = data
	}
	payload, err := common.Marshal(reqBody)
	if err != nil {
		return 0, ""
	}
	status, respBody, err := PoolMgmtRoundTrip(
		context.Background(), baseURL, secret,
		"POST", "/v0/management/api-call", strings.NewReader(string(payload)), "application/json",
	)
	if err != nil || status < 200 || status >= 300 {
		return 0, ""
	}
	var parsed struct {
		StatusCode int    `json:"status_code"`
		Body       string `json:"body"`
	}
	if err := common.Unmarshal(respBody, &parsed); err != nil {
		return 0, ""
	}
	return parsed.StatusCode, parsed.Body
}

// codexHeavyProbe runs a minimal inference against the account's cheapest model
// (fetched from the pool). Returns ok=false if the model list can't be resolved,
// so the caller keeps the light verdict rather than misclassifying.
func codexHeavyProbe(baseURL, secret, authIndex string) (int, string, bool) {
	model, ok := cheapestCodexModel(baseURL, secret, authIndex)
	if !ok {
		return 0, "", false
	}
	// Minimal valid codex /responses body confirmed against production: stream
	// required, no max_output_tokens, store off.
	body := fmt.Sprintf(
		`{"model":%q,"input":[{"role":"user","content":[{"type":"input_text","text":"ping"}]}],"stream":true,"store":false}`,
		model,
	)
	code, respBody := codexApiCall(baseURL, secret, authIndex, "POST", codexProbeURL, map[string]string{
		"Content-Type": "application/json",
		"User-Agent":   codexProbeUserAgent,
		"originator":   codexProbeOriginator,
		"OpenAI-Beta":  "responses=experimental",
	}, body)
	return code, respBody, true
}

// cheapestCodexModel picks the least costly model the account supports (prefers
// a *-mini) from the pool's per-account model list, so the heavy probe burns as
// little as possible.
func cheapestCodexModel(baseURL, secret, authIndex string) (string, bool) {
	// The models endpoint keys off the file name, not auth_index, so resolve the
	// name from the account list first.
	entries, err := listPoolEntries(baseURL, secret)
	if err != nil {
		return "", false
	}
	name := ""
	for _, e := range entries {
		if e.AuthIndex == authIndex {
			name = e.Name
			break
		}
	}
	if name == "" {
		return "", false
	}
	body, err := PoolMgmtRequest(baseURL, secret, "GET", "/v0/management/auth-files/models?name="+url.QueryEscape(name), nil)
	if err != nil {
		return "", false
	}
	var parsed struct {
		Models []struct {
			ID string `json:"id"`
		} `json:"models"`
	}
	if err := common.Unmarshal(body, &parsed); err != nil || len(parsed.Models) == 0 {
		return "", false
	}
	best := ""
	for _, m := range parsed.Models {
		if strings.Contains(m.ID, "mini") {
			return m.ID, true
		}
		if best == "" {
			best = m.ID
		}
	}
	return best, best != ""
}

// ProbePoolConcurrency bounds the fan-out so a full-pool verify stays gentle on
// the upstream while still finishing a 500-account pool in a couple of minutes.
const ProbePoolConcurrency = 8

// ---------------------------------------------------------------------------
// verify-all background job (single-node, IsMasterNode-guarded). 501 accounts ×
// ~2s ÷ 8 concurrency ≈ 2 min, too long to hold an HTTP request, so the fan-out
// runs in a goroutine and the frontend polls progress.

type ProbeJobSnapshot struct {
	Running     bool          `json:"running"`
	Total       int           `json:"total"`
	Done        int           `json:"done"`
	StartedAt   int64         `json:"started_at"`
	FinishedAt  int64         `json:"finished_at"`
	Heavy       bool          `json:"heavy"`
	AutoDisable bool          `json:"auto_disable"`
	Disabled    int           `json:"disabled"`
	Results     []ProbeResult `json:"results"`
	Error       string        `json:"error"`
}

type probeJob struct {
	mu   sync.Mutex
	snap ProbeJobSnapshot
}

var probeJobs sync.Map // poolID -> *probeJob

// StartProbePoolJob kicks off a background verify-all for the pool. It refuses
// to start a second run while one is in flight, and only the master node runs
// it. When autoDisable is set, credential-dead / subscription-expired accounts
// are disabled as their verdicts land.
func StartProbePoolJob(poolID string, heavy, autoDisable bool) (ProbeJobSnapshot, error) {
	if !common.IsMasterNode {
		return ProbeJobSnapshot{}, fmt.Errorf("verify-all runs on the master node only")
	}
	if err := EnsurePoolProvider(poolID, common.PoolProviderCodex, "Codex account verification"); err != nil {
		return ProbeJobSnapshot{}, err
	}
	baseURL, secret, ok := common.ResolvePoolMgmt(poolID)
	if !ok {
		return ProbeJobSnapshot{}, fmt.Errorf("pool management is not configured for pool: %s", poolID)
	}

	jobAny, _ := probeJobs.LoadOrStore(poolID, &probeJob{})
	job := jobAny.(*probeJob)

	job.mu.Lock()
	if job.snap.Running {
		snap := job.snap
		job.mu.Unlock()
		return snap, fmt.Errorf("a verify-all is already running for pool: %s", poolID)
	}
	job.snap = ProbeJobSnapshot{Running: true, Heavy: heavy, AutoDisable: autoDisable, StartedAt: time.Now().Unix()}
	job.mu.Unlock()

	gopool.Go(func() { runProbePoolJob(job, poolID, baseURL, secret, heavy, autoDisable) })
	return job.Snapshot(), nil
}

func runProbePoolJob(job *probeJob, poolID, baseURL, secret string, heavy, autoDisable bool) {
	entries, err := listPoolEntries(baseURL, secret)
	if err != nil {
		job.finish(err)
		return
	}
	targets := make([]probeTarget, 0, len(entries))
	for _, e := range entries {
		if e.Disabled {
			continue
		}
		targets = append(targets, probeTarget{
			Name: e.Name, AuthIndex: e.AuthIndex,
			SubscriptionActiveUntil: e.IDToken.SubscriptionActiveUntil,
		})
	}

	job.mu.Lock()
	job.snap.Total = len(targets)
	job.mu.Unlock()

	sem := make(chan struct{}, ProbePoolConcurrency)
	var wg sync.WaitGroup
	for _, tgt := range targets {
		wg.Add(1)
		sem <- struct{}{}
		go func(t probeTarget) {
			defer wg.Done()
			defer func() { <-sem }()
			r := probeWithMgmt(baseURL, secret, t, heavy)
			disabled := false
			if autoDisable && r.Verdict.dead() {
				if err := disablePoolEntry(baseURL, secret, t.Name); err == nil {
					disabled = true
				} else {
					common.SysError("pool verify-all: failed to disable " + t.Name + ": " + err.Error())
				}
			}
			job.record(r, disabled)
		}(tgt)
	}
	wg.Wait()
	job.finish(nil)
}

func (j *probeJob) record(r ProbeResult, disabled bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.snap.Results = append(j.snap.Results, r)
	j.snap.Done++
	if disabled {
		j.snap.Disabled++
	}
}

func (j *probeJob) finish(err error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.snap.Running = false
	j.snap.FinishedAt = time.Now().Unix()
	if err != nil {
		j.snap.Error = err.Error()
	}
}

// Snapshot returns a copy of the job state safe to serialize. Results is always
// a non-nil slice so it marshals as a JSON array (never null) — the frontend
// reads `.length`/iterates it, so a null here (empty pool / job just started /
// all accounts disabled) would crash the page.
func (j *probeJob) Snapshot() ProbeJobSnapshot {
	j.mu.Lock()
	defer j.mu.Unlock()
	snap := j.snap
	snap.Results = append([]ProbeResult{}, j.snap.Results...)
	return snap
}

// GetProbePoolJob returns the latest verify-all snapshot for a pool, or ok=false
// if none has ever run.
func GetProbePoolJob(poolID string) (ProbeJobSnapshot, bool) {
	jobAny, ok := probeJobs.Load(poolID)
	if !ok {
		return ProbeJobSnapshot{}, false
	}
	return jobAny.(*probeJob).Snapshot(), true
}
