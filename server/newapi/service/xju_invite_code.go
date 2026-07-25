package service

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"gorm.io/gorm"
)

var ErrRegistrationInviteInvalid = errors.New("registration invite code is required or invalid")

// RegistrationInvite is the normalized meaning of the registration aff_code.
// A personal code identifies a reusable inviter. An admin code is single-use
// and must be consumed in the same transaction that creates the account.
type RegistrationInvite struct {
	InviterID int
	AdminCode string
}

// ResolveRegistrationInviteWithTx applies the same invite-only rules to every
// registration entry point. When invite-only registration is disabled, valid
// personal codes are still attributed, while unknown/admin codes are ignored.
func ResolveRegistrationInviteWithTx(tx *gorm.DB, affCode string) (RegistrationInvite, error) {
	affCode = strings.TrimSpace(affCode)
	if affCode != "" {
		var inviter model.User
		err := tx.Select("id").Where("aff_code = ?", affCode).First(&inviter).Error
		switch {
		case err == nil:
			return RegistrationInvite{InviterID: inviter.Id}, nil
		case !errors.Is(err, gorm.ErrRecordNotFound):
			return RegistrationInvite{}, err
		}
	}

	if !common.InviteCodeRequired {
		return RegistrationInvite{}, nil
	}
	if affCode == "" {
		return RegistrationInvite{}, ErrRegistrationInviteInvalid
	}
	return RegistrationInvite{AdminCode: affCode}, nil
}

// ConsumeRegistrationInviteWithTx binds a single-use admin code to the new
// user. Personal referral codes require no write and remain reusable.
func ConsumeRegistrationInviteWithTx(tx *gorm.DB, invite RegistrationInvite, userID int) error {
	if invite.AdminCode == "" {
		return nil
	}
	if userID <= 0 {
		return errors.New("new user id is required before consuming invite code")
	}
	if err := model.ConsumeInviteCodeWithTx(tx, invite.AdminCode, userID); err != nil {
		if errors.Is(err, model.ErrInviteCodeUnavailable) {
			return ErrRegistrationInviteInvalid
		}
		return err
	}
	return nil
}
