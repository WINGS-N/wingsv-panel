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

// WGBackend values for a vk-turn-proxy node's managed provisioning (ServerNode.
// WGBackend): "own" mints a peer on the node's own wg interface via the relay
// management gRPC; "xui" mints a client on the node's configured 3x-ui inbound.
// Empty falls back to the panel-global XUI_WG_* default.
const (
	WGBackendOwn = "own"
	WGBackendXUI = "xui"
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

// ListServerNodesByOwner returns nodes of the given kind owned by ownerAdminID
// (0 = panel-local nodes the owner manages; a positive id = an admin's own
// external endpoints). An empty kind matches every kind.
func (s *Store) ListServerNodesByOwner(kind string, ownerAdminID int64) ([]dbmodel.ServerNode, error) {
	q := s.gdb.Order("created_at asc").Where("owner_admin_id = ?", ownerAdminID)
	if kind != "" {
		q = q.Where("kind = ?", kind)
	}
	var nodes []dbmodel.ServerNode
	if err := q.Find(&nodes).Error; err != nil {
		return nil, err
	}
	return nodes, nil
}

// ListServerNodesByOwners lists nodes owned by any of the given admin ids. It lets
// the owner see both their own nodes and the panel-local (owner_admin_id 0) ones
// in a single view.
func (s *Store) ListServerNodesByOwners(kind string, ownerIDs []int64) ([]dbmodel.ServerNode, error) {
	if len(ownerIDs) == 0 {
		return nil, nil
	}
	q := s.gdb.Order("created_at asc").Where("owner_admin_id IN ?", ownerIDs)
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

// UpdateServerNode updates a node's editable fields (name, endpoint, token, and
// the wg-backend provisioning config). Uses a column map so a field can be
// cleared back to its zero value (e.g. wg_backend -> "").
func (s *Store) UpdateServerNode(n dbmodel.ServerNode) error {
	res := s.gdb.Model(&dbmodel.ServerNode{}).Where("id = ?", n.ID).Updates(map[string]any{
		"name":            n.Name,
		"grpc_endpoint":   n.GRPCEndpoint,
		"grpc_token":      n.GRPCToken,
		"wg_backend":      n.WGBackend,
		"xui_node_id":     n.XuiNodeID,
		"xui_inbound_tag": n.XuiInboundTag,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
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

// UpdateXuiNodeStatus records a 3x-ui node's reachability plus its Xray core
// state and version, so the UI can show whether Xray is running.
// UpdateServerNodeRelayVersion records a vk-turn-proxy node's relay build version
// (git tag) as polled from its management gRPC. No-op on an empty version.
func (s *Store) UpdateServerNodeRelayVersion(id, version string) error {
	if version == "" {
		return nil
	}
	return s.gdb.Model(&dbmodel.ServerNode{}).
		Where("id = ?", id).
		Update("relay_version", version).Error
}

func (s *Store) UpdateXuiNodeStatus(id, status, xrayState, xrayVersion string, lastSeen int64) error {
	res := s.gdb.Model(&dbmodel.ServerNode{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status": status, "last_seen_at": lastSeen,
			"xray_state": xrayState, "xray_version": xrayVersion,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
