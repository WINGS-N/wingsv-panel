package storage

import (
	"errors"
	"time"

	"gorm.io/gorm"

	"v.wingsnet.org/internal/storage/dbmodel"
)

type Client struct {
	ID                      string
	OwnerAdminID            int64
	Name                    string
	TokenHash               string
	HWID                    string
	DeviceName              string
	DeviceModel             string
	OSVersion               string
	AppVersion              string
	CreatedAt               time.Time
	LastSeenAt              time.Time
	Online                  bool
	LogRuntimeEnabled       bool
	LogProxyEnabled         bool
	LogXRayEnabled          bool
	SyncMode                string
	PeriodicIntervalMinutes int
	HasRootAccess           bool
	VkOAuthAuthorized       bool
	// RemoteControl records the client's management type: true = panel controls
	// the config remotely over Guardian; false = the panel only issues the config
	// once (provision-only). It decides the shape of every regenerated link.
	RemoteControl bool
}

// toStorageClient maps the gorm row model to the app-facing Client, restoring the
// time / bool types and the sync-mode / interval defaults the reads used to apply.
func toStorageClient(m dbmodel.Client) Client {
	c := Client{
		ID:                      m.ID,
		OwnerAdminID:            m.OwnerAdminID,
		Name:                    m.Name,
		TokenHash:               m.TokenHash,
		HWID:                    m.HWID,
		DeviceName:              m.DeviceName,
		DeviceModel:             m.DeviceModel,
		OSVersion:               m.OSVersion,
		AppVersion:              m.AppVersion,
		CreatedAt:               time.UnixMilli(m.CreatedAtUnix).UTC(),
		LastSeenAt:              time.UnixMilli(m.LastSeenAt).UTC(),
		Online:                  m.Online != 0,
		LogRuntimeEnabled:       m.LogRuntimeEnabled != 0,
		LogProxyEnabled:         m.LogProxyEnabled != 0,
		LogXRayEnabled:          m.LogXrayEnabled != 0,
		SyncMode:                m.SyncMode,
		PeriodicIntervalMinutes: int(m.PeriodicIntervalMinutes),
		HasRootAccess:           m.HasRootAccess != 0,
		VkOAuthAuthorized:       m.VKOAuthAuthorized != 0,
		RemoteControl:           m.RemoteControl != 0,
	}
	if c.SyncMode == "" {
		c.SyncMode = "always"
	}
	if c.PeriodicIntervalMinutes <= 0 {
		c.PeriodicIntervalMinutes = 30
	}
	return c
}

func (s *Store) CreateClient(id string, ownerAdminID int64, name, tokenHash string, tokenPlain []byte) (Client, error) {
	// The zero-valued columns carry gorm `default` tags, so they take their schema
	// defaults (sync_mode=always, periodic=30, remote_control=1, ...) on insert.
	m := dbmodel.Client{
		ID:            id,
		OwnerAdminID:  ownerAdminID,
		Name:          name,
		TokenHash:     tokenHash,
		TokenPlain:    tokenPlain,
		CreatedAtUnix: time.Now().UTC().UnixMilli(),
	}
	if err := s.gdb.Create(&m).Error; err != nil {
		return Client{}, err
	}
	return s.FindClientByID(id)
}

// GetClientToken returns the plaintext token stored at client creation, if it
// is still available. Older clients created before token_plain was added
// return ErrNotFound — the admin must regenerate the wingsv:// link by
// recreating the client.
func (s *Store) GetClientToken(id string, ownerAdminID int64) ([]byte, error) {
	var m dbmodel.Client
	err := s.gdb.Select("token_plain").
		Where("id = ? AND owner_admin_id = ?", id, ownerAdminID).
		First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if len(m.TokenPlain) == 0 {
		return nil, ErrNotFound
	}
	return m.TokenPlain, nil
}

