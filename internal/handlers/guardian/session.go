package guardian

import (
	"context"
	"net"

	"crypto/subtle"
	"google.golang.org/grpc/peer"
	"log"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	"v.wingsnet.org/internal/auth"
	guardianpb "v.wingsnet.org/internal/gen/guardianpb"
	"v.wingsnet.org/internal/guardianhub"
	"v.wingsnet.org/internal/storage"
)

// Everything in this file is transport-neutral: the WebSocket handler and the
// gRPC service both drive it, so a device sees the same protocol either way.

func (h *Handler) authenticate(hello *guardianpb.ClientHello) (storage.Client, bool) {
	client, err := h.store.FindClientByID(hello.GetClientId())
	if err != nil {
		return storage.Client{}, false
	}
	if !auth.VerifyClientToken(client.TokenHash, hello.GetClientToken()) {
		return storage.Client{}, false
	}
	if auth.ClientTokenHashNeedsUpgrade(client.TokenHash) {
		upgraded := auth.HashClientToken(hello.GetClientToken())
		if err := h.store.UpgradeClientTokenHash(client.ID, upgraded); err == nil {
			client.TokenHash = upgraded
		}
	}
	if !hwidMatches(client, hello) {
		log.Printf("guardian: hwid mismatch for client=%s, refusing", client.ID)
		return storage.Client{}, false
	}
	return client, true
}

// hwidMatches ties the token to the device that first used it. The token is a
// bearer credential that rides along in settings backups, so without this a
// restore onto another device would hand that device full control of this
// client. The first presented hwid binds (markOnline stores it); an admin
// rotating the token clears the binding so the client can move.
//
// A device that reports no hwid is not rejected: the value comes from the
// subscription hwid payload, which is not always present, and refusing would
// lock those clients out entirely.
func hwidMatches(client storage.Client, hello *guardianpb.ClientHello) bool {
	presented := strings.TrimSpace(hello.GetHwid())
	bound := strings.TrimSpace(client.HWID)
	if bound == "" || presented == "" {
		return true
	}
	return subtle.ConstantTimeCompare([]byte(bound), []byte(presented)) == 1
}

func (h *Handler) markOnline(ctx context.Context, client storage.Client, hello *guardianpb.ClientHello) {
	dev := storage.ClientDeviceInfo{
		HWID:        hello.GetHwid(),
		DeviceName:  hello.GetDeviceName(),
		DeviceModel: hello.GetDeviceModel(),
		OSVersion:   hello.GetOsVersion(),
		AppVersion:  hello.GetAppVersion(),
		PeerIP:      peerIP(ctx),
	}
	_ = h.store.UpdateClientPresence(client.ID, true, &dev)
	h.hub.FanoutToAdmin(client.OwnerAdminID, guardianhub.AdminEvent{
		ClientID: client.ID,
		Frame: &guardianpb.Frame{
			Payload: &guardianpb.Frame_StatusUpdate{
				StatusUpdate: &guardianpb.StatusUpdate{Runtime: &guardianpb.RuntimeState{}},
			},
		},
	})
}

// markOffline is a no-op when a replacement session already holds the hub slot,
// so a reconnect that races the old session's teardown keeps the fresh presence.
func (h *Handler) markOffline(client storage.Client) {
	if h.hub.ClientSink(client.ID) != nil {
		return
	}
	_ = h.store.UpdateClientPresence(client.ID, false, nil)
	h.hub.FanoutToAdmin(client.OwnerAdminID, guardianhub.AdminEvent{
		ClientID: client.ID,
		Frame: &guardianpb.Frame{
			Payload: &guardianpb.Frame_Error{
				Error: &guardianpb.ServerError{Code: "offline", Message: "client disconnected"},
			},
		},
	})
}

func (h *Handler) logControlFor(clientID string) *guardianpb.LogControl {
	cur, err := h.store.FindClientByID(clientID)
	if err != nil {
		return nil
	}
	return &guardianpb.LogControl{
		RuntimeEnabled: cur.LogRuntimeEnabled,
		ProxyEnabled:   cur.LogProxyEnabled,
		XrayEnabled:    cur.LogXRayEnabled,
	}
}

// configPushFor returns nil unless the panel holds a config newer than the one the
// device says it already applied. The device reports that version on every entry,
// so admin edits that landed while it was away are not rolled back to a stale
// snapshot, and a device that is already current is not pushed to at all.
func (h *Handler) configPushFor(clientID string, deviceVersion uint64) *guardianpb.ConfigPush {
	cfg, err := h.store.GetClientConfig(clientID)
	if err != nil || len(cfg.ConfigProto) == 0 || cfg.ConfigVersion <= deviceVersion {
		return nil
	}
	parsed, perr := unmarshalDesired(cfg.ConfigProto)
	if perr != nil {
		return nil
	}
	parsed.ConfigVersion = cfg.ConfigVersion
	return &guardianpb.ConfigPush{Config: parsed, Revision: cfg.Revision}
}

func (h *Handler) drainCommands(clientID string) []*guardianpb.Command {
	pending, err := h.store.DrainPendingCommands(clientID)
	if err != nil {
		return nil
	}
	commands := make([]*guardianpb.Command, 0, len(pending))
	for _, pc := range pending {
		cmdID, idErr := auth.GenerateClientID()
		if idErr != nil {
			continue
		}
		commands = append(commands, &guardianpb.Command{
			Type:           guardianpb.CommandType(pc.CommandType),
			Id:             cmdID,
			SubscriptionId: pc.SubscriptionID,
		})
	}
	return commands
}

