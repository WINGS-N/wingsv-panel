package storage

import (
	"os"
	"testing"
	"time"

	"v.wingsnet.org/internal/storage/dbmodel"
)

func cleanReset(t *testing.T, st *Store) {
	t.Helper()
	all := dbmodel.All()
	for i := len(all) - 1; i >= 0; i-- {
		_ = st.Gorm().Migrator().DropTable(all[i])
	}
	if err := dbmodel.AutoMigrate(st.Gorm()); err != nil {
		t.Fatalf("re-migrate: %v", err)
	}
}

func exerciseDialect(t *testing.T, driver Driver, dsn string) {
	st, err := Open(Options{Driver: driver, DSN: dsn})
	if err != nil {
		t.Fatalf("Open %s: %v", driver, err)
	}
	t.Cleanup(func() { _ = st.Close() })
	cleanReset(t, st)

	admin, err := st.CreateAdmin("owner", "hash", true, "owner")
	if err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	if _, err := st.CreateClient("c1", admin.ID, "dev", "th", []byte("raw")); err != nil {
		t.Fatalf("CreateClient: %v", err)
	}

	if err := st.UpdateClientPresence("c1", true, nil); err != nil {
		t.Fatalf("UpdateClientPresence: %v", err)
	}
	if err := st.UpdateClientRootAccess("c1", true); err != nil {
		t.Fatalf("UpdateClientRootAccess: %v", err)
	}
	c, err := st.FindClientByID("c1")
	if err != nil {
		t.Fatalf("FindClientByID: %v", err)
	}
	if !c.Online || !c.HasRootAccess {
		t.Fatalf("boolean columns not read back: online=%v root=%v", c.Online, c.HasRootAccess)
	}

	v1, err := st.UpsertClientConfig("c1", []byte{1}, "r1")
	if err != nil {
		t.Fatalf("UpsertClientConfig: %v", err)
	}
	v2, err := st.UpsertClientConfig("c1", []byte{2}, "r2")
	if err != nil {
		t.Fatalf("UpsertClientConfig 2: %v", err)
	}
	if v2 != v1+1 {
		t.Fatalf("config_version did not increment: %d -> %d", v1, v2)
	}
	cfg, err := st.GetClientConfig("c1")
	if err != nil || string(cfg.ConfigProto) != string([]byte{2}) || cfg.ConfigVersion != v2 {
		t.Fatalf("GetClientConfig: %+v err=%v", cfg, err)
	}

	if err := st.KVSet("k", []byte("v")); err != nil {
		t.Fatalf("KVSet: %v", err)
	}
	if got, err := st.KVGet("k"); err != nil || string(got) != "v" {
		t.Fatalf("KVGet: %q err=%v", got, err)
	}
	if err := st.SetPlatformSetting("mode", "open"); err != nil {
		t.Fatalf("SetPlatformSetting: %v", err)
	}
	if got, err := st.GetPlatformSetting("mode", "x"); err != nil || got != "open" {
		t.Fatalf("GetPlatformSetting: %q err=%v", got, err)
	}

	if err := st.AppendClientLogs("c1", 1, 0, []LogLine{{TS: time.Now(), Text: "a"}, {TS: time.Now(), Text: "b"}}); err != nil {
		t.Fatalf("AppendClientLogs: %v", err)
	}
	logs, err := st.ReadClientLogs("c1", 1, -1, 10)
	if err != nil || len(logs) != 2 {
		t.Fatalf("ReadClientLogs: %d err=%v", len(logs), err)
	}

	if err := st.UpsertPackageMetadata([]PackageMetadata{{Package: "com.x", Label: "X", IconPNG: []byte{9}}}); err != nil {
		t.Fatalf("UpsertPackageMetadata: %v", err)
	}
	if err := st.UpsertPackageMetadata([]PackageMetadata{{Package: "com.x", Label: ""}}); err != nil {
		t.Fatalf("UpsertPackageMetadata merge: %v", err)
	}
	m, err := st.GetPackageMetadataMap([]string{"com.x"})
	if err != nil || m["com.x"].Label != "X" || string(m["com.x"].IconPNG) != string([]byte{9}) {
		t.Fatalf("package metadata merge lost old values: %+v err=%v", m["com.x"], err)
	}

	if err := st.AppendAudit(AuditEntry{ActorAdminID: admin.ID, Action: "test.action", IP: "1.2.3.4"}); err != nil {
		t.Fatalf("AppendAudit: %v", err)
	}
	entries, err := st.ListAudit(AuditFilter{})
	if err != nil || len(entries) != 1 || entries[0].Action != "test.action" {
		t.Fatalf("ListAudit: %d err=%v", len(entries), err)
	}
}

func TestPostgresRuntime(t *testing.T) {
	dsn := os.Getenv("WV_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("set WV_TEST_PG_DSN to run the PostgreSQL runtime test")
	}
	exerciseDialect(t, DriverPostgres, dsn)
}

func TestMySQLRuntime(t *testing.T) {
	dsn := os.Getenv("WV_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("set WV_TEST_MYSQL_DSN to run the MariaDB runtime test")
	}
	exerciseDialect(t, DriverMySQL, dsn)
}
