// Package xuiclient is the panel's gRPC client to a WINGS-N/3x-ui node's Panel
// API (M1). It creates/reads inbound clients and reads server status/traffic on
// nodes whose kind is "xui", using the per-node bearer token for auth.
package xuiclient

import (
	"context"
	"fmt"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"

	"v.wingsnet.org/internal/gen/xuipb"
	"v.wingsnet.org/internal/storage/dbmodel"
	"v.wingsnet.org/internal/tokenaead"
)

// Client talks to 3x-ui nodes. Each call uses the node's stored gRPC token.
type Client struct {
	creds       credentials.TransportCredentials
	dialContext func(context.Context, string) (net.Conn, error)
}

// Option configures a Client.
type Option func(*Client)

// WithTransportCredentials pins the gRPC transport credentials for every node,
// overriding the per-node token-derived transport (used by tests).
func WithTransportCredentials(c credentials.TransportCredentials) Option {
	return func(x *Client) { x.creds = c }
}

// WithContextDialer overrides how connections are dialed (used by tests).
func WithContextDialer(d func(context.Context, string) (net.Conn, error)) Option {
	return func(x *Client) { x.dialContext = d }
}

func New(opts ...Option) *Client {
	// No transport is fixed here: the connection is encrypted with keys derived
	// from the node's own token, so the credentials are built per node in dial.
	c := &Client{}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Client) dial(node dbmodel.ServerNode) (*grpc.ClientConn, error) {
	// The management channel is AES-256-GCM keyed by the shared node token
	// (tokenaead), not TLS: 3x-ui has no certificate of its own to offer, and the
	// previous InsecureSkipVerify TLS encrypted the stream without authenticating
	// the peer - so a MITM could take the bearer token straight off the wire.
	// Here a wrong token simply cannot decrypt the stream.
	//
	// The secret is the token's SHA-256 hex digest, because that is the only form
	// the node has: `x-ui grpc-connect` stores crypto.HashTokenSHA256(token) and
	// never keeps the original.
	creds := c.creds
	if creds == nil {
		if node.GRPCToken == "" {
			return nil, fmt.Errorf("node %s has no gRPC token to key the transport", node.ID)
		}
		// The key derivation is chosen per node from what that node last answered
		// on, because the fleet is mid-migration from SHA-256 to SHA-512 and
		// nodes update on their own schedule. The shared secret itself stays the
		// SHA-256 digest either way: that is the only form a node holds.
		creds = tokenaead.ClientFor(tokenaead.HashSecret(node.GRPCToken), node.ID, tokenaead.Peers)
	}
	dialOpts := []grpc.DialOption{grpc.WithTransportCredentials(creds)}
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
	InboundID  int64
	Enable     bool
	Up         int64
	Down       int64
	Total      int64
	ExpiryTime int64
	LastOnline int64
}

// WireguardClient is a wg peer minted on a 3x-ui node's WireGuard inbound.
type WireguardClient struct {
	PrivateKey      string
	PublicKey       string
	Address         string
	ServerPublicKey string
	MTU             uint32
	Endpoint        string
}

// CreateWireguardClient adds a wg peer to a node's WireGuard inbound and returns
// its config. Idempotent on clientID. An empty inboundTag selects the first
// WireGuard inbound.
func (c *Client) CreateWireguardClient(ctx context.Context, node dbmodel.ServerNode, inboundTag, clientID, clientName string) (WireguardClient, error) {
	conn, err := c.dial(node)
	if err != nil {
		return WireguardClient{}, fmt.Errorf("dial xui node %s: %w", node.GRPCEndpoint, err)
	}
	defer func() { _ = conn.Close() }()
	resp, err := xuipb.NewPanelClient(conn).CreateWireguardClient(
		authCtx(ctx, node.GRPCToken),
		&xuipb.CreateWireguardClientRequest{InboundTag: inboundTag, ClientId: clientID, ClientName: clientName},
	)
	if err != nil {
		return WireguardClient{}, err
	}
	return WireguardClient{
		PrivateKey:      resp.GetPrivateKey(),
		PublicKey:       resp.GetPublicKey(),
		Address:         resp.GetAddress(),
		ServerPublicKey: resp.GetServerPublicKey(),
		MTU:             resp.GetMtu(),
		Endpoint:        resp.GetEndpoint(),
	}, nil
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

// ListOnlineClients returns the emails of clients currently online on the node.
func (c *Client) ListOnlineClients(ctx context.Context, node dbmodel.ServerNode) ([]string, error) {
	conn, err := c.dial(node)
	if err != nil {
		return nil, fmt.Errorf("dial xui node %s: %w", node.GRPCEndpoint, err)
	}
	defer func() { _ = conn.Close() }()
	resp, err := xuipb.NewPanelClient(conn).ListOnlineClients(authCtx(ctx, node.GRPCToken), &xuipb.ListOnlineClientsRequest{})
	if err != nil {
		return nil, err
	}
	return resp.GetEmails(), nil
}

// GetClientTraffic reads one client's cumulative counters by email.
func (c *Client) GetClientTraffic(ctx context.Context, node dbmodel.ServerNode, email string) (ClientTraffic, error) {
	conn, err := c.dial(node)
	if err != nil {
		return ClientTraffic{}, fmt.Errorf("dial xui node %s: %w", node.GRPCEndpoint, err)
	}
	defer func() { _ = conn.Close() }()
	t, err := xuipb.NewPanelClient(conn).GetClientTraffic(authCtx(ctx, node.GRPCToken), &xuipb.GetClientTrafficRequest{Email: email})
	if err != nil {
		return ClientTraffic{}, err
	}
	return ClientTraffic{
		Email: t.GetEmail(), InboundID: t.GetInboundId(), Enable: t.GetEnable(),
		Up: t.GetUp(), Down: t.GetDown(), Total: t.GetTotal(),
		ExpiryTime: t.GetExpiryTime(), LastOnline: t.GetLastOnline(),
	}, nil
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
