package storage

import (
	"database/sql"
	"errors"
	"time"
)

type ClientConfig struct {
	ClientID      string
	ConfigProto   []byte
	Revision      string
	UpdatedAt     time.Time
	ConfigVersion uint64
}

// UpsertClientConfig сохраняет конфиг и инкрементирует config_version. Returned
// version — это новое значение, которое нужно прокинуть в proto перед отправкой
// клиенту: устройство должно сохранить ту же версию, чтобы на reconnect-welcome
// сервер не «перезатёр» свежий админский редактор старым DB-снимком.
func (s *Store) UpsertClientConfig(clientID string, configProto []byte, revision string) (uint64, error) {
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(`
		INSERT INTO client_configs (client_id, config_proto, revision, updated_at, config_version)
		VALUES (?, ?, ?, ?, 1)
		ON CONFLICT(client_id) DO UPDATE SET
			config_proto = excluded.config_proto,
			revision = excluded.revision,
			updated_at = excluded.updated_at,
			config_version = client_configs.config_version + 1`,
		clientID, configProto, revision, now)
	if err != nil {
		return 0, err
	}
	var version uint64
	if err := s.db.QueryRow(`SELECT config_version FROM client_configs WHERE client_id = ?`, clientID).Scan(&version); err != nil {
		return 0, err
	}
	return version, nil
}

func (s *Store) GetClientConfig(clientID string) (ClientConfig, error) {
	row := s.db.QueryRow(`SELECT client_id, config_proto, revision, updated_at, config_version FROM client_configs WHERE client_id = ?`, clientID)
	var c ClientConfig
	var updatedAt int64
	err := row.Scan(&c.ClientID, &c.ConfigProto, &c.Revision, &updatedAt, &c.ConfigVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return ClientConfig{}, ErrNotFound
	}
	if err != nil {
		return ClientConfig{}, err
	}
	c.UpdatedAt = time.UnixMilli(updatedAt).UTC()
	return c, nil
}

func (s *Store) UpsertClientReportedConfig(clientID string, configProto []byte) error {
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(`
		INSERT INTO client_reported_configs (client_id, config_proto, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(client_id) DO UPDATE SET config_proto = excluded.config_proto, updated_at = excluded.updated_at`,
		clientID, configProto, now)
	return err
}

func (s *Store) UpsertClientRuntime(clientID string, runtimeProto []byte) error {
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(`
		INSERT INTO client_runtime (client_id, runtime_proto, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(client_id) DO UPDATE SET runtime_proto = excluded.runtime_proto, updated_at = excluded.updated_at`,
		clientID, runtimeProto, now)
	return err
}

func (s *Store) GetClientRuntime(clientID string) ([]byte, time.Time, error) {
	row := s.db.QueryRow(`SELECT runtime_proto, updated_at FROM client_runtime WHERE client_id = ?`, clientID)
	var b []byte
	var updatedAt int64
	err := row.Scan(&b, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, time.Time{}, ErrNotFound
	}
	if err != nil {
		return nil, time.Time{}, err
	}
	return b, time.UnixMilli(updatedAt).UTC(), nil
}

type PackageMetadata struct {
	Package string
	Label   string
	IconPNG []byte
}

func (s *Store) UpsertClientInstalledApps(clientID string, appsProto []byte) error {
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(`
		INSERT INTO client_installed_apps (client_id, apps_proto, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(client_id) DO UPDATE SET apps_proto = excluded.apps_proto, updated_at = excluded.updated_at`,
		clientID, appsProto, now)
	return err
}

func (s *Store) GetClientInstalledApps(clientID string) ([]byte, time.Time, error) {
	row := s.db.QueryRow(`SELECT apps_proto, updated_at FROM client_installed_apps WHERE client_id = ?`, clientID)
	var b []byte
	var updatedAt int64
	err := row.Scan(&b, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, time.Time{}, ErrNotFound
	}
	if err != nil {
		return nil, time.Time{}, err
	}
	return b, time.UnixMilli(updatedAt).UTC(), nil
}

func (s *Store) UpsertPackageMetadata(items []PackageMetadata) error {
	if len(items) == 0 {
		return nil
	}
	now := time.Now().UTC().UnixMilli()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.Prepare(`
		INSERT INTO package_metadata (package, label, icon_png, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(package) DO UPDATE SET
			label = CASE WHEN excluded.label != '' THEN excluded.label ELSE package_metadata.label END,
			icon_png = COALESCE(NULLIF(excluded.icon_png, X''), package_metadata.icon_png),
			updated_at = excluded.updated_at`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, item := range items {
		if item.Package == "" {
			continue
		}
		if _, err := stmt.Exec(item.Package, item.Label, item.IconPNG, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) GetPackageMetadataMap(packages []string) (map[string]PackageMetadata, error) {
	if len(packages) == 0 {
		return map[string]PackageMetadata{}, nil
	}
	placeholders := make([]byte, 0, 2*len(packages)-1)
	args := make([]any, 0, len(packages))
	for i, p := range packages {
		if i > 0 {
			placeholders = append(placeholders, ',')
		}
		placeholders = append(placeholders, '?')
		args = append(args, p)
	}
	rows, err := s.db.Query(`SELECT package, label, icon_png FROM package_metadata WHERE package IN (`+string(placeholders)+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]PackageMetadata{}
	for rows.Next() {
		var m PackageMetadata
		if err := rows.Scan(&m.Package, &m.Label, &m.IconPNG); err != nil {
			return nil, err
		}
		out[m.Package] = m
	}
	return out, rows.Err()
}

func (s *Store) GetClientReportedConfig(clientID string) ([]byte, time.Time, error) {
	row := s.db.QueryRow(`SELECT config_proto, updated_at FROM client_reported_configs WHERE client_id = ?`, clientID)
	var b []byte
	var updatedAt int64
	err := row.Scan(&b, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, time.Time{}, ErrNotFound
	}
	if err != nil {
		return nil, time.Time{}, err
	}
	return b, time.UnixMilli(updatedAt).UTC(), nil
}
