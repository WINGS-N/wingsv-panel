package storage

import (
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"time"

	"gorm.io/gorm"

	"v.wingsnet.org/internal/storage/dbmodel"
)

// AppSession - сессия приложения на одном устройстве
type AppSession struct {
	ID         string
	AdminID    int64
	DeviceName string
	CreatedAt  time.Time
	LastSeenAt time.Time
	ExpiresAt  time.Time
}

// HashAppToken сводит токен к хешу, в котором он и хранится
func HashAppToken(token string) string {
	sum := sha512.Sum512_256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// CreateAppSession заводит сессию под уже выданный токен
func (s *Store) CreateAppSession(id, token string, adminID int64, deviceName string, ttl time.Duration) (AppSession, error) {
	now := time.Now().UTC()
	row := dbmodel.AppSession{
		ID:            id,
		AdminID:       adminID,
		TokenHash:     HashAppToken(token),
		DeviceName:    deviceName,
		CreatedAtUnix: now.UnixMilli(),
		LastSeenAt:    now.UnixMilli(),
		ExpiresAt:     now.Add(ttl).UnixMilli(),
	}
	if err := s.gdb.Create(&row).Error; err != nil {
		return AppSession{}, err
	}
	return toAppSession(row), nil
}

// LookupAppSession находит живую сессию по токену и отмечает её свежей
func (s *Store) LookupAppSession(token string) (AppSession, error) {
	var row dbmodel.AppSession
	err := s.gdb.Where("token_hash = ?", HashAppToken(token)).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return AppSession{}, ErrNotFound
	}
	if err != nil {
		return AppSession{}, err
	}
	now := time.Now().UTC()
	if row.ExpiresAt > 0 && now.UnixMilli() > row.ExpiresAt {
		_ = s.DeleteAppSession(row.ID)
		return AppSession{}, ErrNotFound
	}
	// Отметка живости нужна экрану устройств: сессия, молчащая месяц, это
	// потерянный телефон, и владелец должен видеть, какую отзывать
	_ = s.gdb.Model(&dbmodel.AppSession{}).Where("id = ?", row.ID).
		Update("last_seen_at", now.UnixMilli()).Error
	row.LastSeenAt = now.UnixMilli()
	return toAppSession(row), nil
}

// ListAppSessions перечисляет устройства аккаунта
func (s *Store) ListAppSessions(adminID int64) ([]AppSession, error) {
	var rows []dbmodel.AppSession
	if err := s.gdb.Where("admin_id = ?", adminID).Order("last_seen_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]AppSession, 0, len(rows))
	for _, row := range rows {
		out = append(out, toAppSession(row))
	}
	return out, nil
}

// DeleteAppSession отзывает одну сессию
func (s *Store) DeleteAppSession(id string) error {
	return s.gdb.Where("id = ?", id).Delete(&dbmodel.AppSession{}).Error
}

// PurgeExpiredAppSessions чистит протухшие
func (s *Store) PurgeExpiredAppSessions() error {
	return s.gdb.Where("expires_at > 0 AND expires_at < ?", time.Now().UTC().UnixMilli()).
		Delete(&dbmodel.AppSession{}).Error
}

func toAppSession(row dbmodel.AppSession) AppSession {
	return AppSession{
		ID:         row.ID,
		AdminID:    row.AdminID,
		DeviceName: row.DeviceName,
		CreatedAt:  time.UnixMilli(row.CreatedAtUnix).UTC(),
		LastSeenAt: time.UnixMilli(row.LastSeenAt).UTC(),
		ExpiresAt:  time.UnixMilli(row.ExpiresAt).UTC(),
	}
}
