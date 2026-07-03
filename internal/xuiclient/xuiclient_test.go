package xuiclient

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"

	"v.wingsnet.org/internal/gen/xuipb"
	"v.wingsnet.org/internal/storage/dbmodel"
)

type stubPanel struct {
	xuipb.UnimplementedPanelServer
	gotToken   string
	gotPayload string
}

func (s *stubPanel) AddClient(ctx context.Context, req *xuipb.AddClientRequest) (*xuipb.MutationResponse, error) {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if vals := md.Get("authorization"); len(vals) > 0 {
			s.gotToken = vals[0]
		}
	}
	s.gotPayload = req.GetPayloadJson()
	return &xuipb.MutationResponse{Ok: true, NeedRestart: true}, nil
}

func TestAddClientSendsTokenAndPayload(t *testing.T) {
	lis := bufconn.Listen(1 << 20)
	gs := grpc.NewServer()
	stub := &stubPanel{}
	xuipb.RegisterPanelServer(gs, stub)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)

	client := New(
		WithTransportCredentials(insecure.NewCredentials()),
		WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
	)
	node := dbmodel.ServerNode{ID: "x1", Kind: "xui", GRPCEndpoint: "passthrough:///bufnet", GRPCToken: "xui-tok"}
	needRestart, err := client.AddClient(context.Background(), node, `{"email":"u1"}`)
	if err != nil {
		t.Fatalf("AddClient: %v", err)
	}
	if !needRestart {
		t.Fatal("want need_restart=true")
	}
	if stub.gotToken != "Bearer xui-tok" {
		t.Fatalf("token = %q, want 'Bearer xui-tok'", stub.gotToken)
	}
	if stub.gotPayload != `{"email":"u1"}` {
		t.Fatalf("payload = %q", stub.gotPayload)
	}
}
