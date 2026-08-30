package xuiclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/test/bufconn"

	"v.wingsnet.org/internal/gen/xuipb"
	"v.wingsnet.org/internal/storage/dbmodel"
	"v.wingsnet.org/internal/tokenaead"
)

// storedNodeSecret mirrors what 3x-ui persists for the panel token:
// crypto.HashTokenSHA256(token), i.e. the lowercase hex SHA-256 digest. It is
// duplicated here on purpose - if the two implementations ever diverge, the
// management channel silently stops keying the same way, and this test is the
// only place that would catch it.
func storedNodeSecret(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// The panel derives the transport secret from the raw token; the node only ever
// has the stored digest. They must land on the same value or nothing connects.
func TestHashSecretMatchesWhatTheNodeStores(t *testing.T) {
	const token = "xui-tok"
	if got, want := tokenaead.HashSecret(token), storedNodeSecret(token); got != want {
		t.Fatalf("HashSecret = %q, node stores %q", got, want)
	}
}

// serveAEAD stands in for an updated node, which accepts both derivations.
func serveAEAD(t *testing.T, secret string, stub xuipb.PanelServer) *bufconn.Listener {
	t.Helper()
	return serveAEADWith(t, stub, tokenaead.ServerAny(secret, tokenaead.SHA512, tokenaead.Legacy256))
}

func serveAEADWith(t *testing.T, stub xuipb.PanelServer, creds credentials.TransportCredentials) *bufconn.Listener {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	gs := grpc.NewServer(grpc.Creds(creds))
	xuipb.RegisterPanelServer(gs, stub)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)
	return lis
}

// freshPeers isolates the process-wide derivation memory from other tests.
func freshPeers(t *testing.T) {
	t.Helper()
	previous := tokenaead.Peers
	tokenaead.Peers = tokenaead.NewPreference()
	t.Cleanup(func() { tokenaead.Peers = previous })
}

// End to end over the real transport: a node keyed by the stored digest accepts a
// panel that only knows the raw token, with no certificate anywhere.
func TestAEADTransportRoundTrip(t *testing.T) {
	const token = "xui-tok"
	stub := &stubPanel{}
	lis := serveAEAD(t, storedNodeSecret(token), stub)

	client := New(WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.DialContext(ctx)
	}))
	node := dbmodel.ServerNode{ID: "x1", Kind: "xui", GRPCEndpoint: "passthrough:///bufnet", GRPCToken: token}

	needRestart, err := client.AddClient(context.Background(), node, `{"email":"u1"}`)
	if err != nil {
		t.Fatalf("AddClient over tokenaead: %v", err)
	}
	if !needRestart {
		t.Fatal("want need_restart=true")
	}
	if stub.gotPayload != `{"email":"u1"}` {
		t.Fatalf("payload = %q", stub.gotPayload)
	}
}

// A wrong token must not merely fail authorization - it must fail to decrypt, so
// the call never reaches the service at all. This is the property that replaces
// the certificate check the old InsecureSkipVerify dial never performed.
func TestAEADTransportRejectsWrongToken(t *testing.T) {
	stub := &stubPanel{}
	lis := serveAEAD(t, storedNodeSecret("the-real-token"), stub)

	client := New(WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.DialContext(ctx)
	}))
	node := dbmodel.ServerNode{ID: "x1", Kind: "xui", GRPCEndpoint: "passthrough:///bufnet", GRPCToken: "wrong-token"}

	if _, err := client.AddClient(context.Background(), node, `{"email":"u1"}`); err == nil {
		t.Fatal("want failure with a mismatched token, got success")
	}
	if stub.gotPayload != "" {
		t.Fatalf("request reached the service despite a bad token: %q", stub.gotPayload)
	}
}

// A node with no token cannot key the transport, and dialing must say so rather
// than falling back to something unencrypted.
func TestDialRefusesNodeWithoutToken(t *testing.T) {
	client := New()
	node := dbmodel.ServerNode{ID: "x1", Kind: "xui", GRPCEndpoint: "passthrough:///bufnet"}
	if _, err := client.dial(node); err == nil {
		t.Fatal("want an error for a node with no gRPC token")
	}
}

// A node that has not been updated yet speaks only the old derivation. The panel
// must find that out and settle on it by itself, because nobody is going to
// configure a per-node setting for 42 admins' servers.
func TestPanelConvergesOnANodeStillOnTheOldDerivation(t *testing.T) {
	freshPeers(t)
	const token = "xui-tok"
	stub := &stubPanel{}
	lis := serveAEADWith(t, stub, tokenaead.Server(storedNodeSecret(token)))

	client := New(WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.DialContext(ctx)
	}))
	node := dbmodel.ServerNode{ID: "old-node", Kind: "xui", GRPCEndpoint: "passthrough:///bufnet", GRPCToken: token}

	// The first attempt is spent discovering the node is older. That cost is the
	// price of having no handshake to ask on.
	if _, err := client.AddClient(context.Background(), node, `{"email":"u1"}`); err == nil {
		t.Fatal("a legacy-only node accepted the new derivation")
	}
	if v, ok := tokenaead.Peers.Known("old-node"); !ok || v != tokenaead.Legacy256 {
		t.Fatalf("panel did not remember the node as legacy: %v ok=%v", v, ok)
	}

	// Every attempt after that works, without anything being configured.
	for i := 0; i < 3; i++ {
		if _, err := client.AddClient(context.Background(), node, `{"email":"u1"}`); err != nil {
			t.Fatalf("attempt %d after converging: %v", i+2, err)
		}
	}
}

// An updated node must be reached on SHA-512 on the very first call, or the
// migration would be pointless.
func TestUpdatedNodeIsDialledOnTheNewDerivationImmediately(t *testing.T) {
	freshPeers(t)
	const token = "xui-tok"
	stub := &stubPanel{}
	lis := serveAEAD(t, storedNodeSecret(token), stub)

	client := New(WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.DialContext(ctx)
	}))
	node := dbmodel.ServerNode{ID: "new-node", Kind: "xui", GRPCEndpoint: "passthrough:///bufnet", GRPCToken: token}

	if _, err := client.AddClient(context.Background(), node, `{"email":"u1"}`); err != nil {
		t.Fatalf("AddClient: %v", err)
	}
	if v, ok := tokenaead.Peers.Known("new-node"); !ok || v != tokenaead.SHA512 {
		t.Fatalf("node settled on %v ok=%v, want sha512", v, ok)
	}
}
