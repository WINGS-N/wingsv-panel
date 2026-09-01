package storage

import (
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"v.wingsnet.org/internal/storage/dbmodel"
)

// TOTPState - состояние 2FA у аккаунта
type TOTPState struct {
	Secret    string
	Confirmed bool
}

// hashBackupCode приводит код к хешу, в котором он и лежит
func hashBackupCode(code string) string {
	sum := sha512.Sum512_256([]byte(strings.ToLower(strings.TrimSpace(code))))
	return hex.EncodeToString(sum[:])
}

// StartTOTP кладёт неподтверждённый секрет, затирая прежнюю попытку
func (s *Store) StartTOTP(adminID int64, secret string) error {
	if err := s.gdb.Where("admin_id = ?", adminID).Delete(&dbmodel.AdminTOTP{}).Error; err != nil {
		return err
	}
	return s.gdb.Create(&dbmodel.AdminTOTP{
		AdminID:       adminID,
		Secret:        secret,
		CreatedAtUnix: time.Now().UTC().UnixMilli(),
	}).Error
}

// ConfirmTOTP включает 2FA
func (s *Store) ConfirmTOTP(adminID int64) error {
	return s.gdb.Model(&dbmodel.AdminTOTP{}).Where("admin_id = ?", adminID).
		Update("confirmed_at", time.Now().UTC().UnixMilli()).Error
}

// TOTPFor читает состояние 2FA
func (s *Store) TOTPFor(adminID int64) (TOTPState, error) {
	var row dbmodel.AdminTOTP
	err := s.gdb.Where("admin_id = ?", adminID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return TOTPState{}, ErrNotFound
	}
	if err != nil {
		return TOTPState{}, err
	}
	return TOTPState{Secret: row.Secret, Confirmed: row.ConfirmedAt > 0}, nil
}

// DisableTOTP снимает 2FA вместе с резервными кодами
func (s *Store) DisableTOTP(adminID int64) error {
	if err := s.gdb.Where("admin_id = ?", adminID).Delete(&dbmodel.AdminTOTP{}).Error; err != nil {
		return err
	}
	return s.gdb.Where("admin_id = ?", adminID).Delete(&dbmodel.AdminTOTPBackup{}).Error
}

// SetBackupCodes заменяет набор резервных кодов
func (s *Store) SetBackupCodes(adminID int64, codes []string) error {
	if err := s.gdb.Where("admin_id = ?", adminID).Delete(&dbmodel.AdminTOTPBackup{}).Error; err != nil {
		return err
	}
	for _, code := range codes {
		row := dbmodel.AdminTOTPBackup{AdminID: adminID, CodeHash: hashBackupCode(code)}
		if err := s.gdb.Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

// UseBackupCode гасит код и сообщает, подошёл ли он. Гашение стоит в самом
// UPDATE: два входа одновременно иначе прошли бы по одному коду
func (s *Store) UseBackupCode(adminID int64, code string) bool {
	res := s.gdb.Model(&dbmodel.AdminTOTPBackup{}).
		Where("admin_id = ? AND code_hash = ? AND used_at = 0", adminID, hashBackupCode(code)).
		Update("used_at", time.Now().UTC().UnixMilli())
	return res.Error == nil && res.RowsAffected == 1
}

// BackupCodesLeft - сколько кодов ещё не потрачено
func (s *Store) BackupCodesLeft(adminID int64) int {
	var count int64
	if err := s.gdb.Model(&dbmodel.AdminTOTPBackup{}).
		Where("admin_id = ? AND used_at = 0", adminID).Count(&count).Error; err != nil {
		return 0
	}
	return int(count)
}
