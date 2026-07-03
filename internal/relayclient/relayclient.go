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

// CreatePeer implements provisioning.PeerProvisioner.
func (p *Provisioner) CreatePeer(ctx context.Context, node dbmodel.ServerNode, publicKey string) (provisioning.Peer, error) {
	dialOpts := []grpc.DialOption{grpc.WithTransportCredentials(p.creds)}
	if p.dialContext != nil {
		dialOpts = append(dialOpts, grpc.WithContextDialer(p.dialContext))
	}
	conn, err := grpc.NewClient(node.GRPCEndpoint, dialOpts...)
	if err != nil {
		return provisioning.Peer{}, fmt.Errorf("dial node %s: %w", node.GRPCEndpoint, err)
	}
	defer func() { _ = conn.Close() }()

	if p.token != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+p.token)
	}
	peer, err := relaypb.NewRelayClient(conn).CreatePeer(ctx, &relaypb.CreatePeerRequest{PublicKey: publicKey})
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
