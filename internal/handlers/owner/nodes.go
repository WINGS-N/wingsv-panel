package owner

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"v.wingsnet.org/internal/storage"
	"v.wingsnet.org/internal/storage/dbmodel"
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

// handleNodes lists every managed server node with its status (GET) or registers
// a new panel-local node (POST). The list mirrors the owner all-clients view:
// panel-local nodes (owner_admin_id 0) plus the external endpoints admins added,
// tagged with the owning admin.
func (h *Handler) handleNodes(w http.ResponseWriter, r *http.Request, owner storage.Admin) {
	switch r.Method {
	case http.MethodGet:
		h.respondListNodes(w, r)
	case http.MethodPost:
		h.respondCreateNode(w, r, owner)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

type createNodeRequest struct {
	Name         string `json:"name"`
	Kind         string `json:"kind"`
	GRPCEndpoint string `json:"grpc_endpoint"`
	GRPCToken    string `json:"grpc_token"`
}

// respondCreateNode registers a panel-local node (owner_admin_id 0) the owner
// manages: a vk-turn-proxy relay or a 3x-ui server the panel polls over gRPC.
func (h *Handler) respondCreateNode(w http.ResponseWriter, r *http.Request, owner storage.Admin) {
	var req createNodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	kind := strings.TrimSpace(req.Kind)
	if kind != storage.ServerNodeVKTurnProxy && kind != storage.ServerNodeXUI {
		writeError(w, http.StatusBadRequest, "kind must be vk_turn_proxy or xui")
		return
	}
	endpoint := strings.TrimSpace(req.GRPCEndpoint)
	if endpoint == "" {
		writeError(w, http.StatusBadRequest, "grpc_endpoint is required")
		return
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	node, err := h.store.CreateServerNode(dbmodel.ServerNode{
		ID:           hex.EncodeToString(raw),
		Kind:         kind,
		Name:         strings.TrimSpace(req.Name),
		GRPCEndpoint: endpoint,
		GRPCToken:    strings.TrimSpace(req.GRPCToken),
		OwnerAdminID: 0,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = h.store.AppendAudit(storage.AuditEntry{
		ActorAdminID: owner.ID, ActorUsername: owner.Username,
		Action: "owner.node_added", Message: endpoint, IP: clientIP(r),
	})
	writeJSON(w, http.StatusCreated, map[string]any{"id": node.ID})
}

// handleNodeByID lets the owner delete any node.
func (h *Handler) handleNodeByID(w http.ResponseWriter, r *http.Request, owner storage.Admin) {
	id := strings.TrimPrefix(r.URL.Path, "/api/owner/nodes/")
	if id == "" {
		writeError(w, http.StatusNotFound, "node id missing")
		return
	}
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	node, err := h.store.GetServerNode(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "node not found")
		return
	}
	if err := h.store.DeleteServerNode(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = h.store.AppendAudit(storage.AuditEntry{
		ActorAdminID: owner.ID, ActorUsername: owner.Username,
		Action: "owner.node_deleted", Message: node.GRPCEndpoint, IP: clientIP(r),
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handler) respondListNodes(w http.ResponseWriter, r *http.Request) {
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
