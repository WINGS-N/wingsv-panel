package collector

import (
	"context"
	"errors"
	"testing"
	"time"

	"v.wingsnet.org/internal/relayclient"
	"v.wingsnet.org/internal/storage/dbmodel"
)

type fakeStore struct {
	nodes    []dbmodel.ServerNode
	samples  []dbmodel.TrafficSample
	flows    map[string][]dbmodel.FlowSnapshot
	conns    []dbmodel.ConnectionLog
	statuses map[string]string
	pruned   bool
}

func newFakeStore(nodes ...dbmodel.ServerNode) *fakeStore {
	return &fakeStore{nodes: nodes, flows: map[string][]dbmodel.FlowSnapshot{}, statuses: map[string]string{}}
}

func (f *fakeStore) ListServerNodes(kind string) ([]dbmodel.ServerNode, error) { return f.nodes, nil }
func (f *fakeStore) InsertTrafficSample(s dbmodel.TrafficSample) error {
	f.samples = append(f.samples, s)
	return nil
}
func (f *fakeStore) ReplaceFlows(nodeID string, flows []dbmodel.FlowSnapshot) error {
	f.flows[nodeID] = flows
	return nil
}
func (f *fakeStore) RecordConnections(rows []dbmodel.ConnectionLog) error {
	f.conns = append(f.conns, rows...)
	return nil
}
func (f *fakeStore) PruneTrafficBefore(time.Time) error     { f.pruned = true; return nil }
func (f *fakeStore) PruneConnectionsBefore(time.Time) error { return nil }
func (f *fakeStore) UpdateServerNodeStatus(id, status string, _ int64) error {
	f.statuses[id] = status
	return nil
}

type fakeRelay struct {
	status    relayclient.RelayStatus
	stats     relayclient.FlowStats
	flows     []relayclient.Flow
	statusErr error
}

func (r *fakeRelay) NodeStatus(context.Context, dbmodel.ServerNode) (relayclient.RelayStatus, error) {
	return r.status, r.statusErr
}
func (r *fakeRelay) FlowStats(context.Context, dbmodel.ServerNode) (relayclient.FlowStats, error) {
	return r.stats, nil
}
func (r *fakeRelay) ListFlows(context.Context, dbmodel.ServerNode) ([]relayclient.Flow, error) {
	return r.flows, nil
}

func TestCollectOncePersistsAndNotifies(t *testing.T) {
	store := newFakeStore(dbmodel.ServerNode{ID: "n1", Kind: "vk_turn_proxy", GRPCEndpoint: "x:1"})
	relay := &fakeRelay{
		status: relayclient.RelayStatus{PeerCount: 3},
		stats:  relayclient.FlowStats{ActiveStreams: 2, ActiveSessions: 1, ServerRxBytes: 1000, ServerTxBytes: 500},
		flows: []relayclient.Flow{
			{SessionID: "s1", StreamID: 0, ClientIP: "1.1.1.1", Remote: "8.8.8.8:53", Protocol: "udp", RxRate: 10},
		},
	}
	var notified []string
	c := New(store, relay, Options{
		Now:         func() time.Time { return time.Unix(1000, 0) },
		OnCollected: func(nodeID string) { notified = append(notified, nodeID) },
	})
	c.CollectOnce(context.Background())

	if len(store.samples) != 1 || store.samples[0].RxBytes != 1000 || store.samples[0].PeerCount != 3 {
		t.Fatalf("sample not persisted correctly: %+v", store.samples)
	}
	if store.samples[0].TsUnix != 1000 {
		t.Fatalf("sample ts should use injected now, got %d", store.samples[0].TsUnix)
	}
	if len(store.flows["n1"]) != 1 || store.flows["n1"][0].SampledUnix != 1000 {
		t.Fatalf("flow snapshot wrong: %+v", store.flows["n1"])
	}
	if len(store.conns) != 1 || store.conns[0].FirstSeenUnix != 1000 {
		t.Fatalf("connection log wrong: %+v", store.conns)
	}
	if store.statuses["n1"] != "online" {
		t.Fatalf("node should be marked online, got %q", store.statuses["n1"])
	}
	if !store.pruned {
		t.Fatal("prune should have run")
	}
	if len(notified) != 1 || notified[0] != "n1" {
		t.Fatalf("OnCollected not fired for n1: %v", notified)
	}
}

func TestCollectNodeMarksOfflineOnError(t *testing.T) {
	store := newFakeStore(dbmodel.ServerNode{ID: "n1", Kind: "vk_turn_proxy"})
	relay := &fakeRelay{statusErr: errors.New("dial fail")}
	c := New(store, relay, Options{Now: func() time.Time { return time.Unix(1, 0) }})
	c.CollectOnce(context.Background())
	if store.statuses["n1"] != "offline" {
		t.Fatalf("node should be marked offline, got %q", store.statuses["n1"])
	}
	if len(store.samples) != 0 {
		t.Fatalf("no sample should be written when the node is unreachable, got %d", len(store.samples))
	}
}
