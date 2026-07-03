// Package bus is a cross-replica message bus so a multi-replica panel can deliver
// admin fanout and device ConfigPush to whichever replica holds the target WS.
// The Postgres implementation rides the shared HA database (LISTEN/NOTIFY over an
// outbox table); single-node sqlite deploys use the no-op bus and stay in-process.
package bus

import "context"

// Bus delivers a payload published on a logical channel to every subscribed
// replica, including the publisher. Register handlers with Subscribe before
// Run; Publish is safe to call concurrently once Run is going.
type Bus interface {
	Subscribe(channel string, handler func(payload []byte))
	Publish(ctx context.Context, channel string, payload []byte) error
	Run(ctx context.Context) error
	Close() error
}

// Nop is a no-op bus for single-node deploys: nothing crosses replicas because
// there is only one. The hub falls back to purely local delivery with it.
type Nop struct{}

func (Nop) Subscribe(string, func([]byte)) {}
func (Nop) Publish(context.Context, string, []byte) error {
	return nil
}
func (Nop) Run(ctx context.Context) error {
	<-ctx.Done()
	return nil
}
func (Nop) Close() error { return nil }
