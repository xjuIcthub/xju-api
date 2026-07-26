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
package service

import (
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// xju-api:new — private-pool usage has the same priced quota and token
// accounting as shared pools, but it does not consume the user's balance.
func TestPrivatePoolBillingSkipsUserBalanceAndKeepsUsageAccounting(t *testing.T) {
	truncate(t)
	const (
		userID  = 9101
		tokenID = 9102
	)
	seedUser(t, userID, 0)
	seedToken(t, tokenID, userID, "private-billing", 1000)

	c, _ := gin.CreateTestContext(nil)
	info := &relaycommon.RelayInfo{
		UserId:                   userID,
		TokenId:                  tokenID,
		TokenKey:                 "private-billing",
		PrivatePoolBalanceExempt: true,
		ForcePreConsume:          true,
	}

	// The service-package test DB does not run model.initCol, so start with a
	// zero pre-consume and exercise the same funding/token delta in settlement.
	apiErr := PreConsumeBilling(c, 0, info)
	require.Nil(t, apiErr)
	require.NotNil(t, info.Billing)
	assert.Equal(t, BillingSourcePrivatePool, info.BillingSource)
	assert.Equal(t, 0, getUserQuota(t, userID))
	assert.Equal(t, 1000, getTokenRemainQuota(t, tokenID))

	require.NoError(t, SettleBilling(c, info, 150))
	assert.Equal(t, 0, getUserQuota(t, userID))
	assert.Equal(t, 850, getTokenRemainQuota(t, tokenID))

	model.UpdateUserUsedQuotaAndRequestCount(userID, 150)
	var user model.User
	require.NoError(t, model.DB.Select("used_quota", "request_count").First(&user, userID).Error)
	assert.Equal(t, 150, user.UsedQuota)
	assert.Equal(t, 1, user.RequestCount)
}

func TestSharedPoolBillingStillConsumesUserBalance(t *testing.T) {
	truncate(t)
	const (
		userID  = 9201
		tokenID = 9202
	)
	seedUser(t, userID, 2000)
	seedToken(t, tokenID, userID, "shared-billing", 1000)

	c, _ := gin.CreateTestContext(nil)
	info := &relaycommon.RelayInfo{
		UserId:          userID,
		TokenId:         tokenID,
		TokenKey:        "shared-billing",
		ForcePreConsume: true,
	}

	apiErr := PreConsumeBilling(c, 0, info)
	require.Nil(t, apiErr)
	assert.Equal(t, BillingSourceWallet, info.BillingSource)
	assert.Equal(t, 2000, getUserQuota(t, userID))
	assert.Equal(t, 1000, getTokenRemainQuota(t, tokenID))

	require.NoError(t, SettleBilling(c, info, 150))
	assert.Equal(t, 1850, getUserQuota(t, userID))
	assert.Equal(t, 850, getTokenRemainQuota(t, tokenID))
}

func TestClaudePoolUnlimitedSwitchSkipsUserBalance(t *testing.T) {
	truncate(t)
	t.Setenv("POOL_REGISTRY_FILE", filepath.Join(t.TempDir(), "pools.json"))
	const (
		userID    = 9251
		tokenID   = 9252
		channelID = 9253
		groupKey  = "claude-unlimited"
	)
	require.NoError(t, common.AddPoolToRegistry(common.PoolEntry{
		ID: groupKey, Label: "Claude Unlimited", Provider: common.PoolProviderClaude,
		MgmtURL: "http://claude-unlimited:8319", MgmtSecret: "secret",
		ChannelID: channelID, Kind: common.PoolKindAdmin, GroupKey: groupKey,
	}))
	oldEnabled := common.ClaudePoolUnlimitedEnabled
	common.ClaudePoolUnlimitedEnabled = true
	t.Cleanup(func() { common.ClaudePoolUnlimitedEnabled = oldEnabled })

	seedUser(t, userID, 0)
	seedToken(t, tokenID, userID, "claude-unlimited", 1000)
	c, _ := gin.CreateTestContext(nil)
	info := &relaycommon.RelayInfo{
		UserId: userID, TokenId: tokenID, TokenKey: "claude-unlimited",
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: channelID},
		UsingGroup:  groupKey, ForcePreConsume: true,
	}

	apiErr := PreConsumeBilling(c, 0, info)
	require.Nil(t, apiErr)
	assert.Equal(t, BillingSourceClaudePool, info.BillingSource)
	require.NoError(t, SettleBilling(c, info, 150))
	assert.Equal(t, 0, getUserQuota(t, userID))
	assert.Equal(t, 850, getTokenRemainQuota(t, tokenID))
}

func TestClaudePoolUnlimitedSwitchCanBeDisabled(t *testing.T) {
	truncate(t)
	t.Setenv("POOL_REGISTRY_FILE", filepath.Join(t.TempDir(), "pools.json"))
	const (
		userID    = 9261
		tokenID   = 9262
		channelID = 9263
		groupKey  = "claude-billed"
	)
	require.NoError(t, common.AddPoolToRegistry(common.PoolEntry{
		ID: groupKey, Label: "Claude Billed", Provider: common.PoolProviderClaude,
		MgmtURL: "http://claude-billed:8319", MgmtSecret: "secret",
		ChannelID: channelID, Kind: common.PoolKindAdmin, GroupKey: groupKey,
	}))
	oldEnabled := common.ClaudePoolUnlimitedEnabled
	common.ClaudePoolUnlimitedEnabled = false
	t.Cleanup(func() { common.ClaudePoolUnlimitedEnabled = oldEnabled })

	seedUser(t, userID, 2000)
	seedToken(t, tokenID, userID, "claude-billed", 1000)
	c, _ := gin.CreateTestContext(nil)
	info := &relaycommon.RelayInfo{
		UserId: userID, TokenId: tokenID, TokenKey: "claude-billed",
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: channelID},
		UsingGroup:  groupKey, ForcePreConsume: true,
	}

	apiErr := PreConsumeBilling(c, 0, info)
	require.Nil(t, apiErr)
	assert.Equal(t, BillingSourceWallet, info.BillingSource)
	require.NoError(t, SettleBilling(c, info, 150))
	assert.Equal(t, 1850, getUserQuota(t, userID))
	assert.Equal(t, 850, getTokenRemainQuota(t, tokenID))
}

func TestPrivatePoolLegacyPostConsumeSkipsUserBalance(t *testing.T) {
	truncate(t)
	const (
		userID  = 9301
		tokenID = 9302
	)
	seedUser(t, userID, 0)
	seedToken(t, tokenID, userID, "private-legacy", 500)
	info := &relaycommon.RelayInfo{
		UserId:                   userID,
		TokenId:                  tokenID,
		TokenKey:                 "private-legacy",
		PrivatePoolBalanceExempt: true,
		BillingSource:            BillingSourcePrivatePool,
	}

	require.NoError(t, PostConsumeQuota(info, 50, 0, true))
	assert.Equal(t, BillingSourcePrivatePool, info.BillingSource)
	assert.Equal(t, 0, getUserQuota(t, userID))
	assert.Equal(t, 450, getTokenRemainQuota(t, tokenID))
}

func TestPrivatePoolRealtimeFixedPriceMarksBalanceExemptFunding(t *testing.T) {
	c, _ := gin.CreateTestContext(nil)
	info := &relaycommon.RelayInfo{
		UsePrice:                 true,
		PrivatePoolBalanceExempt: true,
	}

	require.NoError(t, PreWssConsumeQuota(c, info, &dto.RealtimeUsage{}))
	assert.Equal(t, BillingSourcePrivatePool, info.BillingSource)
}
