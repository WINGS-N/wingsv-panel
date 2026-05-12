package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/coder/websocket"

	guardianpb "v.wingsnet.org/internal/gen/guardianpb"
	"v.wingsnet.org/internal/guardianhub"
	"v.wingsnet.org/internal/storage"
)

const adminWriteTimeout = 5 * time.Second

// adminSink buffers AdminEvents and forwards them to one admin WS.
type adminSink struct {
	conn *websocket.Conn
	ch   chan guardianhub.AdminEvent
}

func (s *adminSink) SendEvent(ev guardianhub.AdminEvent) {
	select {
	case s.ch <- ev:
	default:
		// Drop on backpressure to avoid blocking the hub.
	}
}

// RegisterWS exposes the admin live-events WS endpoint. Sessions are
// authenticated through the existing session cookie.
func (h *Handler) RegisterWS(mux *http.ServeMux) {
	mux.HandleFunc("/api/admin/ws", h.handleWS)
}

func (h *Handler) handleWS(w http.ResponseWriter, r *http.Request) {
	admin, err := h.auth.Authenticate(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
		CompressionMode:    websocket.CompressionDisabled,
	})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	sink := &adminSink{conn: conn, ch: make(chan guardianhub.AdminEvent, 64)}
	h.hub.AttachAdmin(admin.ID, sink)
	defer h.hub.DetachAdmin(admin.ID, sink)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Discard inbound WS frames; admin uses REST endpoints for actions.
	go func() {
		for {
			if _, _, err := conn.Read(ctx); err != nil {
				cancel()
				return
			}
		}
	}()

	h.sendInitialSnapshot(ctx, sink, admin)

	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-sink.ch:
			if err := writeEvent(ctx, conn, ev); err != nil {
				return
			}
		}
	}
}

// wsEvent is the JSON envelope the admin SPA receives over /api/admin/ws.
// Wrapping protobuf payloads as JSON keeps the browser dependency-free —
// no protobufjs / ts-proto required client-side.
type wsEvent struct {
	ClientID string          `json:"client_id"`
	Kind     string          `json:"kind"`
	Payload  json.RawMessage `json:"payload,omitempty"`
}

func writeEvent(ctx context.Context, conn *websocket.Conn, ev guardianhub.AdminEvent) error {
	if ev.Frame == nil {
		return nil
	}
	envelope, err := buildAdminEnvelope(ev.ClientID, ev.Frame)
	if err != nil {
		return err
	}
	if envelope.Kind == "" {
		return nil
	}
	bytesJSON, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	writeCtx, cancel := context.WithTimeout(ctx, adminWriteTimeout)
	defer cancel()
	return conn.Write(writeCtx, websocket.MessageText, bytesJSON)
}

func buildAdminEnvelope(clientID string, frame *guardianpb.Frame) (wsEvent, error) {
	envelope := wsEvent{ClientID: clientID}
	switch payload := frame.GetPayload().(type) {
	case *guardianpb.Frame_StateReport:
		envelope.Kind = "state_report"
		raw, err := protoToJSON(payload.StateReport)
		if err != nil {
			return envelope, err
		}
		envelope.Payload = raw
	case *guardianpb.Frame_StatusUpdate:
		envelope.Kind = "status_update"
		raw, err := protoToJSON(payload.StatusUpdate)
		if err != nil {
			return envelope, err
		}
		envelope.Payload = raw
	case *guardianpb.Frame_LogChunk:
		envelope.Kind = "log_chunk"
		raw, err := protoToJSON(payload.LogChunk)
		if err != nil {
			return envelope, err
		}
		envelope.Payload = raw
	case *guardianpb.Frame_CommandAck:
		envelope.Kind = "command_ack"
		raw, err := protoToJSON(payload.CommandAck)
		if err != nil {
			return envelope, err
		}
		envelope.Payload = raw
	case *guardianpb.Frame_Error:
		envelope.Kind = "error"
		raw, err := protoToJSON(payload.Error)
		if err != nil {
			return envelope, err
		}
		envelope.Payload = raw
	case *guardianpb.Frame_InstalledApps:
		envelope.Kind = "installed_apps"
	}
	return envelope, nil
}

func (h *Handler) sendInitialSnapshot(ctx context.Context, sink *adminSink, admin storage.Admin) {
	// Initial snapshot is delivered via the REST GET /api/admin/clients call
	// the SPA performs on dashboard load. The WS itself starts fresh — only
	// future state changes flow through it.
	_ = ctx
	_ = sink
	_ = admin
}