// UpdateClientToken replaces token hash + plaintext token. Used by the rotate
// flow — after this, the previous token becomes invalid.
func (s *Store) UpdateClientToken(id string, ownerAdminID int64, tokenHash string, tokenPlain []byte) error {
	res := s.gdb.Model(&dbmodel.Client{}).
		Where("id = ? AND owner_admin_id = ?", id, ownerAdminID).
		// Clearing hwid releases the device binding: rotating a token is how a
		// client is moved to another device, so the next one to present the new
		// token binds instead of being rejected as a mismatch.
		Updates(map[string]any{"token_hash": tokenHash, "token_plain": tokenPlain, "hwid": ""})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteClient(id string, ownerAdminID int64) error {
	res := s.gdb.Where("id = ? AND owner_admin_id = ?", id, ownerAdminID).Delete(&dbmodel.Client{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) FindClientByID(id string) (Client, error) {
	var m dbmodel.Client
	err := s.gdb.Where("id = ?", id).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Client{}, ErrNotFound
	}
	if err != nil {
		return Client{}, err
	}
	return toStorageClient(m), nil
}

func (s *Store) ListClientsByOwner(ownerAdminID int64) ([]Client, error) {
	var ms []dbmodel.Client
	if err := s.gdb.Where("owner_admin_id = ?", ownerAdminID).Order("created_at DESC").Find(&ms).Error; err != nil {
		return nil, err
	}
	return mapStorageClients(ms), nil
}

func (s *Store) ListAllClients() ([]Client, error) {
	var ms []dbmodel.Client
	if err := s.gdb.Order("created_at DESC").Find(&ms).Error; err != nil {
		return nil, err
	}
	return mapStorageClients(ms), nil
}

func mapStorageClients(ms []dbmodel.Client) []Client {
	if len(ms) == 0 {
		return nil
	}
	out := make([]Client, 0, len(ms))
	for _, m := range ms {
		out = append(out, toStorageClient(m))
	}
	return out
}

type ClientCounts struct {
	Total  int
	Online int
}

func (s *Store) CountClients() (ClientCounts, error) {
	return s.countClients(s.gdb.Model(&dbmodel.Client{}))
}

func (s *Store) CountClientsByOwner(ownerAdminID int64) (ClientCounts, error) {
	return s.countClients(s.gdb.Model(&dbmodel.Client{}).Where("owner_admin_id = ?", ownerAdminID))
}

func (s *Store) countClients(q *gorm.DB) (ClientCounts, error) {
	var r struct {
		Total  int
		Online int
	}
	if err := q.Select("COUNT(1) AS total, COALESCE(SUM(online),0) AS online").Scan(&r).Error; err != nil {
		return ClientCounts{}, err
	}
	return ClientCounts{Total: r.Total, Online: r.Online}, nil
}

func (s *Store) UpdateClientPresence(id string, online bool, devInfo *ClientDeviceInfo) error {
	updates := map[string]any{
		"online":       boolToInt(online),
		"last_seen_at": time.Now().UTC().UnixMilli(),
	}
	if devInfo != nil {
		updates["hwid"] = devInfo.HWID
		updates["device_name"] = devInfo.DeviceName
		updates["device_model"] = devInfo.DeviceModel
		updates["os_version"] = devInfo.OSVersion
		updates["app_version"] = devInfo.AppVersion
	}
	return s.gdb.Model(&dbmodel.Client{}).Where("id = ?", id).Updates(updates).Error
}

type ClientDeviceInfo struct {
	HWID        string
	DeviceName  string
	DeviceModel string
	OSVersion   string
	AppVersion  string
}

func (s *Store) UpdateClientLogControl(id string, ownerAdminID int64, runtime, proxy, xray bool) error {
	res := s.gdb.Model(&dbmodel.Client{}).
		Where("id = ? AND owner_admin_id = ?", id, ownerAdminID).
		Updates(map[string]any{
			"log_runtime_enabled": boolToInt(runtime),
			"log_proxy_enabled":   boolToInt(proxy),
			"log_xray_enabled":    boolToInt(xray),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) MarkAllClientsOffline() error {
	// Only the currently-online rows need flipping; the end state (all offline) is
	// identical to an unconditional UPDATE, and the WHERE keeps gorm from blocking
	// a global update.
	return s.gdb.Model(&dbmodel.Client{}).Where("online <> ?", 0).Update("online", 0).Error
}

// UpdateClientRootAccess persists the latest has_root_access signal the device
// sent in its RuntimeState. Panel uses this to hide / strip root-only config
// blocks when the client has no root grant.
func (s *Store) UpdateClientRootAccess(id string, hasRoot bool) error {
	return s.gdb.Model(&dbmodel.Client{}).Where("id = ?", id).Update("has_root_access", boolToInt(hasRoot)).Error
}

// UpdateClientVkOAuthAuthorized persists the latest vk_oauth_authorized signal
// the device sent in its RuntimeState. Panel uses this to gate the
// "Generate VK link" admin button - without an active VK OAuth token on the
// device the command would just bounce with an error.
func (s *Store) UpdateClientVkOAuthAuthorized(id string, authorized bool) error {
	return s.gdb.Model(&dbmodel.Client{}).Where("id = ?", id).Update("vk_oauth_authorized", boolToInt(authorized)).Error
}

func (s *Store) SetClientRemoteControl(id string, ownerAdminID int64, remoteControl bool) error {
	return s.gdb.Model(&dbmodel.Client{}).
		Where("id = ? AND owner_admin_id = ?", id, ownerAdminID).
		Update("remote_control", boolToInt(remoteControl)).Error
}

func (s *Store) UpdateClientSync(id string, ownerAdminID int64, mode string, intervalMinutes int) error {
	if mode == "" {
		mode = "always"
	}
	if intervalMinutes <= 0 {
		intervalMinutes = 30
	}
	res := s.gdb.Model(&dbmodel.Client{}).
		Where("id = ? AND owner_admin_id = ?", id, ownerAdminID).
		Updates(map[string]any{"sync_mode": mode, "periodic_interval_minutes": intervalMinutes})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// UpgradeClientTokenHash rewrites only the stored hash, leaving the token and the
// device binding untouched. Used to migrate a legacy bcrypt row to the current
// digest after that token has already verified.
func (s *Store) UpgradeClientTokenHash(id string, tokenHash string) error {
	return s.gdb.Model(&dbmodel.Client{}).
		Where("id = ?", id).
		Update("token_hash", tokenHash).Error
}
