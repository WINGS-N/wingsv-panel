package admin

import (
	"net/http"
	"strconv"
	"strings"

	"v.wingsnet.org/internal/auth"
	"v.wingsnet.org/internal/statsview"
	"v.wingsnet.org/internal/storage"
)

// An admin sees the traffic dashboard for the external vk-turn-proxy / 3x-ui
// nodes they registered themselves (owner_admin_id == admin.ID). The owner also
// owns the panel-local nodes (owner_admin_id 0), so their "my servers" view
// includes those too via localExtra.

// localExtra returns the panel-local owner id (0) as an extra scope for the owner,
// so the owner's my-servers stats also cover the nodes added in the owner console.
func localExtra(admin storage.Admin) []int64 {
	if auth.IsOwner(admin) {
		return []int64{0}
	}
	return nil
}

func (h *Handler) handleStatsTraffic(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	out, err := statsview.BuildTraffic(h.store, admin.ID, r.URL.Query().Get("range"), r.URL.Query().Get("sample"), r.URL.Query().Get("node"), localExtra(admin)...)
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
	flows, err := statsview.BuildFlows(h.store, admin.ID, r.URL.Query().Get("node"), localExtra(admin)...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read flows")
		return
	}
	names, _ := statsview.ClientNames(h.store, admin.ID, localExtra(admin)...)
	writeJSON(w, http.StatusOK, map[string]any{"flows": flows, "client_names": names})
}

func (h *Handler) handleStatsXrayFlows(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	node, err := h.store.GetServerNode(strings.TrimSpace(r.URL.Query().Get("node")))
	allowed := err == nil && node.Kind == storage.ServerNodeXUI &&
		(node.OwnerAdminID == admin.ID || (auth.IsOwner(admin) && node.OwnerAdminID == 0))
	if !allowed {
		writeError(w, http.StatusNotFound, "unknown xui node")
		return
	}
	flows, names, err := statsview.BuildXrayFlows(h.xui, node)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read xray flows")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"flows": flows, "client_names": names})
}

func (h *Handler) handleStatsFlowHistory(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	flows, err := statsview.BuildFlowHistory(h.store, admin.ID, r.URL.Query().Get("window"), localExtra(admin)...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read flow history")
		return
	}
	names, _ := statsview.ClientNames(h.store, admin.ID, localExtra(admin)...)
	writeJSON(w, http.StatusOK, map[string]any{"flows": flows, "client_names": names})
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
	conns, total, err := statsview.BuildConnections(h.store, admin.ID, limit, offset, r.URL.Query().Get("node"), localExtra(admin)...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read connection log")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"connections": conns, "total": total, "limit": limit, "offset": offset,
	})
}
