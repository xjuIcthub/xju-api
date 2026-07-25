/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package router

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const paidPoolSuccessResponse = `{"id":"chatcmpl_paid_pool","object":"chat.completion","created":1784836000,"model":"gpt-5","choices":[{"index":0,"message":{"role":"assistant","content":"OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`

func TestDefaultPoolRequiresPositiveQuotaAndAccountsSuccessfulUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupRelayIntegrationDB(t)

	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(paidPoolSuccessResponse))
	}))
	defer upstream.Close()

	seedAdvancedPoolChannel(t, upstream.URL, "default", "default-paid-pool-key", "default paid pool")
	seedRelayUser(t, 31, "default", 2_000_000)
	seedRelayUser(t, 32, "default", 0)
	seedRelayUser(t, 33, "default", -100)
	seedRelayToken(t, 31, "defaultpositivequota000000000000000001", "default", common.TokenStatusEnabled, -1)
	seedRelayToken(t, 32, "defaultzeroquota00000000000000000001", "default", common.TokenStatusEnabled, -1)
	seedRelayToken(t, 33, "defaultnegativequota00000000000000001", "default", common.TokenStatusEnabled, -1)

	engine := gin.New()
	SetRelayRouter(engine)
	payload := []byte(`{"model":"gpt-5","messages":[{"role":"user","content":"hello"}]}`)

	positive := performRelayIntegrationRequest(engine, "/v1/chat/completions", "sk-defaultpositivequota000000000000000001", payload)
	require.Equal(t, http.StatusOK, positive.Code, positive.Body.String())

	for _, testCase := range []struct {
		name      string
		userID    int
		token     string
		wantQuota int
	}{
		{name: "zero", userID: 32, token: "sk-defaultzeroquota00000000000000000001", wantQuota: 0},
		{name: "negative", userID: 33, token: "sk-defaultnegativequota00000000000000001", wantQuota: -100},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := performRelayIntegrationRequest(engine, "/v1/chat/completions", testCase.token, payload)
			assert.Equal(t, http.StatusForbidden, recorder.Code, recorder.Body.String())

			user := loadRelayBillingUser(t, testCase.userID)
			assert.Equal(t, testCase.wantQuota, user.Quota)
			assert.Zero(t, user.UsedQuota)
			assert.Zero(t, user.RequestCount)
		})
	}

	assert.Equal(t, int32(1), upstreamCalls.Load(), "non-positive Default balances must be rejected before the upstream")
	paidUser := loadRelayBillingUser(t, 31)
	assert.Less(t, paidUser.Quota, 2_000_000)
	assert.Greater(t, paidUser.UsedQuota, 0)
	assert.Equal(t, 1, paidUser.RequestCount)
}

func TestPrivatePoolAllowsNonPositiveQuotaAndRejectsForgedOwnership(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupRelayIntegrationDB(t)

	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(paidPoolSuccessResponse))
	}))
	defer upstream.Close()

	zeroGroup := common.PrivatePoolGroupKey(41)
	negativeGroup := common.PrivatePoolGroupKey(42)
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(fmt.Sprintf(
		`{"default":1,%q:1,%q:1}`,
		zeroGroup,
		negativeGroup,
	)))
	zeroChannelID := seedAdvancedPoolChannel(t, upstream.URL, zeroGroup, "private-zero-key", "zero balance private pool")
	negativeChannelID := seedAdvancedPoolChannel(t, upstream.URL, negativeGroup, "private-negative-key", "negative balance private pool")

	registryPath := filepath.Join(t.TempDir(), "pool-registry.json")
	t.Setenv("POOL_REGISTRY_FILE", registryPath)
	require.NoError(t, common.SavePoolRegistry([]common.PoolEntry{
		{
			ID: "private-41", Label: "Zero balance", MgmtURL: upstream.URL, MgmtSecret: "secret-41",
			ChannelID: zeroChannelID, OwnerUserID: 41, Kind: common.PoolKindPrivate, GroupKey: zeroGroup,
		},
		{
			ID: "private-42", Label: "Negative balance", MgmtURL: upstream.URL, MgmtSecret: "secret-42",
			ChannelID: negativeChannelID, OwnerUserID: 42, Kind: common.PoolKindPrivate, GroupKey: negativeGroup,
		},
	}))

	seedRelayUser(t, 41, "default", 0)
	seedRelayUser(t, 42, "default", -100)
	seedRelayUser(t, 43, "default", 2_000_000)
	seedRelayToken(t, 41, "privatezeroquota000000000000000000001", zeroGroup, common.TokenStatusEnabled, -1)
	seedRelayToken(t, 42, "privatenegativequota0000000000000001", negativeGroup, common.TokenStatusEnabled, -1)
	seedRelayToken(t, 43, "privateforgedowner000000000000000001", zeroGroup, common.TokenStatusEnabled, -1)

	engine := gin.New()
	SetRelayRouter(engine)
	payload := []byte(`{"model":"gpt-5","messages":[{"role":"user","content":"private"}]}`)

	for _, testCase := range []struct {
		name      string
		userID    int
		token     string
		wantQuota int
	}{
		{name: "zero", userID: 41, token: "sk-privatezeroquota000000000000000000001", wantQuota: 0},
		{name: "negative", userID: 42, token: "sk-privatenegativequota0000000000000001", wantQuota: -100},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := performRelayIntegrationRequest(engine, "/v1/chat/completions", testCase.token, payload)
			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

			user := loadRelayBillingUser(t, testCase.userID)
			assert.Equal(t, testCase.wantQuota, user.Quota, "private-pool usage must not consume Default balance")
			assert.Greater(t, user.UsedQuota, 0, "private-pool usage remains visible in total usage")
			assert.Equal(t, 1, user.RequestCount)
		})
	}

	forged := performRelayIntegrationRequest(engine, "/v1/chat/completions", "sk-privateforgedowner000000000000000001", payload)
	assert.Equal(t, http.StatusForbidden, forged.Code, forged.Body.String())
	assert.Equal(t, int32(2), upstreamCalls.Load(), "a forged private group must never reach its owner's upstream")
	attacker := loadRelayBillingUser(t, 43)
	assert.Equal(t, 2_000_000, attacker.Quota)
	assert.Zero(t, attacker.UsedQuota)
	assert.Zero(t, attacker.RequestCount)
}

