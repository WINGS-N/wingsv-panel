package storage

import (
	"database/sql"
	"errors"
	"time"
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
	_, err := s.db.Exec(
		`INSERT INTO admin_sessions (id, admin_id, expires_at, created_at) VALUES (?, ?, ?, ?)`,
		id, adminID, expiresAt.UnixMilli(), now.UnixMilli(),
	)
	if err != nil {
		return AdminSession{}, err
	}
	return AdminSession{ID: id, AdminID: adminID, ExpiresAt: expiresAt, CreatedAt: now}, nil
}

func (s *Store) LookupSession(id string) (AdminSession, error) {
	row := s.db.QueryRow(
		`SELECT id, admin_id, expires_at, created_at FROM admin_sessions WHERE id = ?`,
		id,
	)
	var sess AdminSession
	var expiresAt, createdAt int64
	err := row.Scan(&sess.ID, &sess.AdminID, &expiresAt, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return AdminSession{}, ErrNotFound
	}
	if err != nil {
		return AdminSession{}, err
	}
	sess.ExpiresAt = time.UnixMilli(expiresAt).UTC()
	sess.CreatedAt = time.UnixMilli(createdAt).UTC()
	if time.Now().UTC().After(sess.ExpiresAt) {
		return AdminSession{}, ErrNotFound
	}
	return sess, nil
}

func (s *Store) DeleteSession(id string) error {
	_, err := s.db.Exec(`DELETE FROM admin_sessions WHERE id = ?`, id)
	return err
}

func (s *Store) PurgeExpiredSessions() error {
	_, err := s.db.Exec(`DELETE FROM admin_sessions WHERE expires_at < ?`, time.Now().UTC().UnixMilli())
	return err
}
