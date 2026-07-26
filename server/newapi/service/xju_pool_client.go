package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// xju-api:new — 池管理 API 的唯一 HTTP round-trip helper(REFACTOR-PLAN §5.2)。
//
// 收敛自 controller/pool_auth 的 poolMgmtClient+poolMgmtRoundTrip 与本包
// pool_cleanup 的 poolCleanupClient+poolMgmtRequest 两份近似重复;
// 池地址/密钥的 env 解析已由 common.ResolvePoolMgmt 统一,这里只管传输。

var poolMgmtClient = &http.Client{Timeout: 20 * time.Second}

// PoolMgmtRoundTrip performs one authenticated call to a pool's management API
// and returns the raw status + body, so callers can either pipe it through or
// merge it with locally-computed results (the batch importer does the latter).
func PoolMgmtRoundTrip(ctx context.Context, baseURL, secret, method, path string, body io.Reader, contentType string) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, body)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+secret)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := poolMgmtClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, payload, nil
}

// PoolMgmtRequest is the sweep-side convenience wrapper: background context,
// JSON content type whenever a body is present, and a hard error for any
// non-2xx response (the sweep treats every failure the same way).
func PoolMgmtRequest(baseURL, secret, method, path string, body io.Reader) ([]byte, error) {
	contentType := ""
	if body != nil {
		contentType = "application/json"
	}
	status, data, err := PoolMgmtRoundTrip(context.Background(), baseURL, secret, method, path, body, contentType)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("pool management HTTP %d: %s", status, strings.TrimSpace(string(data)))
	}
	return data, nil
}

// EnsurePoolProvider prevents provider-specific account operations from being
// sent to an incompatible pool (for example ChatGPT Wham quota calls against a
// Claude OAuth credential).
func EnsurePoolProvider(poolID, expectedProvider, feature string) error {
	pool, ok := common.FindConfiguredPoolInfo(poolID)
	if !ok {
		return fmt.Errorf("pool is not configured: %s", poolID)
	}
	provider := common.NormalizePoolProvider(pool.Provider)
	if provider != expectedProvider {
		return fmt.Errorf("%s is available for %s pools only", feature, expectedProvider)
	}
	return nil
}
