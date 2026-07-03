package admin

import (
	"net/http"
	"strings"

	"v.wingsnet.org/internal/auth"
	"v.wingsnet.org/internal/storage"
)

// handleWGPeers lists the managed WireGuard peers of the admin's clients (the
// owner sees every peer).
func (h *Handler) handleWGPeers(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	peers, err := h.store.ListClientWGPeersForOwner(admin.ID, auth.IsOwner(admin))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list wg peers")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"peers": peers})
}

// handleWGPeerByID revokes one of the admin's peers at
// /api/admin/wgpeers/{clientID}/{nodeID}.
func (h *Handler) handleWGPeerByID(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/admin/wgpeers/")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		writeError(w, http.StatusBadRequest, "expected /api/admin/wgpeers/{clientId}/{nodeId}")
		return
	}
	// A non-owner may only revoke peers of their own clients.
	if !auth.IsOwner(admin) {
		client, err := h.store.FindClientByID(parts[0])
		if err != nil || client.OwnerAdminID != admin.ID {
			writeError(w, http.StatusNotFound, "peer not found")
			return
		}
	}
	if err := h.store.DeleteClientWGPeer(parts[0], parts[1]); err != nil {
		writeError(w, http.StatusNotFound, "peer not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
