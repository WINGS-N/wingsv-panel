package storage

import (
	"errors"
	"fmt"
	"log"
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

// clientOwnedTables are every table keyed by client_id. Deleting a client used to
// drop only its own row, so each of these kept the rows forever: unreachable (every
// lookup resolves the client first) but still counted, still dumped, still growing -
// one panel had 32k orphaned log rows carrying most of its database size.
var clientOwnedTables = []string{
	"client_configs",
	"client_installed_apps",
	"client_logs",
	"client_reported_configs",
	"client_runtime",
	"client_traffic",
	"client_wg_peers",
	"pending_commands",
}

// DeleteClient removes the client and everything keyed to it, in one transaction so
// a failure part-way cannot leave the very orphans this is here to prevent.
func (s *Store) DeleteClient(id string, ownerAdminID int64) error {
	return s.gdb.Transaction(func(tx *gorm.DB) error {
		res := tx.Where("id = ? AND owner_admin_id = ?", id, ownerAdminID).Delete(&dbmodel.Client{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		for _, table := range clientOwnedTables {
			if err := tx.Exec("DELETE FROM "+table+" WHERE client_id = ?", id).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// PurgeOrphanClientRows deletes rows left behind by clients removed before the
// delete cascaded. Idempotent, and cheap once the backlog is gone: the DELETEs
// match nothing on a clean database.
func (s *Store) PurgeOrphanClientRows() error {
	total := int64(0)
	for _, table := range clientOwnedTables {
		res := s.gdb.Exec(
			"DELETE FROM " + table + " WHERE client_id NOT IN (SELECT id FROM clients)",
		)
		if res.Error != nil {
			return fmt.Errorf("storage: purge orphans from %s: %w", table, res.Error)
		}
		total += res.RowsAffected
	}
	if total > 0 {
		log.Printf("storage: purged %d orphaned client row(s) left by deletes that did not cascade", total)
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
	clients, _, err := s.PageClientsByOwner(ownerAdminID, 0, 0)
	return clients, err
}

// PageClientsByOwner отдаёт страницу вместе с общим числом. Нулевой limit
// означает всё, как раньше: не каждому вызову нужна страница
func (s *Store) PageClientsByOwner(ownerAdminID int64, limit, offset int) ([]Client, int64, error) {
	return s.pageClients(s.gdb.Where("owner_admin_id = ?", ownerAdminID), limit, offset)
}

func (s *Store) ListAllClients() ([]Client, error) {
	clients, _, err := s.PageAllClients(0, 0)
	return clients, err
}

// PageAllClients - то же для всей платформы
func (s *Store) PageAllClients(limit, offset int) ([]Client, int64, error) {
	return s.pageClients(s.gdb.Session(&gorm.Session{}), limit, offset)
}

func (s *Store) pageClients(q *gorm.DB, limit, offset int) ([]Client, int64, error) {
	var total int64
	if err := q.Model(&dbmodel.Client{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	q = q.Model(&dbmodel.Client{}).Order("created_at DESC")
	if limit > 0 {
		q = q.Limit(limit).Offset(offset)
	}
	var ms []dbmodel.Client
	if err := q.Find(&ms).Error; err != nil {
		return nil, 0, err
	}
	return mapStorageClients(ms), total, nil
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
