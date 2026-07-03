package storage

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"v.wingsnet.org/internal/storage/dbmodel"
)

// UpsertClientWGPeer stores (or refreshes) the peer a client holds on a node.
func (s *Store) UpsertClientWGPeer(peer dbmodel.ClientWGPeer) error {
	if peer.CreatedAtUnix == 0 {
		peer.CreatedAtUnix = time.Now().Unix()
	}
	return s.gdb.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "client_id"}, {Name: "node_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"public_key", "private_key", "allowed_ips", "server_public_key", "endpoint",
		}),
	}).Create(&peer).Error
}

// GetClientWGPeer returns the peer a client holds on a node, or ErrNotFound.
func (s *Store) GetClientWGPeer(clientID, nodeID string) (dbmodel.ClientWGPeer, error) {
	var peer dbmodel.ClientWGPeer
	err := s.gdb.Where("client_id = ? AND node_id = ?", clientID, nodeID).First(&peer).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return dbmodel.ClientWGPeer{}, ErrNotFound
	}
	if err != nil {
		return dbmodel.ClientWGPeer{}, err
	}
	return peer, nil
}

// ListClientWGPeers returns every peer a client holds across nodes.
func (s *Store) ListClientWGPeers(clientID string) ([]dbmodel.ClientWGPeer, error) {
	var peers []dbmodel.ClientWGPeer
	if err := s.gdb.Where("client_id = ?", clientID).Order("created_at asc").Find(&peers).Error; err != nil {
		return nil, err
	}
	return peers, nil
}

// DeleteClientWGPeer removes one client-node peer, reporting ErrNotFound when absent.
func (s *Store) DeleteClientWGPeer(clientID, nodeID string) error {
	res := s.gdb.Where("client_id = ? AND node_id = ?", clientID, nodeID).Delete(&dbmodel.ClientWGPeer{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
