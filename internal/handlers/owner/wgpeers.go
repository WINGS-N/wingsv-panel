package owner

import (
	"net/http"
	"strings"

	"v.wingsnet.org/internal/storage"
)

// handleWGPeers lists every managed WireGuard peer (owner sees all).
func (h *Handler) handleWGPeers(w http.ResponseWriter, r *http.Request, _ storage.Admin) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	peers, err := h.store.ListClientWGPeersForOwner(0, true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list wg peers")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"peers": peers})
}

// handleWGPeerByID revokes one peer at /api/owner/wgpeers/{clientID}/{nodeID}.
// It removes the panel record; the peer is not re-issued unless the client
// re-provisions.
func (h *Handler) handleWGPeerByID(w http.ResponseWriter, r *http.Request, _ storage.Admin) {
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/owner/wgpeers/")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		writeError(w, http.StatusBadRequest, "expected /api/owner/wgpeers/{clientId}/{nodeId}")
		return
	}
	if err := h.store.DeleteClientWGPeer(parts[0], parts[1]); err != nil {
		writeError(w, http.StatusNotFound, "peer not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
