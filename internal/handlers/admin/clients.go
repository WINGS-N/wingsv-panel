package admin

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"v.wingsnet.org/internal/auth"
	guardianpb "v.wingsnet.org/internal/gen/guardianpb"
	wingsvpb "v.wingsnet.org/internal/gen/wingsvpb"
	"v.wingsnet.org/internal/preview"
	"v.wingsnet.org/internal/storage"
)

var (
	protoMarshaller = protojson.MarshalOptions{
		// EmitUnpopulated keeps every field in the JSON — the panel forms
		// need to render an input for every proto field even when the device
		// has it at its default, otherwise switches/inputs would just be
		// missing for "default" sections.
		EmitUnpopulated: true,
		UseProtoNames:   false,
	}
	protoUnmarshaller = protojson.UnmarshalOptions{
		DiscardUnknown: true,
	}
)

func protoToJSON(message proto.Message) (json.RawMessage, error) {
	if message == nil {
		return json.RawMessage("null"), nil
	}
	bytesProto, err := protoMarshaller.Marshal(message)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(bytesProto), nil
}

func jsonToProto(raw json.RawMessage, message proto.Message) error {
	if len(raw) == 0 {
		return errors.New("empty json proto")
	}
	return protoUnmarshaller.Unmarshal(raw, message)
}

type clientView struct {
	ID                      string `json:"id"`
	Name                    string `json:"name"`
	HWID                    string `json:"hwid"`
	DeviceName              string `json:"device_name"`
	DeviceModel             string `json:"device_model"`
	OSVersion               string `json:"os_version"`
	AppVersion              string `json:"app_version"`
	CreatedAt               string `json:"created_at"`
	LastSeenAt              string `json:"last_seen_at"`
	Online                  bool   `json:"online"`
	LogRuntimeEnabled       bool   `json:"log_runtime_enabled"`
	LogProxyEnabled         bool   `json:"log_proxy_enabled"`
	LogXRayEnabled          bool   `json:"log_xray_enabled"`
	SyncMode                string `json:"sync_mode"`
	PeriodicIntervalMinutes int    `json:"periodic_interval_minutes"`
	BackendType             string `json:"backend_type"`
	HasRootAccess           bool   `json:"has_root_access"`
}

func toClientView(c storage.Client) clientView {
	return clientView{
		ID:                      c.ID,
		Name:                    c.Name,
		HWID:                    c.HWID,
		DeviceName:              c.DeviceName,
		DeviceModel:             c.DeviceModel,
		OSVersion:               c.OSVersion,
		AppVersion:              c.AppVersion,
		CreatedAt:               c.CreatedAt.Format(time.RFC3339),
		LastSeenAt:              c.LastSeenAt.Format(time.RFC3339),
		Online:                  c.Online,
		LogRuntimeEnabled:       c.LogRuntimeEnabled,
		LogProxyEnabled:         c.LogProxyEnabled,
		LogXRayEnabled:          c.LogXRayEnabled,
		SyncMode:                c.SyncMode,
		PeriodicIntervalMinutes: c.PeriodicIntervalMinutes,
		HasRootAccess:           c.HasRootAccess,
	}
}

// backendTypeLabel maps the proto enum to a short user-facing label.
func backendTypeLabel(t wingsvpb.BackendType) string {
	switch t {
	case wingsvpb.BackendType_BACKEND_TYPE_VK_TURN_WIREGUARD:
		return "VK TURN + WG"
	case wingsvpb.BackendType_BACKEND_TYPE_XRAY:
		return "Xray"
	case wingsvpb.BackendType_BACKEND_TYPE_AMNEZIAWG:
		return "AmneziaWG"
	case wingsvpb.BackendType_BACKEND_TYPE_WIREGUARD:
		return "WireGuard"
	case wingsvpb.BackendType_BACKEND_TYPE_AMNEZIAWG_PLAIN:
		return "AmneziaWG plain"
	case wingsvpb.BackendType_BACKEND_TYPE_WB_STREAM:
		return "WB Stream"
	}
	return ""
}

// hydrateBackendType reads the desired_config blob (if any) and adds the
// backend label to the view. Best-effort — parser errors are swallowed.
func (h *Handler) hydrateBackendType(view *clientView, clientID string) {
	cfg, err := h.store.GetClientConfig(clientID)
	if err != nil || len(cfg.ConfigProto) == 0 {
		return
	}
	parsed := &wingsvpb.Config{}
	if err := proto.Unmarshal(cfg.ConfigProto, parsed); err != nil {
		return
	}
	view.BackendType = backendTypeLabel(parsed.GetBackend())
}

