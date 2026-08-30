package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"v.wingsnet.org/internal/gen/headpb"
)

func liveUpdate(up float64) *headpb.LiveUpdate {
	return &headpb.LiveUpdate{
		UnixMs: time.Now().UnixMilli(),
		Global: &headpb.GlobalCounters{
			NodesOnline: 3,
			Sessions:    12,
			UpBytes:     1000,
			DownBytes:   9000,
			UpRateBps:   up,
			DownRateBps: 400,
		},
	}
}

// A frozen number reads as live traffic that is not happening, so a stale
// snapshot must not be served at all.
func TestStaleSnapshotIsNotServed(t *testing.T) {
	live := &federationLive{}
	if _, fresh := live.snapshot(); fresh {
		t.Error("an empty cache reported fresh")
	}
	live.set(liveUpdate(100))
	if _, fresh := live.snapshot(); !fresh {
		t.Error("a just-set snapshot reported stale")
	}
	live.updated = time.Now().Add(-time.Minute)
	if _, fresh := live.snapshot(); fresh {
		t.Error("a minute-old snapshot was still served as live")
	}
}

// The public feed carries aggregates and nothing else: no node id, no donor, no
// profile. This is the invariant, not a formatting detail.
func TestPublicPayloadCarriesOnlyAggregates(t *testing.T) {
	payload, err := json.Marshal(publicStats(liveUpdate(250)))
	if err != nil {
		t.Fatal(err)
	}
	body := string(payload)
	for _, forbidden := range []string{"node_id", "donor", "profile", "email", "client"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("public payload mentions %q: %s", forbidden, body)
		}
	}
	var got publicStatsPayload
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatal(err)
	}
	if got.NodesOnline != 3 || got.UsersOnline != 12 || got.UpRateBps != 250 {
		t.Errorf("payload = %+v", got)
	}
}

// With no head configured the endpoint does not exist, rather than existing and
// answering zeroes: a counter reading zero is a claim, absence is not.
func TestStatsEndpointIsAbsentWithoutAHead(t *testing.T) {
	s := &Server{fedLive: &federationLive{}}
	rec := httptest.NewRecorder()
	s.handleFederationStats(rec, httptest.NewRequest(http.MethodGet, "/api/federation/stats", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// A subscriber that goes away must not leave the handler pushing forever.
func TestStatsStreamStopsWithTheClient(t *testing.T) {
	live := &federationLive{}
	live.set(liveUpdate(10))
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/api/federation/stats", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		streamFederationStats(rec, req, live)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the handler kept streaming after the client left")
	}
	if !strings.Contains(rec.Body.String(), "data: ") {
		t.Errorf("no event was written: %q", rec.Body.String())
	}
}
