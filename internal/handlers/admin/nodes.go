package admin

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"v.wingsnet.org/internal/storage"
	"v.wingsnet.org/internal/storage/dbmodel"
)

type adminNodeView struct {
	ID             string `json:"id"`
	Kind           string `json:"kind"`
	Name           string `json:"name"`
	GRPCEndpoint   string `json:"grpc_endpoint"`
	Status         string `json:"status"`
	PeerCount      uint32 `json:"peer_count"`
	ActiveSessions uint32 `json:"active_sessions"`
	LastSeen       int64  `json:"last_seen"`
	CreatedAt      int64  `json:"created_at"`
}

func (h *Handler) adminGRPCAllowed() bool {
	v, _ := h.store.GetPlatformSetting(storage.SettingAllowAdminGRPC, "false")
	return v == "true"
}

func (h *Handler) nodeToView(n dbmodel.ServerNode) adminNodeView {
	view := adminNodeView{
		ID: n.ID, Kind: n.Kind, Name: n.Name, GRPCEndpoint: n.GRPCEndpoint,
		Status: n.Status, LastSeen: n.LastSeenAt, CreatedAt: n.CreatedAtUnix,
	}
	if latest, err := h.store.LatestTrafficSample(n.ID); err == nil {
		view.PeerCount = latest.PeerCount
		view.ActiveSessions = latest.ActiveSessions
	}
	return view
}

// handleNodes lists an admin's own external gRPC nodes and, when the owner has
// enabled it, lets them register a new one.
func (h *Handler) handleNodes(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	switch r.Method {
	case http.MethodGet:
		nodes, err := h.store.ListServerNodesByOwner("", admin.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		out := make([]adminNodeView, 0, len(nodes))
		for _, n := range nodes {
			out = append(out, h.nodeToView(n))
		}
		writeJSON(w, http.StatusOK, map[string]any{"nodes": out, "allow_grpc": h.adminGRPCAllowed()})
	case http.MethodPost:
		h.handleCreateNode(w, r, admin)
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

func (h *Handler) handleCreateNode(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	if !h.adminGRPCAllowed() {
		writeError(w, http.StatusForbidden, "registering your own gRPC endpoints is disabled by the owner")
		return
	}
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
	id, err := randomNodeID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	node, err := h.store.CreateServerNode(dbmodel.ServerNode{
		ID:           id,
		Kind:         kind,
		Name:         strings.TrimSpace(req.Name),
		GRPCEndpoint: endpoint,
		GRPCToken:    strings.TrimSpace(req.GRPCToken),
		OwnerAdminID: admin.ID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = h.store.AppendAudit(storage.AuditEntry{
		ActorAdminID: admin.ID, ActorUsername: admin.Username,
		Action: "admin.node_added", Message: endpoint, IP: clientIP(r),
	})
	writeJSON(w, http.StatusCreated, h.nodeToView(node))
}

// handleNodeByID deletes one of the admin's own nodes.
func (h *Handler) handleNodeByID(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	id := strings.TrimPrefix(r.URL.Path, "/api/admin/nodes/")
	if id == "" {
		writeError(w, http.StatusNotFound, "node id missing")
		return
	}
	node, err := h.store.GetServerNode(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "node not found")
		return
	}
	if node.OwnerAdminID != admin.ID {
		writeError(w, http.StatusForbidden, "not owned")
		return
	}
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err := h.store.DeleteServerNode(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = h.store.AppendAudit(storage.AuditEntry{
		ActorAdminID: admin.ID, ActorUsername: admin.Username,
		Action: "admin.node_deleted", Message: node.GRPCEndpoint, IP: clientIP(r),
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func randomNodeID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}
