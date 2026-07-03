package storage

import (
	"testing"

	"v.wingsnet.org/internal/storage/dbmodel"
)

func TestTrafficSamplesRoundTrip(t *testing.T) {
	st := openTemp(t)
	for i := int64(1); i <= 3; i++ {
		if err := st.InsertTrafficSample(dbmodel.TrafficSample{
			NodeID: "n1", TsUnix: 100 + i, RxBytes: uint64(i * 10), TxBytes: uint64(i * 5), ActiveSessions: uint32(i),
		}); err != nil {
			t.Fatalf("InsertTrafficSample: %v", err)
		}
	}
	if err := st.InsertTrafficSample(dbmodel.TrafficSample{NodeID: "n2", TsUnix: 999, RxBytes: 7}); err != nil {
		t.Fatalf("InsertTrafficSample n2: %v", err)
	}

	samples, err := st.ListTrafficSamples("n1", 102)
	if err != nil {
		t.Fatalf("ListTrafficSamples: %v", err)
	}
	if len(samples) != 2 || samples[0].TsUnix != 102 || samples[1].TsUnix != 103 {
		t.Fatalf("want 2 samples ordered from ts=102, got %+v", samples)
	}

	latest, err := st.LatestTrafficSample("n1")
	if err != nil {
		t.Fatalf("LatestTrafficSample: %v", err)
	}
	if latest.TsUnix != 103 || latest.RxBytes != 30 {
		t.Fatalf("latest wrong: %+v", latest)
	}

	if _, err := st.LatestTrafficSample("missing"); err != ErrNotFound {
		t.Fatalf("want ErrNotFound for missing node, got %v", err)
	}
}

func TestReplaceFlows(t *testing.T) {
	st := openTemp(t)
	first := []dbmodel.FlowSnapshot{
		{NodeID: "n1", SessionID: "s1", StreamID: 0, ClientIP: "1.1.1.1", RxRate: 100},
		{NodeID: "n1", SessionID: "s1", StreamID: 1, ClientIP: "1.1.1.1", RxRate: 300},
	}
	if err := st.ReplaceFlows("n1", first); err != nil {
		t.Fatalf("ReplaceFlows: %v", err)
	}
	flows, err := st.ListFlows()
	if err != nil {
		t.Fatalf("ListFlows: %v", err)
	}
	if len(flows) != 2 || flows[0].RxRate != 300 {
		t.Fatalf("want 2 flows busiest-first, got %+v", flows)
	}

	// A fresh poll with a single flow must fully replace the prior snapshot.
	if err := st.ReplaceFlows("n1", []dbmodel.FlowSnapshot{{NodeID: "n1", SessionID: "s2", StreamID: 0}}); err != nil {
		t.Fatalf("ReplaceFlows 2: %v", err)
	}
	flows, _ = st.ListFlows()
	if len(flows) != 1 || flows[0].SessionID != "s2" {
		t.Fatalf("want single replaced flow, got %+v", flows)
	}
}

func TestRecordConnectionsUpsert(t *testing.T) {
	st := openTemp(t)
	row := dbmodel.ConnectionLog{
		NodeID: "n1", SessionID: "s1", StreamID: 0, StartedUnix: 500,
		ClientIP: "2.2.2.2", Remote: "8.8.8.8:53", Protocol: "udp",
		RxBytes: 10, TxBytes: 20, FirstSeenUnix: 500, LastSeenUnix: 500,
	}
	if err := st.RecordConnections([]dbmodel.ConnectionLog{row}); err != nil {
		t.Fatalf("RecordConnections insert: %v", err)
	}
	// Same identity, later sighting: first_seen stays, last_seen + bytes update.
	row.RxBytes = 999
	row.LastSeenUnix = 600
	row.FirstSeenUnix = 600
	if err := st.RecordConnections([]dbmodel.ConnectionLog{row}); err != nil {
		t.Fatalf("RecordConnections update: %v", err)
	}
	rows, err := st.ListConnectionLog(10)
	if err != nil {
		t.Fatalf("ListConnectionLog: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 connection row after upsert, got %d", len(rows))
	}
	if rows[0].FirstSeenUnix != 500 || rows[0].LastSeenUnix != 600 || rows[0].RxBytes != 999 {
		t.Fatalf("upsert kept first_seen, refreshed last_seen+bytes: got %+v", rows[0])
	}
}
