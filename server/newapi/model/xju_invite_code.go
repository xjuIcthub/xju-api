package model

import (
	"errors"
	"strconv"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

var ErrInviteCodeUnavailable = errors.New("invite code is invalid, expired, disabled, or already used")

// InviteCode is a single-use registration invite code. Admins generate them in
// batches; a code is consumed (Status -> Used) the moment a new user registers
// with it. Codes may carry an optional expiry and can be disabled by an admin.
type InviteCode struct {
	Id          int            `json:"id"`
	Code        string         `json:"code" gorm:"type:char(32);uniqueIndex"`
	Status      int            `json:"status" gorm:"default:1"` // 1=enabled/unused, 2=disabled, 3=used
	CreatorId   int            `json:"creator_id" gorm:"index"`
	UsedUserId  int            `json:"used_user_id"`
	CreatedTime int64          `json:"created_time" gorm:"bigint"`
	UsedTime    int64          `json:"used_time" gorm:"bigint"`
	ExpiredTime int64          `json:"expired_time" gorm:"bigint"` // 0 means never expires
	Count       int            `json:"count" gorm:"-:all"`         // api request only: batch size
	ValidDays   int            `json:"valid_days" gorm:"-:all"`    // api request only: 0 means never
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

func GetAllInviteCodes(startIdx int, num int) (codes []*InviteCode, total int64, err error) {
	if err = DB.Model(&InviteCode{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err = DB.Order("id desc").Limit(num).Offset(startIdx).Find(&codes).Error
	return codes, total, err
}

func SearchInviteCodes(keyword string, status string, startIdx int, num int) (codes []*InviteCode, total int64, err error) {
	query := DB.Model(&InviteCode{})

	if keyword != "" {
		if id, e := strconv.Atoi(keyword); e == nil {
			query = query.Where("id = ? OR code LIKE ?", id, keyword+"%")
		} else {
			query = query.Where("code LIKE ?", keyword+"%")
		}
	}

	if status != "" {
		now := common.GetTimestamp()
		switch status {
		case "expired":
			query = query.Where(
				"status = ? AND expired_time != 0 AND expired_time < ?",
				common.InviteCodeStatusEnabled, now,
			)
		case strconv.Itoa(common.InviteCodeStatusEnabled):
			query = query.Where(
				"status = ? AND (expired_time = 0 OR expired_time >= ?)",
				common.InviteCodeStatusEnabled, now,
			)
		case strconv.Itoa(common.InviteCodeStatusDisabled):
			query = query.Where("status = ?", common.InviteCodeStatusDisabled)
		case strconv.Itoa(common.InviteCodeStatusUsed):
			query = query.Where("status = ?", common.InviteCodeStatusUsed)
		}
	}

	if err = query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err = query.Order("id desc").Limit(num).Offset(startIdx).Find(&codes).Error
	return codes, total, err
}

func GetInviteCodeById(id int) (*InviteCode, error) {
	if id == 0 {
		return nil, errors.New("id is empty")
	}
	code := InviteCode{Id: id}
	err := DB.First(&code, "id = ?", id).Error
	return &code, err
}

func (ic *InviteCode) Insert() error {
	return DB.Create(ic).Error
}

func (ic *InviteCode) Delete() error {
	return DB.Delete(ic).Error
}

// SetInviteCodeStatus flips a code between enabled and disabled. It never
// touches a used code (whose status must stay Used).
func SetInviteCodeStatus(id int, status int) error {
	if id == 0 {
		return errors.New("id is empty")
	}
	return DB.Model(&InviteCode{}).
		Where("id = ? AND status != ?", id, common.InviteCodeStatusUsed).
		Update("status", status).Error
}

func DeleteInviteCodeById(id int) error {
	if id == 0 {
		return errors.New("id is empty")
	}
	ic := InviteCode{Id: id}
	if err := DB.Where(ic).First(&ic).Error; err != nil {
		return err
	}
	return ic.Delete()
}

// DeleteInvalidInviteCodes prunes codes that can never be used again: those
// already used or disabled, plus enabled ones whose expiry has passed.
func DeleteInvalidInviteCodes() (int64, error) {
	now := common.GetTimestamp()
	result := DB.Where(
		"status IN ? OR (status = ? AND expired_time != 0 AND expired_time < ?)",
		[]int{common.InviteCodeStatusUsed, common.InviteCodeStatusDisabled},
		common.InviteCodeStatusEnabled, now,
	).Delete(&InviteCode{})
	return result.RowsAffected, result.Error
}

// ValidateInviteCode reports whether a code is currently usable (exists, enabled,
// not expired) without consuming it — used for a fast pre-check.
func ValidateInviteCode(code string) error {
	if code == "" {
		return errors.New("invite code required")
	}
	ic := &InviteCode{}
	if err := DB.Where("code = ?", code).First(ic).Error; err != nil {
		return errors.New("invalid invite code")
	}
	if ic.Status != common.InviteCodeStatusEnabled {
		return errors.New("invite code already used or disabled")
	}
	if ic.ExpiredTime != 0 && ic.ExpiredTime < common.GetTimestamp() {
		return errors.New("invite code expired")
	}
	return nil
}

// ConsumeInviteCodeWithTx atomically consumes a single-use registration code
// inside the caller's user-creation transaction. This keeps the new account,
// its reward, and used_user_id in one commit boundary. The compare-and-swap on
// status means concurrent registrations cannot spend the same code twice.
func ConsumeInviteCodeWithTx(tx *gorm.DB, code string, userId int) error {
	if code == "" {
		return ErrInviteCodeUnavailable
	}
	now := common.GetTimestamp()
	result := tx.Model(&InviteCode{}).
		Where("code = ? AND status = ? AND (expired_time = 0 OR expired_time >= ?)",
			code, common.InviteCodeStatusEnabled, now).
		Updates(map[string]interface{}{
			"status":       common.InviteCodeStatusUsed,
			"used_user_id": userId,
			"used_time":    now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrInviteCodeUnavailable
	}
	return nil
}
