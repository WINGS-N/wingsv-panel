package httpapi

import (
	"context"

	"v.wingsnet.org/internal/provisioning"
	"v.wingsnet.org/internal/storage"
	"v.wingsnet.org/internal/xuiclient"
)

// xuiWGProvider mints a client's WireGuard config on a 3x-ui node's WireGuard
// inbound over its Panel gRPC, so DTLS provisioning hands the app a peer on the
// node that actually runs WireGuard rather than on the relay.
type xuiWGProvider struct {
	store      *storage.Store
	xui        *xuiclient.Client
	nodeID     string
	inboundTag string
}

func (p *xuiWGProvider) ProvisionWG(ctx context.Context, clientID string) (provisioning.Peer, error) {
	node, err := p.store.GetServerNode(p.nodeID)
	if err != nil {
		return provisioning.Peer{}, err
	}
	wc, err := p.xui.CreateWireguardClient(ctx, node, p.inboundTag, clientID)
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
