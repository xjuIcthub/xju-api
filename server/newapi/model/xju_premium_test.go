package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
)

func TestPremiumTierForQuotaThresholds(t *testing.T) {
	previous := common.QuotaPerUnit
	common.QuotaPerUnit = 100
	t.Cleanup(func() { common.QuotaPerUnit = previous })

	tests := []struct {
		quota int
		tier  PremiumTier
	}{
		{0, PremiumTierNone},
		{4999, PremiumTierNone},
		{5000, PremiumTierGoldName},
		{9999, PremiumTierGoldName},
		{10000, PremiumTierSilverCrown},
		{99999, PremiumTierSilverCrown},
		{100000, PremiumTierGoldCrown},
	}

	for _, test := range tests {
		assert.Equal(t, test.tier, PremiumTierForQuota(test.quota))
	}
}
