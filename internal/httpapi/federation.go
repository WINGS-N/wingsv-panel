package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"v.wingsnet.org/internal/fedclient"
	"v.wingsnet.org/internal/gen/headpb"
	"v.wingsnet.org/internal/storage"
)

// publicStatsInterval is how often an SSE subscriber is pushed a frame. One
// second, because the point of a live counter is that it visibly moves.
const publicStatsInterval = time.Second

// federationLive holds the newest snapshot from the head. SSE readers and the
// admin socket both serve from here rather than each holding their own stream.
type federationLive struct {
	mu      sync.RWMutex
	latest  *headpb.LiveUpdate
	updated time.Time
}

func (f *federationLive) set(update *headpb.LiveUpdate) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.latest, f.updated = update, time.Now()
}

// snapshot returns the last frame and whether it is recent enough to serve. A
// stale frame is worse than none: a counter frozen at the last real number reads
// as live traffic that is not happening.
func (f *federationLive) snapshot() (*headpb.LiveUpdate, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.latest == nil || time.Since(f.updated) > 15*time.Second {
		return nil, false
	}
	return f.latest, true
}

// StartFederationLive keeps the head's global counters flowing into the panel:
// out to the admin socket as they arrive, and cached for the public SSE feed.
func (s *Server) StartFederationLive(ctx context.Context, client *fedclient.Client, emit func(kind string, payload []byte)) {
	if client == nil || !client.Enabled() {
		return
	}
	go client.StreamGlobal(ctx, func(update *headpb.LiveUpdate) {
		s.fedLive.set(update)
		payload, err := json.Marshal(publicStats(update))
		if err != nil {
			return
		}
		emit("fed_global", payload)
	})
}

// publicStatsPayload is the shape both the admin socket and the public feed
// carry. Aggregates only - no node, no donor, no profile.
type publicStatsPayload struct {
	UnixMs      int64   `json:"unix_ms"`
	NodesOnline uint32  `json:"nodes_online"`
	UsersOnline uint32  `json:"users_online"`
	UpBytes     uint64  `json:"up_bytes"`
	DownBytes   uint64  `json:"down_bytes"`
	UpRateBps   float64 `json:"up_rate_bps"`
	DownRateBps float64 `json:"down_rate_bps"`
}

func publicStats(update *headpb.LiveUpdate) publicStatsPayload {
	global := update.GetGlobal()
	return publicStatsPayload{
		UnixMs:      update.GetUnixMs(),
		NodesOnline: global.GetNodesOnline(),
		UsersOnline: global.GetSessions(),
		UpBytes:     global.GetUpBytes(),
		DownBytes:   global.GetDownBytes(),
		UpRateBps:   global.GetUpRateBps(),
		DownRateBps: global.GetDownRateBps(),
	}
}

// handleFederationStats is the landing page counter: server-sent events rather
// than a websocket, because there is no session to establish, nothing to send
// upstream, and a reverse proxy can cache a second of it in front of a spike.
func (s *Server) handleFederationStats(w http.ResponseWriter, r *http.Request) {
	if !s.federationEnabled() {
		http.NotFound(w, r)
		return
	}
	streamFederationStats(w, r, s.fedLive)
}

func streamFederationStats(w http.ResponseWriter, r *http.Request, live *federationLive) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ticker := time.NewTicker(publicStatsInterval)
	defer ticker.Stop()
	for {
		if update, fresh := live.snapshot(); fresh {
			payload, err := json.Marshal(publicStats(update))
			if err == nil {
				if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
					return
				}
				flusher.Flush()
			}
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

// federationEnabled reads the operator toggle. Absent means off.
func (s *Server) federationEnabled() bool {
	if !s.fedConfigured || s.store == nil {
		return false
	}
	value, err := s.store.GetPlatformSetting(storage.SettingFederationEnabled, "")
	if err != nil {
		return false
	}
	return value == "1" || value == "true"
}
