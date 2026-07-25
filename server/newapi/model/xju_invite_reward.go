package model

import (
	"errors"
	"fmt"
	"sync"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const referralBaseRewardUSD = 5.0

var inviteRewardMu sync.Mutex

// InviteReward is the durable idempotency and audit record for one successful
// personal referral. A unique invitee id guarantees that retries, duplicate
// callbacks, or concurrent registration finalizers can never pay twice.
type InviteReward struct {
	Id                    int   `json:"id"`
	InviterId             int   `json:"inviter_id" gorm:"index;not null"`
	InviteeId             int   `json:"invitee_id" gorm:"uniqueIndex;not null"`
	InviteCount           int   `json:"invite_count" gorm:"not null"`
	InviterBaseQuota      int   `json:"inviter_base_quota" gorm:"not null"`
	InviteeQuota          int   `json:"invitee_quota" gorm:"not null"`
	InviterMilestoneQuota int   `json:"inviter_milestone_quota" gorm:"not null"`
	CreatedAt             int64 `json:"created_at" gorm:"autoCreateTime"`
}

type InviteRewardResult struct {
	Applied                bool
	InviteCount            int
	InviterBaseQuota       int
	InviteeQuota           int
	InviterMilestoneQuota  int
	InviterMilestonePeople int
}

func referralMilestoneReward(inviteCount int) (people int, quota int) {
	switch inviteCount {
	case 3:
		return 3, quotaForUSD(10)
	case 5:
		return 5, quotaForUSD(20)
	case 10:
		return 10, quotaForUSD(50)
	default:
		return 0, 0
	}
}

// ApplyInviteReward pays a successful personal referral directly into both
// users' Default-pool balances. The inviter receives $5 for every invite plus
// one-time bonuses at cumulative counts 3/5/10; the invitee receives $5.
func ApplyInviteReward(inviterId int, inviteeId int) (InviteRewardResult, error) {
	if inviterId <= 0 || inviteeId <= 0 || inviterId == inviteeId {
		return InviteRewardResult{}, errors.New("invalid referral relationship")
	}

	// Referrals are low-frequency. This process lock avoids avoidable SQLite
	// writer collisions, while the database unique key and row lock preserve the
	// same guarantee across multiple application instances.
	inviteRewardMu.Lock()
	defer inviteRewardMu.Unlock()

	result := InviteRewardResult{}
	err := DB.Transaction(func(tx *gorm.DB) error {
		var applyErr error
		result, applyErr = applyInviteRewardWithTx(tx, inviterId, inviteeId)
		return applyErr
	})
	if err != nil {
		return InviteRewardResult{}, err
	}
	if !result.Applied {
		return result, nil
	}

	if err := InvalidateUserCache(inviterId); err != nil {
		common.SysLog(fmt.Sprintf("failed to invalidate inviter cache for user %d: %s", inviterId, err.Error()))
	}
	if err := InvalidateUserCache(inviteeId); err != nil {
		common.SysLog(fmt.Sprintf("failed to invalidate invitee cache for user %d: %s", inviteeId, err.Error()))
	}
	return result, nil
}

// applyInviteRewardWithTx credits both users within the caller's registration
// transaction. This makes account creation and reward issuance all-or-nothing,
// so a transient failure cannot leave a successfully-created but unpaid user.
func applyInviteRewardWithTx(tx *gorm.DB, inviterId int, inviteeId int) (InviteRewardResult, error) {
	result := InviteRewardResult{}
	if inviterId <= 0 || inviteeId <= 0 || inviterId == inviteeId {
		return result, errors.New("invalid referral relationship")
	}

	baseQuota := quotaForUSD(referralBaseRewardUSD)
	placeholder := InviteReward{
		InviterId:        inviterId,
		InviteeId:        inviteeId,
		InviterBaseQuota: baseQuota,
		InviteeQuota:     baseQuota,
	}
	create := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "invitee_id"}},
		DoNothing: true,
	}).Create(&placeholder)
	if create.Error != nil {
		return result, create.Error
	}
	if create.RowsAffected == 0 {
		return result, nil
	}

	var inviter User
	if err := lockForUpdate(tx).Select("id").First(&inviter, "id = ?", inviterId).Error; err != nil {
		return result, fmt.Errorf("load inviter: %w", err)
	}
	var invitee User
	if err := tx.Select("id", "inviter_id").First(&invitee, "id = ?", inviteeId).Error; err != nil {
		return result, fmt.Errorf("load invitee: %w", err)
	}
	if invitee.InviterId != inviterId {
		return result, errors.New("invitee referral attribution mismatch")
	}

	var rewardedInvites int64
	if err := tx.Model(&InviteReward{}).Where("inviter_id = ?", inviterId).Count(&rewardedInvites).Error; err != nil {
		return result, fmt.Errorf("count rewarded invites: %w", err)
	}
	result.InviteCount = int(rewardedInvites)
	result.InviterBaseQuota = baseQuota
	result.InviteeQuota = baseQuota
	result.InviterMilestonePeople, result.InviterMilestoneQuota = referralMilestoneReward(result.InviteCount)
	inviterReward := result.InviterBaseQuota + result.InviterMilestoneQuota

	if err := tx.Model(&User{}).Where("id = ?", inviterId).Updates(map[string]interface{}{
		"aff_count":   gorm.Expr("aff_count + ?", 1),
		"aff_history": gorm.Expr("aff_history + ?", inviterReward),
		"quota":       gorm.Expr("quota + ?", inviterReward),
	}).Error; err != nil {
		return result, fmt.Errorf("credit inviter: %w", err)
	}
	if err := tx.Model(&User{}).Where("id = ?", inviteeId).
		Update("quota", gorm.Expr("quota + ?", result.InviteeQuota)).Error; err != nil {
		return result, fmt.Errorf("credit invitee: %w", err)
	}
	if err := tx.Model(&InviteReward{}).Where("id = ?", placeholder.Id).Updates(map[string]interface{}{
		"invite_count":            result.InviteCount,
		"inviter_milestone_quota": result.InviterMilestoneQuota,
	}).Error; err != nil {
		return result, fmt.Errorf("finalize invite reward record: %w", err)
	}

	result.Applied = true
	return result, nil
}
