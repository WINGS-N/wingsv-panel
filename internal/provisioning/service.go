// Package provisioning serves the Provisioning gRPC API the panel exposes to
// vk-turn-proxy nodes. During app self-enroll a node forwards the client's panel
// token; the panel verifies it and returns (creating on first use) that client's
// WireGuard config for the node. Peer creation on the node is delegated to a
// PeerProvisioner so the panel stays testable without a live relay.
package provisioning

import (
	"context"
	"crypto/subtle"
	"errors"
	"log"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"v.wingsnet.org/internal/auth"
	"v.wingsnet.org/internal/gen/provisioningpb"
	"v.wingsnet.org/internal/storage"
	"v.wingsnet.org/internal/storage/dbmodel"
)

// verifyNodeBearer authenticates the calling relay: a node that has a management
// credential (grpc_token) must present it as an "authorization: Bearer <token>"
// header - its panel-token - on its outbound provisioning calls. This is the
// relay->panel half of the mutual auth; the panel->relay half is the AES-GCM
// management transport keyed by the same token. A node with no credential (empty
// grpc_token, e.g. a panel-local node) is not challenged, so it keeps working.
func verifyNodeBearer(ctx context.Context, node dbmodel.ServerNode) error {
	if node.GRPCToken == "" {
		return nil
	}
	md, _ := metadata.FromIncomingContext(ctx)
	var presented string
	for _, v := range md.Get("authorization") {
		presented = strings.TrimSpace(strings.TrimPrefix(v, "Bearer "))
		break
	}
	if subtle.ConstantTimeCompare([]byte(presented), []byte(node.GRPCToken)) != 1 {
		return status.Error(codes.Unauthenticated, "invalid node token")
	}
	return nil
}

const defaultMTU = 1280

// Peer is the WireGuard peer a node minted for a client.
type Peer struct {
	PublicKey       string
	PrivateKey      string
	AllowedIPs      string
	ServerPublicKey string
}

// PeerProvisioner creates a peer on a vk-turn-proxy node. The production
// implementation is a Relay gRPC client; tests use a fake. An empty publicKey /
// allowedIPs asks the node to generate them; passing both replicates an existing
// peer onto another node with the same key and address (roaming).
type PeerProvisioner interface {
	CreatePeer(ctx context.Context, node dbmodel.ServerNode, publicKey, allowedIPs string) (Peer, error)
}

// XUIProvisioner mints a client's WireGuard config on a SPECIFIC 3x-ui node's
// inbound. The per-node wg backend uses it: the calling vk-turn-proxy node names
// the 3x-ui node + inbound tag it forwards to, so each admin picks their own
// target in the panel instead of a single global env var.
type XUIProvisioner interface {
	ProvisionXUIClient(ctx context.Context, xuiNode dbmodel.ServerNode, inboundTag, clientID, clientName string) (Peer, error)
}

// Service implements provisioningpb.ProvisioningServer.
type Service struct {
	provisioningpb.UnimplementedProvisioningServer
	store       *storage.Store
	provisioner PeerProvisioner
	xuiProv     XUIProvisioner
	allowedIPs  string
	mtu         uint32
}

func NewService(store *storage.Store, provisioner PeerProvisioner) *Service {
	return &Service{store: store, provisioner: provisioner, allowedIPs: "0.0.0.0/0", mtu: defaultMTU}
}

// SetXUIProvisioner wires the per-node 3x-ui provisioner used when a node's
// WGBackend is "xui".
func (s *Service) SetXUIProvisioner(p XUIProvisioner) {
	s.xuiProv = p
}

// Register attaches the service to a gRPC server.
func (s *Service) Register(gs *grpc.Server) {
	provisioningpb.RegisterProvisioningServer(gs, s)
}

// authorizeClientOnNode reports whether a client may be provisioned on the calling
// node. An admin's client belongs on that admin's own nodes; a panel-local node
// (owner_admin_id 0) is the owner's alone, so only an owner's client may use it.
func (s *Service) authorizeClientOnNode(client storage.Client, node dbmodel.ServerNode) error {
	if node.OwnerAdminID == client.OwnerAdminID {
		return nil
	}
	if node.OwnerAdminID == 0 {
		owner, err := s.store.FindAdminByID(client.OwnerAdminID)
		if err == nil && auth.IsOwner(owner) {
			return nil
		}
	}
	log.Printf("provisioning: refusing client=%s (owner_admin=%d) on node=%s (owner_admin=%d)",
		client.ID, client.OwnerAdminID, node.ID, node.OwnerAdminID)
	return status.Error(codes.PermissionDenied, "client is not allowed on this node")
}

