package storage

import (
	"database/sql"
	"errors"
	"time"
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
}

func (s *Store) CreateClient(id string, ownerAdminID int64, name, tokenHash string, tokenPlain []byte) (Client, error) {
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO clients (id, owner_admin_id, name, token_hash, token_plain, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		id, ownerAdminID, name, tokenHash, tokenPlain, now,
	)
	if err != nil {
		return Client{}, err
	}
	return s.FindClientByID(id)
}

// GetClientToken returns the plaintext token stored at client creation, if it
// is still available. Older clients created before token_plain was added
// return ErrNotFound — the admin must regenerate the wingsv:// link by
// recreating the client.
func (s *Store) GetClientToken(id string, ownerAdminID int64) ([]byte, error) {
	row := s.db.QueryRow(
		`SELECT token_plain FROM clients WHERE id = ? AND owner_admin_id = ?`,
		id, ownerAdminID,
	)
	var token []byte
	err := row.Scan(&token)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if len(token) == 0 {
		return nil, ErrNotFound
	}
	return token, nil
}

// UpdateClientToken replaces token hash + plaintext token. Used by the rotate
// flow — after this, the previous token becomes invalid.
func (s *Store) UpdateClientToken(id string, ownerAdminID int64, tokenHash string, tokenPlain []byte) error {
	res, err := s.db.Exec(
		`UPDATE clients SET token_hash = ?, token_plain = ? WHERE id = ? AND owner_admin_id = ?`,
		tokenHash, tokenPlain, id, ownerAdminID,
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

func (s *Store) DeleteClient(id string, ownerAdminID int64) error {
	res, err := s.db.Exec(`DELETE FROM clients WHERE id = ? AND owner_admin_id = ?`, id, ownerAdminID)
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

func (s *Store) FindClientByID(id string) (Client, error) {
	row := s.db.QueryRow(`
		SELECT id, owner_admin_id, name, token_hash, hwid, device_name, device_model,
		       os_version, app_version, created_at, last_seen_at, online,
		       log_runtime_enabled, log_proxy_enabled, log_xray_enabled,
		       sync_mode, periodic_interval_minutes,
		       has_root_access
		FROM clients WHERE id = ?`, id)
	return scanClient(row)
}

func (s *Store) ListClientsByOwner(ownerAdminID int64) ([]Client, error) {
	rows, err := s.db.Query(`
		SELECT id, owner_admin_id, name, token_hash, hwid, device_name, device_model,
		       os_version, app_version, created_at, last_seen_at, online,
		       log_runtime_enabled, log_proxy_enabled, log_xray_enabled,
		       sync_mode, periodic_interval_minutes,
		       has_root_access
		FROM clients WHERE owner_admin_id = ? ORDER BY created_at DESC`, ownerAdminID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Client
	for rows.Next() {
		c, err := scanClientRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) ListAllClients() ([]Client, error) {
	rows, err := s.db.Query(`
		SELECT id, owner_admin_id, name, token_hash, hwid, device_name, device_model,
		       os_version, app_version, created_at, last_seen_at, online,
		       log_runtime_enabled, log_proxy_enabled, log_xray_enabled,
		       sync_mode, periodic_interval_minutes,
		       has_root_access
		FROM clients ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Client
	for rows.Next() {
		c, err := scanClientRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

type ClientCounts struct {
	Total  int
	Online int
}

func (s *Store) CountClients() (ClientCounts, error) {
	var c ClientCounts
	row := s.db.QueryRow(`SELECT COUNT(1), COALESCE(SUM(online), 0) FROM clients`)
	if err := row.Scan(&c.Total, &c.Online); err != nil {
		return ClientCounts{}, err
	}
	return c, nil
}

func (s *Store) CountClientsByOwner(ownerAdminID int64) (ClientCounts, error) {
	var c ClientCounts
	row := s.db.QueryRow(
		`SELECT COUNT(1), COALESCE(SUM(online), 0) FROM clients WHERE owner_admin_id = ?`,
		ownerAdminID,
	)
	if err := row.Scan(&c.Total, &c.Online); err != nil {
		return ClientCounts{}, err
	}
	return c, nil
}

func (s *Store) UpdateClientPresence(id string, online bool, devInfo *ClientDeviceInfo) error {
	now := time.Now().UTC().UnixMilli()
	if devInfo != nil {
		_, err := s.db.Exec(`
			UPDATE clients SET online = ?, last_seen_at = ?,
			                   hwid = ?, device_name = ?, device_model = ?, os_version = ?, app_version = ?
			WHERE id = ?`,
			boolToInt(online), now,
			devInfo.HWID, devInfo.DeviceName, devInfo.DeviceModel, devInfo.OSVersion, devInfo.AppVersion,
			id)
		return err
	}
	_, err := s.db.Exec(`UPDATE clients SET online = ?, last_seen_at = ? WHERE id = ?`,
		boolToInt(online), now, id)
	return err
}

type ClientDeviceInfo struct {
	HWID        string
	DeviceName  string
	DeviceModel string
	OSVersion   string
	AppVersion  string
}

func (s *Store) UpdateClientLogControl(id string, ownerAdminID int64, runtime, proxy, xray bool) error {
	res, err := s.db.Exec(`
		UPDATE clients SET log_runtime_enabled = ?, log_proxy_enabled = ?, log_xray_enabled = ?
		WHERE id = ? AND owner_admin_id = ?`,
		boolToInt(runtime), boolToInt(proxy), boolToInt(xray), id, ownerAdminID)
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

func (s *Store) MarkAllClientsOffline() error {
	_, err := s.db.Exec(`UPDATE clients SET online = 0`)
	return err
}

// UpdateClientRootAccess persists the latest has_root_access signal the device
// sent in its RuntimeState. Panel uses this to hide / strip root-only config
// blocks when the client has no root grant.
func (s *Store) UpdateClientRootAccess(id string, hasRoot bool) error {
	_, err := s.db.Exec(`UPDATE clients SET has_root_access = ? WHERE id = ?`, boolToInt(hasRoot), id)
	return err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanClient(row *sql.Row) (Client, error) {
	c, err := scanClientFromScanner(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Client{}, ErrNotFound
	}
	return c, err
}

func scanClientRows(rows *sql.Rows) (Client, error) {
	return scanClientFromScanner(rows)
}

func scanClientFromScanner(scanner rowScanner) (Client, error) {
	var c Client
	var createdAt, lastSeenAt int64
	var online, logRuntime, logProxy, logXRay, hasRoot int
	err := scanner.Scan(
		&c.ID, &c.OwnerAdminID, &c.Name, &c.TokenHash,
		&c.HWID, &c.DeviceName, &c.DeviceModel, &c.OSVersion, &c.AppVersion,
		&createdAt, &lastSeenAt, &online,
		&logRuntime, &logProxy, &logXRay,
		&c.SyncMode, &c.PeriodicIntervalMinutes,
		&hasRoot,
	)
	if err != nil {
		return Client{}, err
	}
	c.CreatedAt = time.UnixMilli(createdAt).UTC()
	c.LastSeenAt = time.UnixMilli(lastSeenAt).UTC()
	c.Online = online != 0
	c.LogRuntimeEnabled = logRuntime != 0
	c.LogProxyEnabled = logProxy != 0
	c.LogXRayEnabled = logXRay != 0
	c.HasRootAccess = hasRoot != 0
	if c.SyncMode == "" {
		c.SyncMode = "always"
	}
	if c.PeriodicIntervalMinutes <= 0 {
		c.PeriodicIntervalMinutes = 30
	}
	return c, nil
}

func (s *Store) UpdateClientSync(id string, ownerAdminID int64, mode string, intervalMinutes int) error {
	if mode == "" {
		mode = "always"
	}
	if intervalMinutes <= 0 {
		intervalMinutes = 30
	}
	res, err := s.db.Exec(`
		UPDATE clients SET sync_mode = ?, periodic_interval_minutes = ?
		WHERE id = ? AND owner_admin_id = ?`,
		mode, intervalMinutes, id, ownerAdminID)
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
