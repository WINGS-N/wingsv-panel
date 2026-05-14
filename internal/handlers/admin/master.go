package admin

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	"v.wingsnet.org/internal/auth"
	guardianpb "v.wingsnet.org/internal/gen/guardianpb"
	wingsvpb "v.wingsnet.org/internal/gen/wingsvpb"
	"v.wingsnet.org/internal/preview"
	"v.wingsnet.org/internal/storage"
)

// Allowed scope-flag tokens stored in admin_master_config.scope_flags.
// Each one corresponds to a top-level Config section that can be bulk-applied
// across the admin's clients without touching their per-client identity
// fields (Xray profiles, WireGuard private keys, WB Stream room ids, etc.).
const (
	scopeTurn           = "turn"
	scopeXraySettings   = "xray_settings"
	scopeXrayRouting    = "xray_routing"
	scopeByeDpi         = "byedpi"
	scopeAppPreferences = "app_preferences"
	scopeAppRouting     = "app_routing"
	scopeSync           = "sync"
)

func (h *Handler) handleMasterConfig(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	switch r.Method {
	case http.MethodGet:
		h.respondMasterConfigGet(w, admin)
	case http.MethodPut:
		h.respondMasterConfigPut(w, r, admin)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *Handler) respondMasterConfigGet(w http.ResponseWriter, admin storage.Admin) {
	m, err := h.store.GetMasterConfig(admin.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	cfgJSON := json.RawMessage("null")
	if len(m.ConfigProto) > 0 {
		parsed := &wingsvpb.Config{}
		if err := proto.Unmarshal(m.ConfigProto, parsed); err == nil {
			parsed.Guardian = nil
			if raw, err := protoToJSON(parsed); err == nil {
				cfgJSON = raw
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"config":                    cfgJSON,
		"sync_mode":                 m.SyncMode,
		"periodic_interval_minutes": m.PeriodicIntervalMinutes,
		"scope_flags":               normalizeScopeFlags(m.ScopeFlags),
		"updated_at":                formatTSStr(m.UpdatedAt),
	})
}

type masterConfigPutRequest struct {
	Config                  json.RawMessage `json:"config"`
	SyncMode                string          `json:"sync_mode"`
	PeriodicIntervalMinutes int             `json:"periodic_interval_minutes"`
	ScopeFlags              []string        `json:"scope_flags"`
}

func (h *Handler) respondMasterConfigPut(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	var req masterConfigPutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	cfgBytes, err := masterConfigBytesFromJSON(req.Config)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	flags := normalizeScopeFlagsList(req.ScopeFlags)
	periodic := req.PeriodicIntervalMinutes
	if periodic < 0 {
		periodic = 0
	}
	mode := normalizeSyncMode(req.SyncMode)
	if req.SyncMode == "" {
		mode = ""
	}
	if err := h.store.SaveMasterConfig(storage.MasterConfig{
		AdminID:                 admin.ID,
		ConfigProto:             cfgBytes,
		SyncMode:                mode,
		PeriodicIntervalMinutes: periodic,
		ScopeFlags:              strings.Join(flags, ","),
	}); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleMasterConfigApply pushes the saved master config to every client
// owned by this admin. Only sections listed in scope_flags are touched.
func (h *Handler) handleMasterConfigApply(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	master, err := h.store.GetMasterConfig(admin.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	flags := scopeFlagSet(normalizeScopeFlags(master.ScopeFlags))
	if len(flags) == 0 {
		writeError(w, http.StatusBadRequest, "no scope flags set")
		return
	}
	masterCfg := &wingsvpb.Config{}
	if len(master.ConfigProto) > 0 {
		if err := proto.Unmarshal(master.ConfigProto, masterCfg); err != nil {
			writeError(w, http.StatusInternalServerError, "saved master config corrupted")
			return
		}
	}
	clients, err := h.store.ListClientsByOwner(admin.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	pushed := 0
	for _, client := range clients {
		desired := &wingsvpb.Config{Ver: 1}
		if cfg, err := h.store.GetClientConfig(client.ID); err == nil && len(cfg.ConfigProto) > 0 {
			merged := &wingsvpb.Config{}
			if err := proto.Unmarshal(cfg.ConfigProto, merged); err == nil {
				desired = merged
			}
		}
		mergeMasterIntoConfig(desired, masterCfg, flags)
		if !client.HasRootAccess {
			stripRootOnlyBlocks(desired)
		}
		bytes, err := proto.Marshal(desired)
		if err != nil {
			continue
		}
		configVersion, _ := h.store.UpsertClientConfig(client.ID, bytes, "master-"+strconv.FormatInt(time.Now().Unix(), 10))
		desired.ConfigVersion = configVersion
		if flags[scopeSync] && master.SyncMode != "" {
			_ = h.store.UpdateClientSync(client.ID, admin.ID, master.SyncMode, master.PeriodicIntervalMinutes)
		}
		if sink := h.hub.ClientSink(client.ID); sink != nil {
			pushFrame := &guardianpb.Frame{
				Payload: &guardianpb.Frame_ConfigPush{
					ConfigPush: &guardianpb.ConfigPush{Config: desired, Revision: "master"},
				},
			}
			_ = sink.SendFrame(pushFrame)
			if flags[scopeSync] && master.SyncMode != "" {
				syncCfg := &wingsvpb.Config{
					Ver: 1,
					Guardian: &wingsvpb.Guardian{
						SyncMode:                syncModeToProto(master.SyncMode),
						PeriodicIntervalMinutes: uint32(master.PeriodicIntervalMinutes),
					},
				}
				_ = sink.SendFrame(&guardianpb.Frame{
					Payload: &guardianpb.Frame_ConfigPush{
						ConfigPush: &guardianpb.ConfigPush{Config: syncCfg, Revision: "master-sync"},
					},
				})
			}
			pushed++
		}
	}
	_ = h.store.AppendAudit(storage.AuditEntry{
		ActorAdminID: admin.ID, ActorUsername: admin.Username,
		Action:  "client.master_applied",
		Message: strings.Join(normalizeScopeFlags(master.ScopeFlags), ","),
		IP:      clientIP(r),
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"clients_total":  len(clients),
		"clients_pushed": pushed,
	})
}

// handleMasterConfigSeed returns a config blueprint loaded from an existing
// client owned by this admin or parsed from a wingsv:// link. The admin can
// then trim it in the UI before saving as the master template. Per-client
// identity (Guardian sync, WG private keys, WB Stream rooms, Xray profiles)
// is stripped — only the bulk-applicable sections survive.
func (h *Handler) handleMasterConfigSeed(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		FromClientID string `json:"from_client_id"`
		FromLink     string `json:"from_wingsv_link"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	cfg, err := h.resolveMasterSeed(req.FromClientID, req.FromLink, admin)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if cfg == nil {
		writeError(w, http.StatusBadRequest, "no source provided")
		return
	}
	stripMasterIdentity(cfg)
	raw, err := protoToJSON(cfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"config": raw})
}

func (h *Handler) resolveMasterSeed(fromClientID, fromLink string, admin storage.Admin) (*wingsvpb.Config, error) {
	if id := strings.TrimSpace(fromClientID); id != "" {
		client, err := h.store.FindClientByID(id)
		if err != nil {
			return nil, errors.New("seed client not found")
		}
		if client.OwnerAdminID != admin.ID && !auth.IsOwner(admin) {
			return nil, errors.New("seed client not owned by admin")
		}
		cfg, err := h.store.GetClientConfig(id)
		if err != nil || len(cfg.ConfigProto) == 0 {
			return nil, errors.New("seed client has no config")
		}
		out := &wingsvpb.Config{}
		if err := proto.Unmarshal(cfg.ConfigProto, out); err != nil {
			return nil, errors.New("seed client config corrupted")
		}
		return out, nil
	}
	if link := strings.TrimSpace(fromLink); link != "" {
		out, err := preview.ParseLinkConfig(link)
		if err != nil {
			return nil, errors.New("invalid link (expected wingsv:// or vless://)")
		}
		return out, nil
	}
	return nil, nil
}

// stripRootOnlyBlocks silently zeroes out config sections that require root on
// the device. Called before pushing config to clients whose latest StateReport
// said has_root_access=false. The admin panel could still send a config with
// these blocks (e.g. a master template), but the device-side gating is best
// effort: a brand-new client that never reported anything keeps the default
// (no root), and we err on the side of "don't poke a non-rooted device with
// root settings that would freeze its UI".
func stripRootOnlyBlocks(cfg *wingsvpb.Config) {
	if cfg == nil {
		return
	}
	cfg.Root = nil
	cfg.Sharing = nil
	cfg.Xposed = nil
}

// stripMasterIdentity removes per-client identity fields so the seed only
// carries the sections that make sense for bulk application.
func stripMasterIdentity(cfg *wingsvpb.Config) {
	if cfg == nil {
		return
	}
	cfg.Guardian = nil
	cfg.Wg = nil
	cfg.Awg = nil
	cfg.WbStream = nil
	cfg.Sharing = nil
	cfg.Xposed = nil
	cfg.SubscriptionHwid = nil
	if cfg.Xray != nil {
		cfg.Xray.Profiles = nil
		cfg.Xray.ActiveProfileId = ""
		cfg.Xray.Subscriptions = nil
	}
}

func masterConfigBytesFromJSON(raw json.RawMessage) ([]byte, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	parsed := &wingsvpb.Config{}
	if err := protoUnmarshaller.Unmarshal(raw, parsed); err != nil {
		return nil, errors.New("invalid config: " + err.Error())
	}
	parsed.Guardian = nil
	return proto.Marshal(parsed)
}

func normalizeScopeFlagsList(in []string) []string {
	allowed := map[string]bool{
		scopeTurn: true, scopeXraySettings: true, scopeXrayRouting: true,
		scopeByeDpi: true, scopeAppPreferences: true, scopeAppRouting: true, scopeSync: true,
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, raw := range in {
		v := strings.TrimSpace(raw)
		if !allowed[v] || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

func normalizeScopeFlags(s string) []string {
	if s == "" {
		return []string{}
	}
	return normalizeScopeFlagsList(strings.Split(s, ","))
}

func scopeFlagSet(flags []string) map[string]bool {
	out := make(map[string]bool, len(flags))
	for _, f := range flags {
		out[f] = true
	}
	return out
}

// mergeMasterIntoConfig overwrites only the fields whose scope is enabled.
// Per-client identity (Xray profiles, WireGuard keys, WB Stream room ids,
// Sharing prefs, Xposed prefs, Subscriptions, etc.) is preserved.
func mergeMasterIntoConfig(target, master *wingsvpb.Config, flags map[string]bool) {
	if target == nil || master == nil {
		return
	}
	if flags[scopeTurn] {
		target.Turn = master.GetTurn()
	}
	if flags[scopeXraySettings] {
		if target.Xray == nil {
			target.Xray = &wingsvpb.Xray{}
		}
		target.Xray.Settings = master.GetXray().GetSettings()
	}
	if flags[scopeXrayRouting] {
		if target.Xray == nil {
			target.Xray = &wingsvpb.Xray{}
		}
		target.Xray.Routing = master.GetXray().GetRouting()
	}
	if flags[scopeByeDpi] {
		target.ByeDpi = master.GetByeDpi()
	}
	if flags[scopeAppPreferences] {
		target.AppPreferences = master.GetAppPreferences()
	}
	if flags[scopeAppRouting] {
		target.AppRouting = master.GetAppRouting()
	}
}

func formatTSStr(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
