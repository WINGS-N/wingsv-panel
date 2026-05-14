// Package guardian implements the Guardian WebSocket endpoint used by WINGS V
// devices to maintain a live management channel with the panel.
//
// Wire format: each WS binary frame carries one guardianpb.Frame; auth happens
// inside the protocol via ClientHello (token never appears in URL or headers).
package guardian

import (
	"context"
	"encoding/base64"
	"errors"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"google.golang.org/protobuf/proto"

	"v.wingsnet.org/internal/auth"
	guardianpb "v.wingsnet.org/internal/gen/guardianpb"
	wingsvpb "v.wingsnet.org/internal/gen/wingsvpb"
	"v.wingsnet.org/internal/guardianhub"
	"v.wingsnet.org/internal/storage"
)

const (
	protocolVersion   uint32 = 1
	helloTimeout             = 10 * time.Second
	writeTimeout             = 5 * time.Second
	heartbeatInterval        = 25 * time.Second
)

type Handler struct {
	store *storage.Store
	hub   *guardianhub.Hub
}

func New(store *storage.Store, hub *guardianhub.Hub) *Handler {
	return &Handler{store: store, hub: hub}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/guardian/ws", h.handleWS)
}

func (h *Handler) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
		// permessage-deflate sometimes gets mangled by ingresses (Traefik in
		// HTTP/2 mode in particular), so we ship raw frames.
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		log.Printf("guardian: ws accept failed: %v", err)
		return
	}
	conn.SetReadLimit(32 << 20)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	helloCtx, helloCancel := context.WithTimeout(ctx, helloTimeout)
	defer helloCancel()
	frame, err := readFrame(helloCtx, conn)
	if err != nil {
		_ = conn.Close(websocket.StatusPolicyViolation, "hello timeout")
		return
	}
	hello := frame.GetClientHello()
	if hello == nil {
		_ = sendError(ctx, conn, "expected_hello", "first frame must be ClientHello")
		_ = conn.Close(websocket.StatusPolicyViolation, "no hello")
		return
	}

	client, ok := h.authenticateHello(hello)
	if !ok {
		_ = writeFrame(ctx, conn, &guardianpb.Frame{
			Payload: &guardianpb.Frame_ServerHello{
				ServerHello: &guardianpb.ServerHello{Accepted: false, ErrorMessage: "invalid credentials", ProtocolVersion: protocolVersion},
			},
		})
		_ = conn.Close(websocket.StatusPolicyViolation, "auth failed")
		return
	}

	if err := writeFrame(ctx, conn, &guardianpb.Frame{
		Payload: &guardianpb.Frame_ServerHello{
			ServerHello: &guardianpb.ServerHello{Accepted: true, ProtocolVersion: protocolVersion},
		},
	}); err != nil {
		return
	}

	h.runSession(ctx, conn, client, hello)
}

func (h *Handler) authenticateHello(hello *guardianpb.ClientHello) (storage.Client, bool) {
	client, err := h.store.FindClientByID(hello.GetClientId())
	if err != nil {
		return storage.Client{}, false
	}
	if !auth.VerifyClientToken(client.TokenHash, hello.GetClientToken()) {
		return storage.Client{}, false
	}
	return client, true
}

type session struct {
	conn    *websocket.Conn
	client  storage.Client
	hub     *guardianhub.Hub
	store   *storage.Store
	writeMu sync.Mutex
	closed  chan struct{}
}

