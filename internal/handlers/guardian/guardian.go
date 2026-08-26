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
	// Liveness is tracked via application-level Heartbeat data frames (the client
	// sends one every 25s), NOT WS Ping/Pong control frames: some HTTP/2 ingresses
	// mangle control frames, which made the old conn.Ping watchdog kill healthy
	// sessions every ~20s. If no frame arrives within this window the read times
	// out and the session is recycled.
	frameReadTimeout = 75 * time.Second
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
		// permessage-deflate cuts the config-push / status frames noticeably. It is
		// negotiated, so a client that does not offer it (or an ingress that strips
		// the extension) simply falls back to raw frames - no break. No-context-
		// takeover keeps each frame independently inflatable, which the app's OkHttp
		// client handles most reliably.
		CompressionMode: websocket.CompressionNoContextTakeover,
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

	client, ok := h.authenticate(hello)
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

	h.markOnline(client, hello)
	h.hub.AttachClient(client.ID, sess)
	defer func() {
		h.hub.DetachClient(client.ID, sess)
		h.markOffline(client)
	}()

	for _, frame := range h.welcomeFrames(client, hello.GetLastAppliedConfigVersion()) {
		if err := sess.SendFrame(frame); err != nil {
			return
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

	// Dead-connection detection is the read deadline below (frameReadTimeout):
	// the client heartbeats every 25s, so a 75s silence means the socket is gone.
	// We deliberately do NOT use WS Ping/Pong here - the HTTP/2 ingress in front
	// of the panel mangles control frames, so conn.Ping was timing out on healthy
	// sessions and recycling them every ~20s.

	for {
		select {
		case <-ctx.Done():
			log.Printf("guardian: session ctx-done client=%s lifetime=%s",
				client.ID, time.Since(connectedAt).Truncate(time.Second))
			return
		default:
		}
		readCtx, cancelRead := context.WithTimeout(ctx, frameReadTimeout)
		frame, err := readFrame(readCtx, conn)
		cancelRead()
		if err != nil {
			log.Printf("guardian: session read err client=%s err=%v lifetime=%s",
				client.ID, err, time.Since(connectedAt).Truncate(time.Second))
			return
		}
		h.handleClientFrame(client, sess, frame)
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
