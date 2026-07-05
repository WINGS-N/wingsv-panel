// Package statsview builds the node-traffic dashboard payloads (aggregate
// traffic, live flows and connection log) for a given node owner. The owner
// console passes ownerAdminID 0 to view the panel-local nodes; an admin passes
// their own id to view the external vk-turn-proxy / 3x-ui endpoints they added.
package statsview

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"v.wingsnet.org/internal/storage"
	"v.wingsnet.org/internal/storage/dbmodel"
	"v.wingsnet.org/internal/xuiclient"
)

// trafficCache memoises the heavy traffic aggregation for a few seconds. The
// collector only writes new samples every persist interval, so serving a slightly
// stale payload keeps many dashboard pollers (every 3s) from each re-scanning a
// week-to-month of samples - the read cost that would otherwise sink sqlite.
const trafficCacheTTL = 5 * time.Second

var (
	trafficCacheMu sync.Mutex
	trafficCache   = map[string]trafficCacheEntry{}
)

type trafficCacheEntry struct {
	at  time.Time
	val Traffic
}

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
	Rx1h           uint64 `json:"rx_1h"`
	Tx1h           uint64 `json:"tx_1h"`
	Rx24h          uint64 `json:"rx_24h"`
	Tx24h          uint64 `json:"tx_24h"`
	Rx7d           uint64 `json:"rx_7d"`
	Tx7d           uint64 `json:"tx_7d"`
	RxAll          uint64 `json:"rx_all"`
	TxAll          uint64 `json:"tx_all"`
	CurRxRate      uint64 `json:"cur_rx_rate"`
	CurTxRate      uint64 `json:"cur_tx_rate"`
}

