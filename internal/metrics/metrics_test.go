package metrics

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"v.wingsnet.org/internal/relayclient"
	"v.wingsnet.org/internal/storage"
	"v.wingsnet.org/internal/storage/dbmodel"
)

type fakeHub struct{ clients, admins int }

func (f fakeHub) ClientCount() int { return f.clients }
func (f fakeHub) AdminCount() int  { return f.admins }

type fakeNodes struct {
	st  relayclient.RelayStatus
	err error
}

func (f fakeNodes) NodeStatus(context.Context, dbmodel.ServerNode) (relayclient.RelayStatus, error) {
	return f.st, f.err
}

func (f fakeNodes) FlowStats(context.Context, dbmodel.ServerNode) (relayclient.FlowStats, error) {
	return relayclient.FlowStats{
		ActiveStreams: 4, TotalSessions: 9, AvgSessionLifetimeSeconds: 30,
		StreamsByProtocol: map[string]uint32{"mu": 3, "mainline": 1},
	}, nil
}

func TestCollectorExposition(t *testing.T) {
	st, err := storage.Open(storage.Options{Driver: storage.DriverSQLite, DSN: filepath.Join(t.TempDir(), "m.db")})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	admin, err := st.CreateAdmin("owner", "hash", false, "owner")
	if err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	if _, err := st.CreateClient("c1", admin.ID, "dev", "th", nil); err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	if _, err := st.CreateServerNode(dbmodel.ServerNode{ID: "n1", Kind: storage.ServerNodeVKTurnProxy, Name: "relay"}); err != nil {
		t.Fatalf("CreateServerNode: %v", err)
	}

	collector := NewCollector(st, fakeHub{clients: 2, admins: 1}, fakeNodes{
		st: relayclient.RelayStatus{PeerCount: 5, ActiveSessions: 3, RxBytes: 100, TxBytes: 200},
	})
	srv := httptest.NewServer(Handler(collector))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	text := string(body)

	for _, want := range []string{
		"wingsv_clients_total 1",
		"wingsv_admins_total 1",
		"wingsv_ws_clients 2",
		"wingsv_ws_admins 1",
		`wingsv_server_nodes{kind="vk_turn_proxy"} 1`,
		`wingsv_vkturn_up{node="n1"} 1`,
		`wingsv_vkturn_peers{node="n1"} 5`,
		`wingsv_vkturn_active_sessions{node="n1"} 3`,
		`wingsv_vkturn_rx_bytes{node="n1"} 100`,
		`wingsv_vkturn_tx_bytes{node="n1"} 200`,
		`wingsv_vkturn_active_streams{node="n1"} 4`,
		`wingsv_vkturn_avg_session_lifetime_seconds{node="n1"} 30`,
		`wingsv_vkturn_sessions_total{node="n1"} 9`,
		`wingsv_vkturn_streams_by_protocol{node="n1",protocol="mu"} 3`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("metrics output missing %q", want)
		}
	}
}
