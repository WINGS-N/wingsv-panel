package storage

import (
	"testing"

	"v.wingsnet.org/internal/storage/dbmodel"
)

// seedManagedClient creates an owner admin, one client owned by it, a node and a
// wg peer, returning the owner id. The peer lets peer_traffic attribute to the
// client via the client_wg_peers join.
func seedManagedClient(t *testing.T, st *Store) int64 {
	t.Helper()
	admin := dbmodel.Admin{Username: "owner", PasswordHash: "h", Role: "owner", CreatedAtUnix: 1, UpdatedAtUnix: 1}
	if err := st.gdb.Create(&admin).Error; err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	client := dbmodel.Client{ID: "c1", OwnerAdminID: admin.ID, Name: "dev", TokenHash: "h", SyncMode: "always", CreatedAtUnix: 1}
	if err := st.gdb.Create(&client).Error; err != nil {
		t.Fatalf("seed client: %v", err)
	}
	if _, err := st.CreateServerNode(dbmodel.ServerNode{ID: "n1", Kind: ServerNodeVKTurnProxy, Name: "n1"}); err != nil {
		t.Fatalf("seed node: %v", err)
	}
	if err := st.UpsertClientWGPeer(dbmodel.ClientWGPeer{ClientID: "c1", NodeID: "n1", PublicKey: "pub1"}); err != nil {
		t.Fatalf("seed peer: %v", err)
	}
	return admin.ID
}

func setPeerTraffic(t *testing.T, st *Store, rx, tx uint64) {
	t.Helper()
	if err := st.UpsertPeerTraffic([]dbmodel.PeerTraffic{
		{NodeID: "n1", PublicKey: "pub1", RxBytes: rx, TxBytes: tx, SampledUnix: 1},
	}); err != nil {
		t.Fatalf("UpsertPeerTraffic: %v", err)
	}
}

func mustControl(t *testing.T, st *Store) ClientControl {
	t.Helper()
	c, err := st.GetClientControl("c1")
	if err != nil {
		t.Fatalf("GetClientControl: %v", err)
	}
	return c
}

func TestClientTrafficAccumulateAndLimit(t *testing.T) {
	st := openTemp(t)
	owner := seedManagedClient(t, st)

	if err := st.SetClientTrafficLimit("c1", owner, 1000, 0, 0); err != nil {
		t.Fatalf("SetClientTrafficLimit: %v", err)
	}

	// First reading: whole cumulative counts as usage.
	setPeerTraffic(t, st, 300, 200)
	if err := st.AccumulateClientTraffic(); err != nil {
		t.Fatalf("accumulate 1: %v", err)
	}
	if c := mustControl(t, st); c.UsedBytes != 500 || c.Blocked() {
		t.Fatalf("after first accumulate: used=%d blocked=%v, want 500/false", c.UsedBytes, c.Blocked())
	}

	// Growth: only the non-negative delta adds (rx 300->600, tx 200->500 => +600).
	setPeerTraffic(t, st, 600, 500)
	if err := st.AccumulateClientTraffic(); err != nil {
		t.Fatalf("accumulate 2: %v", err)
	}
	if c := mustControl(t, st); c.UsedBytes != 1100 || !c.Blocked() {
		t.Fatalf("after growth: used=%d blocked=%v, want 1100/true", c.UsedBytes, c.Blocked())
	}

	// Relay restart: a smaller reading adds zero and rebaselines.
	setPeerTraffic(t, st, 10, 5)
	if err := st.AccumulateClientTraffic(); err != nil {
		t.Fatalf("accumulate 3: %v", err)
	}
	if c := mustControl(t, st); c.UsedBytes != 1100 {
		t.Fatalf("after restart: used=%d, want 1100 (no negative delta)", c.UsedBytes)
	}
	// Growth from the rebaselined 10/5.
	setPeerTraffic(t, st, 40, 5)
	if err := st.AccumulateClientTraffic(); err != nil {
		t.Fatalf("accumulate 4: %v", err)
	}
	if c := mustControl(t, st); c.UsedBytes != 1130 {
		t.Fatalf("after post-restart growth: used=%d, want 1130", c.UsedBytes)
	}
}

func TestClientTrafficManualReset(t *testing.T) {
	st := openTemp(t)
	owner := seedManagedClient(t, st)
	// First accumulate baselines at the current reading (usage counts from now).
	setPeerTraffic(t, st, 700, 700)
	if err := st.AccumulateClientTraffic(); err != nil {
		t.Fatalf("accumulate baseline: %v", err)
	}
	if c := mustControl(t, st); c.UsedBytes != 0 {
		t.Fatalf("baseline used=%d want 0", c.UsedBytes)
	}
	// Growth from the baseline (rx 700->1200 +500, tx 700->900 +200 => 700).
	setPeerTraffic(t, st, 1200, 900)
	if err := st.AccumulateClientTraffic(); err != nil {
		t.Fatalf("accumulate growth: %v", err)
	}
	if c := mustControl(t, st); c.UsedBytes != 700 {
		t.Fatalf("used=%d want 700", c.UsedBytes)
	}
	if err := st.ResetClientTraffic("c1", owner, 0); err != nil {
		t.Fatalf("ResetClientTraffic: %v", err)
	}
	if c := mustControl(t, st); c.UsedBytes != 0 {
		t.Fatalf("after reset used=%d want 0", c.UsedBytes)
	}
	// Accounting resumes from the current reading, not from scratch.
	setPeerTraffic(t, st, 1250, 900)
	if err := st.AccumulateClientTraffic(); err != nil {
		t.Fatalf("accumulate after reset: %v", err)
	}
	if c := mustControl(t, st); c.UsedBytes != 50 {
		t.Fatalf("after reset growth used=%d want 50", c.UsedBytes)
	}
}

