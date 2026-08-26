package admin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"v.wingsnet.org/internal/auth"
	guardianpb "v.wingsnet.org/internal/gen/guardianpb"
	wingsvpb "v.wingsnet.org/internal/gen/wingsvpb"
	"v.wingsnet.org/internal/preview"
	"v.wingsnet.org/internal/relayclient"
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
	VkOAuthAuthorized       bool   `json:"vk_oauth_authorized"`
	RemoteControl           bool   `json:"remote_control"`
	Provisioned             bool   `json:"provisioned"`
	Managed                 bool   `json:"managed"`
	TrafficRx               uint64 `json:"traffic_rx"`
	TrafficTx               uint64 `json:"traffic_tx"`
	// Managed-client traffic limit / enable-disable state (0 limit = unlimited).
	Disabled          bool   `json:"disabled"`
	TrafficLimitBytes uint64 `json:"traffic_limit_bytes"`
	TrafficUsedBytes  uint64 `json:"traffic_used_bytes"`
	ResetPeriodDays   int64  `json:"reset_period_days"`
	NextResetUnix     int64  `json:"next_reset_unix"`
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
		VkOAuthAuthorized:       c.VkOAuthAuthorized,
		RemoteControl:           c.RemoteControl,
	}
}

// backendTypeLabel maps the proto enum to a short user-facing label.
func backendTypeLabel(t wingsvpb.BackendType) string {
	switch t {
	case wingsvpb.BackendType_BACKEND_TYPE_VK_TURN_WIREGUARD, wingsvpb.BackendType_BACKEND_TYPE_VK_TURN:
		return "VK TURN + WG"
	case wingsvpb.BackendType_BACKEND_TYPE_XRAY:
		return "Xray"
	case wingsvpb.BackendType_BACKEND_TYPE_AMNEZIAWG:
		return "VK TURN + AWG"
	case wingsvpb.BackendType_BACKEND_TYPE_WIREGUARD:
		return "WireGuard"
	case wingsvpb.BackendType_BACKEND_TYPE_AMNEZIAWG_PLAIN, wingsvpb.BackendType_BACKEND_TYPE_AMNEZIAWG_TL:
		return "AmneziaWG"
	case wingsvpb.BackendType_BACKEND_TYPE_WB_STREAM:
		return "WB Stream"
	}
	return ""
}

