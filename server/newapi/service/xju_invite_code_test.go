package service

import (
	"errors"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func seedInviteCode(t *testing.T, code string, status int, expiredTime int64) {
	t.Helper()
	require.NoError(t, model.DB.Create(&model.InviteCode{
		Code:        code,
		Status:      status,
		CreatorId:   1,
		CreatedTime: common.GetTimestamp(),
		ExpiredTime: expiredTime,
	}).Error)
	t.Cleanup(func() {
		model.DB.Unscoped().Where("code = ?", code).Delete(&model.InviteCode{})
	})
}

func fetchInviteCode(t *testing.T, code string) *model.InviteCode {
	t.Helper()
	ic := &model.InviteCode{}
	require.NoError(t, model.DB.Where("code = ?", code).First(ic).Error)
	return ic
}

func withInviteCodeRequired(t *testing.T, required bool) {
	t.Helper()
	previous := common.InviteCodeRequired
	common.InviteCodeRequired = required
	t.Cleanup(func() { common.InviteCodeRequired = previous })
}

func createInviteTestUser(t *testing.T, username string, affCode string) model.User {
	t.Helper()
	user := model.User{
		Username:    username,
		DisplayName: username,
		AffCode:     affCode,
		Status:      common.UserStatusEnabled,
		Role:        common.RoleCommonUser,
	}
	require.NoError(t, model.DB.Create(&user).Error)
	t.Cleanup(func() {
		model.DB.Unscoped().Delete(&user)
	})
	return user
}

func TestResolveRegistrationInvite_RequiredOffIgnoresAdminAndUnknownCodes(t *testing.T) {
	withInviteCodeRequired(t, false)
	seedInviteCode(t, "required-off-admin-code", common.InviteCodeStatusEnabled, 0)

	for _, code := range []string{"", "unknown-code", "required-off-admin-code"} {
		err := model.DB.Transaction(func(tx *gorm.DB) error {
			invite, err := ResolveRegistrationInviteWithTx(tx, code)
			require.NoError(t, err)
			assert.Zero(t, invite.InviterID)
			assert.Empty(t, invite.AdminCode)
			return ConsumeRegistrationInviteWithTx(tx, invite, 42)
		})
		require.NoError(t, err)
	}
	assert.Equal(t, common.InviteCodeStatusEnabled, fetchInviteCode(t, "required-off-admin-code").Status)
}

func TestResolveRegistrationInvite_PersonalCodeIsReusable(t *testing.T) {
	withInviteCodeRequired(t, true)
	referrer := createInviteTestUser(t, "transactional-referrer", "share-transactionally")

	for attempt := 0; attempt < 3; attempt++ {
		err := model.DB.Transaction(func(tx *gorm.DB) error {
			invite, err := ResolveRegistrationInviteWithTx(tx, "share-transactionally")
			require.NoError(t, err)
			assert.Equal(t, referrer.Id, invite.InviterID)
			assert.Empty(t, invite.AdminCode)
			return ConsumeRegistrationInviteWithTx(tx, invite, 100+attempt)
		})
		require.NoError(t, err)
	}

	var count int64
	require.NoError(t, model.DB.Model(&model.InviteCode{}).
		Where("code = ?", "share-transactionally").Count(&count).Error)
	assert.Zero(t, count)
}

func TestRegistrationInvite_AdminCodeCommitsWithUserID(t *testing.T) {
	withInviteCodeRequired(t, true)
	seedInviteCode(t, "transactional-admin-commit", common.InviteCodeStatusEnabled, 0)

	var user model.User
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		invite, err := ResolveRegistrationInviteWithTx(tx, "transactional-admin-commit")
		if err != nil {
			return err
		}
		user = model.User{
			Username:    "admin-code-commit-user",
			DisplayName: "admin-code-commit-user",
			AffCode:     "admin-code-commit-aff",
			Status:      common.UserStatusEnabled,
			Role:        common.RoleCommonUser,
		}
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		return ConsumeRegistrationInviteWithTx(tx, invite, user.Id)
	})
	require.NoError(t, err)
	t.Cleanup(func() { model.DB.Unscoped().Delete(&user) })

	code := fetchInviteCode(t, "transactional-admin-commit")
	assert.Equal(t, common.InviteCodeStatusUsed, code.Status)
	assert.Equal(t, user.Id, code.UsedUserId)
	assert.NotZero(t, code.UsedTime)
}