func TestClientTrafficPeriodicReset(t *testing.T) {
	st := openTemp(t)
	owner := seedManagedClient(t, st)
	const now = 1_000_000
	// Set the cap+period first (baselines at the current zero reading), then let
	// traffic grow so usage is non-zero before the window elapses.
	if err := st.SetClientTrafficLimit("c1", owner, 2000, 7, now); err != nil {
		t.Fatalf("SetClientTrafficLimit periodic: %v", err)
	}
	setPeerTraffic(t, st, 500, 500)
	if err := st.AccumulateClientTraffic(); err != nil {
		t.Fatalf("accumulate: %v", err)
	}
	// Not yet due.
	reset, err := st.RunDueTrafficResets(now + 7*secondsPerDay - 1)
	if err != nil {
		t.Fatalf("RunDueTrafficResets early: %v", err)
	}
	if len(reset) != 0 {
		t.Fatalf("reset early: %v, want none", reset)
	}
	if c := mustControl(t, st); c.UsedBytes != 1000 {
		t.Fatalf("used before due=%d want 1000", c.UsedBytes)
	}
	// Due.
	reset, err = st.RunDueTrafficResets(now + 7*secondsPerDay)
	if err != nil {
		t.Fatalf("RunDueTrafficResets due: %v", err)
	}
	if len(reset) != 1 || reset[0] != "c1" {
		t.Fatalf("reset due=%v, want [c1]", reset)
	}
	c := mustControl(t, st)
	if c.UsedBytes != 0 {
		t.Fatalf("used after periodic reset=%d want 0", c.UsedBytes)
	}
	if c.NextResetUnix != now+2*7*secondsPerDay {
		t.Fatalf("next reset re-armed to %d, want %d", c.NextResetUnix, now+2*7*secondsPerDay)
	}
}

func TestNodeClientUsageForLimits(t *testing.T) {
	st := openTemp(t)
	owner := seedManagedClient(t, st)
	// An uncapped, enabled client is omitted.
	if rows, err := st.NodeClientUsageForLimits("n1"); err != nil || len(rows) != 0 {
		t.Fatalf("uncapped node usage = %+v (err %v), want empty", rows, err)
	}
	if err := st.SetClientTrafficLimit("c1", owner, 1000, 0, 0); err != nil {
		t.Fatalf("limit: %v", err)
	}
	setPeerTraffic(t, st, 400, 200)
	if err := st.AccumulateClientTraffic(); err != nil {
		t.Fatalf("accumulate: %v", err)
	}
	rows, err := st.NodeClientUsageForLimits("n1")
	if err != nil {
		t.Fatalf("NodeClientUsageForLimits: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows=%+v want 1", rows)
	}
	r := rows[0]
	if r.PublicKey != "pub1" || r.ClientID != "c1" || r.LimitBytes != 1000 || r.UsedBytes != 600 || r.Disabled {
		t.Fatalf("usage row wrong: %+v", r)
	}
	// A different node has no peers for this client.
	if rows, err := st.NodeClientUsageForLimits("n2"); err != nil || len(rows) != 0 {
		t.Fatalf("other-node usage = %+v (err %v), want empty", rows, err)
	}
}

func TestClientDisabledBlocksAndOwnerGate(t *testing.T) {
	st := openTemp(t)
	owner := seedManagedClient(t, st)
	if c := mustControl(t, st); c.Blocked() {
		t.Fatalf("fresh client should not be blocked")
	}
	if err := st.SetClientDisabled("c1", owner, true); err != nil {
		t.Fatalf("SetClientDisabled: %v", err)
	}
	if c := mustControl(t, st); !c.Blocked() || !c.Disabled {
		t.Fatalf("disabled client not blocked: %+v", c)
	}
	if err := st.SetClientDisabled("c1", owner, false); err != nil {
		t.Fatalf("re-enable: %v", err)
	}
	if c := mustControl(t, st); c.Blocked() {
		t.Fatalf("re-enabled client still blocked")
	}
	// Owner gate: a different owner id cannot flip it.
	if err := st.SetClientDisabled("c1", owner+999, true); err != ErrNotFound {
		t.Fatalf("wrong-owner disable = %v, want ErrNotFound", err)
	}
}
