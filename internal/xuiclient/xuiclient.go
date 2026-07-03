// Package xuiclient is the panel's gRPC client to a WINGS-N/3x-ui node's Panel
// API (M1). It creates/reads inbound clients and reads server status/traffic on
// nodes whose kind is "xui", using the per-node bearer token for auth.
package xuiclient

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"

	"v.wingsnet.org/internal/gen/xuipb"
	"v.wingsnet.org/internal/storage/dbmodel"
)

// Client talks to 3x-ui nodes. Each call uses the node's stored gRPC token.
type Client struct {
	creds       credentials.TransportCredentials
	dialContext func(context.Context, string) (net.Conn, error)
}

// Option configures a Client.
type Option func(*Client)

// WithTransportCredentials sets the gRPC transport credentials (pinned-CA mTLS).
func WithTransportCredentials(c credentials.TransportCredentials) Option {
	return func(x *Client) { x.creds = c }
}

// WithContextDialer overrides how connections are dialed (used by tests).
func WithContextDialer(d func(context.Context, string) (net.Conn, error)) Option {
	return func(x *Client) { x.dialContext = d }
}

func New(opts ...Option) *Client {
	// 3x-ui serves its Panel gRPC over TLS (reusing the panel web certificate).
	// Encrypt the transport but skip chain verification - the bearer API token is
	// the authenticator, and the node cert may not match its gRPC hostname.
	c := &Client{creds: credentials.NewTLS(&tls.Config{InsecureSkipVerify: true})}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Client) dial(node dbmodel.ServerNode) (*grpc.ClientConn, error) {
	dialOpts := []grpc.DialOption{grpc.WithTransportCredentials(c.creds)}
	if c.dialContext != nil {
		dialOpts = append(dialOpts, grpc.WithContextDialer(c.dialContext))
	}
	return grpc.NewClient(node.GRPCEndpoint, dialOpts...)
}

func authCtx(ctx context.Context, token string) context.Context {
	if token == "" {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
}

// InboundSummary is one inbound on a 3x-ui node.
type InboundSummary struct {
	ID       int64
	Tag      string
	Remark   string
	Protocol string
	Port     int32
	Enable   bool
	Up       int64
	Down     int64
	Total    int64
}

// ServerStatus mirrors the node's resource + xray state.
type ServerStatus struct {
	Cpu         float64
	MemCurrent  uint64
	MemTotal    uint64
	Uptime      uint64
	XrayState   string
	XrayVersion string
	NetUp       uint64
	NetDown     uint64
}

// ClientTraffic is one inbound client's counters.
type ClientTraffic struct {
	Email      string
	Enable     bool
	Up         int64
	Down       int64
	Total      int64
	ExpiryTime int64
	LastOnline int64
}

// AddClient creates an inbound client from the JSON payload the 3x-ui REST API
// binds, returning whether xray needs a restart to apply it.
func (c *Client) AddClient(ctx context.Context, node dbmodel.ServerNode, payloadJSON string) (bool, error) {
	conn, err := c.dial(node)
	if err != nil {
		return false, fmt.Errorf("dial xui node %s: %w", node.GRPCEndpoint, err)
	}
	defer func() { _ = conn.Close() }()
	resp, err := xuipb.NewPanelClient(conn).AddClient(
		authCtx(ctx, node.GRPCToken),
		&xuipb.AddClientRequest{PayloadJson: payloadJSON},
	)
	if err != nil {
		return false, err
	}
	if !resp.GetOk() {
		return resp.GetNeedRestart(), fmt.Errorf("xui rejected the client")
	}
	return resp.GetNeedRestart(), nil
}

// DeleteClient removes an inbound client by email.
func (c *Client) DeleteClient(ctx context.Context, node dbmodel.ServerNode, email string, keepTraffic bool) (bool, error) {
	conn, err := c.dial(node)
	if err != nil {
		return false, fmt.Errorf("dial xui node %s: %w", node.GRPCEndpoint, err)
	}
	defer func() { _ = conn.Close() }()
	resp, err := xuipb.NewPanelClient(conn).DeleteClient(
		authCtx(ctx, node.GRPCToken),
		&xuipb.DeleteClientRequest{Email: email, KeepTraffic: keepTraffic},
	)
	if err != nil {
		return false, err
	}
	return resp.GetNeedRestart(), nil
}

// ListInbounds returns the node's inbounds with aggregate counters.
func (c *Client) ListInbounds(ctx context.Context, node dbmodel.ServerNode) ([]InboundSummary, error) {
	conn, err := c.dial(node)
	if err != nil {
		return nil, fmt.Errorf("dial xui node %s: %w", node.GRPCEndpoint, err)
	}
	defer func() { _ = conn.Close() }()
	resp, err := xuipb.NewPanelClient(conn).ListInbounds(authCtx(ctx, node.GRPCToken), &xuipb.ListInboundsRequest{})
	if err != nil {
		return nil, err
	}
	out := make([]InboundSummary, 0, len(resp.GetInbounds()))
	for _, in := range resp.GetInbounds() {
		out = append(out, InboundSummary{
			ID: in.GetId(), Tag: in.GetTag(), Remark: in.GetRemark(), Protocol: in.GetProtocol(),
			Port: in.GetPort(), Enable: in.GetEnable(), Up: in.GetUp(), Down: in.GetDown(), Total: in.GetTotal(),
		})
	}
	return out, nil
}

// ServerStatus reads the node's resource + xray status.
func (c *Client) ServerStatus(ctx context.Context, node dbmodel.ServerNode) (ServerStatus, error) {
	conn, err := c.dial(node)
	if err != nil {
		return ServerStatus{}, fmt.Errorf("dial xui node %s: %w", node.GRPCEndpoint, err)
	}
	defer func() { _ = conn.Close() }()
	st, err := xuipb.NewPanelClient(conn).GetServerStatus(authCtx(ctx, node.GRPCToken), &xuipb.GetServerStatusRequest{})
	if err != nil {
		return ServerStatus{}, err
	}
	return ServerStatus{
		Cpu: st.GetCpu(), MemCurrent: st.GetMemCurrent(), MemTotal: st.GetMemTotal(), Uptime: st.GetUptime(),
		XrayState: st.GetXrayState(), XrayVersion: st.GetXrayVersion(), NetUp: st.GetNetUp(), NetDown: st.GetNetDown(),
	}, nil
}
