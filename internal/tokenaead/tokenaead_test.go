package tokenaead

import (
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
