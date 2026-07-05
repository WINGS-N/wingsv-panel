// Package livestats keeps a live, in-memory view of each vk-turn-proxy node's
// rx/tx speed, fed by a long-lived StreamFlowStats subscription per node. The
// dashboard current-speed tile reads it so the number reflects the last second
// instead of the collector's slower DB-sample cadence.
package livestats

import (
	"context"
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
// the delta since the previous snapshot for that node.
func (s *Store) update(id string, rxBytes, txBytes uint64, now time.Time) {
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

// Streamer maintains one StreamFlowStats subscription per vk-turn-proxy node and
// writes each push into a Store.
type Streamer struct {
	store    Lister
	live     *Store
	newRelay func(dbmodel.ServerNode) Relay
}

// NewStreamer wires a streamer over a node lister and a per-node relay factory.
func NewStreamer(store Lister, live *Store, newRelay func(dbmodel.ServerNode) Relay) *Streamer {
	return &Streamer{store: store, live: live, newRelay: newRelay}
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
			s.live.update(node.ID, st.ServerRxBytes, st.ServerTxBytes, time.Now())
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
