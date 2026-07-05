// Package provisioning serves the Provisioning gRPC API the panel exposes to
// vk-turn-proxy nodes. During app self-enroll a node forwards the client's panel
// token; the panel verifies it and returns (creating on first use) that client's
// WireGuard config for the node. Peer creation on the node is delegated to a
// PeerProvisioner so the panel stays testable without a live relay.
package provisioning

import (
	"context"
	"errors"
	"log"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"v.wingsnet.org/internal/auth"
	"v.wingsnet.org/internal/gen/provisioningpb"
	"v.wingsnet.org/internal/storage"
	"v.wingsnet.org/internal/storage/dbmodel"
)

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

// WGProvider mints a client's WireGuard config out-of-band - for example on a
// 3x-ui node over its Panel gRPC, when that node runs the real WireGuard rather
// than the relay. When set it is the panel-global default (legacy XUI_WG_* env),
// used only when the calling node carries no per-node wg backend config.
type WGProvider interface {
	ProvisionWG(ctx context.Context, clientID, clientName string) (Peer, error)
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
	wgProvider  WGProvider
	xuiProv     XUIProvisioner
	allowedIPs  string
	mtu         uint32
}

func NewService(store *storage.Store, provisioner PeerProvisioner) *Service {
	return &Service{store: store, provisioner: provisioner, allowedIPs: "0.0.0.0/0", mtu: defaultMTU}
}

// SetWGProvider sets the panel-global default wg provider (legacy XUI_WG_* env).
// nil restores the relay path.
func (s *Service) SetWGProvider(p WGProvider) {
	s.wgProvider = p
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

func (s *Service) ResolveClientConfig(ctx context.Context, req *provisioningpb.ResolveClientConfigRequest) (*provisioningpb.ResolveClientConfigResponse, error) {
	log.Printf("provisioning: ResolveClientConfig client=%s node=%s", req.GetClientId(), req.GetNodeId())
	client, err := s.store.FindClientByID(req.GetClientId())
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "unknown client")
	}
	if !auth.VerifyClientToken(client.TokenHash, req.GetToken()) {
		return nil, status.Error(codes.Unauthenticated, "invalid client token")
	}

	node, err := s.store.GetServerNode(req.GetNodeId())
	if errors.Is(err, storage.ErrNotFound) {
		return nil, status.Error(codes.NotFound, "unknown node")
	}
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	existing, err := s.store.GetClientWGPeer(req.GetClientId(), req.GetNodeId())
	if err == nil {
		return s.response(existing.PrivateKey, existing.PublicKey, existing.AllowedIPs, existing.ServerPublicKey), nil
	}
	if !errors.Is(err, storage.ErrNotFound) {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Out-of-band wg (a 3x-ui node running the real WireGuard): mint the client on
	// the target 3x-ui inbound, persist it against the calling node and return it.
	// No relay peerstore or cross-node replication - the 3x-ui inbound owns the
	// peer set. Target selection is per-node (node.WGBackend/XuiNodeID/XuiInboundTag),
	// falling back to the panel-global provider (legacy XUI_WG_* env).
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
		return s.response(peer.PrivateKey, peer.PublicKey, peer.AllowedIPs, peer.ServerPublicKey), nil
	}

	// Legacy panel-global default (XUI_WG_* env): only for a node with no explicit
	// per-node backend, so an admin's own node can still opt into "own" wg.
	if node.WGBackend == "" && s.wgProvider != nil {
		log.Printf("provisioning: global wg provider for client=%s", req.GetClientId())
		peer, provErr := s.wgProvider.ProvisionWG(ctx, req.GetClientId(), client.Name)
		if provErr != nil {
			return nil, status.Error(codes.Internal, "provision wg: "+provErr.Error())
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
		return s.response(peer.PrivateKey, peer.PublicKey, peer.AllowedIPs, peer.ServerPublicKey), nil
	}

	// Own wg: the calling node mints the peer on its own wg interface.
	peer, err := s.provisioner.CreatePeer(ctx, node, "", "")
	if err != nil {
		return nil, status.Error(codes.Internal, "create peer: "+err.Error())
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
	s.replicatePeer(ctx, req.GetClientId(), node.ID, peer)
	return s.response(peer.PrivateKey, peer.PublicKey, peer.AllowedIPs, peer.ServerPublicKey), nil
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

func (s *Service) response(privateKey, publicKey, address, serverPublicKey string) *provisioningpb.ResolveClientConfigResponse {
	return &provisioningpb.ResolveClientConfigResponse{
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
