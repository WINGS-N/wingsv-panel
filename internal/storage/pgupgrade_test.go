package storage

import (
	"os"
	"testing"
)

// A live pgsql deployment already has an admins table without the suspension
// columns. Opening the store has to add them, or the first branch cut fails on a
// database nobody thought to migrate by hand.
func TestPostgresUpgradeAddsSuspensionColumns(t *testing.T) {
	dsn := os.Getenv("WV_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("set WV_TEST_PG_DSN")
	}
	st, err := Open(Options{Driver: DriverPostgres, DSN: dsn})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = st.Close() }()

	admin, err := st.CreateAdmin("upgrade-check", "hash", false, RoleAdmin)
	if err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	if _, err := st.SuspendSubtree(admin.ID, "after an upgrade"); err != nil {
		t.Fatalf("SuspendSubtree on an upgraded database: %v", err)
	}
	suspended, reason, err := st.IsSuspended(admin.ID)
	if err != nil || !suspended || reason != "after an upgrade" {
		t.Fatalf("IsSuspended = %v %q err=%v", suspended, reason, err)
	}
}
