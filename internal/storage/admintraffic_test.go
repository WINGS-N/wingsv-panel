package storage

import (
	"path/filepath"
	"testing"
	"time"

	"v.wingsnet.org/internal/storage/dbmodel"
)

func rollupStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(Options{Driver: DriverSQLite, DSN: filepath.Join(t.TempDir(), "rollup.db")})
	if err != nil {
		t.Fatal(err)
	}
	// client_wg_peers points at a real node
	if _, err := st.CreateServerNode(dbmodel.ServerNode{
		ID: "n1", Kind: "vkturn", Name: "n1", GRPCEndpoint: "127.0.0.1:1",
	}); err != nil {
		t.Fatal(err)
	}
	return st
}

// clientWithTraffic creates a client owned by admin and gives it usage the way
// the collector does, through peer_traffic
func clientWithTraffic(t *testing.T, st *Store, id string, adminID int64, node, pubkey string, rx, tx uint64) {
	t.Helper()
	if _, err := st.CreateClient(id, adminID, id, "hash", []byte("raw")); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertClientWGPeer(dbmodel.ClientWGPeer{
		ClientID: id, NodeID: node, PublicKey: pubkey, AllowedIPs: "10.0.0.1/32",
	}); err != nil {
		t.Fatal(err)
	}
	// The first reading is a baseline - nobody knows what happened before the
	// panel started looking - so usage only appears from the second one
	for _, sample := range []dbmodel.PeerTraffic{
		{NodeID: node, PublicKey: pubkey, SampledUnix: time.Now().Unix()},
		{NodeID: node, PublicKey: pubkey, RxBytes: rx, TxBytes: tx, SampledUnix: time.Now().Unix()},
	} {
		if err := st.UpsertPeerTraffic([]dbmodel.PeerTraffic{sample}); err != nil {
			t.Fatal(err)
		}
		if err := st.AccumulateClientTraffic(); err != nil {
			t.Fatal(err)
		}
	}
}

// The whole point: what a branch moved, not just one person
func TestSubtreeTrafficSumsTheWholeBranch(t *testing.T) {
	st := rollupStore(t)
	root := makeAdmin(t, st, "root-one")
	mid := makeAdmin(t, st, "mid")
	leaf := makeAdmin(t, st, "leaf")
	other := makeAdmin(t, st, "other")
	invite(t, st, "t1", root, mid)
	invite(t, st, "t2", mid, leaf)
	invite(t, st, "t3", root, other)

	clientWithTraffic(t, st, "c-root", root, "n1", "pk-root", 100, 0)
	clientWithTraffic(t, st, "c-mid", mid, "n1", "pk-mid", 200, 0)
	clientWithTraffic(t, st, "c-leaf", leaf, "n1", "pk-leaf", 400, 0)
	clientWithTraffic(t, st, "c-other", other, "n1", "pk-other", 800, 0)

	if err := st.RollupAdminTraffic(); err != nil {
		t.Fatal(err)
	}
	usage, err := st.AdminTrafficMap()
	if err != nil {
		t.Fatal(err)
	}

	if got := usage[leaf].SubtreeBytes; got != 400 {
		t.Errorf("leaf subtree = %d, want its own 400", got)
	}
	if got := usage[mid].SubtreeBytes; got != 600 {
		t.Errorf("mid subtree = %d, want 200 + 400", got)
	}
	if got := usage[root].SubtreeBytes; got != 1500 {
		t.Errorf("root subtree = %d, want everything", got)
	}
	// Own usage stays own: the two numbers answer different questions
	if got := usage[root].OwnBytes; got != 100 {
		t.Errorf("root own = %d, want 100", got)
	}
	if got := usage[mid].SubtreeAdmins; got != 1 {
		t.Errorf("mid brought in %d, want 1", got)
	}
	if got := usage[root].SubtreeAdmins; got != 3 {
		t.Errorf("root brought in %d, want 3", got)
	}
	if got := usage[root].SubtreeClients; got != 4 {
		t.Errorf("root subtree clients = %d, want 4", got)
	}
}

// used_bytes is already fed from peer_traffic for both backends, so anything
// that adds the two tables together counts the same bytes twice
func TestTrafficIsNotCountedTwice(t *testing.T) {
	st := rollupStore(t)
	admin := makeAdmin(t, st, "solo")
	clientWithTraffic(t, st, "c1", admin, "n1", "pk1", 300, 700)

	if err := st.RollupAdminTraffic(); err != nil {
		t.Fatal(err)
	}
	usage, err := st.AdminTrafficMap()
	if err != nil {
		t.Fatal(err)
	}
	if got := usage[admin].OwnBytes; got != 1000 {
		t.Errorf("own = %d, want the 1000 bytes counted once", got)
	}
}

// An admin with no clients is still in the tree and still worth showing
func TestAnAdminWithNothingStillGetsARow(t *testing.T) {
	st := rollupStore(t)
	admin := makeAdmin(t, st, "empty")
	if err := st.RollupAdminTraffic(); err != nil {
		t.Fatal(err)
	}
	usage, err := st.AdminTrafficMap()
	if err != nil {
		t.Fatal(err)
	}
	got, ok := usage[admin]
	if !ok {
		t.Fatal("an admin with no clients has no row")
	}
	if got.OwnBytes != 0 || got.SubtreeAdmins != 0 {
		t.Errorf("row = %+v", got)
	}
}

// The job runs on a ticker, so it has to be safe to run over and over
func TestRollupIsIdempotent(t *testing.T) {
	st := rollupStore(t)
	admin := makeAdmin(t, st, "solo")
	clientWithTraffic(t, st, "c1", admin, "n1", "pk1", 500, 0)
	for i := 0; i < 3; i++ {
		if err := st.RollupAdminTraffic(); err != nil {
			t.Fatalf("pass %d: %v", i, err)
		}
	}
	usage, err := st.AdminTrafficMap()
	if err != nil {
		t.Fatal(err)
	}
	if got := usage[admin].OwnBytes; got != 500 {
		t.Errorf("own = %d after three passes, want 500", got)
	}
}

// A malformed edge must not loop the walk
func TestRollupSurvivesACycle(t *testing.T) {
	st := rollupStore(t)
	a := makeAdmin(t, st, "a")
	b := makeAdmin(t, st, "b")
	invite(t, st, "t1", a, b)
	invite(t, st, "t2", b, a)

	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := st.RollupAdminTraffic(); err != nil {
			t.Error(err)
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the rollup did not terminate on a cyclic tree")
	}
}