func (h *Handler) runSession(ctx context.Context, conn *websocket.Conn, client storage.Client, hello *guardianpb.ClientHello) {
	sess := &session{
		conn:   conn,
		client: client,
		hub:    h.hub,
		store:  h.store,
		closed: make(chan struct{}),
	}
	defer close(sess.closed)

	connectedAt := time.Now()
	log.Printf("guardian: session up client=%s device=%s app=%s ip=%s",
		client.ID, hello.GetDeviceModel(), hello.GetAppVersion(), conn.Subprotocol())

	dev := storage.ClientDeviceInfo{
		HWID:        hello.GetHwid(),
		DeviceName:  hello.GetDeviceName(),
		DeviceModel: hello.GetDeviceModel(),
		OSVersion:   hello.GetOsVersion(),
		AppVersion:  hello.GetAppVersion(),
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
	h.hub.AttachClient(client.ID, sess)
	defer func() {
		h.hub.DetachClient(client.ID, sess)
		// Only mark offline if no replacement session has taken over the
		// hub slot. Otherwise the replaced session's defer would clobber
		// the fresh true that the new session just wrote.
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
	}()

	cur, err := h.store.FindClientByID(client.ID)
	if err == nil {
		_ = sess.SendFrame(&guardianpb.Frame{
			Payload: &guardianpb.Frame_LogControl{
				LogControl: &guardianpb.LogControl{
					RuntimeEnabled: cur.LogRuntimeEnabled,
					ProxyEnabled:   cur.LogProxyEnabled,
					XrayEnabled:    cur.LogXRayEnabled,
				},
			},
		})
	}
	if cfg, err := h.store.GetClientConfig(client.ID); err == nil && len(cfg.ConfigProto) > 0 {
		// Race-fix: device reports the config_version it has already applied
		// locally in ClientHello.LastAppliedConfigVersion. If DB hasn't moved
		// past that version, skip the welcome push — otherwise we'd clobber
		// admin edits that landed AFTER the device disconnected but before
		// it reconnected to a fresh ws session.
		deviceVersion := hello.GetLastAppliedConfigVersion()
		if cfg.ConfigVersion > deviceVersion {
			parsed, perr := unmarshalDesired(cfg.ConfigProto)
			if perr == nil {
				parsed.ConfigVersion = cfg.ConfigVersion
				_ = sess.SendFrame(&guardianpb.Frame{
					Payload: &guardianpb.Frame_ConfigPush{
						ConfigPush: &guardianpb.ConfigPush{Config: parsed, Revision: cfg.Revision},
					},
				})
			}
		}
	}

	heartbeatTicker := time.NewTicker(heartbeatInterval)
	defer heartbeatTicker.Stop()
	go func() {
		for {
			select {
			case <-sess.closed:
				return
			case <-ctx.Done():
				return
			case <-heartbeatTicker.C:
				_ = sess.SendFrame(&guardianpb.Frame{
					Payload: &guardianpb.Frame_Heartbeat{Heartbeat: &guardianpb.Heartbeat{TsMs: time.Now().UnixMilli()}},
				})
			}
		}
	}()

	// Server-side WS ping watchdog: a missing pong within 20s marks the TCP
	// dead and forces close, instead of waiting for OS-level TCP timeout
	// (which can be many minutes on idle cellular sockets).
	pingTicker := time.NewTicker(20 * time.Second)
	defer pingTicker.Stop()
	go func() {
		for {
			select {
			case <-sess.closed:
				return
			case <-ctx.Done():
				return
			case <-pingTicker.C:
				pingCtx, cancelPing := context.WithTimeout(ctx, 20*time.Second)
				if err := conn.Ping(pingCtx); err != nil {
					cancelPing()
					log.Printf("guardian: ping failed client=%s err=%v", client.ID, err)
					_ = conn.Close(websocket.StatusGoingAway, "ping timeout")
					return
				}
				cancelPing()
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			log.Printf("guardian: session ctx-done client=%s lifetime=%s",
				client.ID, time.Since(connectedAt).Truncate(time.Second))
			return
		default:
		}
		frame, err := readFrame(ctx, conn)
		if err != nil {
			log.Printf("guardian: session read err client=%s err=%v lifetime=%s",
				client.ID, err, time.Since(connectedAt).Truncate(time.Second))
			return
		}
		h.handleClientFrame(frame, sess)
	}
}

func (h *Handler) handleClientFrame(frame *guardianpb.Frame, sess *session) {
	switch payload := frame.GetPayload().(type) {
	case *guardianpb.Frame_Heartbeat:
		_ = sess.SendFrame(&guardianpb.Frame{
			Payload: &guardianpb.Frame_Heartbeat{Heartbeat: &guardianpb.Heartbeat{TsMs: time.Now().UnixMilli()}},
		})
	case *guardianpb.Frame_StateReport:
		report := payload.StateReport
		if report.GetSnapshot() != nil {
			if b, err := proto.Marshal(report.GetSnapshot()); err == nil {
				_ = h.store.UpsertClientReportedConfig(sess.client.ID, b)
			}
		}
		if report.GetRuntime() != nil {
			runtime := report.GetRuntime()
			if b, err := proto.Marshal(runtime); err == nil {
				_ = h.store.UpsertClientRuntime(sess.client.ID, b)
			}
			_ = h.store.UpdateClientRootAccess(sess.client.ID, runtime.GetHasRootAccess())
		}
		h.hub.FanoutToAdmin(sess.client.OwnerAdminID, guardianhub.AdminEvent{ClientID: sess.client.ID, Frame: frame})
	case *guardianpb.Frame_LogChunk:
		chunk := payload.LogChunk
		base := int64(chunk.GetFirstSeq())
		lines := make([]storage.LogLine, 0, len(chunk.GetLines()))
		for _, l := range chunk.GetLines() {
			lines = append(lines, storage.LogLine{TS: time.UnixMilli(l.GetTsMs()), Text: l.GetText()})
		}
		_ = h.store.AppendClientLogs(sess.client.ID, int32(chunk.GetStream()), base, lines)
		h.hub.FanoutToAdmin(sess.client.OwnerAdminID, guardianhub.AdminEvent{ClientID: sess.client.ID, Frame: frame})
	case *guardianpb.Frame_StatusUpdate:
		if runtime := payload.StatusUpdate.GetRuntime(); runtime != nil {
			if b, err := proto.Marshal(runtime); err == nil {
				_ = h.store.UpsertClientRuntime(sess.client.ID, b)
			}
			_ = h.store.UpdateClientRootAccess(sess.client.ID, runtime.GetHasRootAccess())
		}
		h.hub.FanoutToAdmin(sess.client.OwnerAdminID, guardianhub.AdminEvent{ClientID: sess.client.ID, Frame: frame})
	case *guardianpb.Frame_CommandAck:
		h.hub.FanoutToAdmin(sess.client.OwnerAdminID, guardianhub.AdminEvent{ClientID: sess.client.ID, Frame: frame})
	case *guardianpb.Frame_InstalledApps:
		if b, err := proto.Marshal(payload.InstalledApps); err == nil {
			_ = h.store.UpsertClientInstalledApps(sess.client.ID, b)
		}
		metas := make([]storage.PackageMetadata, 0, len(payload.InstalledApps.GetApps()))
		for _, app := range payload.InstalledApps.GetApps() {
			metas = append(metas, storage.PackageMetadata{
				Package: app.GetPackageName(),
				Label:   app.GetLabel(),
				IconPNG: app.GetIconPng(),
			})
		}
		_ = h.store.UpsertPackageMetadata(metas)
		h.hub.FanoutToAdmin(sess.client.OwnerAdminID, guardianhub.AdminEvent{ClientID: sess.client.ID, Frame: frame})
	default:
		// Unknown / not-yet-implemented frame — drop silently.
	}
}

func unmarshalDesired(b []byte) (*wingsvpb.Config, error) {
	cfg := &wingsvpb.Config{}
	if err := proto.Unmarshal(b, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (s *session) SendFrame(frame *guardianpb.Frame) error {
	if s == nil {
		return errors.New("nil session")
	}
	ctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
	defer cancel()
	return writeFrame(ctx, s.conn, frame)
}

func (s *session) Close(reason string) {
	if s == nil || s.conn == nil {
		return
	}
	_ = s.conn.Close(websocket.StatusPolicyViolation, reason)
}

func readFrame(ctx context.Context, conn *websocket.Conn) (*guardianpb.Frame, error) {
	typ, data, err := conn.Read(ctx)
	if err != nil {
		return nil, err
	}
	if typ != websocket.MessageBinary {
		return nil, errors.New("expected binary frame")
	}
	frame := &guardianpb.Frame{}
	if err := proto.Unmarshal(data, frame); err != nil {
		return nil, err
	}
	return frame, nil
}

func writeFrame(ctx context.Context, conn *websocket.Conn, frame *guardianpb.Frame) error {
	bytesProto, err := proto.Marshal(frame)
	if err != nil {
		return err
	}
	return conn.Write(ctx, websocket.MessageBinary, bytesProto)
}

func sendError(ctx context.Context, conn *websocket.Conn, code, message string) error {
	return writeFrame(ctx, conn, &guardianpb.Frame{
		Payload: &guardianpb.Frame_Error{Error: &guardianpb.ServerError{Code: code, Message: message}},
	})
}

// nolint:unused
var _ = base64.StdEncoding
