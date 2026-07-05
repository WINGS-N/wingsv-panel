// Package livestats keeps a live, in-memory view of each vk-turn-proxy node's
// rx/tx speed, fed by a long-lived StreamFlowStats subscription per node. The
// dashboard current-speed tile reads it so the number reflects the last second
// instead of the collector's slower DB-sample cadence.
package livestats

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"v.wingsnet.org/internal/relayclient"
	"v.wingsnet.org/internal/storage"
	"v.wingsnet.org/internal/storage/dbmodel"
)

// staleAfter drops a node's contribution when no snapshot arrived recently (the
// relay went away), so a dead node stops inflating the aggregate speed.
const staleAfter = 10 * time.Second

// reconcileInterval is how often the streamer re-lists nodes to start/stop streams.
const reconcileInterval = 10 * time.Second

// Store holds the latest per-node rx/tx rate derived from streamed FlowStats.
type Store struct {
	mu    sync.RWMutex
	nodes map[string]nodeSample
}

type nodeSample struct {
	rxRate, txRate   uint64
	rxBytes, txBytes uint64
	at               time.Time
}

// NewStore returns an empty live-stats store.
func NewStore() *Store { return &Store{nodes: map[string]nodeSample{}} }

// update folds a fresh cumulative-byte snapshot in, deriving per-second rates from
// the delta since the previous snapshot for that node, and returns those rates.
func (s *Store) update(id string, rxBytes, txBytes uint64, now time.Time) (rxRate, txRate uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := nodeSample{rxBytes: rxBytes, txBytes: txBytes, at: now}
	if prev, ok := s.nodes[id]; ok {
		if dt := now.Sub(prev.at).Seconds(); dt > 0 {
			if rxBytes >= prev.rxBytes {
				next.rxRate = uint64(float64(rxBytes-prev.rxBytes) / dt)
			}
			if txBytes >= prev.txBytes {
				next.txRate = uint64(float64(txBytes-prev.txBytes) / dt)
			}
		}
	}
	s.nodes[id] = next
	return next.rxRate, next.txRate
}

func (s *Store) drop(id string) {
	s.mu.Lock()
	delete(s.nodes, id)
	s.mu.Unlock()
}

// RatesFor sums the current rx/tx per-second rates over the given node ids,
// skipping entries with no recent snapshot. Safe for concurrent use.
func (s *Store) RatesFor(ids []string) (uint64, uint64) {
	now := time.Now()
	s.mu.RLock()
	defer s.mu.RUnlock()
	var rx, tx uint64
	for _, id := range ids {
		e, ok := s.nodes[id]
		if !ok || now.Sub(e.at) > staleAfter {
			continue
		}
		rx += e.rxRate
		tx += e.txRate
	}
	return rx, tx
}

// Relay is the slice of the relay client the streamer needs.
type Relay interface {
	StreamFlowStats(ctx context.Context, node dbmodel.ServerNode, onStats func(relayclient.FlowStats)) error
}

// Lister lists the vk-turn-proxy nodes to keep streams open for.
type Lister interface {
	ListServerNodes(kind string) ([]dbmodel.ServerNode, error)
}

// NodeStats is the per-node live snapshot pushed to the admin WS on every relay
// update, so the dashboard updates speed/sessions/streams without refetching.
type NodeStats struct {
	NodeID            string            `json:"node_id"`
	RxRate            uint64            `json:"rx_rate"`
	TxRate            uint64            `json:"tx_rate"`
	RxBytes           uint64            `json:"rx_bytes"`
	TxBytes           uint64            `json:"tx_bytes"`
	ActiveStreams     uint32            `json:"active_streams"`
	ActiveSessions    uint32            `json:"active_sessions"`
	TotalSessions     uint64            `json:"total_sessions"`
	StreamsByProtocol map[string]uint32 `json:"streams_by_protocol,omitempty"`
}

// Streamer maintains one StreamFlowStats subscription per vk-turn-proxy node and
// writes each push into a Store, optionally emitting it to the admin WS.
type Streamer struct {
	store    Lister
	live     *Store
	newRelay func(dbmodel.ServerNode) Relay
	emit     func(kind string, payload []byte)
}

// NewStreamer wires a streamer over a node lister and a per-node relay factory.
// emit (optional) pushes each per-node snapshot to the admin WS.
func NewStreamer(store Lister, live *Store, newRelay func(dbmodel.ServerNode) Relay, emit func(kind string, payload []byte)) *Streamer {
	return &Streamer{store: store, live: live, newRelay: newRelay, emit: emit}
}

// Run keeps a stream open per node until ctx is cancelled, reconciling the node
// set on a ticker so added/removed nodes are picked up.
func (s *Streamer) Run(ctx context.Context) {
	active := map[string]context.CancelFunc{}
	defer func() {
		for _, cancel := range active {
			cancel()
		}
	}()
	ticker := time.NewTicker(reconcileInterval)
	defer ticker.Stop()
	s.reconcile(ctx, active)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.reconcile(ctx, active)
		}
	}
}

func (s *Streamer) reconcile(ctx context.Context, active map[string]context.CancelFunc) {
	nodes, err := s.store.ListServerNodes(storage.ServerNodeVKTurnProxy)
	if err != nil {
		return
	}
	seen := make(map[string]bool, len(nodes))
	for _, node := range nodes {
		seen[node.ID] = true
		if _, ok := active[node.ID]; ok {
			continue
		}
		nodeCtx, cancel := context.WithCancel(ctx)
		active[node.ID] = cancel
		go s.streamNode(nodeCtx, node)
	}
	for id, cancel := range active {
		if !seen[id] {
			cancel()
			delete(active, id)
			s.live.drop(id)
		}
	}
}

func (s *Streamer) streamNode(ctx context.Context, node dbmodel.ServerNode) {
	for ctx.Err() == nil {
		err := s.newRelay(node).StreamFlowStats(ctx, node, func(st relayclient.FlowStats) {
			rxRate, txRate := s.live.update(node.ID, st.ServerRxBytes, st.ServerTxBytes, time.Now())
			if s.emit == nil {
				return
			}
			payload, mErr := json.Marshal(NodeStats{
				NodeID: node.ID, RxRate: rxRate, TxRate: txRate,
				RxBytes: st.ServerRxBytes, TxBytes: st.ServerTxBytes,
				ActiveStreams: st.ActiveStreams, ActiveSessions: st.ActiveSessions,
				TotalSessions: st.TotalSessions, StreamsByProtocol: st.StreamsByProtocol,
			})
			if mErr == nil {
				s.emit("node_stats", payload)
			}
		})
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			log.Printf("livestats: node %s flow-stats stream ended: %v", node.ID, err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}
