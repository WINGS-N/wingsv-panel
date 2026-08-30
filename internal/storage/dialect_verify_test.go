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

	// The invite tree walks edges and updates a subtree, which is the newest
	// thing here and the easiest to get wrong on a dialect that is not sqlite
	invited, err := st.CreateAdmin("invited", "hash", false, RoleAdmin)
	if err != nil {
		t.Fatalf("CreateAdmin invited: %v", err)
	}
	deeper, err := st.CreateAdmin("deeper", "hash", false, RoleAdmin)
	if err != nil {
		t.Fatalf("CreateAdmin deeper: %v", err)
	}
	if _, err := st.CreateInvite("tok-1", time.Now().Add(time.Hour), admin.ID); err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	if err := st.RedeemInvite("tok-1", invited.ID); err != nil {
		t.Fatalf("RedeemInvite: %v", err)
	}
	if _, err := st.CreateInvite("tok-2", time.Now().Add(time.Hour), invited.ID); err != nil {
		t.Fatalf("CreateInvite 2: %v", err)
	}
	if err := st.RedeemInvite("tok-2", deeper.ID); err != nil {
		t.Fatalf("RedeemInvite 2: %v", err)
	}
	members, err := st.InviteTree()
	if err != nil {
		t.Fatalf("InviteTree: %v", err)
	}
	depth := map[int64]int{}
	for _, m := range members {
		depth[m.AdminID] = m.Depth
	}
	if depth[invited.ID] != 1 || depth[deeper.ID] != 2 {
		t.Fatalf("tree depths wrong: %+v", depth)
	}
	affected, err := st.SuspendSubtree(invited.ID, "abuse")
	if err != nil {
		t.Fatalf("SuspendSubtree: %v", err)
	}
	if affected != 2 {
		t.Fatalf("SuspendSubtree cut %d, want the branch of two", affected)
	}
	for _, id := range []int64{invited.ID, deeper.ID} {
		suspended, reason, err := st.IsSuspended(id)
		if err != nil || !suspended || reason != "abuse" {
			t.Fatalf("IsSuspended(%d) = %v %q err=%v", id, suspended, reason, err)
		}
	}
	if suspended, _, _ := st.IsSuspended(admin.ID); suspended {
		t.Fatal("the cut reached above the branch it was made at")
	}
	if _, err := st.RestoreSubtree(invited.ID); err != nil {
		t.Fatalf("RestoreSubtree: %v", err)
	}
	if suspended, _, _ := st.IsSuspended(deeper.ID); suspended {
		t.Fatal("restore did not lift the branch")
	}

	// The rollup upserts a batch with an ON CONFLICT clause, which is exactly the
	// kind of thing that behaves differently per dialect
	if err := st.RollupAdminTraffic(); err != nil {
		t.Fatalf("RollupAdminTraffic: %v", err)
	}
	if err := st.RollupAdminTraffic(); err != nil {
		t.Fatalf("RollupAdminTraffic second pass: %v", err)
	}
	usage, err := st.AdminTrafficMap()
	if err != nil {
		t.Fatalf("AdminTrafficMap: %v", err)
	}
	if _, ok := usage[admin.ID]; !ok {
		t.Fatalf("no rollup row for the owner: %+v", usage)
	}
	if got := usage[admin.ID].SubtreeAdmins; got != 2 {
		t.Fatalf("owner brought in %d, want the two below them", got)
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
