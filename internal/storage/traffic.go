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

// ListTrafficSince returns every node's samples at or after sinceUnix, grouped by
// node then oldest-first, so the caller can delta each node's cumulative counters
// into a per-interval traffic series.
func (s *Store) ListTrafficSince(sinceUnix int64) ([]dbmodel.TrafficSample, error) {
	var samples []dbmodel.TrafficSample
	err := s.gdb.
		Where("ts >= ?", sinceUnix).
		Order("node_id asc, ts asc").
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

// ListFlowsForNodes returns the current live flows on the given nodes, busiest
// first. An empty node set returns no flows.
func (s *Store) ListFlowsForNodes(nodeIDs []string) ([]dbmodel.FlowSnapshot, error) {
	if len(nodeIDs) == 0 {
		return nil, nil
	}
	var flows []dbmodel.FlowSnapshot
	err := s.gdb.Where("node_id IN ?", nodeIDs).Order("rx_rate + tx_rate desc").Find(&flows).Error
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

// ListConnectionLogForNodes returns recent connections on the given nodes,
// newest-first, capped at limit. An empty node set returns nothing.
func (s *Store) ListConnectionLogForNodes(nodeIDs []string, limit int) ([]dbmodel.ConnectionLog, error) {
	if len(nodeIDs) == 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 100
	}
	var rows []dbmodel.ConnectionLog
	err := s.gdb.Where("node_id IN ?", nodeIDs).Order("last_seen desc").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// UpsertPeerTraffic refreshes the byte counters for a node's wg peers.
func (s *Store) UpsertPeerTraffic(rows []dbmodel.PeerTraffic) error {
	if len(rows) == 0 {
		return nil
	}
	return s.gdb.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "node_id"}, {Name: "public_key"}},
		DoUpdates: clause.AssignmentColumns([]string{"rx_bytes", "tx_bytes", "sampled_unix"}),
	}).Create(&rows).Error
}

// ClientTrafficMap sums peer traffic per managed client (joined via the peers a
// client holds across nodes), so the client list can render a traffic column in
// one query. Clients with no peers are absent from the map.
func (s *Store) ClientTrafficMap() (map[string][2]uint64, error) {
	var rows []struct {
		ClientID string
		Rx       int64
		Tx       int64
	}
	err := s.gdb.
		Table("client_wg_peers AS cwp").
		Joins("JOIN peer_traffic AS pt ON pt.node_id = cwp.node_id AND pt.public_key = cwp.public_key").
		Group("cwp.client_id").
		Select("cwp.client_id AS client_id, COALESCE(SUM(pt.rx_bytes),0) AS rx, COALESCE(SUM(pt.tx_bytes),0) AS tx").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[string][2]uint64, len(rows))
	for _, r := range rows {
		out[r.ClientID] = [2]uint64{uint64(r.Rx), uint64(r.Tx)}
	}
	return out, nil
}

// PruneConnectionsBefore drops connection-log rows last seen before cutoff.
func (s *Store) PruneConnectionsBefore(cutoff time.Time) error {
	return s.gdb.Where("last_seen < ?", cutoff.Unix()).Delete(&dbmodel.ConnectionLog{}).Error
}
