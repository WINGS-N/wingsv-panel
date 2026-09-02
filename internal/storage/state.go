package storage

import (
	"encoding/json"
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
	// TouchedFields - путь поля к версии, в которой его меняли
	TouchedFields map[string]int64
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
	var m dbmodel.ClientConfig
	err := s.gdb.Where("client_id = ?", clientID).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ClientConfig{}, ErrNotFound
	}
	if err != nil {
		return ClientConfig{}, err
	}
	return ClientConfig{
		ClientID:      m.ClientID,
		ConfigProto:   m.ConfigProto,
		Revision:      m.Revision,
		UpdatedAt:     time.UnixMilli(m.UpdatedAtUnix).UTC(),
		ConfigVersion: uint64(m.ConfigVersion),
		TouchedFields: decodeTouched(m.TouchedFields),
	}, nil
}

// decodeTouched читает отметки о правках. Битую карту не жалко: без неё
// конфликтов просто не видно, а конфиг остаётся целым
func decodeTouched(raw string) map[string]int64 {
	if raw == "" {
		return nil
	}
	out := map[string]int64{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

// UpsertClientConfigPatched сохраняет конфиг и отмечает тронутые поля
func (s *Store) UpsertClientConfigPatched(
	clientID string,
	configProto []byte,
	revision string,
	touched map[string]int64,
) (uint64, error) {
	version, err := s.UpsertClientConfig(clientID, configProto, revision)
	if err != nil {
		return 0, err
	}
	encoded, err := json.Marshal(touched)
	if err != nil {
		return version, err
	}
	err = s.gdb.Model(&dbmodel.ClientConfig{}).
		Where("client_id = ?", clientID).
		Update("touched_fields", string(encoded)).Error
	return version, err
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
	var m dbmodel.ClientRuntime
	err := s.gdb.Where("client_id = ?", clientID).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, time.Time{}, ErrNotFound
	}
	if err != nil {
		return nil, time.Time{}, err
	}
	return m.RuntimeProto, time.UnixMilli(m.UpdatedAtUnix).UTC(), nil
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
	var m dbmodel.ClientInstalledApps
	err := s.gdb.Where("client_id = ?", clientID).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, time.Time{}, ErrNotFound
	}
	if err != nil {
		return nil, time.Time{}, err
	}
	return m.AppsProto, time.UnixMilli(m.UpdatedAtUnix).UTC(), nil
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
	var rows []dbmodel.PackageMetadata
	if err := s.gdb.Select("package", "label", "icon_png").
		Where("package IN ?", packages).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]PackageMetadata, len(rows))
	for _, r := range rows {
		out[r.Package] = PackageMetadata{Package: r.Package, Label: r.Label, IconPNG: r.IconPNG}
	}
	return out, nil
}

func (s *Store) GetClientReportedConfig(clientID string) ([]byte, time.Time, error) {
	var m dbmodel.ClientReportedConfig
	err := s.gdb.Where("client_id = ?", clientID).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, time.Time{}, ErrNotFound
	}
	if err != nil {
		return nil, time.Time{}, err
	}
	return m.ConfigProto, time.UnixMilli(m.UpdatedAtUnix).UTC(), nil
}

// ListClientConfigs returns every stored client config. Used by the startup
// migration that re-points managed VK-TURN profiles at registered relays.
func (s *Store) ListClientConfigs() ([]ClientConfig, error) {
	var rows []dbmodel.ClientConfig
	if err := s.gdb.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]ClientConfig, 0, len(rows))
	for _, m := range rows {
		out = append(out, ClientConfig{
			ClientID:      m.ClientID,
			ConfigProto:   m.ConfigProto,
			Revision:      m.Revision,
			UpdatedAt:     time.UnixMilli(m.UpdatedAtUnix).UTC(),
			ConfigVersion: uint64(m.ConfigVersion),
		})
	}
	return out, nil
}
