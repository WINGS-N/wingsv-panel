package migrate

import (
	"path/filepath"
	"testing"

	"v.wingsnet.org/internal/storage"
	"v.wingsnet.org/internal/storage/dbmodel"
)

func TestMigrateSqliteRoundtrip(t *testing.T) {
	dir := t.TempDir()
	srcOpts := storage.Options{Driver: storage.DriverSQLite, DSN: filepath.Join(dir, "src.db")}
	dstOpts := storage.Options{Driver: storage.DriverSQLite, DSN: filepath.Join(dir, "dst.db")}

	src, err := storage.Open(srcOpts)
	if err != nil {
		t.Fatalf("open src: %v", err)
	}
	g := src.Gorm()
	admin := dbmodel.Admin{Username: "owner", PasswordHash: "h", Role: "owner", CreatedAtUnix: 111, UpdatedAtUnix: 222}
	if err := g.Create(&admin).Error; err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	client := dbmodel.Client{
		ID: "c1", OwnerAdminID: admin.ID, Name: "dev", TokenHash: "th", TokenPlain: []byte("raw"),
		HWID: "hw", SyncMode: "always", PeriodicIntervalMinutes: 30, CreatedAtUnix: 333,
		LogRuntimeEnabled: true, HasRootAccess: true,
	}
	if err := g.Create(&client).Error; err != nil {
		t.Fatalf("seed client: %v", err)
	}
	cfg := dbmodel.ClientConfig{ClientID: "c1", ConfigProto: []byte{1, 2, 3}, Revision: "r1", UpdatedAtUnix: 444, ConfigVersion: 7}
	if err := g.Create(&cfg).Error; err != nil {
		t.Fatalf("seed config: %v", err)
	}
	if err := g.Create(&dbmodel.KV{Key: "k", Value: []byte("v")}).Error; err != nil {
		t.Fatalf("seed kv: %v", err)
	}
	_ = src.Close()

	results, err := Run(srcOpts, dstOpts)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	rows := map[string]int64{}
	for _, r := range results {
		rows[r.Table] = r.Rows
	}
	for _, tbl := range []string{"admins", "clients", "client_configs", "kv"} {
		if rows[tbl] != 1 {
			t.Fatalf("table %s copied %d rows, want 1 (all: %+v)", tbl, rows[tbl], rows)
		}
	}

	dst, err := storage.Open(dstOpts)
	if err != nil {
		t.Fatalf("open dst: %v", err)
	}
	defer dst.Close()

	var gotClient dbmodel.Client
	if err := dst.Gorm().First(&gotClient, "id = ?", "c1").Error; err != nil {
		t.Fatalf("read client: %v", err)
	}
	if gotClient.OwnerAdminID != admin.ID || gotClient.Name != "dev" || gotClient.CreatedAtUnix != 333 ||
		!gotClient.LogRuntimeEnabled || !gotClient.HasRootAccess || string(gotClient.TokenPlain) != "raw" {
		t.Fatalf("client not copied faithfully: %+v", gotClient)
	}
	var gotCfg dbmodel.ClientConfig
	if err := dst.Gorm().First(&gotCfg, "client_id = ?", "c1").Error; err != nil {
		t.Fatalf("read cfg: %v", err)
	}
	if string(gotCfg.ConfigProto) != string([]byte{1, 2, 3}) || gotCfg.ConfigVersion != 7 || gotCfg.UpdatedAtUnix != 444 {
		t.Fatalf("config not copied faithfully: %+v", gotCfg)
	}
	var gotAdmin dbmodel.Admin
	if err := dst.Gorm().First(&gotAdmin, "id = ?", admin.ID).Error; err != nil {
		t.Fatalf("read admin: %v", err)
	}
	if gotAdmin.CreatedAtUnix != 111 || gotAdmin.UpdatedAtUnix != 222 || gotAdmin.Username != "owner" {
		t.Fatalf("admin timestamps not preserved: %+v", gotAdmin)
	}
}
