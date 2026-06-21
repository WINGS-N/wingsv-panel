package storage

import (
	"time"
)

// PendingCommand is a guardian command queued for delivery the next time the
// device hits welcome. command_type is the int32 value of the guardianpb
// CommandType enum; we don't import the proto package here to keep storage
// free of protobuf dependencies.
type PendingCommand struct {
	ID             int64
	ClientID       string
	CommandType    int32
	SubscriptionID string
	QueuedAt       time.Time
	ExpiresAt      time.Time
}

const defaultPendingCommandTTL = 24 * time.Hour

// EnqueuePendingCommand inserts a new pending command row. Each call inserts a
// separate row even when (client, type, subscription) matches an existing row
// — call EnqueuePendingCommandDedup when the caller wants idempotency.
func (s *Store) EnqueuePendingCommand(clientID string, commandType int32, subscriptionID string) (int64, error) {
	return s.EnqueuePendingCommandWithTTL(clientID, commandType, subscriptionID, defaultPendingCommandTTL)
}

// EnqueuePendingCommandWithTTL is like EnqueuePendingCommand but with a
// caller-supplied TTL window.
func (s *Store) EnqueuePendingCommandWithTTL(clientID string, commandType int32, subscriptionID string, ttl time.Duration) (int64, error) {
	now := time.Now().UTC()
	expires := now.Add(ttl)
	res, err := s.db.Exec(
		`INSERT INTO pending_commands (client_id, command_type, subscription_id, queued_at, expires_at) VALUES (?, ?, ?, ?, ?)`,
		clientID, commandType, subscriptionID, now.UnixMilli(), expires.UnixMilli(),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// EnqueuePendingCommandDedup inserts only when no unexpired row with the same
// (client_id, command_type, subscription_id) tuple already exists. Returns
// (id, true) on insert, (0, false) when an existing row covered the request.
func (s *Store) EnqueuePendingCommandDedup(clientID string, commandType int32, subscriptionID string) (int64, bool, error) {
	now := time.Now().UTC().UnixMilli()
	row := s.db.QueryRow(
		`SELECT id FROM pending_commands WHERE client_id = ? AND command_type = ? AND subscription_id = ? AND expires_at > ? LIMIT 1`,
		clientID, commandType, subscriptionID, now,
	)
	var existing int64
	if err := row.Scan(&existing); err == nil {
		return 0, false, nil
	}
	id, err := s.EnqueuePendingCommand(clientID, commandType, subscriptionID)
	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}

// DrainPendingCommands returns (and removes) all unexpired pending commands
// for the given client, ordered by queued_at. Expired rows are dropped as a
// side effect.
func (s *Store) DrainPendingCommands(clientID string) ([]PendingCommand, error) {
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(`DELETE FROM pending_commands WHERE expires_at <= ?`, now)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT id, client_id, command_type, subscription_id, queued_at, expires_at FROM pending_commands WHERE client_id = ? ORDER BY queued_at ASC`,
		clientID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PendingCommand
	for rows.Next() {
		var pc PendingCommand
		var queuedAt, expiresAt int64
		if err := rows.Scan(&pc.ID, &pc.ClientID, &pc.CommandType, &pc.SubscriptionID, &queuedAt, &expiresAt); err != nil {
			return nil, err
		}
		pc.QueuedAt = time.UnixMilli(queuedAt).UTC()
		pc.ExpiresAt = time.UnixMilli(expiresAt).UTC()
		out = append(out, pc)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) > 0 {
		_, err := s.db.Exec(`DELETE FROM pending_commands WHERE client_id = ?`, clientID)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// CountPendingCommands reports how many unexpired pending commands of the
// given type sit in queue for a client. Used by the admin API to surface
// queue length back to the panel UI.
func (s *Store) CountPendingCommands(clientID string, commandType int32) (int, error) {
	now := time.Now().UTC().UnixMilli()
	row := s.db.QueryRow(
		`SELECT COUNT(*) FROM pending_commands WHERE client_id = ? AND command_type = ? AND expires_at > ?`,
		clientID, commandType, now,
	)
	var n int
	if err := row.Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}
