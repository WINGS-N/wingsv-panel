// Package guardianhub coordinates live state between Guardian-protocol clients
// (WINGS V devices over WSS) and admin panel sessions watching them.
//
// The hub is purely in-memory; persistent state lives in the storage package.
// On a multi-instance deploy, the hub would need to be replaced by a Redis
// pub/sub bridge — for v1 we run a single panel instance.
package guardianhub

import (
	"sync"

	guardianpb "v.wingsnet.org/internal/gen/guardianpb"
)

type ClientSink interface {
	SendFrame(frame *guardianpb.Frame) error
	Close(reason string)
}

type AdminEvent struct {
	ClientID string
	Frame    *guardianpb.Frame
}

type AdminSink interface {
	SendEvent(ev AdminEvent)
}

type Hub struct {
	mu      sync.RWMutex
	clients map[string]ClientSink
	admins  map[int64]map[AdminSink]struct{}
}

func New() *Hub {
	return &Hub{
		clients: map[string]ClientSink{},
		admins:  map[int64]map[AdminSink]struct{}{},
	}
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

// ClientCount returns the number of connected device sessions.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// AdminCount returns the number of admin WS sessions across all admins.
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
