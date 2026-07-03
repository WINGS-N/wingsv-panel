// Package statsview builds the node-traffic dashboard payloads (aggregate
// traffic, live flows and connection log) for a given node owner. The owner
// console passes ownerAdminID 0 to view the panel-local nodes; an admin passes
// their own id to view the external vk-turn-proxy / 3x-ui endpoints they added.
package statsview

import (
	"sort"
	"time"

	"v.wingsnet.org/internal/storage"
	"v.wingsnet.org/internal/storage/dbmodel"
)

type SeriesPoint struct {
	Ts      int64  `json:"ts"`
	RxBytes uint64 `json:"rx_bytes"`
	TxBytes uint64 `json:"tx_bytes"`
}

type Node struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Status         string `json:"status"`
	LastSeen       int64  `json:"last_seen"`
	PeerCount      uint32 `json:"peer_count"`
	ActiveSessions uint32 `json:"active_sessions"`
	ActiveStreams  uint32 `json:"active_streams"`
	RxBytes        uint64 `json:"rx_bytes"`
	TxBytes        uint64 `json:"tx_bytes"`
}

type Totals struct {
	Nodes          int    `json:"nodes"`
	NodesOnline    int    `json:"nodes_online"`
	PeerCount      uint32 `json:"peer_count"`
	ActiveSessions uint32 `json:"active_sessions"`
	ActiveStreams  uint32 `json:"active_streams"`
	RxBytes        uint64 `json:"rx_bytes"`
	TxBytes        uint64 `json:"tx_bytes"`
	Rx24h          uint64 `json:"rx_24h"`
	Tx24h          uint64 `json:"tx_24h"`
}

type Traffic struct {
	Mode        string        `json:"mode"`
	GeneratedAt int64         `json:"generated_at"`
	Totals      Totals        `json:"totals"`
	Nodes       []Node        `json:"nodes"`
	Series      []SeriesPoint `json:"series"`
}

type Flow struct {
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

type Connection struct {
	NodeID    string `json:"node_id"`
	SessionID string `json:"session_id"`
	StreamID  uint32 `json:"stream_id"`
	ClientIP  string `json:"client_ip"`
	Remote    string `json:"remote"`
	Protocol  string `json:"protocol"`
	RxBytes   uint64 `json:"rx_bytes"`
	TxBytes   uint64 `json:"tx_bytes"`
	FirstSeen int64  `json:"first_seen"`
	LastSeen  int64  `json:"last_seen"`
}

// BuildTraffic returns the dashboard aggregate for the given owner's nodes.
func BuildTraffic(store *storage.Store, ownerAdminID int64) (Traffic, error) {
	mode, _ := store.GetPanelMode()
	nodes, err := store.ListServerNodesByOwner(storage.ServerNodeVKTurnProxy, ownerAdminID)
	if err != nil {
		return Traffic{}, err
	}
	out := Traffic{Mode: string(mode), GeneratedAt: time.Now().Unix()}
	out.Totals.Nodes = len(nodes)
	owned := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		owned[n.ID] = true
		node := Node{ID: n.ID, Name: n.Name, Status: n.Status, LastSeen: n.LastSeenAt}
		if latest, lerr := store.LatestTrafficSample(n.ID); lerr == nil {
			node.PeerCount = latest.PeerCount
			node.ActiveSessions = latest.ActiveSessions
			node.ActiveStreams = latest.ActiveStreams
			node.RxBytes = latest.RxBytes
			node.TxBytes = latest.TxBytes
		}
		if n.Status == "online" {
			out.Totals.NodesOnline++
		}
		out.Totals.PeerCount += node.PeerCount
		out.Totals.ActiveSessions += node.ActiveSessions
		out.Totals.ActiveStreams += node.ActiveStreams
		out.Totals.RxBytes += node.RxBytes
		out.Totals.TxBytes += node.TxBytes
		out.Nodes = append(out.Nodes, node)
	}
	samples, err := store.ListTrafficSince(time.Now().Add(-24 * time.Hour).Unix())
	if err != nil {
		return Traffic{}, err
	}
	filtered := samples[:0]
	for _, s := range samples {
		if owned[s.NodeID] {
			filtered = append(filtered, s)
		}
	}
	out.Series, out.Totals.Rx24h, out.Totals.Tx24h = aggregateTrafficSeries(filtered, 60)
	return out, nil
}

