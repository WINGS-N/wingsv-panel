package storage

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"v.wingsnet.org/internal/storage/dbmodel"
)

// ErrMatrixIDTaken means the account is already attached to a different admin.
var ErrMatrixIDTaken = errors.New("storage: that matrix account is already linked")

// FindAdminByMatrixID resolves a Matrix account to the admin it belongs to.
func (s *Store) FindAdminByMatrixID(matrixID string) (Admin, error) {
	matrixID = strings.ToLower(strings.TrimSpace(matrixID))
	if matrixID == "" {
		return Admin{}, ErrNotFound
	}
	var row dbmodel.Admin
	err := s.gdb.Where("matrix_id = ?", matrixID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Admin{}, ErrNotFound
	}
	if err != nil {
		return Admin{}, err
	}
	return toStorageAdmin(row), nil
}

// LinkMatrixID attaches a Matrix account to an admin.
//
// One account, one admin: letting two admins share an identity would make the
// invite tree meaningless, since the cost of an identity is the only thing
// holding it up.
func (s *Store) LinkMatrixID(adminID int64, matrixID, subject string) error {
	matrixID = strings.ToLower(strings.TrimSpace(matrixID))
	if matrixID == "" {
		return ErrNotFound
	}
	existing, err := s.FindAdminByMatrixID(matrixID)
	switch {
	case err == nil && existing.ID != adminID:
		return ErrMatrixIDTaken
	case err == nil:
		return nil
	case !errors.Is(err, ErrNotFound):
		return err
	}
	res := s.gdb.Model(&dbmodel.Admin{}).Where("id = ?", adminID).Updates(map[string]any{
		"matrix_id":      matrixID,
		"matrix_subject": subject,
		"updated_at":     time.Now().UTC().UnixMilli(),
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// UnlinkMatrixID detaches the account, leaving the admin on password login.
func (s *Store) UnlinkMatrixID(adminID int64) error {
	return s.gdb.Model(&dbmodel.Admin{}).Where("id = ?", adminID).Updates(map[string]any{
		"matrix_id":      nil,
		"matrix_subject": "",
		"updated_at":     time.Now().UTC().UnixMilli(),
	}).Error
}

// MatrixIDFor reports which account an admin signs in with, if any.
func (s *Store) MatrixIDFor(adminID int64) (string, error) {
	type row struct{ MatrixID *string }
	var got row
	err := s.gdb.Model(&dbmodel.Admin{}).Select("matrix_id").Where("id = ?", adminID).Take(&got).Error
	if err != nil {
		return "", err
	}
	if got.MatrixID == nil {
		return "", nil
	}
	return *got.MatrixID, nil
}

// HasAvatar reports whether the admin already chose a picture.
//
// The answer gates whether a Matrix avatar is imported at all: an avatar somebody
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
