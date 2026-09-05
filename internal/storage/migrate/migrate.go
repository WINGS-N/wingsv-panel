// Package migrate copies every panel table from one database backend to another
// (sqlite / pgsql / mariadb, either direction), the engine behind the
// "wingsv-panel db migrate" command. It reads through the shared dbmodel row
// models, so the copy is dialect-agnostic and stays correct as the schema grows.
package migrate

import (
	"fmt"
	"reflect"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"

	"v.wingsnet.org/internal/storage"
	"v.wingsnet.org/internal/storage/dbmodel"
)

// Result reports how many rows were copied for one table.
type Result struct {
	Table string
	Rows  int64
}

// Run opens both backends, ensures the destination schema exists, and copies
// every table in parent-first order. The destination is expected to be empty.
func Run(src, dst storage.Options) ([]Result, error) {
	source, err := storage.Open(src)
	if err != nil {
		return nil, fmt.Errorf("open source: %w", err)
	}
	defer func() { _ = source.Close() }()
	dest, err := storage.Open(dst)
	if err != nil {
		return nil, fmt.Errorf("open destination: %w", err)
	}
	defer func() { _ = dest.Close() }()

	sg, dg := source.Gorm(), dest.Gorm()
	if err := dbmodel.AutoMigrate(dg); err != nil {
		return nil, fmt.Errorf("prepare destination schema: %w", err)
	}

	steps := []struct {
		table string
		copy  func(src, dst *gorm.DB) (int64, error)
	}{
		{"admins", copyTable[dbmodel.Admin]},
		{"admin_sessions", copyTable[dbmodel.AdminSession]},
		{"clients", copyTable[dbmodel.Client]},
		{"invite_tokens", copyTable[dbmodel.InviteToken]},
		{"audit_log", copyTable[dbmodel.AuditLog]},
		{"admin_master_config", copyTable[dbmodel.AdminMasterConfig]},
		{"client_configs", copyTable[dbmodel.ClientConfig]},
		{"client_reported_configs", copyTable[dbmodel.ClientReportedConfig]},
		{"client_runtime", copyTable[dbmodel.ClientRuntime]},
		{"client_logs", copyTable[dbmodel.ClientLog]},
		{"client_installed_apps", copyTable[dbmodel.ClientInstalledApps]},
		{"pending_commands", copyTable[dbmodel.PendingCommand]},
		{"package_metadata", copyTable[dbmodel.PackageMetadata]},
		{"kv", copyTable[dbmodel.KV]},
		{"platform_settings", copyTable[dbmodel.PlatformSetting]},
		{"server_nodes", copyTable[dbmodel.ServerNode]},
		{"client_wg_peers", copyTable[dbmodel.ClientWGPeer]},
	}

	results := make([]Result, 0, len(steps))
	for _, step := range steps {
		n, err := step.copy(sg, dg)
		if err != nil {
			return results, fmt.Errorf("copy %s: %w", step.table, err)
		}
		results = append(results, Result{Table: step.table, Rows: n})
	}
	return results, nil
}

func copyTable[T any](src, dst *gorm.DB) (int64, error) {
	var rows []T
	if err := src.Find(&rows).Error; err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	for i := range rows {
		normalizeRow(&rows[i])
	}
	if err := dst.CreateInBatches(rows, 200).Error; err != nil {
		return 0, err
	}
	return int64(len(rows)), nil
}

// normalizeRow makes a source row portable to a stricter backend. SQLite is lax:
// it allows NULL in a NOT NULL column and NUL bytes / invalid UTF-8 in text.
// Postgres and MariaDB reject those, so nil byte slices become empty slices and
// text fields are stripped of NUL bytes and coerced to valid UTF-8.
func normalizeRow(row any) {
	v := reflect.ValueOf(row).Elem()
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if !f.CanSet() {
			continue
		}
		switch {
		case f.Kind() == reflect.Slice && f.Type().Elem().Kind() == reflect.Uint8 && f.IsNil():
			f.Set(reflect.MakeSlice(f.Type(), 0, 0))
		case f.Kind() == reflect.String:
			s := f.String()
			if strings.IndexByte(s, 0) >= 0 || !utf8.ValidString(s) {
				f.SetString(strings.ToValidUTF8(strings.ReplaceAll(s, "\x00", ""), ""))
			}
		}
	}
}