func (s *Service) ResolveClientConfig(ctx context.Context, req *provisioningpb.ResolveClientConfigRequest) (*provisioningpb.ResolveClientConfigResponse, error) {
	log.Printf("provisioning: ResolveClientConfig client=%s node=%s", req.GetClientId(), req.GetNodeId())
	client, err := s.store.FindClientByID(req.GetClientId())
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "unknown client")
	}
	if !auth.VerifyClientToken(client.TokenHash, req.GetToken()) {
		return nil, status.Error(codes.Unauthenticated, "invalid client token")
	}

	// Refuse service to a cut-off client (manually disabled or over its traffic
	// limit) before returning any peer, including an already-provisioned one, so
	// re-resolving cannot revive a blocked tunnel. The collector removes the live
	// peer separately.
	if ctrl, cErr := s.store.GetClientControl(req.GetClientId()); cErr == nil && ctrl.Blocked() {
		return nil, status.Error(codes.ResourceExhausted, "client disabled or over traffic limit")
	}

	node, err := s.store.GetServerNode(req.GetNodeId())
	if errors.Is(err, storage.ErrNotFound) {
		return nil, status.Error(codes.NotFound, "unknown node")
	}
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if err := verifyNodeBearer(ctx, node); err != nil {
		return nil, err
	}

	// A valid node token only proves WHICH relay is calling, not that this client
	// may be served there. Without the tenancy check below, any client could have a
	// peer minted on any registered node - an admin's client on the owner's relay -
	// because the endpoint the app dials comes from the saved config, not from the
	// node the panel authorized. Mirrors the HTTP-side rule in
	// resolveVkTurnEndpoint / handleNodeByID.
	if err := s.authorizeClientOnNode(client, node); err != nil {
		return nil, err
	}

	// Report path (own-wg provision-locally, second phase): the calling node
	// already minted the peer on its own interface and re-calls us to record it,
	// so we never dial the node's management API back - that call is re-entrant
	// while the node is blocked inside its own provision handler, and breaks.
	if req.GetWgPublicKey() != "" {
		peer := Peer{
			PublicKey:       req.GetWgPublicKey(),
			PrivateKey:      req.GetWgPrivateKey(),
			AllowedIPs:      req.GetWgAllowedIps(),
			ServerPublicKey: req.GetWgServerPublicKey(),
		}
		if err := s.store.UpsertClientWGPeer(dbmodel.ClientWGPeer{
			ClientID:        req.GetClientId(),
			NodeID:          req.GetNodeId(),
			PublicKey:       peer.PublicKey,
			PrivateKey:      peer.PrivateKey,
			AllowedIPs:      peer.AllowedIPs,
			ServerPublicKey: peer.ServerPublicKey,
		}); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		s.replicatePeer(ctx, req.GetClientId(), req.GetNodeId(), peer)
		return s.response(client.OwnerAdminID, peer.PrivateKey, peer.PublicKey, peer.AllowedIPs, peer.ServerPublicKey), nil
	}

	existing, err := s.store.GetClientWGPeer(req.GetClientId(), req.GetNodeId())
	if err == nil {
		return s.response(client.OwnerAdminID, existing.PrivateKey, existing.PublicKey, existing.AllowedIPs, existing.ServerPublicKey), nil
	}
	if !errors.Is(err, storage.ErrNotFound) {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Out-of-band wg (a 3x-ui node running the real WireGuard): mint the client on
	// the target 3x-ui inbound, persist it against the calling node and return it.
	// No relay peerstore or cross-node replication - the 3x-ui inbound owns the
	// peer set. Target selection is per-node (node.WGBackend/XuiNodeID/XuiInboundTag).
	if xuiNode, tag, ok, xErr := s.resolveXUITarget(node); xErr != nil {
		return nil, status.Error(codes.Internal, xErr.Error())
	} else if ok {
		log.Printf("provisioning: xui provider for client=%s via node=%s inbound=%q", req.GetClientId(), xuiNode.ID, tag)
		peer, provErr := s.xuiProv.ProvisionXUIClient(ctx, xuiNode, tag, req.GetClientId(), client.Name)
		if provErr != nil {
			log.Printf("provisioning: xui provider failed for client=%s: %v", req.GetClientId(), provErr)
			return nil, status.Error(codes.Internal, "provision xui: "+provErr.Error())
		}
		log.Printf("provisioning: xui provider ok for client=%s (address=%s)", req.GetClientId(), peer.AllowedIPs)
		if err := s.store.UpsertClientWGPeer(dbmodel.ClientWGPeer{
			ClientID:        req.GetClientId(),
			NodeID:          req.GetNodeId(),
			PublicKey:       peer.PublicKey,
			PrivateKey:      peer.PrivateKey,
			AllowedIPs:      peer.AllowedIPs,
			ServerPublicKey: peer.ServerPublicKey,
		}); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		return s.response(client.OwnerAdminID, peer.PrivateKey, peer.PublicKey, peer.AllowedIPs, peer.ServerPublicKey), nil
	}

	// Own wg: tell the calling node to mint the peer on its own wg interface and
	// re-call ResolveClientConfig with the wg_* fields to record it (handled by
	// the report path above). We deliberately do NOT dial the node's management
	// CreatePeer here: that is a re-entrant call made while the node is blocked
	// inside its provision handler, and the connection breaks ("broken pipe").
	return &provisioningpb.ResolveClientConfigResponse{ProvisionLocally: true}, nil
}

