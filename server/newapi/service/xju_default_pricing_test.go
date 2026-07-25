package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetDefaultPoolPricingMultiplierPreservesOtherGroups(t *testing.T) {
	previous := ratio_setting.GroupRatio2JSONString()
	require.NoError(t, model.UpdateOption("GroupRatio", `{"default":1,"private-test":0.75}`))
	t.Cleanup(func() { require.NoError(t, model.UpdateOption("GroupRatio", previous)) })

	require.NoError(t, SetDefaultPoolPricingMultiplier(1.8))
	assert.Equal(t, 1.8, GetDefaultPoolPricingMultiplier())
	assert.Equal(t, 0.75, ratio_setting.GetGroupRatio("private-test"))

	var option model.Option
	require.NoError(t, model.DB.First(&option, "key = ?", "GroupRatio").Error)
	assert.JSONEq(t, `{"default":1.8,"private-test":0.75}`, option.Value)
}
