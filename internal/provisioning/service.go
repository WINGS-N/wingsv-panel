// Package provisioning serves the Provisioning gRPC API the panel exposes to
// vk-turn-proxy nodes. During app self-enroll a node forwards the client's panel
// token; the panel verifies it and returns (creating on first use) that client's
// WireGuard config for the node. Peer creation on the node is delegated to a
// PeerProvisioner so the panel stays testable without a live relay.
package provisioning

import (
	"context"
	"errors"

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
// implementation is a Relay gRPC client; tests use a fake.
type PeerProvisioner interface {
	CreatePeer(ctx context.Context, node dbmodel.ServerNode, publicKey string) (Peer, error)
}

// Service implements provisioningpb.ProvisioningServer.
type Service struct {
	provisioningpb.UnimplementedProvisioningServer
	store       *storage.Store
	provisioner PeerProvisioner
	allowedIPs  string
	mtu         uint32
}

func NewService(store *storage.Store, provisioner PeerProvisioner) *Service {
	return &Service{store: store, provisioner: provisioner, allowedIPs: "0.0.0.0/0", mtu: defaultMTU}
}

// Register attaches the service to a gRPC server.
func (s *Service) Register(gs *grpc.Server) {
	provisioningpb.RegisterProvisioningServer(gs, s)
}

func (s *Service) ResolveClientConfig(ctx context.Context, req *provisioningpb.ResolveClientConfigRequest) (*provisioningpb.ResolveClientConfigResponse, error) {
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

	peer, err := s.provisioner.CreatePeer(ctx, node, "")
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
	return s.response(peer.PrivateKey, peer.PublicKey, peer.AllowedIPs, peer.ServerPublicKey), nil
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
