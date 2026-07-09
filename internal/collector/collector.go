// Package collector periodically polls managed server nodes over gRPC and
// persists traffic time-series, the live-flow snapshot and the connection-log
// history that the dashboard, per-client traffic and flow-chain views read.
package collector

import (
	"context"
	"log"
	"time"

	"v.wingsnet.org/internal/relayclient"
	"v.wingsnet.org/internal/storage"
	"v.wingsnet.org/internal/storage/dbmodel"
)

// Store is the subset of storage the collector writes.
type Store interface {
	ListServerNodes(kind string) ([]dbmodel.ServerNode, error)
	InsertTrafficSample(dbmodel.TrafficSample) error
	AccumulateNodeTraffic(nodeID string, rxCumulative, txCumulative uint64) error
	ReplaceFlows(nodeID string, flows []dbmodel.FlowSnapshot) error
	RecordConnections([]dbmodel.ConnectionLog) error
	UpsertPeerTraffic([]dbmodel.PeerTraffic) error
	PruneTrafficBefore(time.Time) error
	PruneConnectionsBefore(time.Time) error
	UpdateServerNodeStatus(id, status string, lastSeen int64) error
	UpdateServerNodeRelayVersion(id, version string) error
	// Managed-client traffic-limit accounting and enforcement.
	AccumulateClientTraffic() error
	RunDueTrafficResets(nowUnix int64) ([]string, error)
	BlockedProvisionedClientIDs() ([]string, error)
	ListClientWGPeers(clientID string) ([]dbmodel.ClientWGPeer, error)
	GetServerNode(id string) (dbmodel.ServerNode, error)
	DeleteClientWGPeer(clientID, nodeID string) error
}

// Relay is the subset of the vk-turn-proxy gRPC client the collector calls.
type Relay interface {
	NodeStatus(ctx context.Context, node dbmodel.ServerNode) (relayclient.RelayStatus, error)
	FlowStats(ctx context.Context, node dbmodel.ServerNode) (relayclient.FlowStats, error)
	ListFlows(ctx context.Context, node dbmodel.ServerNode) ([]relayclient.Flow, error)
	ListPeers(ctx context.Context, node dbmodel.ServerNode) ([]relayclient.Peer, error)
	DeletePeer(ctx context.Context, node dbmodel.ServerNode, publicKey string) error
}

// Options tune the poll cadence and retention. Zero values fall back to defaults.
type Options struct {
	Interval time.Duration
	// PersistInterval decouples how often a full time-series sample plus the flow
	// and connection snapshots are written from how often node status is polled.
	// Status (online/offline) is refreshed every Interval; the heavier rows are
	// written every PersistInterval, so a fast poll cadence does not bloat the DB.
	PersistInterval  time.Duration
	Timeout          time.Duration
	TrafficRetention time.Duration
	ConnRetention    time.Duration
	// OnCollected fires after a node's cycle persists, so the HTTP layer can push
	// a live update. It is called synchronously; keep it cheap (a channel send).
	OnCollected func(nodeID string)
	// OnFlowStats / OnFlows fire with a node's freshly polled aggregate stats and
	// active flows, so the HTTP layer can push them to the admin WS - live data over
	// the socket even when the relay is too old to stream. Called on the persist
	// cycle (where these RPCs already run); keep them cheap.
	OnFlowStats func(nodeID string, stats relayclient.FlowStats)
	OnFlows     func(nodeID string, flows []relayclient.Flow)
	// Now is injectable for tests; defaults to time.Now.
	Now func() time.Time
}

// RelayFactory returns the gRPC client for a node, so each node is polled with
// its own credentials (a panel-local node uses the configured relay token; an
// admin's external node uses the token stored on the node).
type RelayFactory func(node dbmodel.ServerNode) Relay

// offlineThreshold is how many consecutive failed polls mark a node offline.
// A single blip at the 3s cadence must not flap the status, so status only drops
// after a few misses (~9s); it recovers to online on the first success.
const offlineThreshold = 3

// Collector polls vk-turn-proxy nodes on a ticker and persists their stats.
type Collector struct {
	store    Store
	newRelay RelayFactory
	opts     Options
	// failures counts consecutive poll failures per node for offline hysteresis.
	// Only the single Run goroutine touches it, so no lock is needed.
	failures map[string]int
	// lastPersist gates the heavy time-series/flow/connection writes so they run
	// on PersistInterval rather than every poll.
	lastPersist time.Time
}