// BuildFlows returns the current live flows for the given owner's nodes.
func BuildFlows(store *storage.Store, ownerAdminID int64) ([]Flow, error) {
	ids, err := ownedNodeIDs(store, ownerAdminID)
	if err != nil {
		return nil, err
	}
	flows, err := store.ListFlowsForNodes(ids)
	if err != nil {
		return nil, err
	}
	out := make([]Flow, 0, len(flows))
	for _, f := range flows {
		out = append(out, Flow{
			NodeID: f.NodeID, SessionID: f.SessionID, StreamID: f.StreamID,
			ClientIP: f.ClientIP, Remote: f.Remote, Protocol: f.Protocol, Version: f.Version,
			RxBytes: f.RxBytes, TxBytes: f.TxBytes, RxRate: f.RxRate, TxRate: f.TxRate, Started: f.StartedUnix,
		})
	}
	return out, nil
}

// BuildConnections returns the recent connection log for the given owner's nodes.
func BuildConnections(store *storage.Store, ownerAdminID int64, limit int) ([]Connection, error) {
	ids, err := ownedNodeIDs(store, ownerAdminID)
	if err != nil {
		return nil, err
	}
	rows, err := store.ListConnectionLogForNodes(ids, limit)
	if err != nil {
		return nil, err
	}
	out := make([]Connection, 0, len(rows))
	for _, c := range rows {
		out = append(out, Connection{
			NodeID: c.NodeID, SessionID: c.SessionID, StreamID: c.StreamID,
			ClientIP: c.ClientIP, Remote: c.Remote, Protocol: c.Protocol,
			RxBytes: c.RxBytes, TxBytes: c.TxBytes, FirstSeen: c.FirstSeenUnix, LastSeen: c.LastSeenUnix,
		})
	}
	return out, nil
}

func ownedNodeIDs(store *storage.Store, ownerAdminID int64) ([]string, error) {
	nodes, err := store.ListServerNodesByOwner(storage.ServerNodeVKTurnProxy, ownerAdminID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(nodes))
	for _, n := range nodes {
		ids = append(ids, n.ID)
	}
	return ids, nil
}

// aggregateTrafficSeries turns per-node cumulative counters into a combined
// per-bucket delta series (bytes transferred in each bucket, summed across nodes)
// and the 24h totals. Samples must be ordered by node then ascending ts. A counter
// going backwards (relay restart) contributes zero rather than a negative delta.
func aggregateTrafficSeries(samples []dbmodel.TrafficSample, bucketSec int64) ([]SeriesPoint, uint64, uint64) {
	type acc struct{ rx, tx uint64 }
	buckets := map[int64]*acc{}
	var totalRx, totalTx uint64
	var prevNode string
	var prevRx, prevTx uint64
	haveNode := false
	for _, s := range samples {
		if !haveNode || s.NodeID != prevNode {
			prevNode = s.NodeID
			prevRx, prevTx = s.RxBytes, s.TxBytes
			haveNode = true
			continue
		}
		var drx, dtx uint64
		if s.RxBytes >= prevRx {
			drx = s.RxBytes - prevRx
		}
		if s.TxBytes >= prevTx {
			dtx = s.TxBytes - prevTx
		}
		prevRx, prevTx = s.RxBytes, s.TxBytes
		bucket := s.TsUnix - s.TsUnix%bucketSec
		a := buckets[bucket]
		if a == nil {
			a = &acc{}
			buckets[bucket] = a
		}
		a.rx += drx
		a.tx += dtx
		totalRx += drx
		totalTx += dtx
	}
	keys := make([]int64, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	points := make([]SeriesPoint, 0, len(keys))
	for _, k := range keys {
		points = append(points, SeriesPoint{Ts: k, RxBytes: buckets[k].rx, TxBytes: buckets[k].tx})
	}
	return points, totalRx, totalTx
}
