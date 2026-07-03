// Package relayclient dials vk-turn-proxy nodes over the Relay gRPC API to
// create tunnel peers. It implements provisioning.PeerProvisioner, so the
// panel's Provisioning service can mint a client's WireGuard config on the
// assigned node during self-enroll.
package relayclient

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
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
// node's Relay API checks. The transport is TLS by default (chain verification is
// skipped - the node cert may be self-signed and the bearer token authenticates);
// pass WithTransportCredentials to pin the panel CA instead.
func New(token string, opts ...Option) *Provisioner {
	p := &Provisioner{token: token, creds: credentials.NewTLS(&tls.Config{InsecureSkipVerify: true})}
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
// CreatePeer creates (or replicates) a wg peer on a node. An empty publicKey
// asks the node to generate a keypair; an empty allowedIPs asks it to allocate an
// address. Passing both replicates an existing peer onto another node with the
// same key and tunnel address (roaming).
func (p *Provisioner) CreatePeer(ctx context.Context, node dbmodel.ServerNode, publicKey, allowedIPs string) (provisioning.Peer, error) {
	conn, err := p.dial(node)
	if err != nil {
		return provisioning.Peer{}, fmt.Errorf("dial node %s: %w", node.GRPCEndpoint, err)
	}
	defer func() { _ = conn.Close() }()

	peer, err := relaypb.NewRelayClient(conn).CreatePeer(
		p.authCtx(ctx),
		&relaypb.CreatePeerRequest{PublicKey: publicKey, AllowedIps: allowedIPs},
	)
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

// Flow is one active relay stream on a node.
type Flow struct {
	SessionID   string
	StreamID    uint32
	ClientIP    string
	Remote      string
	Protocol    string
	Version     uint32
	RxBytes     uint64
	TxBytes     uint64
	RxRate      uint64
	TxRate      uint64
	StartedUnix int64
}

// FlowStats aggregates a node's relay activity.
type FlowStats struct {
	ActiveStreams             uint32
	ActiveSessions            uint32
	TotalSessions             uint64
	AvgSessionLifetimeSeconds float64
	ServerRxBytes             uint64
	ServerTxBytes             uint64
	StreamsByProtocol         map[string]uint32
}

// ListFlows returns the node's active relay streams (client IP:port, session and
// stream ids, per-flow traffic) for the panel's flow view.
func (p *Provisioner) ListFlows(ctx context.Context, node dbmodel.ServerNode) ([]Flow, error) {
	conn, err := p.dial(node)
	if err != nil {
		return nil, fmt.Errorf("dial node %s: %w", node.GRPCEndpoint, err)
	}
	defer func() { _ = conn.Close() }()

	resp, err := relaypb.NewRelayClient(conn).ListFlows(p.authCtx(ctx), &relaypb.ListFlowsRequest{})
	if err != nil {
		return nil, err
	}
	out := make([]Flow, 0, len(resp.GetFlows()))
	for _, f := range resp.GetFlows() {
		out = append(out, Flow{
			SessionID:   f.GetSessionId(),
			StreamID:    f.GetStreamId(),
			ClientIP:    f.GetClientIp(),
			Remote:      f.GetRemote(),
			Protocol:    f.GetProtocol(),
			Version:     f.GetVersion(),
			RxBytes:     f.GetRxBytes(),
			TxBytes:     f.GetTxBytes(),
			RxRate:      f.GetRxRate(),
			TxRate:      f.GetTxRate(),
			StartedUnix: f.GetStartedUnix(),
		})
	}
	return out, nil
}

// FlowStats returns a node's aggregate relay activity.
func (p *Provisioner) FlowStats(ctx context.Context, node dbmodel.ServerNode) (FlowStats, error) {
	conn, err := p.dial(node)
	if err != nil {
		return FlowStats{}, fmt.Errorf("dial node %s: %w", node.GRPCEndpoint, err)
	}
	defer func() { _ = conn.Close() }()

	st, err := relaypb.NewRelayClient(conn).GetFlowStats(p.authCtx(ctx), &relaypb.GetFlowStatsRequest{})
	if err != nil {
		return FlowStats{}, err
	}
	return FlowStats{
		ActiveStreams:             st.GetActiveStreams(),
		ActiveSessions:            st.GetActiveSessions(),
		TotalSessions:             st.GetTotalSessions(),
		AvgSessionLifetimeSeconds: st.GetAvgSessionLifetimeSeconds(),
		ServerRxBytes:             st.GetServerRxBytes(),
		ServerTxBytes:             st.GetServerTxBytes(),
		StreamsByProtocol:         st.GetStreamsByProtocol(),
	}, nil
}

// NodeStatus queries a node's Relay API for its status and aggregate peer
// traffic.
// Peer is one wg peer's live counters, used to attribute per-client traffic.
type Peer struct {
	PublicKey   string
	AllowedIPs  string
	RxBytes     uint64
	TxBytes     uint64
	CreatedUnix int64
}

// ListPeers returns every wg peer on a node with its byte counters.
func (p *Provisioner) ListPeers(ctx context.Context, node dbmodel.ServerNode) ([]Peer, error) {
	conn, err := p.dial(node)
	if err != nil {
		return nil, fmt.Errorf("dial node %s: %w", node.GRPCEndpoint, err)
	}
	defer func() { _ = conn.Close() }()
	client := relaypb.NewRelayClient(conn)
	resp, err := client.ListPeers(p.authCtx(ctx), &relaypb.ListPeersRequest{})
	if err != nil {
		return nil, err
	}
	out := make([]Peer, 0, len(resp.GetPeers()))
	for _, pr := range resp.GetPeers() {
		out = append(out, Peer{
			PublicKey:   pr.GetPublicKey(),
			AllowedIPs:  pr.GetAllowedIps(),
			RxBytes:     pr.GetRxBytes(),
			TxBytes:     pr.GetTxBytes(),
			CreatedUnix: pr.GetCreatedUnix(),
		})
	}
	return out, nil
}

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