// New builds a collector, filling any unset option with its default.
func New(store Store, newRelay RelayFactory, opts Options) *Collector {
	if opts.Interval <= 0 {
		opts.Interval = 3 * time.Second
	}
	if opts.PersistInterval <= 0 {
		opts.PersistInterval = 15 * time.Second
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 5 * time.Second
	}
	if opts.TrafficRetention <= 0 {
		// A month of samples backs the 24h / 7d / month chart ranges; all-time
		// totals live in the durable NodeTrafficTotal accumulator, not here.
		opts.TrafficRetention = 31 * 24 * time.Hour
	}
	if opts.ConnRetention <= 0 {
		opts.ConnRetention = time.Hour
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &Collector{store: store, newRelay: newRelay, opts: opts, failures: map[string]int{}}
}

// Run polls once immediately, then on every tick until ctx is cancelled.
func (c *Collector) Run(ctx context.Context) {
	c.CollectOnce(ctx)
	ticker := time.NewTicker(c.opts.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.CollectOnce(ctx)
		}
	}
}

// CollectOnce polls every vk-turn-proxy node once and prunes stale rows. A single
// node failing is logged and skipped; it never aborts the cycle.
func (c *Collector) CollectOnce(ctx context.Context) {
	nodes, err := c.store.ListServerNodes(storage.ServerNodeVKTurnProxy)
	if err != nil {
		log.Printf("collector: list nodes: %v", err)
		return
	}
	now := c.opts.Now()
	// Write the heavy rows only every PersistInterval; every cycle still refreshes
	// each node's online/offline status cheaply.
	persist := now.Sub(c.lastPersist) >= c.opts.PersistInterval
	if persist {
		c.lastPersist = now
	}
	for _, node := range nodes {
		if err := c.collectNode(ctx, node, persist); err != nil {
			log.Printf("collector: node %s (%s): %v", node.ID, node.GRPCEndpoint, err)
		}
	}
	// Pruning and traffic-limit enforcement only need the persist cycle - the
	// per-peer counters they read are refreshed there, not every poll.
	if persist {
		c.enforceTrafficLimits(ctx, now)
		if err := c.store.PruneTrafficBefore(now.Add(-c.opts.TrafficRetention)); err != nil {
			log.Printf("collector: prune traffic: %v", err)
		}
		if err := c.store.PruneConnectionsBefore(now.Add(-c.opts.ConnRetention)); err != nil {
			log.Printf("collector: prune connections: %v", err)
		}
	}
}

// enforceTrafficLimits folds the latest per-peer counters into each managed
// client's durable usage, runs any due periodic resets, then deprovisions every
// client that is now cut off (disabled or over its limit) so its tunnel drops.
func (c *Collector) enforceTrafficLimits(ctx context.Context, now time.Time) {
	if err := c.store.AccumulateClientTraffic(); err != nil {
		log.Printf("collector: accumulate client traffic: %v", err)
		return
	}
	if reset, err := c.store.RunDueTrafficResets(now.Unix()); err != nil {
		log.Printf("collector: run due traffic resets: %v", err)
	} else if len(reset) > 0 {
		log.Printf("collector: periodic traffic reset for %d client(s)", len(reset))
	}
	blocked, err := c.store.BlockedProvisionedClientIDs()
	if err != nil {
		log.Printf("collector: list blocked clients: %v", err)
		return
	}
	for _, clientID := range blocked {
		c.deprovisionClient(ctx, clientID)
	}
}

// deprovisionClient removes a cut-off client's wg peers from their relay nodes
// and clears the stored peer records. An xui-backed peer lives on a 3x-ui inbound
// the relay DeletePeer cannot reach, so there we only drop the record; the
// provisioning gate still refuses to re-issue its config.
func (c *Collector) deprovisionClient(ctx context.Context, clientID string) {
	peers, err := c.store.ListClientWGPeers(clientID)
	if err != nil {
		log.Printf("collector: list peers for %s: %v", clientID, err)
		return
	}
	for _, pr := range peers {
		if node, nErr := c.store.GetServerNode(pr.NodeID); nErr == nil && node.WGBackend != storage.WGBackendXUI {
			if derr := c.newRelay(node).DeletePeer(ctx, node, pr.PublicKey); derr != nil {
				log.Printf("collector: delete peer for %s on node %s: %v", clientID, pr.NodeID, derr)
			}
		}
		if derr := c.store.DeleteClientWGPeer(clientID, pr.NodeID); derr != nil {
			log.Printf("collector: clear peer record %s/%s: %v", clientID, pr.NodeID, derr)
		}
	}
	if len(peers) > 0 {
		log.Printf("collector: deprovisioned cut-off client %s (%d peer(s))", clientID, len(peers))
	}
}

