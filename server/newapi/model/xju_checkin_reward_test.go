package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserCheckinAwardsFixedTenCentsOnce(t *testing.T) {
	require.NoError(t, DB.Exec("DELETE FROM checkins").Error)
	require.NoError(t, DB.Exec("DELETE FROM users").Error)

	previousQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 100
	setting := operation_setting.GetCheckinSetting()
	previousEnabled := setting.Enabled
	setting.Enabled = true
	t.Cleanup(func() {
		common.QuotaPerUnit = previousQuotaPerUnit
		setting.Enabled = previousEnabled
	})

	user := createReferralUser(t, "checkin-user", 0)
	checkin, err := UserCheckin(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 10, checkin.QuotaAwarded)

	var paidUser User
	require.NoError(t, DB.First(&paidUser, user.Id).Error)
	assert.Equal(t, 10, paidUser.Quota)

	_, err = UserCheckin(user.Id)
	assert.Error(t, err)
	require.NoError(t, DB.First(&paidUser, user.Id).Error)
	assert.Equal(t, 10, paidUser.Quota)
}