func normalizeSyncMode(value string) string {
	v := strings.ToLower(strings.TrimSpace(value))
	switch v {
	case "periodic":
		return "periodic"
	case "foreground", "foreground_only":
		return "foreground"
	}
	return "always"
}

func syncModeToProto(value string) wingsvpb.GuardianSyncMode {
	switch normalizeSyncMode(value) {
	case "periodic":
		return wingsvpb.GuardianSyncMode_GUARDIAN_SYNC_MODE_PERIODIC
	case "foreground":
		return wingsvpb.GuardianSyncMode_GUARDIAN_SYNC_MODE_FOREGROUND_ONLY
	}
	return wingsvpb.GuardianSyncMode_GUARDIAN_SYNC_MODE_ALWAYS
}

func (h *Handler) handleClients(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	switch r.Method {
	case http.MethodGet:
		clients, err := h.store.ListClientsByOwner(admin.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		out := make([]clientView, 0, len(clients))
		for _, c := range clients {
			view := toClientView(c)
			h.hydrateBackendType(&view, c.ID)
			out = append(out, view)
		}
		writeJSON(w, http.StatusOK, map[string]any{"clients": out})
	case http.MethodPost:
		h.handleCreateClient(w, r, admin)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

type createClientRequest struct {
	Name                    string `json:"name"`
	SeedFromClientID        string `json:"seed_from_client_id"`
	SeedFromLink            string `json:"seed_from_wingsv_link"`
	SyncMode                string `json:"sync_mode"`
	PeriodicIntervalMinutes int    `json:"periodic_interval_minutes"`
}

func (h *Handler) handleCreateClient(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	var req createClientRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	clientID, err := auth.GenerateClientID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	tokenBytes, tokenHash, err := auth.GenerateClientToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := h.store.CreateClient(clientID, admin.ID, name, tokenHash, tokenBytes); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	syncMode := normalizeSyncMode(req.SyncMode)
	periodic := req.PeriodicIntervalMinutes
	if periodic <= 0 {
		periodic = 30
	}
	if err := h.store.UpdateClientSync(clientID, admin.ID, syncMode, periodic); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	seedConfig, err := h.resolveSeedConfig(req, admin)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if seedConfig == nil {
		seedConfig = &wingsvpb.Config{Ver: 1}
	}
	configBytes, err := proto.Marshal(seedConfig)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := h.store.UpsertClientConfig(clientID, configBytes, "1"); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	link, err := preview.BuildWingsLink(&wingsvpb.Config{
		Ver:  1,
		Type: wingsvpb.ConfigType_CONFIG_TYPE_GUARDIAN,
		Guardian: &wingsvpb.Guardian{
			WsUrl:                   deriveWsURL(h.cfg.PublicBaseURL),
			ClientId:                clientID,
			ClientToken:             tokenBytes,
			ClientName:              name,
			SyncMode:                syncModeToProto(syncMode),
			PeriodicIntervalMinutes: uint32(periodic),
		},
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	created, err := h.store.FindClientByID(clientID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"client":           toClientView(created),
		"wingsv_link":      link,
		"client_token_b64": base64.StdEncoding.EncodeToString(tokenBytes),
	})
}

func (h *Handler) resolveSeedConfig(req createClientRequest, admin storage.Admin) (*wingsvpb.Config, error) {
	if id := strings.TrimSpace(req.SeedFromClientID); id != "" {
		client, err := h.store.FindClientByID(id)
		if err != nil {
			return nil, errors.New("seed client not found")
		}
		if client.OwnerAdminID != admin.ID && !auth.IsOwner(admin) {
			return nil, errors.New("seed client not owned by admin")
		}
		cfg, err := h.store.GetClientConfig(id)
		if err != nil {
			return nil, errors.New("seed client has no config")
		}
		out := &wingsvpb.Config{}
		if err := proto.Unmarshal(cfg.ConfigProto, out); err != nil {
			return nil, errors.New("seed client config corrupted")
		}
		out.Guardian = nil
		return out, nil
	}
	if link := strings.TrimSpace(req.SeedFromLink); link != "" {
		out, err := preview.ParseLinkConfig(link)
		if err != nil {
			return nil, errors.New("invalid link (expected wingsv:// or vless://)")
		}
		out.Guardian = nil
		return out, nil
	}
	return nil, nil
}

// deriveWsURL converts an https://host[:port] base URL to wss://host[:port]/api/guardian/ws.
func deriveWsURL(publicBaseURL string) string {
	base := strings.TrimRight(publicBaseURL, "/")
	if strings.HasPrefix(base, "https://") {
		return "wss://" + strings.TrimPrefix(base, "https://") + "/api/guardian/ws"
	}
	if strings.HasPrefix(base, "http://") {
		return "ws://" + strings.TrimPrefix(base, "http://") + "/api/guardian/ws"
	}
	return "wss://" + base + "/api/guardian/ws"
}

func (h *Handler) handleClientByID(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/admin/clients/")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, http.StatusNotFound, "client id missing")
		return
	}
	clientID := parts[0]
	subpath := ""
	if len(parts) == 2 {
		subpath = parts[1]
	}

	client, err := h.store.FindClientByID(clientID)
	if err != nil {
		writeError(w, http.StatusNotFound, "client not found")
		return
	}
	// Owner может работать с клиентами любого админа — это часть концепции
	// Owner Console. Остальные админы — только свои.
	if client.OwnerAdminID != admin.ID && !auth.IsOwner(admin) {
		writeError(w, http.StatusForbidden, "not owned")
		return
	}

	switch {
	case subpath == "" && r.Method == http.MethodGet:
		h.respondClientDetail(w, client)
	case subpath == "" && r.Method == http.MethodDelete:
		h.respondDeleteClient(w, client)
	case subpath == "config" && r.Method == http.MethodGet:
		h.respondClientConfig(w, client.ID)
	case subpath == "config" && r.Method == http.MethodPut:
		h.respondPushClientConfig(w, r, client)
	case subpath == "log-control" && r.Method == http.MethodPut:
		h.respondLogControl(w, r, client)
	case subpath == "sync" && r.Method == http.MethodPut:
		h.respondSync(w, r, client, admin)
	case subpath == "command" && r.Method == http.MethodPost:
		h.respondCommand(w, r, client)
	case subpath == "wingsv-link" && r.Method == http.MethodGet:
		h.respondWingsvLink(w, client, admin)
	case strings.HasPrefix(subpath, "logs") && r.Method == http.MethodGet:
		h.respondLogs(w, r, client)
	case subpath == "rotate-token" && r.Method == http.MethodPost:
		h.respondRotateToken(w, r, client, admin)
	case subpath == "refresh-subscription" && r.Method == http.MethodPost:
		h.respondRefreshSubscription(w, r, client)
	case subpath == "installed-apps" && r.Method == http.MethodGet:
		h.respondInstalledApps(w, client)
	case subpath == "installed-apps/refresh" && r.Method == http.MethodPost:
		h.respondRefreshInstalledApps(w, client)
	default:
		writeError(w, http.StatusNotFound, "unknown route")
	}
}

type decodeLinkRequest struct {
	Link string `json:"link"`
}

// handleDecodeLink turns a wingsv:// link into the same JSON proto shape the
// config tab uses, so admins can prefill the editor by pasting an export from
// any other WINGS V instance. Token bytes inside Guardian blocks are stripped
// — re-pushing them would only confuse the device.
func (h *Handler) handleDecodeLink(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	_ = admin
	var req decodeLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	parsed, err := preview.ParseLinkConfig(strings.TrimSpace(req.Link))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid link: "+err.Error())
		return
	}
	parsed.Guardian = nil
	asJSON, err := protoToJSON(parsed)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"config": asJSON})
}