func (c *Collector) collectNode(ctx context.Context, node dbmodel.ServerNode, persist bool) error {
	ctx, cancel := context.WithTimeout(ctx, c.opts.Timeout)
	defer cancel()

	relay := c.newRelay(node)
	now := c.opts.Now().Unix()
	status, err := relay.NodeStatus(ctx, node)
	if err != nil {
		// Hold the last status through transient blips; drop to offline only after
		// several consecutive misses so a single slow poll doesn't flap the UI.
		c.failures[node.ID]++
		if c.failures[node.ID] >= offlineThreshold {
			_ = c.store.UpdateServerNodeStatus(node.ID, "offline", now)
		}
		return err
	}
	c.failures[node.ID] = 0
	_ = c.store.UpdateServerNodeRelayVersion(node.ID, status.Version)
	// Between persist cycles just keep the status fresh; skip the heavier RPCs and
	// writes so a fast poll cadence stays cheap.
	if !persist {
		return c.store.UpdateServerNodeStatus(node.ID, "online", now)
	}
	stats, err := relay.FlowStats(ctx, node)
	if err != nil {
		return err
	}
	flows, err := relay.ListFlows(ctx, node)
	if err != nil {
		return err
	}
	// Push the freshly polled stats/flows to the admin WS so the dashboard gets live
	// data over the socket without a REST refetch - and without depending on the
	// relay's streaming RPCs (this uses the unary polls above).
	if c.opts.OnFlowStats != nil {
		c.opts.OnFlowStats(node.ID, stats)
	}
	if c.opts.OnFlows != nil {
		c.opts.OnFlows(node.ID, flows)
	}

	if err := c.store.InsertTrafficSample(dbmodel.TrafficSample{
		NodeID:         node.ID,
		TsUnix:         now,
		RxBytes:        stats.ServerRxBytes,
		TxBytes:        stats.ServerTxBytes,
		ActiveStreams:  stats.ActiveStreams,
		ActiveSessions: stats.ActiveSessions,
		PeerCount:      status.PeerCount,
	}); err != nil {
		return err
	}
	if err := c.store.AccumulateNodeTraffic(node.ID, stats.ServerRxBytes, stats.ServerTxBytes); err != nil {
		log.Printf("collector: node %s accumulate traffic: %v", node.ID, err)
	}

	snaps := make([]dbmodel.FlowSnapshot, 0, len(flows))
	conns := make([]dbmodel.ConnectionLog, 0, len(flows))
	for _, f := range flows {
		snaps = append(snaps, dbmodel.FlowSnapshot{
			NodeID: node.ID, SessionID: f.SessionID, StreamID: f.StreamID,
			ClientIP: f.ClientIP, Remote: f.Remote, Protocol: f.Protocol, Version: f.Version,
			RxBytes: f.RxBytes, TxBytes: f.TxBytes, RxRate: f.RxRate, TxRate: f.TxRate,
			StartedUnix: f.StartedUnix, SampledUnix: now,
		})
		conns = append(conns, dbmodel.ConnectionLog{
			NodeID: node.ID, SessionID: f.SessionID, StreamID: f.StreamID, StartedUnix: f.StartedUnix,
			ClientIP: f.ClientIP, Remote: f.Remote, Protocol: f.Protocol,
			RxBytes: f.RxBytes, TxBytes: f.TxBytes, FirstSeenUnix: now, LastSeenUnix: now,
		})
	}
	if err := c.store.ReplaceFlows(node.ID, snaps); err != nil {
		return err
	}
	if err := c.store.RecordConnections(conns); err != nil {
		return err
	}
	// Per-peer counters attribute traffic to managed clients. Best-effort: a node
	// that does not expose peers still gets its flow/traffic stats above.
	if peers, perr := relay.ListPeers(ctx, node); perr == nil {
		peerRows := make([]dbmodel.PeerTraffic, 0, len(peers))
		for _, p := range peers {
			peerRows = append(peerRows, dbmodel.PeerTraffic{
				NodeID: node.ID, PublicKey: p.PublicKey,
				RxBytes: p.RxBytes, TxBytes: p.TxBytes, SampledUnix: now,
			})
		}
		if err := c.store.UpsertPeerTraffic(peerRows); err != nil {
			log.Printf("collector: node %s peer traffic: %v", node.ID, err)
		}
	}
	if err := c.store.UpdateServerNodeStatus(node.ID, "online", now); err != nil {
		return err
	}
	if c.opts.OnCollected != nil {
		c.opts.OnCollected(node.ID)
	}
	return nil
}