func TestPrivatePoolFailureRefundsPrechargeAndNeverFallsBackToDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupRelayIntegrationDB(t)

	originalRetryTimes := common.RetryTimes
	common.RetryTimes = 2
	t.Cleanup(func() { common.RetryTimes = originalRetryTimes })

	var privateCalls atomic.Int32
	privateUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		privateCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"private upstream unavailable","type":"upstream_error"}}`))
	}))
	defer privateUpstream.Close()

	var defaultCalls atomic.Int32
	defaultUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		defaultCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(paidPoolSuccessResponse))
	}))
	defer defaultUpstream.Close()

	const userID = 51
	privateGroup := common.PrivatePoolGroupKey(userID)
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(fmt.Sprintf(`{"default":1,%q:1}`, privateGroup)))
	privateChannelID := seedAdvancedPoolChannel(t, privateUpstream.URL, privateGroup, "private-failure-key", "failing private pool")
	seedAdvancedPoolChannel(t, defaultUpstream.URL, "default", "default-fallback-key", "Default must not be used")

	registryPath := filepath.Join(t.TempDir(), "pool-registry.json")
	t.Setenv("POOL_REGISTRY_FILE", registryPath)
	require.NoError(t, common.SavePoolRegistry([]common.PoolEntry{{
		ID: "private-51", Label: "Failing private pool", MgmtURL: privateUpstream.URL, MgmtSecret: "secret-51",
		ChannelID: privateChannelID, OwnerUserID: userID, Kind: common.PoolKindPrivate, GroupKey: privateGroup,
	}}))

	seedRelayUser(t, userID, "default", 2_000_000)
	const tokenKey = "privatefailuretoken000000000000000001"
	seedRelayToken(t, userID, tokenKey, privateGroup, common.TokenStatusEnabled, -1)
	const initialTokenQuota = 2_000_000
	require.NoError(t, model.DB.Model(&model.Token{}).Where("user_id = ? AND `key` = ?", userID, tokenKey).Updates(map[string]any{
		"remain_quota":      initialTokenQuota,
		"cross_group_retry": true,
	}).Error)

	engine := gin.New()
	SetRelayRouter(engine)
	payload := []byte(`{"model":"gpt-5","messages":[{"role":"user","content":"fail privately"}]}`)
	recorder := performRelayIntegrationRequest(engine, "/v1/chat/completions", "sk-"+tokenKey, payload)
	assert.GreaterOrEqual(t, recorder.Code, http.StatusBadRequest, recorder.Body.String())
	assert.Greater(t, privateCalls.Load(), int32(0))
	assert.Zero(t, defaultCalls.Load(), "a failed private pool request must never retry through Default")

	user := loadRelayBillingUser(t, userID)
	assert.Equal(t, 2_000_000, user.Quota, "private failure must not touch Default balance")
	assert.Zero(t, user.UsedQuota)
	assert.Zero(t, user.RequestCount)

	require.Eventually(t, func() bool {
		var token model.Token
		if err := model.DB.Where("user_id = ? AND `key` = ?", userID, tokenKey).First(&token).Error; err != nil {
			return false
		}
		return token.RemainQuota == initialTokenQuota && token.UsedQuota == 0
	}, 2*time.Second, 10*time.Millisecond, "failed private requests must refund token precharge")
}

func loadRelayBillingUser(t *testing.T, userID int) model.User {
	t.Helper()
	var user model.User
	require.NoError(t, model.DB.Select("quota", "used_quota", "request_count").First(&user, userID).Error)
	return user
}
