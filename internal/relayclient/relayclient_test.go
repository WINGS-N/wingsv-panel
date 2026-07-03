package relayclient

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"

	"v.wingsnet.org/internal/gen/relaypb"
	"v.wingsnet.org/internal/storage/dbmodel"
)

type stubRelay struct {
	relaypb.UnimplementedRelayServer
	gotToken string
	gotPub   string
}

func (s *stubRelay) CreatePeer(ctx context.Context, req *relaypb.CreatePeerRequest) (*relaypb.Peer, error) {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if vals := md.Get("authorization"); len(vals) > 0 {
			s.gotToken = vals[0]
		}
	}
	s.gotPub = req.GetPublicKey()
	return &relaypb.Peer{
		PublicKey: "pub", PrivateKey: "priv", AllowedIps: "10.66.66.2/32", ServerPublicKey: "spub",
	}, nil
}

func TestCreatePeerMapsResponse(t *testing.T) {
	lis := bufconn.Listen(1 << 20)
	gs := grpc.NewServer()
	stub := &stubRelay{}
	relaypb.RegisterRelayServer(gs, stub)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)

	prov := New("s3cret", WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.DialContext(ctx)
	}))
	peer, err := prov.CreatePeer(
		context.Background(),
		dbmodel.ServerNode{ID: "n1", GRPCEndpoint: "passthrough:///bufnet"},
		"clientpub",
	)
	if err != nil {
		t.Fatalf("CreatePeer: %v", err)
	}
	if peer.PublicKey != "pub" || peer.PrivateKey != "priv" ||
		peer.AllowedIPs != "10.66.66.2/32" || peer.ServerPublicKey != "spub" {
		t.Fatalf("mapping wrong: %+v", peer)
	}
	if stub.gotToken != "Bearer s3cret" {
		t.Fatalf("token = %q, want 'Bearer s3cret'", stub.gotToken)
	}
	if stub.gotPub != "clientpub" {
		t.Fatalf("forwarded public key = %q, want clientpub", stub.gotPub)
	}
}
