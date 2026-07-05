package admin

import (
	"log"
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
	// Cut the client off at the node too, not just in the panel: on an xui-backed
	// node delete the vk-turn-proxy/wg client (its email is the panel client id),
	// which also drops its forward wireguard peer. Best-effort - a stale node peer
	// must not block clearing the panel record.
	h.revokePeerOnNode(r, parts[1], parts[0])
	if err := h.store.DeleteClientWGPeer(parts[0], parts[1]); err != nil {
		writeError(w, http.StatusNotFound, "peer not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// revokePeerOnNode best-effort removes a client's actual peer/client from the
// node backing nodeID. Only the xui backend supports a remote delete today; the
// own-wg relay has no delete RPC, so those peers are cleared only in the panel.
func (h *Handler) revokePeerOnNode(r *http.Request, nodeID, clientID string) {
	node, err := h.store.GetServerNode(nodeID)
	if err != nil {
		return
	}
	if node.WGBackend != storage.WGBackendXUI || node.XuiNodeID == "" {
		return
	}
	xuiNode, err := h.store.GetServerNode(node.XuiNodeID)
	if err != nil {
		return
	}
	if _, err := h.xui.DeleteClient(r.Context(), xuiNode, clientID, false); err != nil {
		log.Printf("wgpeers: node-side revoke failed client=%s node=%s: %v", clientID, nodeID, err)
	}
}
