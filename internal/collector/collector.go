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
	ReplaceFlows(nodeID string, flows []dbmodel.FlowSnapshot) error
	RecordConnections([]dbmodel.ConnectionLog) error
	UpsertPeerTraffic([]dbmodel.PeerTraffic) error
	PruneTrafficBefore(time.Time) error
	PruneConnectionsBefore(time.Time) error
	UpdateServerNodeStatus(id, status string, lastSeen int64) error
}

// Relay is the subset of the vk-turn-proxy gRPC client the collector calls.
type Relay interface {
	NodeStatus(ctx context.Context, node dbmodel.ServerNode) (relayclient.RelayStatus, error)
	FlowStats(ctx context.Context, node dbmodel.ServerNode) (relayclient.FlowStats, error)
	ListFlows(ctx context.Context, node dbmodel.ServerNode) ([]relayclient.Flow, error)
	ListPeers(ctx context.Context, node dbmodel.ServerNode) ([]relayclient.Peer, error)
}

// Options tune the poll cadence and retention. Zero values fall back to defaults.
type Options struct {
	Interval         time.Duration
	Timeout          time.Duration
	TrafficRetention time.Duration
	ConnRetention    time.Duration
	// OnCollected fires after a node's cycle persists, so the HTTP layer can push
	// a live update. It is called synchronously; keep it cheap (a channel send).
	OnCollected func(nodeID string)
	// Now is injectable for tests; defaults to time.Now.
	Now func() time.Time
}

// RelayFactory returns the gRPC client for a node, so each node is polled with
// its own credentials (a panel-local node uses the configured relay token; an
// admin's external node uses the token stored on the node).
type RelayFactory func(node dbmodel.ServerNode) Relay

// Collector polls vk-turn-proxy nodes on a ticker and persists their stats.
type Collector struct {
	store    Store
	newRelay RelayFactory
	opts     Options
}

// New builds a collector, filling any unset option with its default.
func New(store Store, newRelay RelayFactory, opts Options) *Collector {
	if opts.Interval <= 0 {
		opts.Interval = 10 * time.Second
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 5 * time.Second
	}
	if opts.TrafficRetention <= 0 {
		opts.TrafficRetention = 7 * 24 * time.Hour
	}
	if opts.ConnRetention <= 0 {
		opts.ConnRetention = 3 * 24 * time.Hour
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &Collector{store: store, newRelay: newRelay, opts: opts}
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
	for _, node := range nodes {
		if err := c.collectNode(ctx, node); err != nil {
			log.Printf("collector: node %s (%s): %v", node.ID, node.GRPCEndpoint, err)
		}
	}
	now := c.opts.Now()
	if err := c.store.PruneTrafficBefore(now.Add(-c.opts.TrafficRetention)); err != nil {
		log.Printf("collector: prune traffic: %v", err)
	}
	if err := c.store.PruneConnectionsBefore(now.Add(-c.opts.ConnRetention)); err != nil {
		log.Printf("collector: prune connections: %v", err)
	}
}

func (c *Collector) collectNode(ctx context.Context, node dbmodel.ServerNode) error {
	ctx, cancel := context.WithTimeout(ctx, c.opts.Timeout)
	defer cancel()

	relay := c.newRelay(node)
	now := c.opts.Now().Unix()
	status, err := relay.NodeStatus(ctx, node)
	if err != nil {
		_ = c.store.UpdateServerNodeStatus(node.ID, "offline", now)
		return err
	}
	stats, err := relay.FlowStats(ctx, node)
	if err != nil {
		return err
	}
	flows, err := relay.ListFlows(ctx, node)
	if err != nil {
		return err
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