// backendTypeLabelForConfig refines the VK TURN label by Turn.tunnel_mode, since
// the new BACKEND_TYPE_VK_TURN carries the WG/AWG choice in Turn rather than in
// the backend enum.
func backendTypeLabelForConfig(cfg *wingsvpb.Config) string {
	b := cfg.GetBackend()
	if (b == wingsvpb.BackendType_BACKEND_TYPE_VK_TURN ||
		b == wingsvpb.BackendType_BACKEND_TYPE_VK_TURN_WIREGUARD) &&
		cfg.GetTurn().GetTunnelMode() == wingsvpb.TunnelMode_TUNNEL_MODE_AMNEZIAWG {
		return "VK TURN + AWG"
	}
	return backendTypeLabel(b)
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
	view.BackendType = backendTypeLabelForConfig(parsed)
	// Managed reflects the config carrying a panel-managed VK-TURN profile, so the
	// UI can offer the traffic-limit / disable controls even while the client is
	// disabled and has no live peer (thus provisioned=false).
	view.Managed = hasManagedTurnProfile(parsed.Turn)
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
		traffic, _ := h.store.ClientTrafficMap()
		provisioned, _ := h.store.ProvisionedClientIDs()
		control, _ := h.store.ClientControlMap(admin.ID)
		out := make([]clientView, 0, len(clients))
		for _, c := range clients {
			view := toClientView(c)
			h.hydrateBackendType(&view, c.ID)
			view.Provisioned = provisioned[c.ID]
			if t, ok := traffic[c.ID]; ok {
				view.TrafficRx = t[0]
				view.TrafficTx = t[1]
			}
			if ctrl, ok := control[c.ID]; ok {
				view.Disabled = ctrl.Disabled
				view.TrafficLimitBytes = ctrl.LimitBytes
				view.TrafficUsedBytes = ctrl.UsedBytes
				view.ResetPeriodDays = ctrl.ResetPeriodDays
				view.NextResetUnix = ctrl.NextResetUnix
			}
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
	// RemoteControl selects the enrollment-link shape: nil/true embeds the
	// Guardian block (WSS remote control); false emits a plain VK-TURN-profile
	// link. Both carry a managed VK-TURN profile when a vk-turn endpoint is set.
	RemoteControl *bool `json:"remote_control"`
	// VkTurnNodeID picks which registered vk-turn relay the managed profile points
	// at; the endpoint is derived from that node's host. Empty falls back to the
	// configured VK_TURN_ENDPOINT.
	VkTurnNodeID string `json:"vk_turn_node_id"`
	// TrafficLimitGB caps the managed client's traffic (GiB; 0 = unlimited);
	// ResetPeriodDays > 0 arms a periodic reset. Applied only to a managed client.
	TrafficLimitGB  float64 `json:"traffic_limit_gb"`
	ResetPeriodDays int64   `json:"reset_period_days"`
}

// vkTurnDefaultDTLSPort is the port a vk-turn relay listens on for the app's DTLS
// data plane (the relay -listen default). The panel pairs it with the node's host
// to form the managed profile endpoint.
const vkTurnDefaultDTLSPort = "56000"

// resolveVkTurnEndpoint returns the DTLS endpoint the app dials for the managed
// VK-TURN profile: the host from the selected node's gRPC endpoint paired with the
// relay DTLS port. An empty nodeID falls back to the configured VK_TURN_ENDPOINT.
func (h *Handler) resolveVkTurnEndpoint(admin storage.Admin, nodeID string) (string, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		if ep := strings.TrimSpace(h.cfg.VkTurnEndpoint); ep != "" {
			return ep, nil
		}
		// No explicit node and no configured VK_TURN_ENDPOINT: fall back to a
		// vk-turn relay the panel manages (the installer registers one on
		// 127.0.0.1), so a single-host install can issue profile links - the
		// link/show path passes no node - without a manual VK_TURN_ENDPOINT.
		nodeID = h.defaultVkTurnNodeID(admin)
		if nodeID == "" {
			return "", nil
		}
	}
	node, err := h.store.GetServerNode(nodeID)
	if err != nil {
		return "", errors.New("unknown vk-turn node")
	}
	if node.Kind != storage.ServerNodeVKTurnProxy {
		return "", errors.New("selected node is not a vk-turn relay")
	}
	if node.OwnerAdminID != admin.ID && !(auth.IsOwner(admin) && node.OwnerAdminID == 0) {
		return "", errors.New("node not available")
	}
	host := node.GRPCEndpoint
	if parsedHost, _, splitErr := net.SplitHostPort(node.GRPCEndpoint); splitErr == nil {
		host = parsedHost
	}
	// A loopback management endpoint means the panel and relay are co-located (the
	// common single-host install, where the installer registers the node on
	// 127.0.0.1). That host is useless to remote clients, so substitute the
	// panel's own public host (from PUBLIC_BASE_URL) - the server's external
	// address - so the profile link carries a reachable DTLS endpoint.
	if isLoopbackHost(host) {
		if pub := publicHostFromBaseURL(h.cfg.PublicBaseURL); pub != "" {
			host = pub
		}
	}
	// Prefer the relay's own DTLS listen port (reported over gRPC); fall back to the
	// convention default when the node is unreachable or does not report it.
	port := vkTurnDefaultDTLSPort
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	if st, sErr := relayclient.New(node.GRPCToken).NodeStatus(ctx, node); sErr == nil {
		if _, reportedPort, pErr := net.SplitHostPort(st.ListenEndpoint); pErr == nil && reportedPort != "" {
			port = reportedPort
		}
	}
	return net.JoinHostPort(host, port), nil
}

