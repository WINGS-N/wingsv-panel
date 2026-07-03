package storage

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"v.wingsnet.org/internal/storage/dbmodel"
)

// InsertTrafficSample appends one time-series point for a node.
func (s *Store) InsertTrafficSample(sample dbmodel.TrafficSample) error {
	if sample.TsUnix == 0 {
		sample.TsUnix = time.Now().Unix()
	}
	return s.gdb.Create(&sample).Error
}

// ListTrafficSamples returns a node's samples at or after sinceUnix, oldest first,
// for a sparkline / time-series render.
func (s *Store) ListTrafficSamples(nodeID string, sinceUnix int64) ([]dbmodel.TrafficSample, error) {
	var samples []dbmodel.TrafficSample
	err := s.gdb.
		Where("node_id = ? AND ts >= ?", nodeID, sinceUnix).
		Order("ts asc").
		Find(&samples).Error
	if err != nil {
		return nil, err
	}
	return samples, nil
}

// LatestTrafficSample returns a node's most recent sample, or ErrNotFound.
func (s *Store) LatestTrafficSample(nodeID string) (dbmodel.TrafficSample, error) {
	var sample dbmodel.TrafficSample
	err := s.gdb.Where("node_id = ?", nodeID).Order("ts desc").First(&sample).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return dbmodel.TrafficSample{}, ErrNotFound
	}
	if err != nil {
		return dbmodel.TrafficSample{}, err
	}
	return sample, nil
}

// PruneTrafficBefore drops samples older than cutoff.
func (s *Store) PruneTrafficBefore(cutoff time.Time) error {
	return s.gdb.Where("ts < ?", cutoff.Unix()).Delete(&dbmodel.TrafficSample{}).Error
}

// ReplaceFlows swaps a node's live-flow snapshot for the freshly polled set in one
// transaction so the connection-chain view never reads a half-written state.
func (s *Store) ReplaceFlows(nodeID string, flows []dbmodel.FlowSnapshot) error {
	return s.gdb.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("node_id = ?", nodeID).Delete(&dbmodel.FlowSnapshot{}).Error; err != nil {
			return err
		}
		if len(flows) == 0 {
			return nil
		}
		return tx.CreateInBatches(&flows, 100).Error
	})
}

// ListFlows returns every current live flow across nodes, busiest first.
func (s *Store) ListFlows() ([]dbmodel.FlowSnapshot, error) {
	var flows []dbmodel.FlowSnapshot
	err := s.gdb.Order("rx_rate + tx_rate desc").Find(&flows).Error
	if err != nil {
		return nil, err
	}
	return flows, nil
}

// RecordConnections upserts the connection-log history for a batch of flows: a
// first sighting inserts with first_seen == last_seen; a repeat refreshes last_seen
// and the byte counters. Identity is (node, session, stream, started).
func (s *Store) RecordConnections(rows []dbmodel.ConnectionLog) error {
	if len(rows) == 0 {
		return nil
	}
	now := time.Now().Unix()
	for i := range rows {
		if rows[i].FirstSeenUnix == 0 {
			rows[i].FirstSeenUnix = now
		}
		if rows[i].LastSeenUnix == 0 {
			rows[i].LastSeenUnix = now
		}
	}
	return s.gdb.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "node_id"}, {Name: "session_id"}, {Name: "stream_id"}, {Name: "started_unix"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"client_ip", "remote", "protocol", "rx_bytes", "tx_bytes", "last_seen",
		}),
	}).Create(&rows).Error
}

// ListConnectionLog returns recent connections newest-first, capped at limit.
func (s *Store) ListConnectionLog(limit int) ([]dbmodel.ConnectionLog, error) {
	if limit <= 0 {
		limit = 100
	}
	var rows []dbmodel.ConnectionLog
	err := s.gdb.Order("last_seen desc").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// PruneConnectionsBefore drops connection-log rows last seen before cutoff.
func (s *Store) PruneConnectionsBefore(cutoff time.Time) error {
	return s.gdb.Where("last_seen < ?", cutoff.Unix()).Delete(&dbmodel.ConnectionLog{}).Error
}
