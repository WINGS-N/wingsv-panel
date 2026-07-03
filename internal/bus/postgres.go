package bus

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// pgChannel is the single LISTEN/NOTIFY channel; the row id travels in the NOTIFY
// payload and the logical channel + bytes live in the bus_messages row.
const pgChannel = "wingsv_bus"

// Postgres is a Bus over the shared HA database. Publish inserts the payload and
// notifies with its row id (NOTIFY payloads are capped ~8KB, so the payload is
// stored, not carried). Every replica LISTENs, reads the row and dispatches to
// its local handlers; a retention prune drops old rows.
type Postgres struct {
	dsn      string
	pool     *pgxpool.Pool
	mu       sync.RWMutex
	handlers map[string][]func([]byte)
}

func NewPostgres(ctx context.Context, dsn string) (*Postgres, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	b := &Postgres{dsn: dsn, pool: pool, handlers: map[string][]func([]byte){}}
	if err := b.ensureTable(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return b, nil
}

func (b *Postgres) ensureTable(ctx context.Context) error {
	_, err := b.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS bus_messages (
			id BIGSERIAL PRIMARY KEY,
			channel TEXT NOT NULL,
			payload BYTEA NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`)
	return err
}

func (b *Postgres) Subscribe(channel string, handler func([]byte)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[channel] = append(b.handlers[channel], handler)
}

func (b *Postgres) Publish(ctx context.Context, channel string, payload []byte) error {
	var id int64
	err := b.pool.QueryRow(ctx,
		`INSERT INTO bus_messages (channel, payload) VALUES ($1, $2) RETURNING id`,
		channel, payload).Scan(&id)
	if err != nil {
		return err
	}
	_, err = b.pool.Exec(ctx, `SELECT pg_notify($1, $2)`, pgChannel, fmt.Sprint(id))
	return err
}

func (b *Postgres) Run(ctx context.Context) error {
	conn, err := pgx.Connect(ctx, b.dsn)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(context.Background()) }()
	if _, err := conn.Exec(ctx, "LISTEN "+pgChannel); err != nil {
		return err
	}
	go b.pruneLoop(ctx)
	for {
		notification, err := conn.WaitForNotification(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		b.dispatch(ctx, notification.Payload)
	}
}

func (b *Postgres) dispatch(ctx context.Context, idStr string) {
	var channel string
	var payload []byte
	if err := b.pool.QueryRow(ctx,
		`SELECT channel, payload FROM bus_messages WHERE id = $1::bigint`, idStr).Scan(&channel, &payload); err != nil {
		return
	}
	b.mu.RLock()
	handlers := append([]func([]byte){}, b.handlers[channel]...)
	b.mu.RUnlock()
	for _, h := range handlers {
		h(payload)
	}
}

func (b *Postgres) pruneLoop(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = b.pool.Exec(ctx, `DELETE FROM bus_messages WHERE created_at < now() - interval '120 seconds'`)
		}
	}
}

func (b *Postgres) Close() error {
	b.pool.Close()
	return nil
}