// isLoopbackHost reports whether host is a loopback address or "localhost".
func isLoopbackHost(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	if h == "localhost" {
		return true
	}
	if ip := net.ParseIP(h); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// defaultVkTurnNodeID picks a vk-turn relay to use when the caller did not name
// one and no VK_TURN_ENDPOINT is configured. It prefers a panel-local node
// (owner_admin_id 0, usable by the owner) and then the admin's own node, so the
// common single-host install "just works". Empty if none is available.
func (h *Handler) defaultVkTurnNodeID(admin storage.Admin) string {
	owners := []int64{admin.ID}
	if auth.IsOwner(admin) {
		owners = []int64{0, admin.ID}
	}
	for _, owner := range owners {
		nodes, err := h.store.ListServerNodesByOwner(storage.ServerNodeVKTurnProxy, owner)
		if err == nil && len(nodes) > 0 {
			return nodes[0].ID
		}
	}
	return ""
}

// publicHostFromBaseURL extracts the bare host (no scheme, no port) from a
// PUBLIC_BASE_URL like "https://1.2.3.4:8443" -> "1.2.3.4".
func publicHostFromBaseURL(base string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		return ""
	}
	if !strings.Contains(base, "://") {
		base = "https://" + base
	}
	if u, err := url.Parse(base); err == nil {
		return u.Hostname()
	}
	return ""
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

	vkTurnEndpoint, err := h.resolveVkTurnEndpoint(admin, req.VkTurnNodeID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// The stored config is the panel's authority: it is what config-on-connect
	// pushes to the device. It MUST carry the same managed profile as the link,
	// or the first Guardian sync would push a managed-less config and strip the
	// client's provisioning (wgProvisioned) right after enrollment.
	if managed := h.managedTurn(clientID, name, tokenBytes, vkTurnEndpoint); managed != nil {
		seedConfig.Turn = managed
		markVkTurnBackend(seedConfig)
		// Stored config is the WS source of truth: keep the full VK-links pool so
		// config-on-connect pushes the rest to the device.
		h.applyAdminVKLinks(seedConfig.Turn, admin.ID, 0)
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

	// A traffic cap only means anything for a managed VK-TURN client (one with a
	// panel-issued self-provisioning profile, i.e. a resolved endpoint).
	if strings.TrimSpace(vkTurnEndpoint) != "" && (req.TrafficLimitGB > 0 || req.ResetPeriodDays > 0) {
		limitBytes := uint64(req.TrafficLimitGB * bytesPerGB)
		period := req.ResetPeriodDays
		if period < 0 {
			period = 0
		}
		if err := h.store.SetClientTrafficLimit(clientID, admin.ID, limitBytes, period, time.Now().Unix()); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	remoteControl := req.RemoteControl == nil || *req.RemoteControl
	// Persist the management type so every regenerated link (share, token
	// rotation) keeps this client's shape instead of defaulting to Guardian.
	if err := h.store.SetClientRemoteControl(clientID, admin.ID, remoteControl); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	link, err := h.buildClientLink(clientID, name, tokenBytes, syncMode, periodic, admin, remoteControl, vkTurnEndpoint)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
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

// buildClientLink builds a client's wingsv:// enrollment link. remoteControl ON
// embeds the Guardian block (WSS remote config control); OFF emits a plain
// VK-TURN-profile link. Both carry a managed VK-TURN profile (when a vk-turn
// endpoint is configured) so the app self-provisions its wg config over DTLS.
func (h *Handler) buildClientLink(
	clientID, name string,
	token []byte,
	syncMode string,
	periodic int,
	admin storage.Admin,
	remoteControl bool,
	vkTurnEndpoint string,
) (string, error) {
	cfg := &wingsvpb.Config{Ver: 1}
	cfg.Turn = h.managedTurn(clientID, name, token, vkTurnEndpoint)
	markVkTurnBackend(cfg)
	// Enrollment link / QR: only a couple of links to bootstrap; the rest arrive
	// over the Guardian WS from the stored config.
	h.applyAdminVKLinks(cfg.Turn, admin.ID, maxEnrollmentVKLinks)
	if remoteControl {
		cfg.Type = wingsvpb.ConfigType_CONFIG_TYPE_GUARDIAN
		cfg.Guardian = &wingsvpb.Guardian{
			WsUrl:                   deriveWsURL(h.cfg.PublicBaseURL),
			ClientId:                clientID,
			ClientToken:             token,
			ClientName:              name,
			SyncMode:                syncModeToProto(syncMode),
			PeriodicIntervalMinutes: uint32(periodic),
			AdminUsername:           admin.Username,
			AdminId:                 admin.ID,
			AdminAvatarVersion:      admin.AvatarVersion,
		}
		return preview.BuildWingsLink(cfg)
	}
	if cfg.Turn == nil {
		return "", errors.New("set VK_TURN_ENDPOINT to issue a profile link without remote control")
	}
	cfg.Type = wingsvpb.ConfigType_CONFIG_TYPE_VK_TURN_PROFILE
	return preview.BuildWingsLink(cfg)
}

// maxEnrollmentVKLinks caps how many VK links the enrollment link / QR carries.
// The QR only needs a couple to bootstrap the relay; the rest of the admin's pool
// reaches a remote-controlled client over the Guardian WS (config-on-connect
// pushes the full stored config). Keeping the QR small keeps it scannable.
const maxEnrollmentVKLinks = 2

// applyAdminVKLinks embeds the admin's shared VK Links pool into turn. The app
// imports these append-only into its own shared settings.vkLinks (any profile
// import only adds links, never wipes), so no merge flag is needed. Only a
// VK-TURN turn (a managed self-provisioning profile) carries links; max <= 0
// embeds the whole pool (the stored config, the WS source of truth), max > 0
// caps it (the enrollment link / QR). No-op when the admin has no links.
func (h *Handler) applyAdminVKLinks(turn *wingsvpb.Turn, adminID int64, max int) {
	if turn == nil || h.store == nil || !hasManagedTurnProfile(turn) {
		return
	}
	links, err := h.store.GetAdminVKLinks(adminID)
	if err != nil || len(links) == 0 {
		return
	}
	merged := mergeVKLinks(turn.Links, links)
	if max > 0 && len(merged) > max {
		merged = merged[:max]
	}
	turn.Links = merged
}

// hasManagedTurnProfile reports whether turn explicitly carries a panel-managed
// VK-TURN (self-provisioning) profile - the only case where VK links belong.
func hasManagedTurnProfile(turn *wingsvpb.Turn) bool {
	if turn == nil {
		return false
	}
	for _, p := range turn.Profiles {
		if isManagedProfile(p) {
			return true
		}
	}
	return false
}

// mergeVKLinks unions existing and added links, trimming blanks and keeping
// first-seen order.
func mergeVKLinks(existing, add []string) []string {
	seen := make(map[string]struct{}, len(existing)+len(add))
	out := make([]string, 0, len(existing)+len(add))
	for _, l := range existing {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		if _, ok := seen[l]; ok {
			continue
		}
		seen[l] = struct{}{}
		out = append(out, l)
	}
	for _, l := range add {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		if _, ok := seen[l]; ok {
			continue
		}
		seen[l] = struct{}{}
		out = append(out, l)
	}
	return out
}

// managedTurn builds the panel-managed VK-TURN profile that lets the app
// self-provision its wg config: it carries the client's panel token and the
// vk-turn endpoint. Returns nil when no vk-turn endpoint is configured.
func (h *Handler) managedTurn(clientID, name string, token []byte, vkTurnEndpoint string) *wingsvpb.Turn {
	if strings.TrimSpace(vkTurnEndpoint) == "" {
		return nil
	}
	return applyManagedTurnProfile(nil, clientID, name, token, vkTurnEndpoint)
}

// isManagedProfile reports whether p is a panel-managed self-provisioning profile
// (server-issued wg config), as opposed to a manually configured one.
func isManagedProfile(p *wingsvpb.TurnProfile) bool {
	return p != nil && p.WgProvisioned
}

// markVkTurnBackend stamps the top-level backend + tunnel mode on a config that
// carries a managed VK-TURN profile. Without this the app leaves config.backend
// UNSPECIFIED, treats the import as backend-preserving (updateBackendType=false)
// and never switches to VK TURN, so it keeps connecting on the old backend and
// never runs the DTLS provisioning exchange.
func markVkTurnBackend(cfg *wingsvpb.Config) {
	if cfg == nil || cfg.Turn == nil {
		return
	}
	hasManaged := false
	for _, p := range cfg.Turn.Profiles {
		if isManagedProfile(p) {
			hasManaged = true
			break
		}
	}
	if !hasManaged {
		return
	}
	cfg.Backend = wingsvpb.BackendType_BACKEND_TYPE_VK_TURN
	if cfg.Turn.TunnelMode == wingsvpb.TunnelMode_TUNNEL_MODE_UNSPECIFIED {
		cfg.Turn.TunnelMode = wingsvpb.TunnelMode_TUNNEL_MODE_WIREGUARD
	}
}

// existingManagedEndpoint returns the vk-turn endpoint of a managed profile already
// present in turn, or "" when there is none.
func existingManagedEndpoint(turn *wingsvpb.Turn) string {
	if turn == nil {
		return ""
	}
	for _, p := range turn.Profiles {
		if isManagedProfile(p) {
			return strings.TrimSpace(p.VkTurnEndpoint)
		}
	}
	return ""
}

// stripManagedTurnProfiles removes any panel-managed profiles from turn, keeping the
// client's manually configured profiles untouched. Returns turn (possibly nil).
func stripManagedTurnProfiles(turn *wingsvpb.Turn) *wingsvpb.Turn {
	if turn == nil {
		return nil
	}
	kept := make([]*wingsvpb.TurnProfile, 0, len(turn.Profiles))
	for _, p := range turn.Profiles {
		if !isManagedProfile(p) {
			kept = append(kept, p)
		}
	}
	turn.Profiles = kept
	if len(kept) == 0 {
		// No VK-TURN profile remains: the shared VK links are orphaned, so a
		// non-provision config must not keep carrying them.
		turn.Links = nil
	}
	if turn.ActiveProfileId != "" && !hasProfile(kept, turn.ActiveProfileId) {
		if len(kept) > 0 {
			turn.ActiveProfileId = kept[0].Id
		} else {
			turn.ActiveProfileId = ""
		}
	}
	return turn
}

func hasProfile(profiles []*wingsvpb.TurnProfile, id string) bool {
	for _, p := range profiles {
		if p.Id == id {
			return true
		}
	}
	return false
}

// applyManagedTurnProfile injects or replaces the panel-managed self-provisioning
// profile in turn, preserving the client's manually configured profiles.
func applyManagedTurnProfile(turn *wingsvpb.Turn, clientID, name string, token []byte, endpoint string) *wingsvpb.Turn {
	managed := &wingsvpb.TurnProfile{
		Id:                clientID,
		Title:             name,
		TransportKind:     "wg",
		VkTurnEndpoint:    strings.TrimSpace(endpoint),
		WgProvisioned:     true,
		ProvisionClientId: clientID,
		ProvisionToken:    token,
		// Force the mu/v1 (mux) TURN session mode for panel-provisioned profiles.
		// Without an inner config the app falls back to its flat "mainline" default;
		// the inner Turn carries only the session mode, the rest stays app-default.
		Config: &wingsvpb.Turn{SessionMode: wingsvpb.TurnSessionMode_TURN_SESSION_MODE_MUX},
	}
	if turn == nil {
		turn = &wingsvpb.Turn{}
	}
	turn = stripManagedTurnProfiles(turn)
	turn.Profiles = append(turn.Profiles, managed)
	if turn.ActiveProfileId == "" {
		turn.ActiveProfileId = managed.Id
	}
	return turn
}

// applyProvisionToConfig injects or strips the panel-managed self-provisioning
// VK-TURN profile on a config being saved for client. The client token and the
// resolved endpoint are filled server-side so they never round-trip through the
// browser.
func (h *Handler) applyProvisionToConfig(admin storage.Admin, client storage.Client, cfg *wingsvpb.Config, enable bool, vkTurnNodeID string) error {
	if !enable {
		cfg.Turn = stripManagedTurnProfiles(cfg.Turn)
		return nil
	}
	token, err := h.store.GetClientToken(client.ID, client.OwnerAdminID)
	if err != nil {
		return errors.New("client token unavailable; recreate the client to enable provisioning")
	}
	endpoint := existingManagedEndpoint(cfg.Turn)
	if strings.TrimSpace(vkTurnNodeID) != "" || endpoint == "" {
		resolved, rErr := h.resolveVkTurnEndpoint(admin, vkTurnNodeID)
		if rErr != nil {
			return rErr
		}
		if strings.TrimSpace(resolved) != "" {
			endpoint = resolved
		}
	}
	if strings.TrimSpace(endpoint) == "" {
		return errors.New("no vk-turn endpoint: select a vk-turn server")
	}
	cfg.Turn = applyManagedTurnProfile(cfg.Turn, client.ID, client.Name, token, endpoint)
	markVkTurnBackend(cfg)
	return nil
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
		h.respondPushClientConfig(w, r, admin, client)
	case subpath == "management" && r.Method == http.MethodPut:
		h.respondSetManagement(w, r, client)
	case subpath == "traffic-limit" && r.Method == http.MethodPut:
		h.respondSetTrafficLimit(w, r, client)
	case subpath == "disabled" && r.Method == http.MethodPut:
		h.respondSetDisabled(w, r, client)
	case subpath == "traffic-reset" && r.Method == http.MethodPost:
		h.respondResetTraffic(w, client)
	case subpath == "log/control" && r.Method == http.MethodPut:
		h.respondLogControl(w, r, client)
	case subpath == "sync" && r.Method == http.MethodPut:
		h.respondSync(w, r, client, admin)
	case subpath == "command" && r.Method == http.MethodPost:
		h.respondCommand(w, r, client)
	case subpath == "link/wingsv" && r.Method == http.MethodGet:
		h.respondWingsvLink(w, r, client, admin)
	case strings.HasPrefix(subpath, "logs") && r.Method == http.MethodGet:
		h.respondLogs(w, r, client)
	case subpath == "token/rotate" && r.Method == http.MethodPost:
		h.respondRotateToken(w, r, client, admin)
	case subpath == "subscription/refresh" && r.Method == http.MethodPost:
		h.respondRefreshSubscription(w, r, client)
	case subpath == "apps/installed" && r.Method == http.MethodGet:
		h.respondInstalledApps(w, client)
	case subpath == "apps/refresh" && r.Method == http.MethodPost:
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

func (h *Handler) respondWingsvLink(w http.ResponseWriter, r *http.Request, client storage.Client, admin storage.Admin) {
	// admin зарезервирован для будущего audit-логирования при owner-доступе;
	// для query-к store достаточно owner_admin_id самого клиента.
	_ = admin
	tokenBytes, err := h.store.GetClientToken(client.ID, client.OwnerAdminID)
	if err != nil {
		writeError(w, http.StatusNotFound, "token not available — recreate the client to regenerate")
		return
	}
	// Default to the client's stored management type; a query param can still
	// override it (remote=0 forces a config-only link, remote=1 forces Guardian).
	remoteControl := client.RemoteControl
	switch r.URL.Query().Get("remote") {
	case "0":
		remoteControl = false
	case "1":
		remoteControl = true
	}
	vkTurnEndpoint, err := h.resolveVkTurnEndpoint(admin, r.URL.Query().Get("vk_turn_node"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	link, err := h.buildClientLink(
		client.ID, client.Name, tokenBytes, client.SyncMode, client.PeriodicIntervalMinutes,
		admin, remoteControl, vkTurnEndpoint,
	)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
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
	// Rebuild the link in the client's own shape (Guardian vs config-only),
	// carrying its managed vk-turn endpoint so a rotated config-only client keeps
	// self-provisioning instead of silently switching to panel control.
	vkTurnEndpoint := ""
	if cfg, cErr := h.store.GetClientConfig(client.ID); cErr == nil && len(cfg.ConfigProto) > 0 {
		parsed := &wingsvpb.Config{}
		if proto.Unmarshal(cfg.ConfigProto, parsed) == nil {
			vkTurnEndpoint = existingManagedEndpoint(parsed.Turn)
		}
	}
	if vkTurnEndpoint == "" {
		vkTurnEndpoint, _ = h.resolveVkTurnEndpoint(admin, "")
	}
	link, err := h.buildClientLink(
		client.ID, client.Name, tokenBytes, client.SyncMode, client.PeriodicIntervalMinutes,
		admin, client.RemoteControl, vkTurnEndpoint,
	)
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
	h.hub.SendToClient(client.ID, &guardianpb.Frame{
		Payload: &guardianpb.Frame_Error{
			Error: &guardianpb.ServerError{Code: "revoked", Message: "client deleted by admin"},
		},
	}, client.Online)
	if sink := h.hub.ClientSink(client.ID); sink != nil {
		sink.Close("revoked")
	}
	if err := h.store.DeleteClient(client.ID, client.OwnerAdminID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

type setManagementRequest struct {
	RemoteControl bool `json:"remote_control"`
}

// respondSetManagement persists the client's management type: full remote control
// versus config-only. It changes the enrollment-link shape (Guardian block present
// or not) and which sections the UI exposes; the stored config's managed VK-TURN
// profile is untouched. Keyed by the client's owner so an owner can retype any
// admin's client.
func (h *Handler) respondSetManagement(w http.ResponseWriter, r *http.Request, client storage.Client) {
	var req setManagementRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := h.store.SetClientRemoteControl(client.ID, client.OwnerAdminID, req.RemoteControl); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"remote_control": req.RemoteControl})
}

const bytesPerGB = 1024 * 1024 * 1024

type setTrafficLimitRequest struct {
	// LimitGB is the cap in gibibytes (base 1024, matching the UI's byte
	// formatting); 0 clears the cap. ResetPeriodDays > 0 arms a periodic reset.
	LimitGB         float64 `json:"limit_gb"`
	ResetPeriodDays int64   `json:"reset_period_days"`
}

// respondSetTrafficLimit sets a managed client's traffic cap and optional
// periodic-reset window. Enforced by the collector and the provisioning gate.
func (h *Handler) respondSetTrafficLimit(w http.ResponseWriter, r *http.Request, client storage.Client) {
	var req setTrafficLimitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.LimitGB < 0 || req.ResetPeriodDays < 0 {
		writeError(w, http.StatusBadRequest, "negative value")
		return
	}
	limitBytes := uint64(req.LimitGB * bytesPerGB)
	if err := h.store.SetClientTrafficLimit(client.ID, client.OwnerAdminID, limitBytes, req.ResetPeriodDays, time.Now().Unix()); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"traffic_limit_bytes": limitBytes,
		"reset_period_days":   req.ResetPeriodDays,
	})
}

type setDisabledRequest struct {
	Disabled bool `json:"disabled"`
}

// respondSetDisabled flips a managed client's manual cutoff. The collector tears
// down its live peer on the next poll; the provisioning gate refuses re-enroll
// immediately.
func (h *Handler) respondSetDisabled(w http.ResponseWriter, r *http.Request, client storage.Client) {
	var req setDisabledRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := h.store.SetClientDisabled(client.ID, client.OwnerAdminID, req.Disabled); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"disabled": req.Disabled})
}

// respondResetTraffic clears a managed client's accumulated usage now, restoring
// service if it was cut off by the cap.
func (h *Handler) respondResetTraffic(w http.ResponseWriter, client storage.Client) {
	if err := h.store.ResetClientTraffic(client.ID, client.OwnerAdminID, time.Now().Unix()); err != nil {
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
	// Provision toggles the panel-managed self-provisioning VK-TURN profile. When
	// non-nil it is applied server-side (the app never sees the client token):
	// true injects/refreshes the managed profile, false strips it. Nil leaves the
	// config's managed profile as-is.
	Provision *bool `json:"provision"`
	// VkTurnNodeID picks which registered vk-turn relay the managed profile points
	// at. Empty keeps the endpoint the config already carries.
	VkTurnNodeID string `json:"vk_turn_node_id"`
}

func (h *Handler) respondPushClientConfig(w http.ResponseWriter, r *http.Request, admin storage.Admin, client storage.Client) {
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
	if req.Provision != nil {
		if err := h.applyProvisionToConfig(admin, client, parsed, *req.Provision, req.VkTurnNodeID); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
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
	h.hub.SendToClient(client.ID, &guardianpb.Frame{
		Payload: &guardianpb.Frame_ConfigPush{
			ConfigPush: &guardianpb.ConfigPush{Config: parsed, Revision: revision},
		},
	}, client.Online)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "revision": revision, "online": client.Online, "config_version": configVersion})
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
	h.hub.SendToClient(client.ID, &guardianpb.Frame{
		Payload: &guardianpb.Frame_LogControl{
			LogControl: &guardianpb.LogControl{
				RuntimeEnabled: req.Runtime,
				ProxyEnabled:   req.Proxy,
				XrayEnabled:    req.XRay,
			},
		},
	}, client.Online)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

type commandRequest struct {
	Type string `json:"type"`
	// Only used for generate_vk_link when the client is offline: admin can
	// enqueue several link generations to fire when the device reconnects.
	// Defaults to 1. Capped at 50 to keep the queue sane.
	Count int `json:"count,omitempty"`
}

const maxQueuedGenerateVkLinkCount = 50

// queueableCommands lists command types that survive an offline client and get
// replayed at the next welcome. Fire-and-forget commands (start/stop/reconnect/
// report_now) stay 503 when offline — running them an hour later would surprise
// the user.
var queueableCommands = map[guardianpb.CommandType]bool{
	guardianpb.CommandType_COMMAND_TYPE_REFRESH_SUBSCRIPTION:      true,
	guardianpb.CommandType_COMMAND_TYPE_REFRESH_ALL_SUBSCRIPTIONS: true,
	guardianpb.CommandType_COMMAND_TYPE_REFRESH_INSTALLED_APPS:    true,
	guardianpb.CommandType_COMMAND_TYPE_GENERATE_VK_LINK:          true,
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
	id, err := auth.GenerateClientID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	frame := &guardianpb.Frame{
		Payload: &guardianpb.Frame_Command{
			Command: &guardianpb.Command{Type: cmdType, Id: id},
		},
	}
	if h.hub.SendToClient(client.ID, frame, client.Online) {
		writeJSON(w, http.StatusAccepted, map[string]any{"id": id})
		return
	}
	if !queueableCommands[cmdType] {
		writeError(w, http.StatusServiceUnavailable, "client offline")
		return
	}
	queued, err := h.enqueueOfflineCommand(client.ID, cmdType, "", req.Count)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"queued": true, "queued_count": queued})
}

// enqueueOfflineCommand persists a queueable command (or several) for an
// offline client. For generate_vk_link the admin may pass a count > 1 so a
// burst lands when the device reconnects. The refresh_* commands are
// idempotent so we dedup them to one pending row.
func (h *Handler) enqueueOfflineCommand(clientID string, cmdType guardianpb.CommandType, subscriptionID string, count int) (int, error) {
	switch cmdType {
	case guardianpb.CommandType_COMMAND_TYPE_GENERATE_VK_LINK:
		if count <= 0 {
			count = 1
		}
		if count > maxQueuedGenerateVkLinkCount {
			count = maxQueuedGenerateVkLinkCount
		}
		queued := 0
		for i := 0; i < count; i++ {
			if _, err := h.store.EnqueuePendingCommand(clientID, int32(cmdType), subscriptionID); err != nil {
				return queued, err
			}
			queued++
		}
		return queued, nil
	case
		guardianpb.CommandType_COMMAND_TYPE_REFRESH_SUBSCRIPTION,
		guardianpb.CommandType_COMMAND_TYPE_REFRESH_ALL_SUBSCRIPTIONS,
		guardianpb.CommandType_COMMAND_TYPE_REFRESH_INSTALLED_APPS:
		_, inserted, err := h.store.EnqueuePendingCommandDedup(clientID, int32(cmdType), subscriptionID)
		if err != nil {
			return 0, err
		}
		if inserted {
			return 1, nil
		}
		return 0, nil
	default:
		return 0, nil
	}
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
	case "refresh_installed_apps":
		return guardianpb.CommandType_COMMAND_TYPE_REFRESH_INSTALLED_APPS, true
	case "generate_vk_link":
		return guardianpb.CommandType_COMMAND_TYPE_GENERATE_VK_LINK, true
	}
	return 0, false
}

type refreshSubscriptionRequest struct {
	SubscriptionID string `json:"subscription_id"`
}

func (h *Handler) respondRefreshSubscription(w http.ResponseWriter, r *http.Request, client storage.Client) {
	var req refreshSubscriptionRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	cmdType := guardianpb.CommandType_COMMAND_TYPE_REFRESH_ALL_SUBSCRIPTIONS
	subID := strings.TrimSpace(req.SubscriptionID)
	if subID != "" {
		cmdType = guardianpb.CommandType_COMMAND_TYPE_REFRESH_SUBSCRIPTION
	}
	id, err := auth.GenerateClientID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	frame := &guardianpb.Frame{
		Payload: &guardianpb.Frame_Command{
			Command: &guardianpb.Command{Type: cmdType, Id: id, SubscriptionId: subID},
		},
	}
	if h.hub.SendToClient(client.ID, frame, client.Online) {
		writeJSON(w, http.StatusAccepted, map[string]any{"id": id})
		return
	}
	queued, err := h.enqueueOfflineCommand(client.ID, cmdType, subID, 0)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"queued": true, "queued_count": queued})
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
	h.hub.SendToClient(client.ID, &guardianpb.Frame{
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
	}, client.Online)
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
	id, err := auth.GenerateClientID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	frame := &guardianpb.Frame{
		Payload: &guardianpb.Frame_Command{
			Command: &guardianpb.Command{Type: guardianpb.CommandType_COMMAND_TYPE_REFRESH_INSTALLED_APPS, Id: id},
		},
	}
	if !h.hub.SendToClient(client.ID, frame, client.Online) {
		writeError(w, http.StatusServiceUnavailable, "client offline")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"id": id})
}
