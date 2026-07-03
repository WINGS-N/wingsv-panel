package storage

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"v.wingsnet.org/internal/storage/dbmodel"
)

// PanelMode selects what the panel manages. It is stored in platform_settings
// and defaults to the control-panel behavior for existing installs.
type PanelMode string

const (
	PanelModeControlPanel PanelMode = "control_panel"
	PanelModeWGAWG        PanelMode = "wg_awg"
	PanelModeXUIAPI       PanelMode = "xui_api"

	panelModeSettingKey = "panel_mode"
)

// ServerNode kinds.
const (
	ServerNodeVKTurnProxy = "vk_turn_proxy"
	ServerNodeXUI         = "xui"
)

func (s *Store) GetPanelMode() (PanelMode, error) {
	var row dbmodel.PlatformSetting
	err := s.gdb.Where("key = ?", panelModeSettingKey).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || row.Value == "" {
		return PanelModeControlPanel, nil
	}
	if err != nil {
		return "", err
	}
	return PanelMode(row.Value), nil
}

func (s *Store) SetPanelMode(mode PanelMode) error {
	row := dbmodel.PlatformSetting{Key: panelModeSettingKey, Value: string(mode)}
	return s.gdb.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value"}),
	}).Create(&row).Error
}

func (s *Store) CreateServerNode(n dbmodel.ServerNode) (dbmodel.ServerNode, error) {
	if n.CreatedAtUnix == 0 {
		n.CreatedAtUnix = time.Now().Unix()
	}
	if n.Status == "" {
		n.Status = "unknown"
	}
	if err := s.gdb.Create(&n).Error; err != nil {
		return dbmodel.ServerNode{}, err
	}
	return n, nil
}

func (s *Store) ListServerNodes(kind string) ([]dbmodel.ServerNode, error) {
	q := s.gdb.Order("created_at asc")
	if kind != "" {
		q = q.Where("kind = ?", kind)
	}
	var nodes []dbmodel.ServerNode
	if err := q.Find(&nodes).Error; err != nil {
		return nil, err
	}
	return nodes, nil
}

func (s *Store) GetServerNode(id string) (dbmodel.ServerNode, error) {
	var n dbmodel.ServerNode
	err := s.gdb.Where("id = ?", id).First(&n).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return dbmodel.ServerNode{}, ErrNotFound
	}
	if err != nil {
		return dbmodel.ServerNode{}, err
	}
	return n, nil
}

func (s *Store) DeleteServerNode(id string) error {
	res := s.gdb.Where("id = ?", id).Delete(&dbmodel.ServerNode{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) UpdateServerNodeStatus(id, status string, lastSeen int64) error {
	res := s.gdb.Model(&dbmodel.ServerNode{}).
		Where("id = ?", id).
		Updates(map[string]any{"status": status, "last_seen_at": lastSeen})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
