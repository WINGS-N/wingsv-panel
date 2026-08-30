package tokenaead

import (
	"bytes"
	"context"
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
		deriveKeyVariant(Legacy256, []byte(secret), "c2s"),
		deriveKeyVariant(SHA512, []byte(secret), "c2s"),
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
