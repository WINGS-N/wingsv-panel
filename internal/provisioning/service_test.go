package provisioning

import (
	"context"
	"path/filepath"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
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

// A valid node token proves which relay is calling, not that the client belongs
// there. An admin must not be able to get a wg peer minted on the owner's
// panel-local node (owner_admin_id 0), nor on another admin's node: the endpoint
// the app dials comes from the saved config, so the node is attacker-chosen.
func TestResolveRefusesClientOnForeignNode(t *testing.T) {
	st, err := storage.Open(storage.Options{Driver: storage.DriverSQLite, DSN: filepath.Join(t.TempDir(), "t.db")})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	plainAdmin, err := st.CreateAdmin("admin1", "hash", false, storage.RoleAdmin)
	if err != nil {
		t.Fatalf("admin: %v", err)
	}
	otherAdmin, err := st.CreateAdmin("admin2", "hash", false, storage.RoleAdmin)
	if err != nil {
		t.Fatalf("other admin: %v", err)
	}
	token, tokenHash, err := auth.GenerateClientToken()
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	if _, err := st.CreateClient("c1", plainAdmin.ID, "dev", tokenHash, token); err != nil {
		t.Fatalf("client: %v", err)
	}
	// The owner's panel-local relay, and a second admin's own relay.
	if _, err := st.CreateServerNode(dbmodel.ServerNode{
		ID: "owner-node", Kind: storage.ServerNodeVKTurnProxy, Name: "owner relay",
		GRPCToken: "nodesecret", OwnerAdminID: 0,
	}); err != nil {
		t.Fatalf("owner node: %v", err)
	}
	if _, err := st.CreateServerNode(dbmodel.ServerNode{
		ID: "other-node", Kind: storage.ServerNodeVKTurnProxy, Name: "other relay",
		GRPCToken: "nodesecret", OwnerAdminID: otherAdmin.ID,
	}); err != nil {
		t.Fatalf("other node: %v", err)
	}
	if _, err := st.CreateServerNode(dbmodel.ServerNode{
		ID: "own-node", Kind: storage.ServerNodeVKTurnProxy, Name: "own relay",
		GRPCToken: "nodesecret", OwnerAdminID: plainAdmin.ID,
	}); err != nil {
		t.Fatalf("own node: %v", err)
	}

	fake := &fakeProvisioner{peer: Peer{PublicKey: "pub"}}
	svc := NewService(st, fake)
	// The relay authenticates correctly - the node token is NOT the boundary here.
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer nodesecret"))

	for _, nodeID := range []string{"owner-node", "other-node"} {
		req := &provisioningpb.ResolveClientConfigRequest{ClientId: "c1", Token: token, NodeId: nodeID}
		_, err := svc.ResolveClientConfig(ctx, req)
		if status.Code(err) != codes.PermissionDenied {
			t.Errorf("resolve on %s = %v, want PermissionDenied", nodeID, err)
		}
	}
	if fake.calls != 0 {
		t.Errorf("no peer may be provisioned for a refused client, calls = %d", fake.calls)
	}

	// The admin's own node still works.
	req := &provisioningpb.ResolveClientConfigRequest{ClientId: "c1", Token: token, NodeId: "own-node"}
	resp, err := svc.ResolveClientConfig(ctx, req)
	if err != nil {
		t.Fatalf("resolve on the admin's own node: %v", err)
	}
	if !resp.GetProvisionLocally() {
		t.Errorf("own node should provision, got %+v", resp)
	}
}

// The owner's own client may use the panel-local node (owner_admin_id 0).
func TestResolveAllowsOwnerClientOnPanelLocalNode(t *testing.T) {
	st, clientID, token := setup(t)
	svc := NewService(st, &fakeProvisioner{peer: Peer{PublicKey: "pub"}})
	// setup() creates an owner admin plus node n1 with owner_admin_id 0.
	resp, err := svc.ResolveClientConfig(context.Background(),
		&provisioningpb.ResolveClientConfigRequest{ClientId: clientID, Token: token, NodeId: "n1"})
	if err != nil {
		t.Fatalf("owner client on panel-local node: %v", err)
	}
	if !resp.GetProvisionLocally() {
		t.Errorf("want provision_locally, got %+v", resp)
	}
}

func TestResolveVerifiesNodeBearer(t *testing.T) {
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
	if _, err := st.CreateServerNode(dbmodel.ServerNode{
		ID: "n1", Kind: storage.ServerNodeVKTurnProxy, Name: "relay", GRPCToken: "nodesecret",
	}); err != nil {
		t.Fatalf("node: %v", err)
	}
	svc := NewService(st, &fakeProvisioner{peer: Peer{
		PublicKey: "pub", PrivateKey: "priv", AllowedIPs: "10.66.66.2/32", ServerPublicKey: "spub",
	}})
	req := &provisioningpb.ResolveClientConfigRequest{ClientId: "c1", Token: token, NodeId: "n1"}

	if _, err := svc.ResolveClientConfig(context.Background(), req); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("no bearer = %v, want Unauthenticated", err)
	}
	wrong := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer nope"))
	if _, err := svc.ResolveClientConfig(wrong, req); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("wrong bearer = %v, want Unauthenticated", err)
	}
	ok := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer nodesecret"))
	resp, err := svc.ResolveClientConfig(ok, req)
	if err != nil {
		t.Fatalf("right bearer: %v", err)
	}
	// own-wg first resolve tells the node to mint the peer locally and re-report.
	if !resp.GetProvisionLocally() {
		t.Fatalf("own-wg resolve should set provision_locally, got %+v", resp)
	}
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

	// First resolve: own-wg asks the node to mint the peer locally (no CreatePeer).
	resp, err := svc.ResolveClientConfig(context.Background(), req)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !resp.GetProvisionLocally() {
		t.Fatalf("first resolve should set provision_locally, got %+v", resp)
	}
	if fake.calls != 0 {
		t.Fatalf("origin node should not be dialed via CreatePeer, calls = %d", fake.calls)
	}

	// Report the locally-minted peer: the panel records it and returns the full
	// config (routing AllowedIPs + MTU come from the panel).
	report := &provisioningpb.ResolveClientConfigRequest{
		ClientId: clientID, Token: token, NodeId: "n1", Hwid: "hw",
		WgPublicKey: "pub", WgPrivateKey: "priv", WgAllowedIps: "10.66.66.2/32", WgServerPublicKey: "spub",
	}
	respR, err := svc.ResolveClientConfig(context.Background(), report)
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	wg := respR.GetWg()
	if wg.GetPrivateKey() != "priv" || wg.GetPublicKey() != "pub" || wg.GetAddress() != "10.66.66.2/32" ||
		wg.GetServerPublicKey() != "spub" || wg.GetAllowedIps() != "0.0.0.0/0" || wg.GetMtu() != 1280 {
		t.Fatalf("wg config wrong: %+v", wg)
	}

	// Re-resolve (no wg fields): returns the stored peer, never provisions again.
	fake.peer = Peer{PublicKey: "DIFFERENT"}
	resp2, err := svc.ResolveClientConfig(context.Background(), req)
	if err != nil {
		t.Fatalf("resolve 2: %v", err)
	}
	if resp2.GetProvisionLocally() {
		t.Fatalf("second resolve should return the stored peer, not provision_locally")
	}
	if resp2.GetWg().GetPublicKey() != "pub" {
		t.Fatalf("second resolve should return the stored peer, got %q", resp2.GetWg().GetPublicKey())
	}
	if fake.calls != 0 {
		t.Fatalf("provisioner should not be called for a single-node own-wg flow: %d", fake.calls)
	}
}

