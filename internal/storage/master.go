package storage

import (
	"errors"
	"time"

	"gorm.io/gorm"
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
	var row dbmodel.AdminMasterConfig
	err := s.gdb.Where("admin_id = ?", adminID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return MasterConfig{AdminID: adminID}, nil
	}
	if err != nil {
		return MasterConfig{}, err
	}
	return MasterConfig{
		AdminID:                 row.AdminID,
		ConfigProto:             row.ConfigProto,
		SyncMode:                row.SyncMode,
		PeriodicIntervalMinutes: int(row.PeriodicIntervalMinutes),
		ScopeFlags:              row.ScopeFlags,
		UpdatedAt:               time.UnixMilli(row.UpdatedAtUnix).UTC(),
	}, nil
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