func TestRegistrationInvite_RollbackRestoresCodeAndUserTogether(t *testing.T) {
	withInviteCodeRequired(t, true)
	seedInviteCode(t, "transactional-admin-rollback", common.InviteCodeStatusEnabled, 0)
	rollbackErr := errors.New("simulate failure after invite consume")

	err := model.DB.Transaction(func(tx *gorm.DB) error {
		invite, err := ResolveRegistrationInviteWithTx(tx, "transactional-admin-rollback")
		if err != nil {
			return err
		}
		user := model.User{
			Username:    "rolled-back-invite-user",
			DisplayName: "rolled-back-invite-user",
			AffCode:     "rolled-back-invite-aff",
			Status:      common.UserStatusEnabled,
			Role:        common.RoleCommonUser,
		}
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		if err := ConsumeRegistrationInviteWithTx(tx, invite, user.Id); err != nil {
			return err
		}
		return rollbackErr
	})
	require.ErrorIs(t, err, rollbackErr)

	var userCount int64
	require.NoError(t, model.DB.Unscoped().Model(&model.User{}).
		Where("username = ?", "rolled-back-invite-user").Count(&userCount).Error)
	assert.Zero(t, userCount)
	code := fetchInviteCode(t, "transactional-admin-rollback")
	assert.Equal(t, common.InviteCodeStatusEnabled, code.Status)
	assert.Zero(t, code.UsedUserId)
	assert.Zero(t, code.UsedTime)
}

func TestRegistrationInvite_RejectsMissingOrUnusableCodes(t *testing.T) {
	withInviteCodeRequired(t, true)
	seedInviteCode(t, "transactional-used-code", common.InviteCodeStatusUsed, 0)
	seedInviteCode(t, "transactional-disabled-code", common.InviteCodeStatusDisabled, 0)
	seedInviteCode(t, "transactional-expired-code", common.InviteCodeStatusEnabled, common.GetTimestamp()-60)

	testCases := []struct {
		name string
		code string
	}{
		{name: "missing", code: ""},
		{name: "unknown", code: "transactional-unknown-code"},
		{name: "used", code: "transactional-used-code"},
		{name: "disabled", code: "transactional-disabled-code"},
		{name: "expired", code: "transactional-expired-code"},
	}
	for index, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			err := model.DB.Transaction(func(tx *gorm.DB) error {
				invite, err := ResolveRegistrationInviteWithTx(tx, testCase.code)
				if err != nil {
					return err
				}
				return ConsumeRegistrationInviteWithTx(tx, invite, 500+index)
			})
			require.ErrorIs(t, err, ErrRegistrationInviteInvalid)
		})
	}
	assert.Equal(t, common.InviteCodeStatusEnabled, fetchInviteCode(t, "transactional-expired-code").Status)
}

func TestRegistrationInvite_ConcurrentSingleUse(t *testing.T) {
	withInviteCodeRequired(t, true)
	seedInviteCode(t, "transactional-race-code", common.InviteCodeStatusEnabled, 0)

	const attempts = 8
	var wait sync.WaitGroup
	errs := make([]error, attempts)
	for index := 0; index < attempts; index++ {
		wait.Add(1)
		go func(attempt int) {
			defer wait.Done()
			errs[attempt] = model.DB.Transaction(func(tx *gorm.DB) error {
				invite, err := ResolveRegistrationInviteWithTx(tx, "transactional-race-code")
				if err != nil {
					return err
				}
				return ConsumeRegistrationInviteWithTx(tx, invite, 700+attempt)
			})
		}(index)
	}
	wait.Wait()

	successes := 0
	for _, err := range errs {
		if err == nil {
			successes++
			continue
		}
		assert.Error(t, err)
	}
	assert.Equal(t, 1, successes)
	code := fetchInviteCode(t, "transactional-race-code")
	assert.Equal(t, common.InviteCodeStatusUsed, code.Status)
	assert.NotZero(t, code.UsedUserId)
}
