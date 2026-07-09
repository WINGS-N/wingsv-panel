package provisioning

import (
	"context"
	"path/filepath"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"v.wingsnet.org/internal/auth"
	"v.wingsnet.org/internal/gen/provisioningpb"
	"v.wingsnet.org/internal/storage"
	"v.wingsnet.org/internal/storage/dbmodel"
)

type fakeProvisioner struct {
	calls int
	nodes []string
	peer  Peer
}

func (f *fakeProvisioner) CreatePeer(_ context.Context, node dbmodel.ServerNode, _, _ string) (Peer, error) {
	f.calls++
	f.nodes = append(f.nodes, node.ID)
	return f.peer, nil
}

func setup(t *testing.T) (*storage.Store, string, []byte) {
	t.Helper()
	st, err := storage.Open(storage.Options{Driver: storage.DriverSQLite, DSN: filepath.Join(t.TempDir(), "t.db")})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	admin, err := st.CreateAdmin("owner", "hash", false, "owner")
	if err != nil {
		t.Fatalf("admin: %v", err)
	}
	tokenBytes, tokenHash, err := auth.GenerateClientToken()
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	if _, err := st.CreateClient("c1", admin.ID, "dev", tokenHash, tokenBytes); err != nil {
		t.Fatalf("client: %v", err)
	}
	if _, err := st.CreateServerNode(dbmodel.ServerNode{ID: "n1", Kind: storage.ServerNodeVKTurnProxy, Name: "relay"}); err != nil {
		t.Fatalf("node: %v", err)
	}
	return st, "c1", tokenBytes
}

func TestResolveRefusesBlockedClient(t *testing.T) {
	st, err := storage.Open(storage.Options{Driver: storage.DriverSQLite, DSN: filepath.Join(t.TempDir(), "t.db")})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	admin, err := st.CreateAdmin("owner", "hash", false, "owner")
	if err != nil {
		t.Fatalf("admin: %v", err)
	}
	token, tokenHash, err := auth.GenerateClientToken()
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	if _, err := st.CreateClient("c1", admin.ID, "dev", tokenHash, token); err != nil {
		t.Fatalf("client: %v", err)
	}
	if _, err := st.CreateServerNode(dbmodel.ServerNode{ID: "n1", Kind: storage.ServerNodeVKTurnProxy, Name: "relay"}); err != nil {
		t.Fatalf("node: %v", err)
	}
	if err := st.SetClientDisabled("c1", admin.ID, true); err != nil {
		t.Fatalf("disable: %v", err)
	}

	fake := &fakeProvisioner{peer: Peer{PublicKey: "pub"}}
	svc := NewService(st, fake)
	_, err = svc.ResolveClientConfig(context.Background(),
		&provisioningpb.ResolveClientConfigRequest{ClientId: "c1", Token: token, NodeId: "n1"})
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("blocked client resolve = %v, want ResourceExhausted", err)
	}
	if fake.calls != 0 {
		t.Fatalf("provisioner should not run for a blocked client, calls=%d", fake.calls)
	}
}

func TestResolveCreatesThenReturnsExisting(t *testing.T) {
	st, clientID, token := setup(t)
	fake := &fakeProvisioner{peer: Peer{PublicKey: "pub", PrivateKey: "priv", AllowedIPs: "10.66.66.2/32", ServerPublicKey: "spub"}}
	svc := NewService(st, fake)
	req := &provisioningpb.ResolveClientConfigRequest{ClientId: clientID, Token: token, NodeId: "n1", Hwid: "hw"}

	resp, err := svc.ResolveClientConfig(context.Background(), req)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	wg := resp.GetWg()
	if wg.GetPrivateKey() != "priv" || wg.GetPublicKey() != "pub" || wg.GetAddress() != "10.66.66.2/32" ||
		wg.GetServerPublicKey() != "spub" || wg.GetAllowedIps() != "0.0.0.0/0" || wg.GetMtu() != 1280 {
		t.Fatalf("wg config wrong: %+v", wg)
	}
	if fake.calls != 1 {
		t.Fatalf("provisioner calls = %d, want 1", fake.calls)
	}

	fake.peer = Peer{PublicKey: "DIFFERENT"}
	resp2, err := svc.ResolveClientConfig(context.Background(), req)
	if err != nil {
		t.Fatalf("resolve 2: %v", err)
	}
	if resp2.GetWg().GetPublicKey() != "pub" {
		t.Fatalf("second resolve should return the stored peer, got %q", resp2.GetWg().GetPublicKey())
	}
	if fake.calls != 1 {
		t.Fatalf("provisioner was called again: %d", fake.calls)
	}
}

func TestResolveReplicatesPeerToOtherNodes(t *testing.T) {
	st, clientID, token := setup(t)
	if _, err := st.CreateServerNode(dbmodel.ServerNode{ID: "n2", Kind: storage.ServerNodeVKTurnProxy, Name: "relay2"}); err != nil {
		t.Fatalf("node n2: %v", err)
	}
	fake := &fakeProvisioner{peer: Peer{PublicKey: "pub", PrivateKey: "priv", AllowedIPs: "10.66.66.2/32", ServerPublicKey: "spub"}}
	svc := NewService(st, fake)
	req := &provisioningpb.ResolveClientConfigRequest{ClientId: clientID, Token: token, NodeId: "n1"}
	if _, err := svc.ResolveClientConfig(context.Background(), req); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if fake.calls != 2 {
		t.Fatalf("provisioner calls = %d, want 2 (origin + one replica)", fake.calls)
	}
	if _, err := st.GetClientWGPeer(clientID, "n1"); err != nil {
		t.Fatalf("peer for origin node n1 missing: %v", err)
	}
	if _, err := st.GetClientWGPeer(clientID, "n2"); err != nil {
		t.Fatalf("peer was not replicated to n2: %v", err)
	}
}

func TestResolveRejectsBadToken(t *testing.T) {
	st, clientID, _ := setup(t)
	svc := NewService(st, &fakeProvisioner{})
	_, err := svc.ResolveClientConfig(context.Background(),
		&provisioningpb.ResolveClientConfigRequest{ClientId: clientID, Token: []byte("wrong"), NodeId: "n1"})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("code = %v, want Unauthenticated", status.Code(err))
	}
}

func TestResolveUnknownNode(t *testing.T) {
	st, clientID, token := setup(t)
	svc := NewService(st, &fakeProvisioner{})
	_, err := svc.ResolveClientConfig(context.Background(),
		&provisioningpb.ResolveClientConfigRequest{ClientId: clientID, Token: token, NodeId: "missing"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("code = %v, want NotFound", status.Code(err))
	}
}
