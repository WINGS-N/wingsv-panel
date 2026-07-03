package owner

import (
	"net/http"
	"strconv"
	"strings"

	"v.wingsnet.org/internal/statsview"
	"v.wingsnet.org/internal/storage"
	"v.wingsnet.org/internal/xuiclient"
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
	out, err := statsview.BuildTraffic(h.store, localNodeOwnerID, r.URL.Query().Get("range"), r.URL.Query().Get("node"))
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
	flows, err := statsview.BuildFlows(h.store, localNodeOwnerID, r.URL.Query().Get("node"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read flows")
		return
	}
	names, _ := statsview.ClientNames(h.store, localNodeOwnerID)
	writeJSON(w, http.StatusOK, map[string]any{"flows": flows, "client_names": names})
}

func (h *Handler) handleStatsXrayFlows(w http.ResponseWriter, r *http.Request, _ storage.Admin) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	node, err := h.store.GetServerNode(strings.TrimSpace(r.URL.Query().Get("node")))
	if err != nil || node.Kind != storage.ServerNodeXUI || node.OwnerAdminID != localNodeOwnerID {
		writeError(w, http.StatusNotFound, "unknown xui node")
		return
	}
	flows, names, err := statsview.BuildXrayFlows(xuiclient.New(), node)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read xray flows")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"flows": flows, "client_names": names})
}

func (h *Handler) handleStatsFlowHistory(w http.ResponseWriter, r *http.Request, _ storage.Admin) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	flows, err := statsview.BuildFlowHistory(h.store, localNodeOwnerID, r.URL.Query().Get("window"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read flow history")
		return
	}
	names, _ := statsview.ClientNames(h.store, localNodeOwnerID)
	writeJSON(w, http.StatusOK, map[string]any{"flows": flows, "client_names": names})
}

func (h *Handler) handleStatsConnections(w http.ResponseWriter, r *http.Request, _ storage.Admin) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	limit, offset := parseLimit(r), parseOffset(r)
	conns, total, err := statsview.BuildConnections(h.store, localNodeOwnerID, limit, offset, r.URL.Query().Get("node"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read connection log")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"connections": conns, "total": total, "limit": limit, "offset": offset,
	})
}

func parseLimit(r *http.Request) int {
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 1000 {
			return n
		}
	}
	return 100
}

func parseOffset(r *http.Request) int {
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 0
}
