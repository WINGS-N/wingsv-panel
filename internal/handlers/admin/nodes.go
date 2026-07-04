package admin

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"v.wingsnet.org/internal/auth"
	"v.wingsnet.org/internal/storage"
	"v.wingsnet.org/internal/storage/dbmodel"
)

var (
	errBadWGBackend = errors.New("wg_backend must be empty, own, or xui")
	errBadWGTarget  = errors.New("xui_node_id is required when wg_backend is xui")
)

type adminNodeView struct {
	ID             string `json:"id"`
	Kind           string `json:"kind"`
	Name           string `json:"name"`
	GRPCEndpoint   string `json:"grpc_endpoint"`
	Status         string `json:"status"`
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

func (h *Handler) adminGRPCAllowed() bool {
	v, _ := h.store.GetPlatformSetting(storage.SettingAllowAdminGRPC, "false")
	return v == "true"
}

func (h *Handler) nodeToView(n dbmodel.ServerNode) adminNodeView {
	view := adminNodeView{
		ID: n.ID, Kind: n.Kind, Name: n.Name, GRPCEndpoint: n.GRPCEndpoint,
		Status: n.Status, XrayState: n.XrayState, XrayVersion: n.XrayVersion,
		WGBackend: n.WGBackend, XuiNodeID: n.XuiNodeID, XuiInboundTag: n.XuiInboundTag,
		LastSeen: n.LastSeenAt, CreatedAt: n.CreatedAtUnix,
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
		// The owner also owns the panel-local nodes (owner_admin_id 0) added in the
		// owner console, so surface them here as their servers too.
		if auth.IsOwner(admin) {
			// owner_admin_id 0 marks the panel-local nodes owned by the owner.
			local, lerr := h.store.ListServerNodesByOwner("", 0)
			if lerr != nil {
				writeError(w, http.StatusInternalServerError, lerr.Error())
				return
			}
			nodes = append(nodes, local...)
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
	// wg-backend config for a vk_turn_proxy node: how it provisions managed wg.
	WGBackend     string `json:"wg_backend"`
	XuiNodeID     string `json:"xui_node_id"`
	XuiInboundTag string `json:"xui_inbound_tag"`
}

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
	backend, xuiNodeID, inboundTag, err := normalizeWGBackend(req.WGBackend, req.XuiNodeID, req.XuiInboundTag)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	id, err := randomNodeID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	node, err := h.store.CreateServerNode(dbmodel.ServerNode{
		ID:            id,
		Kind:          kind,
		Name:          strings.TrimSpace(req.Name),
		GRPCEndpoint:  endpoint,
		GRPCToken:     strings.TrimSpace(req.GRPCToken),
		OwnerAdminID:  admin.ID,
		WGBackend:     backend,
		XuiNodeID:     xuiNodeID,
		XuiInboundTag: inboundTag,
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

// handleNodeByID dispatches /api/admin/nodes/{id}[/clients]: DELETE the node, or
// POST an inbound client onto a 3x-ui node.
func (h *Handler) handleNodeByID(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/admin/nodes/")
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
	// The owner can manage the panel-local nodes; an admin only their own.
	if node.OwnerAdminID != admin.ID && !auth.IsOwner(admin) {
		writeError(w, http.StatusForbidden, "not owned")
		return
	}
	subpath := ""
	if len(parts) == 2 {
		subpath = parts[1]
	}
	switch {
	case subpath == "" && r.Method == http.MethodDelete:
		h.respondDeleteNode(w, r, admin, node)
	case subpath == "" && r.Method == http.MethodPut:
		h.respondUpdateNode(w, r, admin, node)
	case subpath == "inbounds" && r.Method == http.MethodGet:
		h.respondNodeInbounds(w, r, admin, node)
	case subpath == "connect" && r.Method == http.MethodGet:
		respondNodeConnect(w, node)
	case subpath == "clients" && r.Method == http.MethodPost:
		h.respondCreateNodeClient(w, r, admin, node)
	default:
		writeError(w, http.StatusNotFound, "unknown route")
	}
}

// respondNodeConnect returns the stored gRPC bearer token so the UI can rebuild
// the one-line connect command for an already-registered node. The token is a
// shared secret the admin set, not a hashed credential.
func respondNodeConnect(w http.ResponseWriter, node dbmodel.ServerNode) {
	writeJSON(w, http.StatusOK, map[string]any{
		"id":            node.ID,
		"kind":          node.Kind,
		"grpc_endpoint": node.GRPCEndpoint,
		"grpc_token":    node.GRPCToken,
	})
}

// respondUpdateNode edits a node's name/endpoint/token and its wg-backend config.
func (h *Handler) respondUpdateNode(w http.ResponseWriter, r *http.Request, admin storage.Admin, node dbmodel.ServerNode) {
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
	updated, err := h.store.GetServerNode(node.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, h.nodeToView(updated))
}

// respondNodeInbounds lists a 3x-ui node's inbounds so the panel can populate the
// wg-backend inbound dropdown.
func (h *Handler) respondNodeInbounds(w http.ResponseWriter, r *http.Request, _ storage.Admin, node dbmodel.ServerNode) {
	if node.Kind != storage.ServerNodeXUI {
		writeError(w, http.StatusBadRequest, "node is not a 3x-ui node")
		return
	}
	inbounds, err := h.xui.ListInbounds(r.Context(), node)
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

func (h *Handler) respondDeleteNode(w http.ResponseWriter, r *http.Request, admin storage.Admin, node dbmodel.ServerNode) {
	if err := h.store.DeleteServerNode(node.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = h.store.AppendAudit(storage.AuditEntry{
		ActorAdminID: admin.ID, ActorUsername: admin.Username,
		Action: "admin.node_deleted", Message: node.GRPCEndpoint, IP: clientIP(r),
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

type createNodeClientRequest struct {
	// PayloadJSON is the inbound-client settings JSON the 3x-ui REST API binds.
	PayloadJSON string `json:"payload_json"`
}

// respondCreateNodeClient adds an inbound client on a 3x-ui node via its Panel gRPC.
func (h *Handler) respondCreateNodeClient(w http.ResponseWriter, r *http.Request, admin storage.Admin, node dbmodel.ServerNode) {
	if node.Kind != storage.ServerNodeXUI {
		writeError(w, http.StatusBadRequest, "node is not a 3x-ui node")
		return
	}
	var req createNodeClientRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if strings.TrimSpace(req.PayloadJSON) == "" {
		writeError(w, http.StatusBadRequest, "payload_json is required")
		return
	}
	needRestart, err := h.xui.AddClient(r.Context(), node, req.PayloadJSON)
	if err != nil {
		writeError(w, http.StatusBadGateway, "xui add client: "+err.Error())
		return
	}
	_ = h.store.AppendAudit(storage.AuditEntry{
		ActorAdminID: admin.ID, ActorUsername: admin.Username,
		Action: "admin.xui_client_added", Message: node.GRPCEndpoint, IP: clientIP(r),
	})
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "need_restart": needRestart})
}

func randomNodeID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}
