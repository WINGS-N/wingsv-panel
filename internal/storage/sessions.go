package storage

import (
	"errors"
	"time"

	"gorm.io/gorm"

	"v.wingsnet.org/internal/storage/dbmodel"
)

type AdminSession struct {
	ID        string
	AdminID   int64
	ExpiresAt time.Time
	CreatedAt time.Time
}

func (s *Store) CreateSession(id string, adminID int64, ttl time.Duration) (AdminSession, error) {
	now := time.Now().UTC()
	expiresAt := now.Add(ttl)
	row := dbmodel.AdminSession{
		ID:            id,
		AdminID:       adminID,
		ExpiresAt:     expiresAt.UnixMilli(),
		CreatedAtUnix: now.UnixMilli(),
	}
	if err := s.gdb.Create(&row).Error; err != nil {
		return AdminSession{}, err
	}
	return AdminSession{ID: id, AdminID: adminID, ExpiresAt: expiresAt, CreatedAt: now}, nil
}

func (s *Store) LookupSession(id string) (AdminSession, error) {
	var m dbmodel.AdminSession
	err := s.gdb.Where("id = ?", id).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return AdminSession{}, ErrNotFound
	}
	if err != nil {
		return AdminSession{}, err
	}
	sess := AdminSession{
		ID:        m.ID,
		AdminID:   m.AdminID,
		ExpiresAt: time.UnixMilli(m.ExpiresAt).UTC(),
		CreatedAt: time.UnixMilli(m.CreatedAtUnix).UTC(),
	}
	if time.Now().UTC().After(sess.ExpiresAt) {
		return AdminSession{}, ErrNotFound
	}
	return sess, nil
}

func (s *Store) DeleteSession(id string) error {
	return s.gdb.Where("id = ?", id).Delete(&dbmodel.AdminSession{}).Error
}

func (s *Store) PurgeExpiredSessions() error {
	return s.gdb.Where("expires_at < ?", time.Now().UTC().UnixMilli()).Delete(&dbmodel.AdminSession{}).Error
}
