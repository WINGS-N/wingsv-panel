package bus

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestPostgresRoundTrip verifies real LISTEN/NOTIFY delivery across two Bus
// instances (two "replicas") on the same database. Skipped unless a DSN is set:
//
//	WINGSV_TEST_PG_DSN=postgres://user:pass@host:5432/db go test ./internal/bus/
func TestPostgresRoundTrip(t *testing.T) {
	dsn := os.Getenv("WINGSV_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("set WINGSV_TEST_PG_DSN to run the Postgres bus integration test")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	publisher, err := NewPostgres(ctx, dsn)
	if err != nil {
		t.Fatalf("NewPostgres publisher: %v", err)
	}
	defer func() { _ = publisher.Close() }()
	subscriber, err := NewPostgres(ctx, dsn)
	if err != nil {
		t.Fatalf("NewPostgres subscriber: %v", err)
	}
	defer func() { _ = subscriber.Close() }()

	got := make(chan []byte, 1)
	subscriber.Subscribe("test.channel", func(payload []byte) { got <- payload })
	go func() { _ = subscriber.Run(ctx) }()
	time.Sleep(300 * time.Millisecond) // let the LISTEN establish

	// A payload larger than the 8KB NOTIFY limit proves the outbox path.
	big := make([]byte, 20000)
	for i := range big {
		big[i] = byte(i % 251)
	}
	if err := publisher.Publish(ctx, "test.channel", big); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case payload := <-got:
		if len(payload) != len(big) || payload[19999] != big[19999] {
			t.Fatalf("payload mismatch: got %d bytes", len(payload))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the cross-instance notification")
	}
}
