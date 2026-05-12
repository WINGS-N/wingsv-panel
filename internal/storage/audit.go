package storage

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

const (
	SettingRegistrationMode = "registration_mode"
)

type AuditEntry struct {
	ID            int64
	TS            time.Time
	ActorAdminID  int64
	ActorUsername string
	Action        string
	TargetType    string
	TargetID      string
	Message       string
	IP            string
}

type AuditFilter struct {
	ActorAdminID int64
	Action       string
	Since        time.Time
	Until        time.Time
	Limit        int
}

func (s *Store) AppendAudit(entry AuditEntry) error {
	if entry.Action == "" {
		return errors.New("storage: audit action required")
	}
	now := time.Now().UTC().UnixMilli()
	if !entry.TS.IsZero() {
		now = entry.TS.UTC().UnixMilli()
	}
	var actor sql.NullInt64
	if entry.ActorAdminID > 0 {
		actor = sql.NullInt64{Int64: entry.ActorAdminID, Valid: true}
	}
	_, err := s.db.Exec(
		`INSERT INTO audit_log (ts, actor_admin_id, actor_username, action, target_type, target_id, message, ip)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		now, actor, entry.ActorUsername, entry.Action, entry.TargetType, entry.TargetID, entry.Message, entry.IP,
	)
	return err
}

func (s *Store) ListAudit(filter AuditFilter) ([]AuditEntry, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := `SELECT id, ts, COALESCE(actor_admin_id, 0), actor_username, action, target_type, target_id, message, ip
	      FROM audit_log WHERE 1=1`
	var args []any
	if filter.ActorAdminID > 0 {
		q += ` AND actor_admin_id = ?`
		args = append(args, filter.ActorAdminID)
	}
	if filter.Action != "" {
		q += ` AND action = ?`
		args = append(args, filter.Action)
	}
	if !filter.Since.IsZero() {
		q += ` AND ts >= ?`
		args = append(args, filter.Since.UTC().UnixMilli())
	}
	if !filter.Until.IsZero() {
		q += ` AND ts <= ?`
		args = append(args, filter.Until.UTC().UnixMilli())
	}
	q += ` ORDER BY ts DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuditEntry
	for rows.Next() {
		var e AuditEntry
		var ts int64
		if err := rows.Scan(&e.ID, &ts, &e.ActorAdminID, &e.ActorUsername, &e.Action,
			&e.TargetType, &e.TargetID, &e.Message, &e.IP); err != nil {
			return nil, err
		}
		e.TS = time.UnixMilli(ts).UTC()
		out = append(out, e)
	}
	return out, rows.Err()
}

// PruneAuditOlderThan deletes entries older than `cutoff`. Caller decides cadence.
func (s *Store) PruneAuditOlderThan(cutoff time.Time) error {
	_, err := s.db.Exec(`DELETE FROM audit_log WHERE ts < ?`, cutoff.UTC().UnixMilli())
	return err
}

// ===== platform_settings =====

func (s *Store) GetPlatformSetting(key, fallback string) (string, error) {
	var val string
	err := s.db.QueryRow(`SELECT value FROM platform_settings WHERE key = ?`, key).Scan(&val)
	if errors.Is(err, sql.ErrNoRows) {
		return fallback, nil
	}
	if err != nil {
		return "", err
	}
	return val, nil
}

func (s *Store) SetPlatformSetting(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO platform_settings (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value,
	)
	return err
}

// ===== invite_tokens =====

type InviteToken struct {
	Token            string
	CreatedAt        time.Time
	ExpiresAt        time.Time
	UsedAt           time.Time
	UsedByAdminID    int64
	CreatedByAdminID int64
}

func (s *Store) CreateInvite(token string, expiresAt time.Time, createdByAdminID int64) (InviteToken, error) {
	now := time.Now().UTC().UnixMilli()
	exp := int64(0)
	if !expiresAt.IsZero() {
		exp = expiresAt.UTC().UnixMilli()
	}
	_, err := s.db.Exec(
		`INSERT INTO invite_tokens (token, created_at, expires_at, created_by_admin_id) VALUES (?, ?, ?, ?)`,
		token, now, exp, createdByAdminID,
	)
	if err != nil {
		return InviteToken{}, err
	}
	return InviteToken{
		Token:            token,
		CreatedAt:        time.UnixMilli(now).UTC(),
		ExpiresAt:        time.UnixMilli(exp).UTC(),
		CreatedByAdminID: createdByAdminID,
	}, nil
}

func (s *Store) ListInvites(includeUsed bool) ([]InviteToken, error) {
	q := `SELECT token, created_at, COALESCE(expires_at, 0), COALESCE(used_at, 0),
	             COALESCE(used_by_admin_id, 0), COALESCE(created_by_admin_id, 0)
	      FROM invite_tokens`
	if !includeUsed {
		q += ` WHERE used_at = 0`
	}
	q += ` ORDER BY created_at DESC LIMIT 200`
	rows, err := s.db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []InviteToken
	for rows.Next() {
		var it InviteToken
		var c, exp, used int64
		if err := rows.Scan(&it.Token, &c, &exp, &used, &it.UsedByAdminID, &it.CreatedByAdminID); err != nil {
			return nil, err
		}
		it.CreatedAt = time.UnixMilli(c).UTC()
		it.ExpiresAt = time.UnixMilli(exp).UTC()
		it.UsedAt = time.UnixMilli(used).UTC()
		out = append(out, it)
	}
	return out, rows.Err()
}

func (s *Store) DeleteInvite(token string) error {
	_, err := s.db.Exec(`DELETE FROM invite_tokens WHERE token = ?`, token)
	return err
}

// RedeemInvite marks the token as used by the given admin. Returns ErrNotFound
// if the token doesn't exist, is expired, or already used.
func (s *Store) RedeemInvite(token string, adminID int64) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return ErrNotFound
	}
	now := time.Now().UTC().UnixMilli()
	res, err := s.db.Exec(
		`UPDATE invite_tokens SET used_at = ?, used_by_admin_id = ?
		 WHERE token = ? AND used_at = 0
		 AND (expires_at = 0 OR expires_at > ?)`,
		now, adminID, token, now,
	)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}