type Traffic struct {
	Mode        string        `json:"mode"`
	GeneratedAt int64         `json:"generated_at"`
	Range       string        `json:"range"`
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

// trafficRange maps a UI range key to the chart window and its bucket width.
type trafficRange struct {
	window    time.Duration
	bucketSec int64
}

var trafficRanges = map[string]trafficRange{
	"24h":   {window: 24 * time.Hour, bucketSec: 60},
	"7d":    {window: 7 * 24 * time.Hour, bucketSec: 3600},
	"month": {window: 30 * 24 * time.Hour, bucketSec: 6 * 3600},
}

// ResolveRange normalises a range key, defaulting to 24h.
func ResolveRange(key string) string {
	if _, ok := trafficRanges[key]; ok {
		return key
	}
	return "24h"
}

// sampleBuckets maps a UI sample key to its bucket width in seconds. It lets the
// operator pick the chart granularity independently of the range window.
var sampleBuckets = map[string]int64{
	"1m":  60,
	"15m": 15 * 60,
	"30m": 30 * 60,
	"1h":  3600,
}

// resolveSampleSec returns the bucket width for a sample key, or def when the key
// is empty/unknown (so the range's own default width is used).
func resolveSampleSec(key string, def int64) int64 {
	if s, ok := sampleBuckets[key]; ok {
		return s
	}
	return def
}

// BuildTraffic builds the dashboard traffic view for the given owner's nodes. rng
// selects the chart window (24h|7d|month); the tile totals (1h/24h/7d/all-time)
// are always computed regardless of the selected chart range.
func BuildTraffic(store *storage.Store, ownerAdminID int64, rng, sample, nodeFilter string, extraOwners ...int64) (Traffic, error) {
	rng = ResolveRange(rng)
	key := fmt.Sprintf("%d|%v|%s|%s|%s", ownerAdminID, extraOwners, rng, sample, nodeFilter)
	now := time.Now()
	trafficCacheMu.Lock()
	if e, ok := trafficCache[key]; ok && now.Sub(e.at) < trafficCacheTTL {
		trafficCacheMu.Unlock()
		return e.val, nil
	}
	trafficCacheMu.Unlock()

	out, err := buildTraffic(store, ownerAdminID, rng, sample, nodeFilter, extraOwners...)
	if err == nil {
		trafficCacheMu.Lock()
		trafficCache[key] = trafficCacheEntry{at: now, val: out}
		trafficCacheMu.Unlock()
	}
	return out, err
}

func buildTraffic(store *storage.Store, ownerAdminID int64, rng, sample, nodeFilter string, extraOwners ...int64) (Traffic, error) {
	rc := trafficRanges[rng]
	bucketSec := resolveSampleSec(sample, rc.bucketSec)
	mode, _ := store.GetPanelMode()
	nodes, err := store.ListServerNodesByOwners(storage.ServerNodeVKTurnProxy, append([]int64{ownerAdminID}, extraOwners...))
	if err != nil {
		return Traffic{}, err
	}
	if nodeFilter != "" {
		kept := nodes[:0]
		for _, n := range nodes {
			if n.ID == nodeFilter {
				kept = append(kept, n)
			}
		}
		nodes = kept
	}
	out := Traffic{Mode: string(mode), GeneratedAt: time.Now().Unix(), Range: rng}
	out.Totals.Nodes = len(nodes)
	owned := make(map[string]bool, len(nodes))
	ids := make([]string, 0, len(nodes))
	for _, n := range nodes {
		owned[n.ID] = true
		ids = append(ids, n.ID)
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
	// Load enough history to cover both the chart window and the widest tile
	// window (7d), in one query, then slice per need.
	now := time.Now()
	loadWindow := rc.window
	if week := 7 * 24 * time.Hour; week > loadWindow {
		loadWindow = week
	}
	samples, err := store.ListTrafficSince(now.Add(-loadWindow).Unix())
	if err != nil {
		return Traffic{}, err
	}
	filtered := samples[:0]
	for _, s := range samples {
		if owned[s.NodeID] {
			filtered = append(filtered, s)
		}
	}

	out.Series, _, _ = aggregateTrafficSeries(sinceCutoff(filtered, now.Add(-rc.window).Unix()), bucketSec)
	_, out.Totals.Rx1h, out.Totals.Tx1h = aggregateTrafficSeries(sinceCutoff(filtered, now.Add(-time.Hour).Unix()), rc.bucketSec)
	_, out.Totals.Rx24h, out.Totals.Tx24h = aggregateTrafficSeries(sinceCutoff(filtered, now.Add(-24*time.Hour).Unix()), rc.bucketSec)
	_, out.Totals.Rx7d, out.Totals.Tx7d = aggregateTrafficSeries(sinceCutoff(filtered, now.Add(-7*24*time.Hour).Unix()), rc.bucketSec)

	rxAll, txAll, err := store.SumNodeTrafficTotals(ids)
	if err != nil {
		return Traffic{}, err
	}
	out.Totals.RxAll, out.Totals.TxAll = rxAll, txAll
	out.Totals.CurRxRate, out.Totals.CurTxRate = currentRates(filtered)
	return out, nil
}

// currentRates estimates live per-second rx/tx by differencing each node's last
// two cumulative samples (the collector polls every ~10s). Samples must be ordered
// by node then ascending ts. A gap wider than staleWindow is treated as the node
// having been offline and contributes nothing.
func currentRates(samples []dbmodel.TrafficSample) (uint64, uint64) {
	const staleWindow = 120 // seconds
	prev := map[string]dbmodel.TrafficSample{}
	cur := map[string]dbmodel.TrafficSample{}
	for _, s := range samples {
		if c, ok := cur[s.NodeID]; ok {
			prev[s.NodeID] = c
		}
		cur[s.NodeID] = s
	}
	var rx, tx uint64
	for id, c := range cur {
		p, ok := prev[id]
		if !ok {
			continue
		}
		dt := c.TsUnix - p.TsUnix
		if dt <= 0 || dt > staleWindow {
			continue
		}
		if c.RxBytes >= p.RxBytes {
			rx += (c.RxBytes - p.RxBytes) / uint64(dt)
		}
		if c.TxBytes >= p.TxBytes {
			tx += (c.TxBytes - p.TxBytes) / uint64(dt)
		}
	}
	return rx, tx
}

// sinceCutoff returns the tail of an ascending-by-(node,ts) sample slice whose ts
// is at or after cutoff, preserving the node grouping aggregateTrafficSeries needs.
// The slice is scanned rather than binary-searched because it is grouped by node,
// not globally sorted by ts.
func sinceCutoff(samples []dbmodel.TrafficSample, cutoff int64) []dbmodel.TrafficSample {
	out := make([]dbmodel.TrafficSample, 0, len(samples))
	for _, s := range samples {
		if s.TsUnix >= cutoff {
			out = append(out, s)
		}
	}
	return out
}

// BuildFlows returns the current live flows for the given owner's nodes,
// optionally narrowed to a single node (nodeFilter, empty = all owned).
func BuildFlows(store *storage.Store, ownerAdminID int64, nodeFilter string, extraOwners ...int64) ([]Flow, error) {
	ids, err := ownedNodeIDs(store, ownerAdminID, extraOwners...)
	if err != nil {
		return nil, err
	}
	ids = filterNodeIDs(ids, nodeFilter)
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

// BuildXrayFlows turns a 3x-ui node's online clients into flow edges (client email
// -> node -> inbound) sized by cumulative per-client traffic, so the same graph
// component renders Xray activity. Xray does not expose per-connection remotes via
// 3x-ui, so the inbound tag stands in for the destination. Returns the flows and an
// email->name map (identity, so the graph labels clients by email).
func BuildXrayFlows(xui XuiFlowSource, node dbmodel.ServerNode) ([]Flow, map[string]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	inbounds, err := xui.ListInbounds(ctx, node)
	if err != nil {
		return nil, nil, err
	}
	inByID := make(map[int64]xuiclient.InboundSummary, len(inbounds))
	for _, in := range inbounds {
		inByID[in.ID] = in
	}
	emails, err := xui.ListOnlineClients(ctx, node)
	if err != nil {
		return nil, nil, err
	}
	flows := make([]Flow, 0, len(emails))
	names := make(map[string]string, len(emails))
	for _, email := range emails {
		t, terr := xui.GetClientTraffic(ctx, node, email)
		if terr != nil {
			continue
		}
		in := inByID[t.InboundID]
		remote := in.Tag
		if remote == "" {
			remote = in.Remark
		}
		if remote == "" {
			remote = "inbound"
		}
		flows = append(flows, Flow{
			NodeID: node.ID, ClientIP: email, Remote: remote, Protocol: in.Protocol,
			RxBytes: nonNegU64(t.Down), TxBytes: nonNegU64(t.Up),
		})
		names[email] = email
	}
	return flows, names, nil
}

// XuiFlowSource is the subset of the 3x-ui gRPC client BuildXrayFlows needs.
type XuiFlowSource interface {
	ListInbounds(ctx context.Context, node dbmodel.ServerNode) ([]xuiclient.InboundSummary, error)
	ListOnlineClients(ctx context.Context, node dbmodel.ServerNode) ([]string, error)
	GetClientTraffic(ctx context.Context, node dbmodel.ServerNode, email string) (xuiclient.ClientTraffic, error)
}

func nonNegU64(v int64) uint64 {
	if v < 0 {
		return 0
	}
	return uint64(v)
}

// ClientNames maps a managed client's tunnel IP to its name for the owner's
// nodes, so the flow graph can label clients by name instead of raw IP.
func ClientNames(store *storage.Store, ownerAdminID int64, extraOwners ...int64) (map[string]string, error) {
	ids, err := ownedNodeIDs(store, ownerAdminID, extraOwners...)
	if err != nil {
		return nil, err
	}
	return store.ClientNamesByPeerIP(ids)
}

// flowHistoryWindows maps a UI window key to its lookback. Bounded by the
// connection-log retention (one hour), so nothing longer than 1h is offered.
var flowHistoryWindows = map[string]time.Duration{
	"15m": 15 * time.Minute,
	"30m": 30 * time.Minute,
	"1h":  time.Hour,
}

// BuildFlowHistory returns the historical flow graph for the given owner's nodes:
// connection-log paths aggregated over the selected window (default 1h), shaped
// like Flow so the same graph component renders it. Rates are zero (no live rate
// for past traffic).
func BuildFlowHistory(store *storage.Store, ownerAdminID int64, window string, extraOwners ...int64) ([]Flow, error) {
	dur, ok := flowHistoryWindows[window]
	if !ok {
		dur = time.Hour
	}
	ids, err := ownedNodeIDs(store, ownerAdminID, extraOwners...)
	if err != nil {
		return nil, err
	}
	rows, err := store.AggregateConnectionsSince(ids, time.Now().Add(-dur).Unix())
	if err != nil {
		return nil, err
	}
	out := make([]Flow, 0, len(rows))
	for _, r := range rows {
		out = append(out, Flow{
			NodeID: r.NodeID, ClientIP: r.ClientIP, Remote: r.Remote, Protocol: r.Protocol,
			RxBytes: r.RxBytes, TxBytes: r.TxBytes,
		})
	}
	return out, nil
}

// BuildConnections returns a page of the connection log for the given owner's
// nodes, newest-first, plus the total row count so the UI can paginate.
// nodeFilter narrows to a single node (empty = all owned).
func BuildConnections(store *storage.Store, ownerAdminID int64, limit, offset int, nodeFilter string, extraOwners ...int64) ([]Connection, int64, error) {
	ids, err := ownedNodeIDs(store, ownerAdminID, extraOwners...)
	if err != nil {
		return nil, 0, err
	}
	ids = filterNodeIDs(ids, nodeFilter)
	total, err := store.CountConnectionLogForNodes(ids)
	if err != nil {
		return nil, 0, err
	}
	rows, err := store.ListConnectionLogForNodes(ids, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	out := make([]Connection, 0, len(rows))
	for _, c := range rows {
		out = append(out, Connection{
			NodeID: c.NodeID, SessionID: c.SessionID, StreamID: c.StreamID,
			ClientIP: c.ClientIP, Remote: c.Remote, Protocol: c.Protocol,
			RxBytes: c.RxBytes, TxBytes: c.TxBytes, FirstSeen: c.FirstSeenUnix, LastSeen: c.LastSeenUnix,
		})
	}
	return out, total, nil
}

func ownedNodeIDs(store *storage.Store, ownerAdminID int64, extraOwners ...int64) ([]string, error) {
	nodes, err := store.ListServerNodesByOwners(storage.ServerNodeVKTurnProxy, append([]int64{ownerAdminID}, extraOwners...))
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(nodes))
	for _, n := range nodes {
		ids = append(ids, n.ID)
	}
	return ids, nil
}

// filterNodeIDs narrows an owned-node set to a single node for the per-node
// drill-down. An empty filter keeps them all; a filter that names a node the
// caller does not own returns nothing (so no cross-owner data leaks).
func filterNodeIDs(ids []string, nodeFilter string) []string {
	if nodeFilter == "" {
		return ids
	}
	for _, id := range ids {
		if id == nodeFilter {
			return []string{nodeFilter}
		}
	}
	return nil
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
