package admin

import (
	"log"
	"net"
	"strings"

	wingsvpb "v.wingsnet.org/internal/gen/wingsvpb"
	"v.wingsnet.org/internal/storage"

	"google.golang.org/protobuf/proto"
)

// MigrateManagedEndpoints re-points every stored managed VK-TURN profile at a
// relay the panel actually knows about.
//
// The endpoint is baked into each client's config when the profile is created, and
// it used to be filled from the panel-global VK_TURN_ENDPOINT env. That value
// outlived the relay it named: configs kept dialing a host that was no longer a
// registered node, and nothing re-resolved them because the resolve only runs when
// a node is named explicitly. A profile whose host still matches a registered
// vk-turn node keeps it; one that matches nothing has its endpoint cleared, so the
// operator sees "no relay selected" instead of a link quietly pointing somewhere
// dead.
//
// Idempotent: a second run finds nothing left to change.
func MigrateManagedEndpoints(store *storage.Store) error {
	nodes, err := store.ListServerNodes(storage.ServerNodeVKTurnProxy)
	if err != nil {
		return err
	}
	known := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		host := n.GRPCEndpoint
		if parsed, _, splitErr := net.SplitHostPort(n.GRPCEndpoint); splitErr == nil {
			host = parsed
		}
		if host = strings.TrimSpace(host); host != "" {
			known[strings.ToLower(host)] = true
		}
	}
	configs, err := store.ListClientConfigs()
	if err != nil {
		return err
	}
	cleared := 0
	for _, row := range configs {
		cfg := &wingsvpb.Config{}
		if err := proto.Unmarshal(row.ConfigProto, cfg); err != nil {
			// A config we cannot parse is not ours to rewrite; leave it alone.
			continue
		}
		endpoint := existingManagedEndpoint(cfg.Turn)
		if endpoint == "" || endpointHostKnown(endpoint, known) {
			continue
		}
		if !clearManagedEndpoint(cfg.Turn) {
			continue
		}
		blob, err := proto.Marshal(cfg)
		if err != nil {
			return err
		}
		if _, err := store.UpsertClientConfig(row.ClientID, blob, row.Revision); err != nil {
			return err
		}
		cleared++
	}
	if cleared > 0 {
		log.Printf("admin: cleared %d managed vk-turn endpoint(s) naming a relay that is not registered", cleared)
	}
	return nil
}

// endpointHostKnown reports whether the endpoint's host is one of the registered
// relays. Endpoints carry a port the relay reports, so only the host is compared.
func endpointHostKnown(endpoint string, known map[string]bool) bool {
	host := endpoint
	if parsed, _, err := net.SplitHostPort(endpoint); err == nil {
		host = parsed
	}
	return known[strings.ToLower(strings.TrimSpace(host))]
}

// clearManagedEndpoint blanks the endpoint on every managed profile, reporting
// whether anything changed.
func clearManagedEndpoint(turn *wingsvpb.Turn) bool {
	if turn == nil {
		return false
	}
	changed := false
	for _, p := range turn.Profiles {
		if isManagedProfile(p) && strings.TrimSpace(p.VkTurnEndpoint) != "" {
			p.VkTurnEndpoint = ""
			changed = true
		}
	}
	return changed
}
