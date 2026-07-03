// Package guardianhub coordinates live state between Guardian-protocol clients
// (WINGS V devices over WSS) and admin panel sessions watching them.
//
// The hub delivers to locally-held WS sinks directly. On a multi-replica deploy
// it is also given a cross-replica Bus: a message whose target WS lives on a
// different replica is published to the bus, and the replica holding that sink
// delivers it. With the no-op bus (single-node sqlite) everything stays local.
package guardianhub

import (
	"context"
	"encoding/json"
	"sync"

	"google.golang.org/protobuf/proto"

	"v.wingsnet.org/internal/bus"
	guardianpb "v.wingsnet.org/internal/gen/guardianpb"
)

const (
	channelClient = "guardian.client"
	channelAdmin  = "guardian.admin"
)

type ClientSink interface {
	SendFrame(frame *guardianpb.Frame) error
	Close(reason string)
}

type AdminEvent struct {
	ClientID string
	Frame    *guardianpb.Frame
	// Kind, when set with a nil Frame, is a non-device event (e.g. "stats_update")
	// forwarded to the admin WS verbatim instead of being derived from a frame.
	Kind string
}

type AdminSink interface {
	SendEvent(ev AdminEvent)
}

type Hub struct {
	mu      sync.RWMutex
	clients map[string]ClientSink
	admins  map[int64]map[AdminSink]struct{}

	bus        bus.Bus
	instanceID string
	hasBus     bool
}

func New() *Hub {
	return &Hub{
		clients: map[string]ClientSink{},
		admins:  map[int64]map[AdminSink]struct{}{},
		bus:     bus.Nop{},
	}
}

// AttachBus wires a cross-replica bus and registers this hub's handlers. Call it
// before starting the bus. instanceID identifies this replica so admin fanout
// does not double-deliver to the publisher's own local sinks.
func (h *Hub) AttachBus(b bus.Bus, instanceID string) {
	h.bus = b
	h.instanceID = instanceID
	h.hasBus = true
	b.Subscribe(channelClient, h.onBusClient)
	b.Subscribe(channelAdmin, h.onBusAdmin)
}

// AttachClient registers a client connection; if another connection for the
// same client_id is already registered, it gets closed with "replaced".
func (h *Hub) AttachClient(clientID string, sink ClientSink) {
	h.mu.Lock()
	prev := h.clients[clientID]
	h.clients[clientID] = sink
	h.mu.Unlock()
	if prev != nil {
		prev.Close("replaced")
	}
}

func (h *Hub) DetachClient(clientID string, sink ClientSink) {
	h.mu.Lock()
	if h.clients[clientID] == sink {
		delete(h.clients, clientID)
	}
	h.mu.Unlock()
}

func (h *Hub) ClientSink(clientID string) ClientSink {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.clients[clientID]
}

// SendToClient delivers a frame to a device. If its WS is on this replica it is
// sent directly; otherwise, when the client is known online (on another replica)
// and a bus is attached, the frame is published for the holding replica. Returns
// false when the frame was neither sent nor publishable (offline / single-node).
func (h *Hub) SendToClient(clientID string, frame *guardianpb.Frame, online bool) bool {
	if sink := h.ClientSink(clientID); sink != nil {
		_ = sink.SendFrame(frame)
		return true
	}
	if !online || !h.hasBus {
		return false
	}
	raw, err := proto.Marshal(frame)
	if err != nil {
		return false
	}
	env := busEnvelope{Origin: h.instanceID, ClientID: clientID, Frame: raw}
	if err := h.publish(channelClient, env); err != nil {
		return false
	}
	return true
}

func (h *Hub) AttachAdmin(adminID int64, sink AdminSink) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.admins[adminID] == nil {
		h.admins[adminID] = map[AdminSink]struct{}{}
	}
	h.admins[adminID][sink] = struct{}{}
}

func (h *Hub) DetachAdmin(adminID int64, sink AdminSink) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if set, ok := h.admins[adminID]; ok {
		delete(set, sink)
		if len(set) == 0 {
			delete(h.admins, adminID)
		}
	}
}