// welcomeFrames is the opening burst of a live session. Order matters for
// generate_vk_link: the command mutates settings.vkLinks on the device and the
// ConfigPush carries the server's desired list, so a command delivered first
// would be clobbered by the push that follows it.
func (h *Handler) welcomeFrames(client storage.Client, deviceVersion uint64) []*guardianpb.Frame {
	frames := make([]*guardianpb.Frame, 0, 4)
	if control := h.logControlFor(client.ID); control != nil {
		frames = append(frames, &guardianpb.Frame{
			Payload: &guardianpb.Frame_LogControl{LogControl: control},
		})
	}
	if push := h.configPushFor(client.ID, deviceVersion); push != nil {
		frames = append(frames, &guardianpb.Frame{
			Payload: &guardianpb.Frame_ConfigPush{ConfigPush: push},
		})
	}
	for _, cmd := range h.drainCommands(client.ID) {
		frames = append(frames, &guardianpb.Frame{
			Payload: &guardianpb.Frame_Command{Command: cmd},
		})
	}
	return frames
}

func (h *Handler) handleClientFrame(client storage.Client, sink guardianhub.ClientSink, frame *guardianpb.Frame) {
	switch payload := frame.GetPayload().(type) {
	case *guardianpb.Frame_Heartbeat:
		_ = sink.SendFrame(&guardianpb.Frame{
			Payload: &guardianpb.Frame_Heartbeat{Heartbeat: &guardianpb.Heartbeat{TsMs: time.Now().UnixMilli()}},
		})
	case *guardianpb.Frame_StateReport:
		h.storeStateReport(client, payload.StateReport)
		h.hub.FanoutToAdmin(client.OwnerAdminID, guardianhub.AdminEvent{ClientID: client.ID, Frame: frame})
	case *guardianpb.Frame_LogChunk:
		h.storeLogChunk(client, payload.LogChunk)
		h.hub.FanoutToAdmin(client.OwnerAdminID, guardianhub.AdminEvent{ClientID: client.ID, Frame: frame})
	case *guardianpb.Frame_StatusUpdate:
		h.storeRuntime(client, payload.StatusUpdate.GetRuntime())
		h.hub.FanoutToAdmin(client.OwnerAdminID, guardianhub.AdminEvent{ClientID: client.ID, Frame: frame})
	case *guardianpb.Frame_CommandAck:
		h.hub.FanoutToAdmin(client.OwnerAdminID, guardianhub.AdminEvent{ClientID: client.ID, Frame: frame})
	case *guardianpb.Frame_InstalledApps:
		h.storeInstalledApps(client, payload.InstalledApps)
		h.hub.FanoutToAdmin(client.OwnerAdminID, guardianhub.AdminEvent{ClientID: client.ID, Frame: frame})
	default:
		// Unknown / not-yet-implemented frame - drop silently.
	}
}

func (h *Handler) storeStateReport(client storage.Client, report *guardianpb.StateReport) {
	if report == nil {
		return
	}
	if snapshot := report.GetSnapshot(); snapshot != nil {
		if b, err := proto.Marshal(snapshot); err == nil {
			_ = h.store.UpsertClientReportedConfig(client.ID, b)
		}
	}
	h.storeRuntime(client, report.GetRuntime())
}

func (h *Handler) storeRuntime(client storage.Client, runtime *guardianpb.RuntimeState) {
	if runtime == nil {
		return
	}
	if b, err := proto.Marshal(runtime); err == nil {
		_ = h.store.UpsertClientRuntime(client.ID, b)
	}
	_ = h.store.UpdateClientRootAccess(client.ID, runtime.GetHasRootAccess())
	_ = h.store.UpdateClientVkOAuthAuthorized(client.ID, runtime.GetVkOauthAuthorized())
}

func (h *Handler) storeLogChunk(client storage.Client, chunk *guardianpb.LogChunk) {
	if chunk == nil {
		return
	}
	lines := make([]storage.LogLine, 0, len(chunk.GetLines()))
	for _, l := range chunk.GetLines() {
		lines = append(lines, storage.LogLine{TS: time.UnixMilli(l.GetTsMs()), Text: l.GetText()})
	}
	_ = h.store.AppendClientLogs(client.ID, int32(chunk.GetStream()), int64(chunk.GetFirstSeq()), lines)
}

func (h *Handler) storeInstalledApps(client storage.Client, apps *guardianpb.InstalledApps) {
	if apps == nil {
		return
	}
	if b, err := proto.Marshal(apps); err == nil {
		_ = h.store.UpsertClientInstalledApps(client.ID, b)
	}
	metas := make([]storage.PackageMetadata, 0, len(apps.GetApps()))
	for _, app := range apps.GetApps() {
		metas = append(metas, storage.PackageMetadata{
			Package: app.GetPackageName(),
			Label:   app.GetLabel(),
			IconPNG: app.GetIconPng(),
		})
	}
	_ = h.store.UpsertPackageMetadata(metas)
}

// peerIP - адрес соединения. При работающем туннеле он показывает точку выхода,
// а не дом клиента, и это тоже полезно: расхождение с самоотчётом само по себе
// сигнал
func peerIP(ctx context.Context) string {
	info, ok := peer.FromContext(ctx)
	if !ok || info.Addr == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(info.Addr.String())
	if err != nil {
		return info.Addr.String()
	}
	return host
}
