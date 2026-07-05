package httpapi

import (
	"context"

	"v.wingsnet.org/internal/provisioning"
	"v.wingsnet.org/internal/storage"
	"v.wingsnet.org/internal/storage/dbmodel"
	"v.wingsnet.org/internal/xuiclient"
)

// xuiPerNodeProvider mints a client's WireGuard config on a caller-specified 3x-ui
// node + inbound. Unlike xuiWGProvider (fixed to the global XUI_WG_* env), the
// target comes from the calling vk-turn-proxy node's per-node config, so each
// admin picks their own 3x-ui inbound in the panel.
type xuiPerNodeProvider struct {
	xui *xuiclient.Client
}

func (p *xuiPerNodeProvider) ProvisionXUIClient(ctx context.Context, xuiNode dbmodel.ServerNode, inboundTag, clientID, clientName string) (provisioning.Peer, error) {
	wc, err := p.xui.CreateWireguardClient(ctx, xuiNode, inboundTag, clientID, clientName)
	if err != nil {
		return provisioning.Peer{}, err
	}
	return provisioning.Peer{
		PrivateKey:      wc.PrivateKey,
		PublicKey:       wc.PublicKey,
		AllowedIPs:      wc.Address,
		ServerPublicKey: wc.ServerPublicKey,
	}, nil
}

// xuiWGProvider mints a client's WireGuard config on a 3x-ui node's WireGuard
// inbound over its Panel gRPC, so DTLS provisioning hands the app a peer on the
// node that actually runs WireGuard rather than on the relay.
type xuiWGProvider struct {
	store      *storage.Store
	xui        *xuiclient.Client
	nodeID     string
	inboundTag string
}

func (p *xuiWGProvider) ProvisionWG(ctx context.Context, clientID, clientName string) (provisioning.Peer, error) {
	node, err := p.store.GetServerNode(p.nodeID)
	if err != nil {
		return provisioning.Peer{}, err
	}
	wc, err := p.xui.CreateWireguardClient(ctx, node, p.inboundTag, clientID, clientName)
	if err != nil {
		return provisioning.Peer{}, err
	}
	return provisioning.Peer{
		PrivateKey:      wc.PrivateKey,
		PublicKey:       wc.PublicKey,
		AllowedIPs:      wc.Address,
		ServerPublicKey: wc.ServerPublicKey,
	}, nil
}
