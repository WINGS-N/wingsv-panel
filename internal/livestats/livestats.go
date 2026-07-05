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
	StreamFlows(ctx context.Context, node dbmodel.ServerNode, onFlows func([]relayclient.Flow)) error
}

// flowMsg mirrors statsview.Flow's JSON so the pushed flows match the shape the
// dashboard already renders from the REST flows endpoint.
type flowMsg struct {
	NodeID    string `json:"node_id"`
	SessionID string `json:"session_id"`
	StreamID  uint32 `json:"stream_id"`
	ClientIP  string `json:"client_ip"`
	Remote    string `json:"remote"`
	Protocol  string `json:"protocol"`
	Version   uint32 `json:"version"`
	RxBytes   uint64 `json:"rx_bytes"`
	TxBytes   uint64 `json:"tx_bytes"`
	RxRate    uint64 `json:"rx_rate"`
	TxRate    uint64 `json:"tx_rate"`
	Started   int64  `json:"started_unix"`
}

// nodeFlowsMsg is the node_flows WS payload: one node's current active flows.
type nodeFlowsMsg struct {
	NodeID string    `json:"node_id"`
	Flows  []flowMsg `json:"flows"`
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

// flowsEmitInterval throttles node_flows pushes per node: the flow graph does not
// need per-second updates and the full flow list is the bulk of the WS traffic.
const flowsEmitInterval = 2 * time.Second

// Streamer maintains one StreamFlowStats subscription per vk-turn-proxy node and
// writes each push into a Store, optionally emitting it to the admin WS.
type Streamer struct {
	store    Lister
	live     *Store
	newRelay func(dbmodel.ServerNode) Relay
	emit     func(kind string, payload []byte)

	flowMu       sync.Mutex
	lastFlowEmit map[string]time.Time
}

// NewStreamer wires a streamer over a node lister and a per-node relay factory.
// emit (optional) pushes each per-node snapshot to the admin WS.
func NewStreamer(store Lister, live *Store, newRelay func(dbmodel.ServerNode) Relay, emit func(kind string, payload []byte)) *Streamer {
	return &Streamer{store: store, live: live, newRelay: newRelay, emit: emit, lastFlowEmit: map[string]time.Time{}}
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
		go s.streamFlows(nodeCtx, node)
	}
	for id, cancel := range active {
		if !seen[id] {
			cancel()
			delete(active, id)
			s.live.drop(id)
		}
	}
}

// Ingest folds a fresh FlowStats snapshot for a node into the live store and
// pushes it to the admin WS. Called both by the per-node stream (1s) and by the
// collector's unary poll (so live data flows even against a relay too old to
// stream), whichever reports first; the store just keeps the newest.
func (s *Streamer) Ingest(nodeID string, st relayclient.FlowStats) {
	rxRate, txRate := s.live.update(nodeID, st.ServerRxBytes, st.ServerTxBytes, time.Now())
	if s.emit == nil {
		return
	}
	payload, err := json.Marshal(NodeStats{
		NodeID: nodeID, RxRate: rxRate, TxRate: txRate,
		RxBytes: st.ServerRxBytes, TxBytes: st.ServerTxBytes,
		ActiveStreams: st.ActiveStreams, ActiveSessions: st.ActiveSessions,
		TotalSessions: st.TotalSessions, StreamsByProtocol: st.StreamsByProtocol,
	})
	if err == nil {
		s.emit("node_stats", payload)
	}
}

// IngestFlows pushes a node's active-flow list to the admin WS as node_flows,
// throttled to flowsEmitInterval per node (the stream + collector both feed it).
func (s *Streamer) IngestFlows(nodeID string, flows []relayclient.Flow) {
	if s.emit == nil {
		return
	}
	now := time.Now()
	s.flowMu.Lock()
	if last, ok := s.lastFlowEmit[nodeID]; ok && now.Sub(last) < flowsEmitInterval {
		s.flowMu.Unlock()
		return
	}
	s.lastFlowEmit[nodeID] = now
	s.flowMu.Unlock()
	msg := nodeFlowsMsg{NodeID: nodeID, Flows: make([]flowMsg, 0, len(flows))}
	for _, f := range flows {
		msg.Flows = append(msg.Flows, flowMsg{
			NodeID: nodeID, SessionID: f.SessionID, StreamID: f.StreamID,
			ClientIP: f.ClientIP, Remote: f.Remote, Protocol: f.Protocol, Version: f.Version,
			RxBytes: f.RxBytes, TxBytes: f.TxBytes, RxRate: f.RxRate, TxRate: f.TxRate, Started: f.StartedUnix,
		})
	}
	if payload, err := json.Marshal(msg); err == nil {
		s.emit("node_flows", payload)
	}
}

func (s *Streamer) streamNode(ctx context.Context, node dbmodel.ServerNode) {
	for ctx.Err() == nil {
		err := s.newRelay(node).StreamFlowStats(ctx, node, func(st relayclient.FlowStats) {
			s.Ingest(node.ID, st)
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

// streamFlows holds a StreamFlows subscription and pushes each node's active-flow
// snapshot to the admin WS as a node_flows event, driving the live flow graph.
func (s *Streamer) streamFlows(ctx context.Context, node dbmodel.ServerNode) {
	if s.emit == nil {
		return
	}
	for ctx.Err() == nil {
		err := s.newRelay(node).StreamFlows(ctx, node, func(flows []relayclient.Flow) {
			s.IngestFlows(node.ID, flows)
		})
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			log.Printf("livestats: node %s flows stream ended: %v", node.ID, err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}
