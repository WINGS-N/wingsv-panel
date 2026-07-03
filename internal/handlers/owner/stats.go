package owner

import (
	"net/http"
	"strconv"

	"v.wingsnet.org/internal/statsview"
	"v.wingsnet.org/internal/storage"
)

// The node-traffic dashboard here is owner-only: it reports the panel-local
// vk-turn-proxy / 3x-ui nodes the owner deploys and manages (owner_admin_id 0).
// A non-owner admin's own external nodes are surfaced under /api/admin/stats.
const localNodeOwnerID int64 = 0

func (h *Handler) handleStatsTraffic(w http.ResponseWriter, r *http.Request, _ storage.Admin) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	out, err := statsview.BuildTraffic(h.store, localNodeOwnerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to build traffic stats")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) handleStatsFlows(w http.ResponseWriter, r *http.Request, _ storage.Admin) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	flows, err := statsview.BuildFlows(h.store, localNodeOwnerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read flows")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"flows": flows})
}

func (h *Handler) handleStatsConnections(w http.ResponseWriter, r *http.Request, _ storage.Admin) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	conns, err := statsview.BuildConnections(h.store, localNodeOwnerID, parseLimit(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read connection log")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"connections": conns})
}

func parseLimit(r *http.Request) int {
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 1000 {
			return n
		}
	}
	return 100
}