// ClientCount returns the number of device sessions on this replica.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// AdminCount returns the number of admin WS sessions on this replica.
func (h *Hub) AdminCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	total := 0
	for _, set := range h.admins {
		total += len(set)
	}
	return total
}

func (h *Hub) FanoutToAdmin(adminID int64, ev AdminEvent) {
	h.deliverLocalAdmin(adminID, ev)
	if h.hasBus {
		_ = h.publish(channelAdmin, h.adminEnvelope(ev, adminID, false))
	}
}

// BroadcastToAdmins delivers a non-device event to every connected admin WS. The
// stats collector uses it to nudge dashboards to refetch; each client then pulls
// only the scope it is allowed to see.
func (h *Hub) BroadcastToAdmins(ev AdminEvent) {
	h.deliverLocalBroadcast(ev)
	if h.hasBus {
		_ = h.publish(channelAdmin, h.adminEnvelope(ev, 0, true))
	}
}

func (h *Hub) deliverLocalAdmin(adminID int64, ev AdminEvent) {
	h.mu.RLock()
	sinks := make([]AdminSink, 0, len(h.admins[adminID]))
	for sink := range h.admins[adminID] {
		sinks = append(sinks, sink)
	}
	h.mu.RUnlock()
	for _, s := range sinks {
		s.SendEvent(ev)
	}
}

func (h *Hub) deliverLocalBroadcast(ev AdminEvent) {
	h.mu.RLock()
	sinks := make([]AdminSink, 0)
	for _, set := range h.admins {
		for sink := range set {
			sinks = append(sinks, sink)
		}
	}
	h.mu.RUnlock()
	for _, s := range sinks {
		s.SendEvent(ev)
	}
}

// busEnvelope is the JSON payload carried on the bus. Frame is a marshalled
// guardianpb.Frame (empty for a Kind-only admin event).
type busEnvelope struct {
	Origin    string `json:"o"`
	Broadcast bool   `json:"b,omitempty"`
	ClientID  string `json:"c,omitempty"`
	AdminID   int64  `json:"a,omitempty"`
	EventKind string `json:"k,omitempty"`
	Frame     []byte `json:"f,omitempty"`
}

func (h *Hub) adminEnvelope(ev AdminEvent, adminID int64, broadcast bool) busEnvelope {
	env := busEnvelope{
		Origin: h.instanceID, Broadcast: broadcast, AdminID: adminID,
		ClientID: ev.ClientID, EventKind: ev.Kind,
	}
	if ev.Frame != nil {
		if raw, err := proto.Marshal(ev.Frame); err == nil {
			env.Frame = raw
		}
	}
	return env
}

func (h *Hub) publish(channel string, env busEnvelope) error {
	payload, err := json.Marshal(env)
	if err != nil {
		return err
	}
	return h.bus.Publish(context.Background(), channel, payload)
}

func (h *Hub) onBusClient(payload []byte) {
	var env busEnvelope
	if json.Unmarshal(payload, &env) != nil {
		return
	}
	sink := h.ClientSink(env.ClientID)
	if sink == nil {
		return
	}
	frame := &guardianpb.Frame{}
	if proto.Unmarshal(env.Frame, frame) != nil {
		return
	}
	_ = sink.SendFrame(frame)
}

func (h *Hub) onBusAdmin(payload []byte) {
	var env busEnvelope
	if json.Unmarshal(payload, &env) != nil {
		return
	}
	// Skip our own publish: the local sinks were already delivered synchronously.
	if env.Origin == h.instanceID {
		return
	}
	ev := AdminEvent{ClientID: env.ClientID, Kind: env.EventKind}
	if len(env.Frame) > 0 {
		frame := &guardianpb.Frame{}
		if proto.Unmarshal(env.Frame, frame) == nil {
			ev.Frame = frame
		}
	}
	if env.Broadcast {
		h.deliverLocalBroadcast(ev)
		return
	}
	h.deliverLocalAdmin(env.AdminID, ev)
}
