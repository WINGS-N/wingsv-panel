package storage

import (
	"testing"
	"time"
)

func mustAdmin(t *testing.T, st *Store, username string) int64 {
	t.Helper()
	a, err := st.CreateAdmin(username, "hash", false, "admin")
	if err != nil {
		t.Fatalf("CreateAdmin %s: %v", username, err)
	}
	return a.ID
}

// Deleting a client must take its child rows with it. They used to survive: nothing
// could reach them again (every lookup resolves the client first) yet they kept
// accumulating, and on one panel the leftover log rows were most of the database.
func TestDeleteClientCascadesToOwnedRows(t *testing.T) {
	st := openTemp(t)
	owner := mustAdmin(t, st, "owner")
	if _, err := st.CreateClient("c1", owner, "Dev", "hash", []byte("tok")); err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	if _, err := st.UpsertClientConfig("c1", []byte{1, 2, 3}, "r1"); err != nil {
		t.Fatalf("UpsertClientConfig: %v", err)
	}
	if err := st.UpsertClientRuntime("c1", []byte{4}); err != nil {
		t.Fatalf("UpsertClientRuntime: %v", err)
	}
	lines := []LogLine{{Seq: 1, TS: time.Now(), Text: "hello"}}
	if err := st.AppendClientLogs("c1", 0, 1, lines); err != nil {
		t.Fatalf("AppendClientLogs: %v", err)
	}

	if err := st.DeleteClient("c1", owner); err != nil {
		t.Fatalf("DeleteClient: %v", err)
	}
	for _, table := range clientOwnedTables {
		var left int64
		if err := st.Gorm().Raw("SELECT count(*) FROM "+table+" WHERE client_id = ?", "c1").Scan(&left).Error; err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if left != 0 {
			t.Errorf("%s still holds %d row(s) for the deleted client", table, left)
		}
	}
}

// A client that is not the caller's stays untouched, cascade included.
func TestDeleteClientLeavesOtherOwnersAlone(t *testing.T) {
	st := openTemp(t)
	mine := mustAdmin(t, st, "mine-owner")
	theirs := mustAdmin(t, st, "their-owner")
	if _, err := st.CreateClient("mine", mine, "Mine", "h1", []byte("t1")); err != nil {
		t.Fatalf("CreateClient mine: %v", err)
	}
	if _, err := st.CreateClient("theirs", theirs, "Theirs", "h2", []byte("t2")); err != nil {
		t.Fatalf("CreateClient theirs: %v", err)
	}
	if _, err := st.UpsertClientConfig("theirs", []byte{9}, "r"); err != nil {
		t.Fatalf("UpsertClientConfig: %v", err)
	}
	// Wrong owner: the delete must fail and touch nothing.
	if err := st.DeleteClient("theirs", mine); err != ErrNotFound {
		t.Fatalf("DeleteClient across owners = %v, want ErrNotFound", err)
	}
	if _, err := st.GetClientConfig("theirs"); err != nil {
		t.Errorf("other owner's config was removed: %v", err)
	}
}

// PurgeOrphanClientRows clears what earlier, non-cascading deletes left behind and
// does nothing on a clean database.
func TestPurgeOrphanClientRows(t *testing.T) {
	st := openTemp(t)
	// sqlite enforces the client foreign key, so an orphan cannot be created through
	// the normal API - which is exactly why only the driver without those keys grew
	// them. Drop the constraint for a moment to reproduce that state here.
	if err := st.Gorm().Exec("PRAGMA foreign_keys = OFF").Error; err != nil {
		t.Fatalf("disable fks: %v", err)
	}
	if _, err := st.UpsertClientConfig("ghost", []byte{7}, "r"); err != nil {
		t.Fatalf("UpsertClientConfig: %v", err)
	}
	if err := st.Gorm().Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatalf("re-enable fks: %v", err)
	}
	if _, err := st.CreateClient("live", mustAdmin(t, st, "purge-owner"), "Live", "h", []byte("t")); err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	if _, err := st.UpsertClientConfig("live", []byte{8}, "r"); err != nil {
		t.Fatalf("UpsertClientConfig live: %v", err)
	}

	if err := st.PurgeOrphanClientRows(); err != nil {
		t.Fatalf("PurgeOrphanClientRows: %v", err)
	}
	if _, err := st.GetClientConfig("ghost"); err != ErrNotFound {
		t.Errorf("orphan config survived the purge: %v", err)
	}
	if _, err := st.GetClientConfig("live"); err != nil {
		t.Errorf("live client's config was purged: %v", err)
	}
	// Second run is a no-op.
	if err := st.PurgeOrphanClientRows(); err != nil {
		t.Fatalf("second purge: %v", err)
	}
	if _, err := st.GetClientConfig("live"); err != nil {
		t.Errorf("live config lost on the second purge: %v", err)
	}
}
