package model

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupInviteRewardTest(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.Exec("DELETE FROM invite_rewards").Error)
	require.NoError(t, DB.Exec("DELETE FROM users").Error)
	previousQuotaPerUnit := common.QuotaPerUnit
	previousNewUserQuota := common.QuotaForNewUser
	common.QuotaPerUnit = 100
	common.QuotaForNewUser = 0
	t.Cleanup(func() {
		common.QuotaPerUnit = previousQuotaPerUnit
		common.QuotaForNewUser = previousNewUserQuota
	})
}

func createReferralUser(t *testing.T, username string, inviterId int) User {
	t.Helper()
	user := User{
		Username:    username,
		Password:    "password",
		Status:      common.UserStatusEnabled,
		Role:        common.RoleCommonUser,
		InviterId:   inviterId,
		AffCode:     username + "-code",
		DisplayName: username,
	}
	require.NoError(t, DB.Create(&user).Error)
	return user
}

func TestApplyInviteRewardPaysBothUsersAndMilestonesOnce(t *testing.T) {
	setupInviteRewardTest(t)
	inviter := createReferralUser(t, "reward-inviter", 0)

	for inviteCount := 1; inviteCount <= 10; inviteCount++ {
		invitee := createReferralUser(t, fmt.Sprintf("reward-invitee-%d", inviteCount), inviter.Id)
		result, err := ApplyInviteReward(inviter.Id, invitee.Id)
		require.NoError(t, err)
		assert.True(t, result.Applied)
		assert.Equal(t, inviteCount, result.InviteCount)
		assert.Equal(t, 500, result.InviterBaseQuota)
		assert.Equal(t, 500, result.InviteeQuota)

		expectedMilestone := 0
		switch inviteCount {
		case 3:
			expectedMilestone = 1000
		case 5:
			expectedMilestone = 2000
		case 10:
			expectedMilestone = 5000
		}
		assert.Equal(t, expectedMilestone, result.InviterMilestoneQuota)

		var paidInvitee User
		require.NoError(t, DB.First(&paidInvitee, invitee.Id).Error)
		assert.Equal(t, 500, paidInvitee.Quota)
	}

	var paidInviter User
	require.NoError(t, DB.First(&paidInviter, inviter.Id).Error)
	assert.Equal(t, 10, paidInviter.AffCount)
	assert.Equal(t, 13000, paidInviter.Quota)
	assert.Equal(t, 13000, paidInviter.AffHistoryQuota)
	assert.Equal(t, 0, paidInviter.AffQuota)

	var rewardCount int64
	require.NoError(t, DB.Model(&InviteReward{}).Count(&rewardCount).Error)
	assert.Equal(t, int64(10), rewardCount)
}

func TestApplyInviteRewardIsIdempotentUnderConcurrentRetries(t *testing.T) {
	setupInviteRewardTest(t)
	inviter := createReferralUser(t, "retry-inviter", 0)
	invitee := createReferralUser(t, "retry-invitee", inviter.Id)

	const attempts = 8
	results := make([]InviteRewardResult, attempts)
	errs := make([]error, attempts)
	var wait sync.WaitGroup
	for idx := 0; idx < attempts; idx++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			results[index], errs[index] = ApplyInviteReward(inviter.Id, invitee.Id)
		}(idx)
	}
	wait.Wait()

	applied := 0
	for idx := range results {
		require.NoError(t, errs[idx])
		if results[idx].Applied {
			applied++
		}
	}
	assert.Equal(t, 1, applied)

	var gotInviter User
	var gotInvitee User
	require.NoError(t, DB.First(&gotInviter, inviter.Id).Error)
	require.NoError(t, DB.First(&gotInvitee, invitee.Id).Error)
	assert.Equal(t, 1, gotInviter.AffCount)
	assert.Equal(t, 500, gotInviter.Quota)
	assert.Equal(t, 500, gotInvitee.Quota)
}

func TestApplyInviteRewardMilestonesIgnoreLegacyAffCount(t *testing.T) {
	setupInviteRewardTest(t)
	inviter := createReferralUser(t, "legacy-count-inviter", 0)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", inviter.Id).Update("aff_count", 99).Error)
	invitee := createReferralUser(t, "legacy-count-invitee", inviter.Id)

	result, err := ApplyInviteReward(inviter.Id, invitee.Id)
	require.NoError(t, err)
	assert.True(t, result.Applied)
	assert.Equal(t, 1, result.InviteCount)
	assert.Zero(t, result.InviterMilestoneQuota)

	var paidInviter User
	require.NoError(t, DB.First(&paidInviter, inviter.Id).Error)
	assert.Equal(t, 100, paidInviter.AffCount)
	assert.Equal(t, 500, paidInviter.Quota)
}

func TestInsertWithTxKeepsUserAndInviteRewardAtomic(t *testing.T) {
	setupInviteRewardTest(t)
	inviter := createReferralUser(t, "atomic-inviter", 0)
	rollbackErr := errors.New("simulate registration rollback")

	rolledBackInvitee := User{
		Username:    "atomic-rolled-back-invitee",
		Password:    "password",
		DisplayName: "atomic-rolled-back-invitee",
		Status:      common.UserStatusEnabled,
		Role:        common.RoleCommonUser,
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := rolledBackInvitee.InsertWithTx(tx, inviter.Id); err != nil {
			return err
		}
		return rollbackErr
	})
	require.ErrorIs(t, err, rollbackErr)

	var userCount int64
	require.NoError(t, DB.Unscoped().Model(&User{}).
		Where("username = ?", rolledBackInvitee.Username).Count(&userCount).Error)
	assert.Zero(t, userCount)
	var rewardCount int64
	require.NoError(t, DB.Model(&InviteReward{}).Count(&rewardCount).Error)
	assert.Zero(t, rewardCount)
	var unchangedInviter User
	require.NoError(t, DB.First(&unchangedInviter, inviter.Id).Error)
	assert.Zero(t, unchangedInviter.Quota)
	assert.Zero(t, unchangedInviter.AffCount)

	committedInvitee := User{
		Username:    "atomic-committed-invitee",
		Password:    "password",
		DisplayName: "atomic-committed-invitee",
		Status:      common.UserStatusEnabled,
		Role:        common.RoleCommonUser,
	}
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return committedInvitee.InsertWithTx(tx, inviter.Id)
	}))

	var paidInviter User
	var paidInvitee User
	require.NoError(t, DB.First(&paidInviter, inviter.Id).Error)
	require.NoError(t, DB.First(&paidInvitee, committedInvitee.Id).Error)
	assert.Equal(t, 500, paidInviter.Quota)
	assert.Equal(t, 500, paidInvitee.Quota)
	assert.Equal(t, 1, paidInviter.AffCount)
	require.NoError(t, DB.Model(&InviteReward{}).Count(&rewardCount).Error)
	assert.Equal(t, int64(1), rewardCount)
}

func TestGetUserUsageSummaryIncludesAllUsers(t *testing.T) {
	setupInviteRewardTest(t)
	first := createReferralUser(t, "summary-one", 0)
	second := createReferralUser(t, "summary-two", 0)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", first.Id).Update("used_quota", 125).Error)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", second.Id).Update("used_quota", 375).Error)

	summary, err := GetUserUsageSummary()
	require.NoError(t, err)
	assert.Equal(t, int64(500), summary.TotalUsedQuota)
}
