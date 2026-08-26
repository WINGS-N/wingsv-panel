// Package guardian implements the device-facing management channel used by
// WINGS V devices to talk to the panel.
//
// The transport is gRPC (see grpc.go); the protocol itself lives in session.go so
// it stays independent of the pipe carrying it. Auth happens inside the protocol
// via ClientHello, so the token never appears in a URL or a header.
package guardian

import (
	"time"

	"google.golang.org/protobuf/proto"

	wingsvpb "v.wingsnet.org/internal/gen/wingsvpb"
	"v.wingsnet.org/internal/guardianhub"
	"v.wingsnet.org/internal/storage"
)

const (
	protocolVersion uint32 = 1
	helloTimeout           = 10 * time.Second
)

type Handler struct {
	store *storage.Store
	hub   *guardianhub.Hub
}

func New(store *storage.Store, hub *guardianhub.Hub) *Handler {
	return &Handler{store: store, hub: hub}
}

func unmarshalDesired(b []byte) (*wingsvpb.Config, error) {
	cfg := &wingsvpb.Config{}
	if err := proto.Unmarshal(b, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Compile-time proof that the gRPC session still satisfies what the hub fans out
// to, so a protocol change cannot silently break delivery to devices.
var _ guardianhub.ClientSink = (*grpcSession)(nil)
