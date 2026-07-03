package storage

import (
	"errors"
	"testing"

	"v.wingsnet.org/internal/storage/dbmodel"
)

func seedClientAndNodes(t *testing.T, st *Store) {
	t.Helper()
	admin := dbmodel.Admin{Username: "owner", PasswordHash: "h", Role: "owner", CreatedAtUnix: 1, UpdatedAtUnix: 1}
	if err := st.gdb.Create(&admin).Error; err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	client := dbmodel.Client{ID: "c1", OwnerAdminID: admin.ID, Name: "dev", TokenHash: "h", SyncMode: "always", CreatedAtUnix: 1}
	if err := st.gdb.Create(&client).Error; err != nil {
		t.Fatalf("seed client: %v", err)
	}
	for _, id := range []string{"n1", "n2"} {
		if _, err := st.CreateServerNode(dbmodel.ServerNode{ID: id, Kind: ServerNodeVKTurnProxy, Name: id}); err != nil {
			t.Fatalf("seed node %s: %v", id, err)
		}
	}
}

func TestClientWGPeerUpsertAndList(t *testing.T) {
	st := openTemp(t)
	seedClientAndNodes(t, st)

	peer := dbmodel.ClientWGPeer{
		ClientID: "c1", NodeID: "n1", PublicKey: "pub1", PrivateKey: "priv1",
		AllowedIPs: "10.66.66.2/32", ServerPublicKey: "spub", Endpoint: "1.2.3.4:56000",
	}
	if err := st.UpsertClientWGPeer(peer); err != nil {
		t.Fatalf("UpsertClientWGPeer: %v", err)
	}
	got, err := st.GetClientWGPeer("c1", "n1")
	if err != nil {
		t.Fatalf("GetClientWGPeer: %v", err)
	}
	if got.PublicKey != "pub1" || got.AllowedIPs != "10.66.66.2/32" || got.CreatedAtUnix == 0 {
		t.Fatalf("peer stored wrong: %+v", got)
	}

	updated := peer
	updated.PublicKey = "pub2"
	updated.AllowedIPs = "10.66.66.3/32"
	if err := st.UpsertClientWGPeer(updated); err != nil {
		t.Fatalf("UpsertClientWGPeer update: %v", err)
	}
	got, err = st.GetClientWGPeer("c1", "n1")
	if err != nil {
		t.Fatalf("GetClientWGPeer after update: %v", err)
	}
	if got.PublicKey != "pub2" || got.AllowedIPs != "10.66.66.3/32" {
		t.Fatalf("upsert did not update: %+v", got)
	}

	if err := st.UpsertClientWGPeer(dbmodel.ClientWGPeer{ClientID: "c1", NodeID: "n2", PublicKey: "pubN2"}); err != nil {
		t.Fatalf("second node peer: %v", err)
	}
	peers, err := st.ListClientWGPeers("c1")
	if err != nil {
		t.Fatalf("ListClientWGPeers: %v", err)
	}
	if len(peers) != 2 {
		t.Fatalf("list len = %d, want 2", len(peers))
	}

	if err := st.DeleteClientWGPeer("c1", "n1"); err != nil {
		t.Fatalf("DeleteClientWGPeer: %v", err)
	}
	if _, err := st.GetClientWGPeer("c1", "n1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get after delete = %v, want ErrNotFound", err)
	}
	if err := st.DeleteClientWGPeer("c1", "n1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete missing = %v, want ErrNotFound", err)
	}
}