// GetClientUsage returns the cap usage for the capped or disabled managed clients
// with a peer on the calling node, so the relay can report used/remaining to the
// app and optionally enforce a cutoff. Keyed by wg peer public key.
func (s *Service) GetClientUsage(ctx context.Context, req *provisioningpb.GetClientUsageRequest) (*provisioningpb.GetClientUsageResponse, error) {
	node, err := s.store.GetServerNode(req.GetNodeId())
	if errors.Is(err, storage.ErrNotFound) {
		return nil, status.Error(codes.NotFound, "unknown node")
	}
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if err := verifyNodeBearer(ctx, node); err != nil {
		return nil, err
	}
	rows, err := s.store.NodeClientUsageForLimits(req.GetNodeId())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	usage := make([]*provisioningpb.ClientUsage, 0, len(rows))
	for _, r := range rows {
		remaining := uint64(0)
		if r.LimitBytes > r.UsedBytes {
			remaining = r.LimitBytes - r.UsedBytes
		}
		usage = append(usage, &provisioningpb.ClientUsage{
			PublicKey:      r.PublicKey,
			ClientId:       r.ClientID,
			LimitBytes:     r.LimitBytes,
			UsedBytes:      r.UsedBytes,
			RemainingBytes: remaining,
			Disabled:       r.Disabled,
		})
	}
	return &provisioningpb.GetClientUsageResponse{Usage: usage}, nil
}

// resolveXUITarget returns the 3x-ui node + inbound tag a node provisions through
// when its WGBackend is "xui". ok=false means this node does not use a per-node
// 3x-ui target (backend "own", or unset for the legacy global path).
func (s *Service) resolveXUITarget(node dbmodel.ServerNode) (dbmodel.ServerNode, string, bool, error) {
	if node.WGBackend != storage.WGBackendXUI {
		return dbmodel.ServerNode{}, "", false, nil
	}
	if s.xuiProv == nil || node.XuiNodeID == "" {
		return dbmodel.ServerNode{}, "", false, errors.New("node wg backend is xui but its 3x-ui target is not configured")
	}
	xuiNode, err := s.store.GetServerNode(node.XuiNodeID)
	if err != nil {
		return dbmodel.ServerNode{}, "", false, err
	}
	return xuiNode, node.XuiInboundTag, true, nil
}

// replicatePeer applies the just-minted peer onto every other vk-turn-proxy node
// with the same key and tunnel address, so a client roams across relay IPs behind
// a DNS load balancer. Best-effort: a node that is down is skipped and picked up
// on its next resolve. Roaming also requires the nodes to share wg server keys
// (a deploy-time concern documented in the HA notes).
func (s *Service) replicatePeer(ctx context.Context, clientID, originNodeID string, peer Peer) {
	nodes, err := s.store.ListServerNodes(storage.ServerNodeVKTurnProxy)
	if err != nil {
		return
	}
	for _, n := range nodes {
		if n.ID == originNodeID {
			continue
		}
		replica, err := s.provisioner.CreatePeer(ctx, n, peer.PublicKey, peer.AllowedIPs)
		if err != nil {
			log.Printf("provisioning: replicate peer for %s to node %s: %v", clientID, n.ID, err)
			continue
		}
		_ = s.store.UpsertClientWGPeer(dbmodel.ClientWGPeer{
			ClientID:        clientID,
			NodeID:          n.ID,
			PublicKey:       peer.PublicKey,
			PrivateKey:      peer.PrivateKey,
			AllowedIPs:      peer.AllowedIPs,
			ServerPublicKey: replica.ServerPublicKey,
		})
	}
}

// response builds the resolved config, including the owner's whole VK link pool.
//
// The pool travels on every resolve rather than only the first: the enrollment QR
// carries a single link to stay scannable, a reinstalled client has nothing but
// that one, and the panel is the only side that knows the rest. The app merges
// append-only, so sending all of them costs nothing and needs no bookkeeping
// about which one the QR happened to get.
func (s *Service) response(
	ownerAdminID int64,
	privateKey, publicKey, address, serverPublicKey string,
) *provisioningpb.ResolveClientConfigResponse {
	links, err := s.store.GetAdminVKLinks(ownerAdminID)
	if err != nil {
		// A pool we could not read is not a reason to fail an enrolment that
		// otherwise worked: the client still gets its tunnel and the link it
		// already has
		log.Printf("provisioning: could not read vk links for admin %d: %v", ownerAdminID, err)
		links = nil
	}
	return &provisioningpb.ResolveClientConfigResponse{
		VkLinks: links,
		Wg: &provisioningpb.WireguardConfig{
			PrivateKey:      privateKey,
			PublicKey:       publicKey,
			Address:         address,
			ServerPublicKey: serverPublicKey,
			AllowedIps:      s.allowedIPs,
			Mtu:             s.mtu,
		},
	}
}
