package storage

import (
	"database/sql"
	"errors"
	"time"

	"gorm.io/gorm/clause"

	"v.wingsnet.org/internal/storage/dbmodel"
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
	row := s.queryRow(
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
	row := dbmodel.AdminMasterConfig{
		AdminID:                 m.AdminID,
		ConfigProto:             m.ConfigProto,
		SyncMode:                m.SyncMode,
		PeriodicIntervalMinutes: int64(m.PeriodicIntervalMinutes),
		ScopeFlags:              m.ScopeFlags,
		UpdatedAtUnix:           time.Now().UTC().UnixMilli(),
	}
	return s.gdb.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "admin_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"config_proto", "sync_mode", "periodic_interval_minutes", "scope_flags", "updated_at",
		}),
	}).Create(&row).Error
}
