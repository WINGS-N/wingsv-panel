package storage

import (
	"database/sql"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"v.wingsnet.org/internal/storage/dbmodel"
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
	row := dbmodel.ClientConfig{ClientID: clientID, ConfigProto: configProto, Revision: revision, UpdatedAtUnix: now, ConfigVersion: 1}
	if err := s.gdb.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "client_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"config_proto":   configProto,
			"revision":       revision,
			"updated_at":     now,
			"config_version": gorm.Expr("client_configs.config_version + 1"),
		}),
	}).Create(&row).Error; err != nil {
		return 0, err
	}
	var version uint64
	if err := s.gdb.Model(&dbmodel.ClientConfig{}).Where("client_id = ?", clientID).Pluck("config_version", &version).Error; err != nil {
		return 0, err
	}
	return version, nil
}

func (s *Store) GetClientConfig(clientID string) (ClientConfig, error) {
	row := s.queryRow(`SELECT client_id, config_proto, revision, updated_at, config_version FROM client_configs WHERE client_id = ?`, clientID)
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
	return s.gdb.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "client_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"config_proto", "updated_at"}),
	}).Create(&dbmodel.ClientReportedConfig{
		ClientID: clientID, ConfigProto: configProto, UpdatedAtUnix: time.Now().UTC().UnixMilli(),
	}).Error
}

func (s *Store) UpsertClientRuntime(clientID string, runtimeProto []byte) error {
	return s.gdb.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "client_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"runtime_proto", "updated_at"}),
	}).Create(&dbmodel.ClientRuntime{
		ClientID: clientID, RuntimeProto: runtimeProto, UpdatedAtUnix: time.Now().UTC().UnixMilli(),
	}).Error
}

func (s *Store) GetClientRuntime(clientID string) ([]byte, time.Time, error) {
	row := s.queryRow(`SELECT runtime_proto, updated_at FROM client_runtime WHERE client_id = ?`, clientID)
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
	return s.gdb.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "client_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"apps_proto", "updated_at"}),
	}).Create(&dbmodel.ClientInstalledApps{
		ClientID: clientID, AppsProto: appsProto, UpdatedAtUnix: time.Now().UTC().UnixMilli(),
	}).Error
}

func (s *Store) GetClientInstalledApps(clientID string) ([]byte, time.Time, error) {
	row := s.queryRow(`SELECT apps_proto, updated_at FROM client_installed_apps WHERE client_id = ?`, clientID)
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
	packages := make([]string, 0, len(items))
	for _, item := range items {
		if item.Package != "" {
			packages = append(packages, item.Package)
		}
	}
	// Preserve the stored label / icon when the incoming value is empty. Done in
	// Go rather than a dialect-specific CASE/NULLIF upsert expression.
	existing, err := s.GetPackageMetadataMap(packages)
	if err != nil {
		return err
	}
	now := time.Now().UTC().UnixMilli()
	rows := make([]dbmodel.PackageMetadata, 0, len(items))
	for _, item := range items {
		if item.Package == "" {
			continue
		}
		label, icon := item.Label, item.IconPNG
		if prev, ok := existing[item.Package]; ok {
			if label == "" {
				label = prev.Label
			}
			if len(icon) == 0 {
				icon = prev.IconPNG
			}
		}
		rows = append(rows, dbmodel.PackageMetadata{
			Package: item.Package, Label: label, IconPNG: icon, UpdatedAtUnix: now,
		})
	}
	if len(rows) == 0 {
		return nil
	}
	return s.gdb.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "package"}},
		DoUpdates: clause.AssignmentColumns([]string{"label", "icon_png", "updated_at"}),
	}).CreateInBatches(rows, 100).Error
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
	rows, err := s.query(`SELECT package, label, icon_png FROM package_metadata WHERE package IN (`+string(placeholders)+`)`, args...)
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
	row := s.queryRow(`SELECT config_proto, updated_at FROM client_reported_configs WHERE client_id = ?`, clientID)
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