func (h *Handler) respondWingsvLink(w http.ResponseWriter, client storage.Client, admin storage.Admin) {
	// admin зарезервирован для будущего audit-логирования при owner-доступе;
	// для query-к store достаточно owner_admin_id самого клиента.
	_ = admin
	tokenBytes, err := h.store.GetClientToken(client.ID, client.OwnerAdminID)
	if err != nil {
		writeError(w, http.StatusNotFound, "token not available — recreate the client to regenerate")
		return
	}
	link, err := preview.BuildWingsLink(&wingsvpb.Config{
		Ver:  1,
		Type: wingsvpb.ConfigType_CONFIG_TYPE_GUARDIAN,
		Guardian: &wingsvpb.Guardian{
			WsUrl:                   deriveWsURL(h.cfg.PublicBaseURL),
			ClientId:                client.ID,
			ClientToken:             tokenBytes,
			ClientName:              client.Name,
			SyncMode:                syncModeToProto(client.SyncMode),
			PeriodicIntervalMinutes: uint32(client.PeriodicIntervalMinutes),
		},
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"wingsv_link": link})
}

func (h *Handler) respondRotateToken(w http.ResponseWriter, r *http.Request, client storage.Client, admin storage.Admin) {
	tokenBytes, tokenHash, err := auth.GenerateClientToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.store.UpdateClientToken(client.ID, client.OwnerAdminID, tokenHash, tokenBytes); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Drop the live device session if present — it's authenticated with the
	// old token and will reconnect once the new wingsv:// link is applied.
	if sink := h.hub.ClientSink(client.ID); sink != nil {
		sink.Close("token_rotated")
	}
	link, err := preview.BuildWingsLink(&wingsvpb.Config{
		Ver:  1,
		Type: wingsvpb.ConfigType_CONFIG_TYPE_GUARDIAN,
		Guardian: &wingsvpb.Guardian{
			WsUrl:                   deriveWsURL(h.cfg.PublicBaseURL),
			ClientId:                client.ID,
			ClientToken:             tokenBytes,
			ClientName:              client.Name,
			SyncMode:                syncModeToProto(client.SyncMode),
			PeriodicIntervalMinutes: uint32(client.PeriodicIntervalMinutes),
		},
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = h.store.AppendAudit(storage.AuditEntry{
		ActorAdminID: admin.ID, ActorUsername: admin.Username,
		Action:     "client.token_rotated",
		TargetType: "client", TargetID: client.ID,
		IP: clientIP(r),
	})
	writeJSON(w, http.StatusOK, map[string]any{"wingsv_link": link})
}

func (h *Handler) respondClientDetail(w http.ResponseWriter, client storage.Client) {
	cfg, _ := h.store.GetClientConfig(client.ID)
	runtimeBytes, runtimeAt, _ := h.store.GetClientRuntime(client.ID)
	reportedBytes, reportedAt, _ := h.store.GetClientReportedConfig(client.ID)

	desiredJSON := json.RawMessage("null")
	if len(cfg.ConfigProto) > 0 {
		parsed := &wingsvpb.Config{}
		if err := proto.Unmarshal(cfg.ConfigProto, parsed); err == nil {
			parsed.Guardian = nil
			if data, err := protoToJSON(parsed); err == nil {
				desiredJSON = data
			}
		}
	}
	reportedJSON := json.RawMessage("null")
	if len(reportedBytes) > 0 {
		parsed := &wingsvpb.Config{}
		if err := proto.Unmarshal(reportedBytes, parsed); err == nil {
			parsed.Guardian = nil
			if data, err := protoToJSON(parsed); err == nil {
				reportedJSON = data
			}
		}
	}
	runtimeJSON := json.RawMessage("null")
	if len(runtimeBytes) > 0 {
		parsed := &guardianpb.RuntimeState{}
		if err := proto.Unmarshal(runtimeBytes, parsed); err == nil {
			if data, err := protoToJSON(parsed); err == nil {
				runtimeJSON = data
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"client":                  toClientView(client),
		"desired_revision":        cfg.Revision,
		"desired_updated":         tsRFC(cfg.UpdatedAt),
		"desired_config":          desiredJSON,
		"reported_config":         reportedJSON,
		"reported_config_updated": tsRFC(reportedAt),
		"runtime":                 runtimeJSON,
		"runtime_updated":         tsRFC(runtimeAt),
	})
}

func tsRFC(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func (h *Handler) respondDeleteClient(w http.ResponseWriter, client storage.Client) {
	if sink := h.hub.ClientSink(client.ID); sink != nil {
		_ = sink.SendFrame(&guardianpb.Frame{
			Payload: &guardianpb.Frame_Error{
				Error: &guardianpb.ServerError{Code: "revoked", Message: "client deleted by admin"},
			},
		})
		sink.Close("revoked")
	}
	if err := h.store.DeleteClient(client.ID, client.OwnerAdminID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handler) respondClientConfig(w http.ResponseWriter, clientID string) {
	cfg, err := h.store.GetClientConfig(clientID)
	if err != nil {
		writeError(w, http.StatusNotFound, "no config")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"config_proto_b64": base64.StdEncoding.EncodeToString(cfg.ConfigProto),
		"revision":         cfg.Revision,
		"updated_at":       cfg.UpdatedAt.Format(time.RFC3339),
	})
}

type pushConfigRequest struct {
	Config   json.RawMessage `json:"config"`
	Revision string          `json:"revision"`
}

func (h *Handler) respondPushClientConfig(w http.ResponseWriter, r *http.Request, client storage.Client) {
	var req pushConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	parsed := &wingsvpb.Config{}
	if err := jsonToProto(req.Config, parsed); err != nil {
		writeError(w, http.StatusBadRequest, "invalid config json: "+err.Error())
		return
	}
	// Never let an admin push a Guardian credential block to a client; the
	// panel rebinding mid-session would brick remote management.
	parsed.Guardian = nil
	if !client.HasRootAccess {
		stripRootOnlyBlocks(parsed)
	}
	bytesProto, err := proto.Marshal(parsed)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	revision := strings.TrimSpace(req.Revision)
	if revision == "" {
		revision = strconv.FormatInt(time.Now().UnixMilli(), 10)
	}
	configVersion, err := h.store.UpsertClientConfig(client.ID, bytesProto, revision)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Stamp ConfigVersion in the outgoing proto so device persists the same
	// version locally and reports it back in next ClientHello.
	parsed.ConfigVersion = configVersion
	online := h.hub.ClientSink(client.ID) != nil
	if sink := h.hub.ClientSink(client.ID); sink != nil {
		_ = sink.SendFrame(&guardianpb.Frame{
			Payload: &guardianpb.Frame_ConfigPush{
				ConfigPush: &guardianpb.ConfigPush{Config: parsed, Revision: revision},
			},
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "revision": revision, "online": online, "config_version": configVersion})
}

type logControlRequest struct {
	Runtime bool `json:"runtime"`
	Proxy   bool `json:"proxy"`
	XRay    bool `json:"xray"`
}

func (h *Handler) respondLogControl(w http.ResponseWriter, r *http.Request, client storage.Client) {
	var req logControlRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := h.store.UpdateClientLogControl(client.ID, client.OwnerAdminID, req.Runtime, req.Proxy, req.XRay); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if sink := h.hub.ClientSink(client.ID); sink != nil {
		_ = sink.SendFrame(&guardianpb.Frame{
			Payload: &guardianpb.Frame_LogControl{
				LogControl: &guardianpb.LogControl{
					RuntimeEnabled: req.Runtime,
					ProxyEnabled:   req.Proxy,
					XrayEnabled:    req.XRay,
				},
			},
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

type commandRequest struct {
	Type string `json:"type"`
}

func (h *Handler) respondCommand(w http.ResponseWriter, r *http.Request, client storage.Client) {
	var req commandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	cmdType, ok := parseCommandType(req.Type)
	if !ok {
		writeError(w, http.StatusBadRequest, "unknown command type")
		return
	}
	sink := h.hub.ClientSink(client.ID)
	if sink == nil {
		writeError(w, http.StatusServiceUnavailable, "client offline")
		return
	}
	id, err := auth.GenerateClientID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := sink.SendFrame(&guardianpb.Frame{
		Payload: &guardianpb.Frame_Command{
			Command: &guardianpb.Command{Type: cmdType, Id: id},
		},
	}); err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"id": id})
}

func parseCommandType(name string) (guardianpb.CommandType, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "start_tunnel", "start":
		return guardianpb.CommandType_COMMAND_TYPE_START_TUNNEL, true
	case "stop_tunnel", "stop":
		return guardianpb.CommandType_COMMAND_TYPE_STOP_TUNNEL, true
	case "reconnect":
		return guardianpb.CommandType_COMMAND_TYPE_RECONNECT, true
	case "report_now", "report":
		return guardianpb.CommandType_COMMAND_TYPE_REPORT_NOW, true
	case "refresh_subscription":
		return guardianpb.CommandType_COMMAND_TYPE_REFRESH_SUBSCRIPTION, true
	case "refresh_all_subscriptions":
		return guardianpb.CommandType_COMMAND_TYPE_REFRESH_ALL_SUBSCRIPTIONS, true
	}
	return 0, false
}

type refreshSubscriptionRequest struct {
	SubscriptionID string `json:"subscription_id"`
}

func (h *Handler) respondRefreshSubscription(w http.ResponseWriter, r *http.Request, client storage.Client) {
	var req refreshSubscriptionRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	sink := h.hub.ClientSink(client.ID)
	if sink == nil {
		writeError(w, http.StatusServiceUnavailable, "client offline")
		return
	}
	id, err := auth.GenerateClientID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	cmdType := guardianpb.CommandType_COMMAND_TYPE_REFRESH_ALL_SUBSCRIPTIONS
	if strings.TrimSpace(req.SubscriptionID) != "" {
		cmdType = guardianpb.CommandType_COMMAND_TYPE_REFRESH_SUBSCRIPTION
	}
	if err := sink.SendFrame(&guardianpb.Frame{
		Payload: &guardianpb.Frame_Command{
			Command: &guardianpb.Command{Type: cmdType, Id: id, SubscriptionId: req.SubscriptionID},
		},
	}); err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"id": id})
}

func parseLogStream(name string) (guardianpb.LogStream, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "runtime":
		return guardianpb.LogStream_LOG_STREAM_RUNTIME, true
	case "proxy":
		return guardianpb.LogStream_LOG_STREAM_PROXY, true
	case "xray":
		return guardianpb.LogStream_LOG_STREAM_XRAY, true
	}
	return 0, false
}

func (h *Handler) respondLogs(w http.ResponseWriter, r *http.Request, client storage.Client) {
	streamParam := r.URL.Query().Get("stream")
	stream, ok := parseLogStream(streamParam)
	if !ok {
		writeError(w, http.StatusBadRequest, "stream param required: runtime|proxy|xray")
		return
	}
	since, _ := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	lines, err := h.store.ReadClientLogs(client.ID, int32(stream), since, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]map[string]any, 0, len(lines))
	for _, l := range lines {
		out = append(out, map[string]any{"seq": l.Seq, "ts": l.TS.Format(time.RFC3339Nano), "text": l.Text})
	}
	writeJSON(w, http.StatusOK, map[string]any{"lines": out})
}

type syncRequest struct {
	SyncMode                string `json:"sync_mode"`
	PeriodicIntervalMinutes int    `json:"periodic_interval_minutes"`
}

func (h *Handler) respondSync(w http.ResponseWriter, r *http.Request, client storage.Client, admin storage.Admin) {
	var req syncRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	mode := normalizeSyncMode(req.SyncMode)
	periodic := req.PeriodicIntervalMinutes
	if periodic <= 0 {
		periodic = 30
	}
	if err := h.store.UpdateClientSync(client.ID, client.OwnerAdminID, mode, periodic); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Push the new sync mode to the device live. Carries only sync_mode +
	// interval inside Guardian (no credentials) — applyConfigPush keeps the
	// credential fields untouched and just re-applies the runner.
	if sink := h.hub.ClientSink(client.ID); sink != nil {
		_ = sink.SendFrame(&guardianpb.Frame{
			Payload: &guardianpb.Frame_ConfigPush{
				ConfigPush: &guardianpb.ConfigPush{
					Config: &wingsvpb.Config{
						Ver: 1,
						Guardian: &wingsvpb.Guardian{
							SyncMode:                syncModeToProto(mode),
							PeriodicIntervalMinutes: uint32(periodic),
						},
					},
					Revision: "sync",
				},
			},
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"sync_mode": mode, "periodic_interval_minutes": periodic})
}

func (h *Handler) respondInstalledApps(w http.ResponseWriter, client storage.Client) {
	b, updatedAt, err := h.store.GetClientInstalledApps(client.ID)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"apps": []any{}, "updated_at": ""})
		return
	}
	parsed := &guardianpb.InstalledApps{}
	if err := proto.Unmarshal(b, parsed); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]map[string]any, 0, len(parsed.GetApps()))
	for _, app := range parsed.GetApps() {
		entry := map[string]any{
			"package":     app.GetPackageName(),
			"label":       app.GetLabel(),
			"system":      app.GetSystem(),
			"recommended": app.GetRecommended(),
		}
		if len(app.GetIconPng()) > 0 {
			entry["icon"] = "data:image/png;base64," + base64.StdEncoding.EncodeToString(app.GetIconPng())
		}
		out = append(out, entry)
	}
	writeJSON(w, http.StatusOK, map[string]any{"apps": out, "updated_at": updatedAt.Format(time.RFC3339Nano)})
}

func (h *Handler) respondRefreshInstalledApps(w http.ResponseWriter, client storage.Client) {
	sink := h.hub.ClientSink(client.ID)
	if sink == nil {
		writeError(w, http.StatusServiceUnavailable, "client offline")
		return
	}
	id, err := auth.GenerateClientID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := sink.SendFrame(&guardianpb.Frame{
		Payload: &guardianpb.Frame_Command{
			Command: &guardianpb.Command{Type: guardianpb.CommandType_COMMAND_TYPE_REFRESH_INSTALLED_APPS, Id: id},
		},
	}); err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"id": id})
}
