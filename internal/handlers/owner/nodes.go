package owner

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"v.wingsnet.org/internal/storage"
	"v.wingsnet.org/internal/storage/dbmodel"
	"v.wingsnet.org/internal/xuiclient"
)

var (
	errBadWGBackend = errors.New("wg_backend must be empty, own, or xui")
	errBadWGTarget  = errors.New("xui_node_id is required when wg_backend is xui")
)

// normalizeWGBackend validates the wg-backend selection and its 3x-ui target.
func normalizeWGBackend(backend, xuiNodeID, inboundTag string) (string, string, string, error) {
	backend = strings.TrimSpace(backend)
	switch backend {
	case "", storage.WGBackendOwn:
		return backend, "", "", nil
	case storage.WGBackendXUI:
		if strings.TrimSpace(xuiNodeID) == "" {
			return "", "", "", errBadWGTarget
		}
		return storage.WGBackendXUI, strings.TrimSpace(xuiNodeID), strings.TrimSpace(inboundTag), nil
	default:
		return "", "", "", errBadWGBackend
	}
}

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
	XrayState      string `json:"xray_state"`
	XrayVersion    string `json:"xray_version"`
	WGBackend      string `json:"wg_backend"`
	XuiNodeID      string `json:"xui_node_id"`
	XuiInboundTag  string `json:"xui_inbound_tag"`
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
	Name          string `json:"name"`
	Kind          string `json:"kind"`
	GRPCEndpoint  string `json:"grpc_endpoint"`
	GRPCToken     string `json:"grpc_token"`
	WGBackend     string `json:"wg_backend"`
	XuiNodeID     string `json:"xui_node_id"`
	XuiInboundTag string `json:"xui_inbound_tag"`
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
	backend, xuiNodeID, inboundTag, err := normalizeWGBackend(req.WGBackend, req.XuiNodeID, req.XuiInboundTag)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	node, err := h.store.CreateServerNode(dbmodel.ServerNode{
		ID:            hex.EncodeToString(raw),
		Kind:          kind,
		Name:          strings.TrimSpace(req.Name),
		GRPCEndpoint:  endpoint,
		GRPCToken:     strings.TrimSpace(req.GRPCToken),
		OwnerAdminID:  0,
		WGBackend:     backend,
		XuiNodeID:     xuiNodeID,
		XuiInboundTag: inboundTag,
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

// handleNodeByID dispatches /api/owner/nodes/{id}[/inbounds]: DELETE / PUT the
// node, or list a 3x-ui node's inbounds for the wg-backend dropdown.
func (h *Handler) handleNodeByID(w http.ResponseWriter, r *http.Request, owner storage.Admin) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/owner/nodes/")
	parts := strings.SplitN(rest, "/", 2)
	id := parts[0]
	if id == "" {
		writeError(w, http.StatusNotFound, "node id missing")
		return
	}
	node, err := h.store.GetServerNode(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "node not found")
		return
	}
	subpath := ""
	if len(parts) == 2 {
		subpath = parts[1]
	}
	switch {
	case subpath == "" && r.Method == http.MethodDelete:
		if err := h.store.DeleteServerNode(id); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		_ = h.store.AppendAudit(storage.AuditEntry{
			ActorAdminID: owner.ID, ActorUsername: owner.Username,
			Action: "owner.node_deleted", Message: node.GRPCEndpoint, IP: clientIP(r),
		})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case subpath == "" && r.Method == http.MethodPut:
		h.respondUpdateNode(w, r, node)
	case subpath == "inbounds" && r.Method == http.MethodGet:
		h.respondNodeInbounds(w, r, node)
	case subpath == "connect" && r.Method == http.MethodGet:
		respondNodeConnect(w, node)
	default:
		writeError(w, http.StatusNotFound, "unknown route")
	}
}

// respondNodeConnect returns the stored gRPC bearer token so the UI can rebuild
// the one-line connect command for an already-registered node. The token is a
// shared secret the owner set, not a hashed credential.
func respondNodeConnect(w http.ResponseWriter, node dbmodel.ServerNode) {
	writeJSON(w, http.StatusOK, map[string]any{
		"id":            node.ID,
		"kind":          node.Kind,
		"grpc_endpoint": node.GRPCEndpoint,
		"grpc_token":    node.GRPCToken,
	})
}

func (h *Handler) respondUpdateNode(w http.ResponseWriter, r *http.Request, node dbmodel.ServerNode) {
	var req createNodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	backend, xuiNodeID, inboundTag, err := normalizeWGBackend(req.WGBackend, req.XuiNodeID, req.XuiInboundTag)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	node.Name = strings.TrimSpace(req.Name)
	if e := strings.TrimSpace(req.GRPCEndpoint); e != "" {
		node.GRPCEndpoint = e
	}
	// Only overwrite the gRPC bearer token when a new one is supplied; an edit that
	// leaves it blank must keep the existing token, or the node loses auth and goes
	// offline.
	if t := strings.TrimSpace(req.GRPCToken); t != "" {
		node.GRPCToken = t
	}
	node.WGBackend = backend
	node.XuiNodeID = xuiNodeID
	node.XuiInboundTag = inboundTag
	if err := h.store.UpdateServerNode(node); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handler) respondNodeInbounds(w http.ResponseWriter, r *http.Request, node dbmodel.ServerNode) {
	if node.Kind != storage.ServerNodeXUI {
		writeError(w, http.StatusBadRequest, "node is not a 3x-ui node")
		return
	}
	inbounds, err := xuiclient.New().ListInbounds(r.Context(), node)
	if err != nil {
		writeError(w, http.StatusBadGateway, "xui list inbounds: "+err.Error())
		return
	}
	out := make([]map[string]any, 0, len(inbounds))
	for _, in := range inbounds {
		out = append(out, map[string]any{
			"tag": in.Tag, "remark": in.Remark, "protocol": in.Protocol, "port": in.Port,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"inbounds": out})
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
			Status:        n.Status,
			OwnerID:       n.OwnerAdminID,
			Local:         n.OwnerAdminID == 0,
			XrayState:     n.XrayState,
			XrayVersion:   n.XrayVersion,
			WGBackend:     n.WGBackend,
			XuiNodeID:     n.XuiNodeID,
			XuiInboundTag: n.XuiInboundTag,
			LastSeen:      n.LastSeenAt,
			CreatedAt:     n.CreatedAtUnix,
		}
		if n.OwnerAdminID != 0 {
			view.OwnerName = nameByID[n.OwnerAdminID]
		}
		if latest, lerr := h.store.LatestTrafficSample(n.ID); lerr == nil {
			view.PeerCount = latest.PeerCount
			view.ActiveSessions = latest.ActiveSessions
		}
		// Fall back to the panel's provisioned-peer count when the relay reports 0
		// (xui-backend node, or relay unreachable).
		if c := uint32(h.store.CountClientWGPeersForNode(n.ID)); c > view.PeerCount {
			view.PeerCount = c
		}
		out = append(out, view)
	}
	writeJSON(w, http.StatusOK, map[string]any{"nodes": out})
}
