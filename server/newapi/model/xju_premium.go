package model

import "github.com/QuantumNous/new-api/common"

// PremiumTier is a presentation-only identity derived from the user's current
// Default-pool balance. It is deliberately not persisted: spending or topping
// up quota must change the identity immediately without a second source of
// truth drifting out of sync.
type PremiumTier string

const (
	PremiumTierNone        PremiumTier = "none"
	PremiumTierGoldName    PremiumTier = "gold_name"
	PremiumTierSilverCrown PremiumTier = "silver_crown"
	PremiumTierGoldCrown   PremiumTier = "gold_crown"
)

func quotaForUSD(amount float64) int {
	return common.QuotaRound(amount * common.QuotaPerUnit)
}

// PremiumTierForQuota applies the product thresholds in USD-equivalent quota:
// $50 flowing gold name, $100 silver crown, and $1,000 gold crown.
func PremiumTierForQuota(quota int) PremiumTier {
	if quota <= 0 || common.QuotaPerUnit <= 0 {
		return PremiumTierNone
	}
	switch {
	case quota >= quotaForUSD(1000):
		return PremiumTierGoldCrown
	case quota >= quotaForUSD(100):
		return PremiumTierSilverCrown
	case quota >= quotaForUSD(50):
		return PremiumTierGoldName
	default:
		return PremiumTierNone
	}
}

func (user *User) populateComputedFields() {
	if user == nil {
		return
	}
	user.PremiumTier = PremiumTierForQuota(user.Quota)
}
