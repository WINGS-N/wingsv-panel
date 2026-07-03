package guardianhub

import (
	"context"
	"sync"
	"testing"

	guardianpb "v.wingsnet.org/internal/gen/guardianpb"
)

// fakeBus fans a publish out to every subscribed handler synchronously, so two
// hubs sharing one fakeBus behave like two replicas on the same channel.
type fakeBus struct {
	mu       sync.Mutex
	handlers map[string][]func([]byte)
}

func newFakeBus() *fakeBus { return &fakeBus{handlers: map[string][]func([]byte){}} }

func (b *fakeBus) Subscribe(channel string, handler func([]byte)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[channel] = append(b.handlers[channel], handler)
}
func (b *fakeBus) Publish(_ context.Context, channel string, payload []byte) error {
	b.mu.Lock()
	handlers := append([]func([]byte){}, b.handlers[channel]...)
	b.mu.Unlock()
	for _, h := range handlers {
		h(payload)
	}
	return nil
}
func (b *fakeBus) Run(ctx context.Context) error { <-ctx.Done(); return nil }
func (b *fakeBus) Close() error                  { return nil }

type recordingClient struct{ frames []*guardianpb.Frame }

func (c *recordingClient) SendFrame(f *guardianpb.Frame) error {
	c.frames = append(c.frames, f)
	return nil
}
func (c *recordingClient) Close(string) {}

type recordingAdmin struct{ events []AdminEvent }

func (a *recordingAdmin) SendEvent(ev AdminEvent) { a.events = append(a.events, ev) }

func twoReplicas() (*Hub, *Hub) {
	shared := newFakeBus()
	a, b := New(), New()
	a.AttachBus(shared, "A")
	b.AttachBus(shared, "B")
	return a, b
}

func TestSendToClientCrossesReplicas(t *testing.T) {
	a, b := twoReplicas()
	sink := &recordingClient{}
	b.AttachClient("c1", sink) // the device WS lives on replica B

	// Replica A pushes to c1; it has no local sink, so it must publish and B delivers.
	if !a.SendToClient("c1", &guardianpb.Frame{}, true) {
		t.Fatal("SendToClient should report delivered (published to bus)")
	}
	if len(sink.frames) != 1 {
		t.Fatalf("replica B sink got %d frames, want 1", len(sink.frames))
	}
}

func TestSendToClientOfflineNoBusTarget(t *testing.T) {
	a, _ := twoReplicas()
	// c1 is connected nowhere; online=false means it must not be reported delivered.
	if a.SendToClient("c1", &guardianpb.Frame{}, false) {
		t.Fatal("SendToClient to an offline client should return false")
	}
}

func TestFanoutToAdminCrossesReplicasOnce(t *testing.T) {
	a, b := twoReplicas()
	adminA := &recordingAdmin{}
	adminB := &recordingAdmin{}
	a.AttachAdmin(7, adminA)
	b.AttachAdmin(7, adminB)

	a.FanoutToAdmin(7, AdminEvent{ClientID: "c1", Kind: "status_update"})

	// The publisher delivers to its own admin locally; the other replica delivers
	// via the bus. Neither must double-deliver.
	if len(adminA.events) != 1 {
		t.Fatalf("admin on publishing replica got %d events, want 1", len(adminA.events))
	}
	if len(adminB.events) != 1 {
		t.Fatalf("admin on other replica got %d events, want 1", len(adminB.events))
	}
}

func TestBroadcastToAdminsReachesAllReplicas(t *testing.T) {
	a, b := twoReplicas()
	adminA := &recordingAdmin{}
	adminB := &recordingAdmin{}
	a.AttachAdmin(1, adminA)
	b.AttachAdmin(2, adminB)

	a.BroadcastToAdmins(AdminEvent{Kind: "stats_update"})

	if len(adminA.events) != 1 || len(adminB.events) != 1 {
		t.Fatalf("broadcast reached A=%d B=%d, want 1/1", len(adminA.events), len(adminB.events))
	}
}