func TestResolveReplicatesPeerToOtherNodes(t *testing.T) {
	st, clientID, token := setup(t)
	if _, err := st.CreateServerNode(dbmodel.ServerNode{ID: "n2", Kind: storage.ServerNodeVKTurnProxy, Name: "relay2"}); err != nil {
		t.Fatalf("node n2: %v", err)
	}
	fake := &fakeProvisioner{peer: Peer{PublicKey: "pub", PrivateKey: "priv", AllowedIPs: "10.66.66.2/32", ServerPublicKey: "spub"}}
	svc := NewService(st, fake)
	// Report the peer the origin node (n1) minted locally; the panel records it on
	// n1 and replicates it to n2 via CreatePeer (one call: the origin is not dialed).
	report := &provisioningpb.ResolveClientConfigRequest{
		ClientId: clientID, Token: token, NodeId: "n1",
		WgPublicKey: "pub", WgPrivateKey: "priv", WgAllowedIps: "10.66.66.2/32", WgServerPublicKey: "spub",
	}
	if _, err := svc.ResolveClientConfig(context.Background(), report); err != nil {
		t.Fatalf("report: %v", err)
	}
	if fake.calls != 1 {
		t.Fatalf("provisioner calls = %d, want 1 (replica to n2 only; origin not dialed)", fake.calls)
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

// The enrollment QR carries one link to stay scannable, so the rest of the pool
// has to arrive on the provision the same enrolment performs. Without this a
// client is one dead link away from not connecting at all.
func TestResolveCarriesTheOwnersWholeLinkPool(t *testing.T) {
	st, clientID, token := setup(t)
	admins, err := st.ListAdmins()
	if err != nil || len(admins) == 0 {
		t.Fatalf("admins: %v", err)
	}
	pool := []string{"https://vk.com/call/a", "https://vk.com/call/b", "https://vk.com/call/c"}
	if err := st.SetAdminVKLinks(admins[0].ID, pool); err != nil {
		t.Fatalf("set links: %v", err)
	}

	svc := NewService(st, &fakeProvisioner{peer: Peer{PublicKey: "pub"}})
	// On the node's own wg the first call only says "mint it yourself"; the config
	// - and the pool with it - comes back when the node reports the peer
	if _, err := svc.ResolveClientConfig(context.Background(),
		&provisioningpb.ResolveClientConfigRequest{ClientId: clientID, Token: token, NodeId: "n1"}); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	resp, err := svc.ResolveClientConfig(context.Background(), &provisioningpb.ResolveClientConfigRequest{
		ClientId: clientID, Token: token, NodeId: "n1", Hwid: "hw",
		WgPublicKey: "pub", WgPrivateKey: "priv", WgAllowedIps: "10.66.66.2/32", WgServerPublicKey: "spub",
	})
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	got := resp.GetVkLinks()
	if len(got) != len(pool) {
		t.Fatalf("provision carried %d links, want the whole pool of %d: %v", len(got), len(pool), got)
	}
	for i, want := range pool {
		if got[i] != want {
			t.Errorf("link %d = %q, want %q", i, got[i], want)
		}
	}
}

// An admin with no pool is not an error: the client still gets its tunnel and
// whatever link its QR already carried.
func TestResolveWithNoLinksStillProvisions(t *testing.T) {
	st, clientID, token := setup(t)
	svc := NewService(st, &fakeProvisioner{peer: Peer{PublicKey: "pub"}})
	if _, err := svc.ResolveClientConfig(context.Background(),
		&provisioningpb.ResolveClientConfigRequest{ClientId: clientID, Token: token, NodeId: "n1"}); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	resp, err := svc.ResolveClientConfig(context.Background(), &provisioningpb.ResolveClientConfigRequest{
		ClientId: clientID, Token: token, NodeId: "n1", Hwid: "hw",
		WgPublicKey: "pub", WgPrivateKey: "priv", WgAllowedIps: "10.66.66.2/32", WgServerPublicKey: "spub",
	})
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	if len(resp.GetVkLinks()) != 0 {
		t.Errorf("links = %v, want none", resp.GetVkLinks())
	}
}
