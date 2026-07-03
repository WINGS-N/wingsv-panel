// Package relayclient dials vk-turn-proxy nodes over the Relay gRPC API to
// create tunnel peers. It implements provisioning.PeerProvisioner, so the
// panel's Provisioning service can mint a client's WireGuard config on the
// assigned node during self-enroll.
package relayclient

import (
	"context"
	"fmt"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"v.wingsnet.org/internal/gen/relaypb"
	"v.wingsnet.org/internal/provisioning"
	"v.wingsnet.org/internal/storage/dbmodel"
)

// Provisioner creates peers by calling a node's Relay gRPC API. Nodes are few
// and self-enroll is rare, so it opens a short-lived connection per call.
type Provisioner struct {
	token       string
	creds       credentials.TransportCredentials
	dialContext func(context.Context, string) (net.Conn, error)
}

// Option configures a Provisioner.
type Option func(*Provisioner)

// WithTransportCredentials sets the gRPC transport credentials (for pinned-CA
// mTLS to the node). The default is insecure.
func WithTransportCredentials(c credentials.TransportCredentials) Option {
	return func(p *Provisioner) { p.creds = c }
}

// WithContextDialer overrides how connections are dialed (used by tests).
func WithContextDialer(d func(context.Context, string) (net.Conn, error)) Option {
	return func(p *Provisioner) { p.dialContext = d }
}

// New builds a Provisioner. token, when set, is sent as a bearer credential the
// node's Relay API checks.
func New(token string, opts ...Option) *Provisioner {
	p := &Provisioner{token: token, creds: insecure.NewCredentials()}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func (p *Provisioner) dial(node dbmodel.ServerNode) (*grpc.ClientConn, error) {
	dialOpts := []grpc.DialOption{grpc.WithTransportCredentials(p.creds)}
	if p.dialContext != nil {
		dialOpts = append(dialOpts, grpc.WithContextDialer(p.dialContext))
	}
	return grpc.NewClient(node.GRPCEndpoint, dialOpts...)
}

func (p *Provisioner) authCtx(ctx context.Context) context.Context {
	if p.token != "" {
		return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+p.token)
	}
	return ctx
}

// CreatePeer implements provisioning.PeerProvisioner.
func (p *Provisioner) CreatePeer(ctx context.Context, node dbmodel.ServerNode, publicKey string) (provisioning.Peer, error) {
	conn, err := p.dial(node)
	if err != nil {
		return provisioning.Peer{}, fmt.Errorf("dial node %s: %w", node.GRPCEndpoint, err)
	}
	defer func() { _ = conn.Close() }()

	peer, err := relaypb.NewRelayClient(conn).CreatePeer(p.authCtx(ctx), &relaypb.CreatePeerRequest{PublicKey: publicKey})
	if err != nil {
		return provisioning.Peer{}, err
	}
	return provisioning.Peer{
		PublicKey:       peer.GetPublicKey(),
		PrivateKey:      peer.GetPrivateKey(),
		AllowedIPs:      peer.GetAllowedIps(),
		ServerPublicKey: peer.GetServerPublicKey(),
	}, nil
}

// RelayStatus is a node's live status, used by the metrics collector.
type RelayStatus struct {
	PeerCount      uint32
	ActiveSessions uint64
	RxBytes        uint64
	TxBytes        uint64
}

// NodeStatus queries a node's Relay API for its status and aggregate peer
// traffic.
func (p *Provisioner) NodeStatus(ctx context.Context, node dbmodel.ServerNode) (RelayStatus, error) {
	conn, err := p.dial(node)
	if err != nil {
		return RelayStatus{}, fmt.Errorf("dial node %s: %w", node.GRPCEndpoint, err)
	}
	defer func() { _ = conn.Close() }()

	client := relaypb.NewRelayClient(conn)
	ctx = p.authCtx(ctx)
	st, err := client.GetStatus(ctx, &relaypb.GetStatusRequest{})
	if err != nil {
		return RelayStatus{}, err
	}
	rs := RelayStatus{PeerCount: st.GetPeerCount(), ActiveSessions: st.GetActiveSessions()}
	if peers, err := client.ListPeers(ctx, &relaypb.ListPeersRequest{}); err == nil {
		for _, peer := range peers.GetPeers() {
			rs.RxBytes += peer.GetRxBytes()
			rs.TxBytes += peer.GetTxBytes()
		}
	}
	return rs, nil
}
