package storage

import (
	"time"

	"v.wingsnet.org/internal/storage/dbmodel"
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
	row := dbmodel.PendingCommand{
		ClientID:       clientID,
		CommandType:    int64(commandType),
		SubscriptionID: subscriptionID,
		QueuedAt:       now.UnixMilli(),
		ExpiresAt:      expires.UnixMilli(),
	}
	if err := s.gdb.Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

// EnqueuePendingCommandDedup inserts only when no unexpired row with the same
// (client_id, command_type, subscription_id) tuple already exists. Returns
// (id, true) on insert, (0, false) when an existing row covered the request.
func (s *Store) EnqueuePendingCommandDedup(clientID string, commandType int32, subscriptionID string) (int64, bool, error) {
	now := time.Now().UTC().UnixMilli()
	var count int64
	if err := s.gdb.Model(&dbmodel.PendingCommand{}).
		Where("client_id = ? AND command_type = ? AND subscription_id = ? AND expires_at > ?",
			clientID, commandType, subscriptionID, now).
		Count(&count).Error; err != nil {
		return 0, false, err
	}
	if count > 0 {
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
	if err := s.gdb.Where("expires_at <= ?", now).Delete(&dbmodel.PendingCommand{}).Error; err != nil {
		return nil, err
	}
	var rows []dbmodel.PendingCommand
	if err := s.gdb.Where("client_id = ?", clientID).Order("queued_at ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	var out []PendingCommand
	for _, r := range rows {
		out = append(out, PendingCommand{
			ID:             r.ID,
			ClientID:       r.ClientID,
			CommandType:    int32(r.CommandType),
			SubscriptionID: r.SubscriptionID,
			QueuedAt:       time.UnixMilli(r.QueuedAt).UTC(),
			ExpiresAt:      time.UnixMilli(r.ExpiresAt).UTC(),
		})
	}
	if len(out) > 0 {
		if err := s.gdb.Where("client_id = ?", clientID).Delete(&dbmodel.PendingCommand{}).Error; err != nil {
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
	var count int64
	if err := s.gdb.Model(&dbmodel.PendingCommand{}).
		Where("client_id = ? AND command_type = ? AND expires_at > ?", clientID, commandType, now).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return int(count), nil
}
