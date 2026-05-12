package storage

import (
	"database/sql"
	"errors"
	"time"
)

// MasterConfig is the per-admin "shared across all my clients" set of settings.
// `ScopeFlags` is a comma-separated whitelist of section names (turn,
// xray_settings, xray_routing, byedpi, app_preferences, app_routing, sync) —
// the apply endpoint only touches sections enabled here so the admin can
// bulk-edit, say, only VK TURN credentials without clobbering Xray rules.
type MasterConfig struct {
	AdminID                 int64
	ConfigProto             []byte
	SyncMode                string
	PeriodicIntervalMinutes int
	ScopeFlags              string
	UpdatedAt               time.Time
}

func (s *Store) GetMasterConfig(adminID int64) (MasterConfig, error) {
	row := s.db.QueryRow(
		`SELECT admin_id, config_proto, sync_mode, periodic_interval_minutes, scope_flags, updated_at
		 FROM admin_master_config WHERE admin_id = ?`,
		adminID,
	)
	var m MasterConfig
	var ts int64
	err := row.Scan(&m.AdminID, &m.ConfigProto, &m.SyncMode, &m.PeriodicIntervalMinutes, &m.ScopeFlags, &ts)
	if errors.Is(err, sql.ErrNoRows) {
		return MasterConfig{AdminID: adminID}, nil
	}
	if err != nil {
		return MasterConfig{}, err
	}
	m.UpdatedAt = time.UnixMilli(ts).UTC()
	return m, nil
}

func (s *Store) SaveMasterConfig(m MasterConfig) error {
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO admin_master_config (admin_id, config_proto, sync_mode, periodic_interval_minutes, scope_flags, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(admin_id) DO UPDATE SET
		   config_proto = excluded.config_proto,
		   sync_mode = excluded.sync_mode,
		   periodic_interval_minutes = excluded.periodic_interval_minutes,
		   scope_flags = excluded.scope_flags,
		   updated_at = excluded.updated_at`,
		m.AdminID, m.ConfigProto, m.SyncMode, m.PeriodicIntervalMinutes, m.ScopeFlags, now,
	)
	return err
}
