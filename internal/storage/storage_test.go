package storage

import (
	"path/filepath"
	"testing"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	st, err := Open(Options{Driver: DriverSQLite, DSN: filepath.Join(t.TempDir(), "test.db")})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestOpenSqliteCreatesSchema(t *testing.T) {
	st := openTemp(t)
	if err := st.DB().Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}
	tables := []string{
		"admins", "admin_sessions", "clients", "client_configs",
		"client_reported_configs", "client_runtime", "client_logs", "kv",
		"client_installed_apps", "pending_commands", "package_metadata",
		"audit_log", "platform_settings", "invite_tokens", "admin_master_config",
	}
	for _, name := range tables {
		if !st.Gorm().Migrator().HasTable(name) {
			t.Errorf("missing table %q", name)
		}
	}
}

func TestNormalizeDriver(t *testing.T) {
	cases := map[string]Driver{
		"":           DriverSQLite,
		"sqlite":     DriverSQLite,
		"sqlite3":    DriverSQLite,
		"pgsql":      DriverPostgres,
		"postgres":   DriverPostgres,
		"postgresql": DriverPostgres,
		"mariadb":    DriverMySQL,
		"mysql":      DriverMySQL,
	}
	for in, want := range cases {
		got, err := NormalizeDriver(in)
		if err != nil {
			t.Errorf("NormalizeDriver(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("NormalizeDriver(%q) = %q, want %q", in, got, want)
		}
	}
	if _, err := NormalizeDriver("oracle"); err == nil {
		t.Error("NormalizeDriver(oracle) should error")
	}
}
