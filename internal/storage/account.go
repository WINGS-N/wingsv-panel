package storage

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"v.wingsnet.org/internal/storage/dbmodel"
)

// ErrAccountTaken means the account is already attached to a different admin.
var ErrAccountTaken = errors.New("storage: that account is already linked")

// FindAdminByAccount resolves an account subject to the admin it belongs to.
//
// Ищем по subject, а не по имени: имя у провайдера человек меняет когда захочет,
// а номер выдаётся один раз и навсегда
func (s *Store) FindAdminByAccount(subject string) (Admin, error) {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return Admin{}, ErrNotFound
	}
	var row dbmodel.Admin
	err := s.gdb.Where("account_subject = ?", subject).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Admin{}, ErrNotFound
	}
	if err != nil {
		return Admin{}, err
	}
	return toStorageAdmin(row), nil
}

// LinkAccount attaches an account to an admin.
//
// One account, one admin: letting two admins share an identity would make the
// invite tree meaningless, since the cost of an identity is the only thing
// holding it up.
func (s *Store) LinkAccount(adminID int64, subject, name string) error {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return ErrNotFound
	}
	existing, err := s.FindAdminByAccount(subject)
	switch {
	case err == nil && existing.ID != adminID:
		return ErrAccountTaken
	case err == nil:
		return nil
	case !errors.Is(err, ErrNotFound):
		return err
	}
	res := s.gdb.Model(&dbmodel.Admin{}).Where("id = ?", adminID).Updates(map[string]any{
		"account_subject": subject,
		"account_name":    strings.TrimSpace(name),
		"updated_at":      time.Now().UTC().UnixMilli(),
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// UnlinkAccount detaches the account, leaving the admin on password login.
func (s *Store) UnlinkAccount(adminID int64) error {
	return s.gdb.Model(&dbmodel.Admin{}).Where("id = ?", adminID).Updates(map[string]any{
		"account_subject": nil,
		"account_name":    "",
		"updated_at":      time.Now().UTC().UnixMilli(),
	}).Error
}

// AccountNameFor reports which account an admin signs in with, if any.
func (s *Store) AccountNameFor(adminID int64) (string, error) {
	type row struct {
		AccountSubject *string
		AccountName    string
	}
	var got row
	err := s.gdb.Model(&dbmodel.Admin{}).
		Select("account_subject", "account_name").Where("id = ?", adminID).Take(&got).Error
	if err != nil {
		return "", err
	}
	if got.AccountSubject == nil {
		return "", nil
	}
	if got.AccountName != "" {
		return got.AccountName, nil
	}
	return *got.AccountSubject, nil
}

// HasAvatar reports whether the admin already chose a picture.
//
// The answer gates whether the provider's avatar is imported at all: an avatar somebody
// uploaded themselves is their decision, and an identity provider has no business
// overwriting it.
func (s *Store) HasAvatar(adminID int64) (bool, error) {
	type row struct{ AvatarPNG []byte }
	var got row
	err := s.gdb.Model(&dbmodel.Admin{}).Select("avatar_png").Where("id = ?", adminID).Take(&got).Error
	if err != nil {
		return false, err
	}
	return len(got.AvatarPNG) > 0, nil
}

// HasAccount отвечает, привязана ли учётка. По ней решается, пускать ли человека
// дальше входа: панель переезжает на общий вход, и пароль остаётся только
// дверью, через которую эту учётку заводят
func (s *Store) HasAccount(adminID int64) (bool, error) {
	type row struct{ AccountSubject *string }
	var got row
	err := s.gdb.Model(&dbmodel.Admin{}).
		Select("account_subject").Where("id = ?", adminID).Take(&got).Error
	if err != nil {
		return false, err
	}
	return got.AccountSubject != nil && strings.TrimSpace(*got.AccountSubject) != "", nil
}
