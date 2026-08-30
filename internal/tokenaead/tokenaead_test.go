package tokenaead

import (
	"bytes"
	"context"
	"encoding/hex"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func startServer(t *testing.T, token string) (addr string, stop func()) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer(grpc.Creds(Server(token)))
	healthpb.RegisterHealthServer(srv, health.NewServer())
	go func() { _ = srv.Serve(lis) }()
	return lis.Addr().String(), srv.Stop
}

func dialCheck(t *testing.T, addr string, creds credentials.TransportCredentials) error {
	t.Helper()
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(creds))
	if err != nil {
		return err
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err = healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{})
	return err
}

func TestRoundTripSameToken(t *testing.T) {
	addr, stop := startServer(t, "s3cr3t-token-abc")
	defer stop()
	if err := dialCheck(t, addr, Client("s3cr3t-token-abc")); err != nil {
		t.Fatalf("same-token RPC failed: %v", err)
	}
}

func TestWrongTokenFails(t *testing.T) {
	addr, stop := startServer(t, "s3cr3t-token-abc")
	defer stop()
	if err := dialCheck(t, addr, Client("WRONG-token-xyz")); err == nil {
		t.Fatal("wrong-token RPC unexpectedly succeeded")
	}
}

// The two variants must not interoperate: if a SHA-512 peer could talk to a
// SHA-256 one, the derivation would not actually be doing anything.
func TestVariantsDoNotInteroperate(t *testing.T) {
	const secret = "shared-secret"
	if bytes.Equal(
		deriveKey(Legacy256, []byte(secret), "c2s"),
		deriveKey(SHA512, []byte(secret), "c2s"),
	) {
		t.Fatal("both variants derived the same key")
	}
}

// The default has to stay SHA-256, because every deployed 3x-ui node and relay
// derives that way and a changed default silently breaks all of them.
func TestDefaultStaysLegacyForDeployedPeers(t *testing.T) {
	const secret = "shared-secret"
	if !bytes.Equal(
		Client(secret).c2s,
		ClientVariant(secret, Legacy256).c2s,
	) {
		t.Error("Client no longer derives the way deployed nodes do")
	}
	if !bytes.Equal(Server(secret).s2c, ServerVariant(secret, Legacy256).s2c) {
		t.Error("Server no longer derives the way deployed nodes do")
	}
}

// This package exists in four repositories - the panel, the federation head and
// agent, the 3x-ui fork and the vk-turn-proxy relay - and every pair of them has
// to derive identical keys or the management channel silently stops connecting.
// These vectors are the cheap way to catch a copy drifting: they are the same
// literals in all four, so a change that only lands in one fails there.
func TestKeyDerivationFixture(t *testing.T) {
	const fixtureSecret = "interop-fixture-secret"
	for _, tc := range []struct {
		variant Variant
		c2s     string
	}{
		{Legacy256, "2118ec3bf68fdafbe7114b757123a0b32ea1b53399fcd017a4a1bc04940352c5"},
		{SHA512, "2697bf261454d5cace02abca3205991d3d77c4ea73f257d57459252285603eaf"},
	} {
		got := hex.EncodeToString(deriveKey(tc.variant, []byte(fixtureSecret), "c2s"))
		if got != tc.c2s {
			t.Errorf("%s c2s = %s, want %s (this copy has drifted from the other repositories)", tc.variant, got, tc.c2s)
		}
	}
}

// A peer stuck on the old derivation must be reached through the very same
// Creds the first, failed attempt used. grpc keeps one Creds for the life of a
// ClientConn and reconnects through it, so if the keys were fixed when the
// Creds was built, the preference could flip for ever and every retry would
// still carry the derivation that had already been refused.
func TestOneCredsSwitchesDerivationAfterAFailure(t *testing.T) {
	const token = "a-peer-that-never-upgraded"
	// The server speaks only the old derivation, as a deployed relay does.
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer(grpc.Creds(ServerVariant(token, Legacy256)))
	healthpb.RegisterHealthServer(srv, health.NewServer())
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	pref := NewPreference()
	creds := ClientFor(token, "legacy-peer", pref)
	if creds.variant != SHA512 {
		t.Fatalf("first attempt used %v, want sha512 to be tried first", creds.variant)
	}

	// The first dial is expected to fail; what matters is that the second one,
	// through this same Creds, gets through.
	_ = dialCheck(t, lis.Addr().String(), creds)

	var lastErr error
	for i := 0; i < 5; i++ {
		if lastErr = dialCheck(t, lis.Addr().String(), creds); lastErr == nil {
			break
		}
	}
	if lastErr != nil {
		t.Fatalf("the same Creds never reached the legacy peer: %v", lastErr)
	}
	if v, ok := pref.Known("legacy-peer"); !ok || v != Legacy256 {
		t.Errorf("preference settled on %v (known=%v), want the legacy derivation", v, ok)
	}
}
