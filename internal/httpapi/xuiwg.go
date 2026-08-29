package httpapi

import (
	"context"

	"v.wingsnet.org/internal/provisioning"
	"v.wingsnet.org/internal/storage/dbmodel"
	"v.wingsnet.org/internal/xuiclient"
)

// xuiPerNodeProvider mints a client's WireGuard config on a caller-specified 3x-ui
// node + inbound. The target comes from the calling vk-turn-proxy node's per-node
// config, so each admin picks their own 3x-ui inbound in the panel.
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
