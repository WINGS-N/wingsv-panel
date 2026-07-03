package admin

import (
	"net/http"
	"strconv"

	"v.wingsnet.org/internal/statsview"
	"v.wingsnet.org/internal/storage"
)

// An admin sees the traffic dashboard only for the external vk-turn-proxy /
// 3x-ui nodes they registered themselves (owner_admin_id == admin.ID). The
// panel-local nodes the owner runs are owner-only and never surfaced here.

func (h *Handler) handleStatsTraffic(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	out, err := statsview.BuildTraffic(h.store, admin.ID, r.URL.Query().Get("range"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to build traffic stats")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) handleStatsFlows(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	flows, err := statsview.BuildFlows(h.store, admin.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read flows")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"flows": flows})
}

func (h *Handler) handleStatsConnections(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}
	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			offset = n
		}
	}
	conns, total, err := statsview.BuildConnections(h.store, admin.ID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read connection log")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"connections": conns, "total": total, "limit": limit, "offset": offset,
	})
}
