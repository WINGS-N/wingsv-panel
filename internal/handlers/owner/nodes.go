package owner

import (
	"net/http"

	"v.wingsnet.org/internal/storage"
)

type nodeView struct {
	ID             string `json:"id"`
	Kind           string `json:"kind"`
	Name           string `json:"name"`
	GRPCEndpoint   string `json:"grpc_endpoint"`
	Status         string `json:"status"`
	OwnerID        int64  `json:"owner_admin_id"`
	OwnerName      string `json:"owner_name"`
	Local          bool   `json:"local"`
	PeerCount      uint32 `json:"peer_count"`
	ActiveSessions uint32 `json:"active_sessions"`
	LastSeen       int64  `json:"last_seen"`
	CreatedAt      int64  `json:"created_at"`
}

// handleNodes lists every managed server node with its status, mirroring the
// owner all-clients view: panel-local nodes (owner_admin_id 0) plus the external
// endpoints admins registered, tagged with the owning admin.
func (h *Handler) handleNodes(w http.ResponseWriter, r *http.Request, _ storage.Admin) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	admins, err := h.store.ListAdmins()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	nameByID := make(map[int64]string, len(admins))
	for _, a := range admins {
		nameByID[a.ID] = a.Username
	}
	nodes, err := h.store.ListServerNodes("")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]nodeView, 0, len(nodes))
	for _, n := range nodes {
		view := nodeView{
			ID:           n.ID,
			Kind:         n.Kind,
			Name:         n.Name,
			GRPCEndpoint: n.GRPCEndpoint,
			Status:       n.Status,
			OwnerID:      n.OwnerAdminID,
			Local:        n.OwnerAdminID == 0,
			LastSeen:     n.LastSeenAt,
			CreatedAt:    n.CreatedAtUnix,
		}
		if n.OwnerAdminID != 0 {
			view.OwnerName = nameByID[n.OwnerAdminID]
		}
		if latest, lerr := h.store.LatestTrafficSample(n.ID); lerr == nil {
			view.PeerCount = latest.PeerCount
			view.ActiveSessions = latest.ActiveSessions
		}
		out = append(out, view)
	}
	writeJSON(w, http.StatusOK, map[string]any{"nodes": out})
}
