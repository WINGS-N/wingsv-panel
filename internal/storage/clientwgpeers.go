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

// ClientWGPeerView is a managed WireGuard peer enriched with its client and node
// names, for the WG-configs management screen.
type ClientWGPeerView struct {
	ClientID        string `json:"client_id"`
	ClientName      string `json:"client_name"`
	OwnerAdminID    int64  `json:"owner_admin_id"`
	NodeID          string `json:"node_id"`
	NodeName        string `json:"node_name"`
	PublicKey       string `json:"public_key"`
	AllowedIPs      string `json:"allowed_ips"`
	ServerPublicKey string `json:"server_public_key"`
	Endpoint        string `json:"endpoint"`
	CreatedAtUnix   int64  `json:"created_at"`
}

// ListClientWGPeersForOwner lists managed WireGuard peers, newest-first, joined
// with client and node names. all=true (owner) returns every peer; otherwise only
// peers of clients owned by ownerAdminID.
func (s *Store) ListClientWGPeersForOwner(ownerAdminID int64, all bool) ([]ClientWGPeerView, error) {
	q := s.gdb.Table("client_wg_peers AS p").
		Joins("JOIN clients AS c ON c.id = p.client_id").
		Joins("LEFT JOIN server_nodes AS n ON n.id = p.node_id").
		Select("p.client_id AS client_id, c.name AS client_name, c.owner_admin_id AS owner_admin_id, " +
			"p.node_id AS node_id, COALESCE(n.name, '') AS node_name, p.public_key AS public_key, " +
			"p.allowed_ips AS allowed_ips, p.server_public_key AS server_public_key, p.endpoint AS endpoint, " +
			"p.created_at AS created_at_unix").
		Order("p.created_at desc")
	if !all {
		q = q.Where("c.owner_admin_id = ?", ownerAdminID)
	}
	var rows []ClientWGPeerView
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// ProvisionedClientIDs returns the set of client ids that have at least one
// managed WireGuard peer, i.e. clients whose VK TURN provisioning is active. Used
// to gate wg-only UI (traffic) on the client list.
func (s *Store) ProvisionedClientIDs() (map[string]bool, error) {
	var ids []string
	if err := s.gdb.Table("client_wg_peers").Distinct("client_id").Pluck("client_id", &ids).Error; err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return set, nil
}

// XUIBackedPeer is a managed WireGuard peer whose provisioning node forwards to a
// 3x-ui inbound, so its traffic and online state live on the 3x-ui node rather
// than the relay. VKTurnNodeID is the vk-turn-proxy node the peer is keyed under
// (matching peer_traffic / ClientTrafficMap); RemoteControl distinguishes
// panel-controlled clients from provision-only ones.
type XUIBackedPeer struct {
	ClientID      string
	VKTurnNodeID  string
	PublicKey     string
	RemoteControl bool
}

// XUIBackedPeersForNode lists the managed peers whose vk-turn-proxy node uses the
// given 3x-ui node as its wg backend. The collector polls the 3x-ui node for
// their traffic/online instead of the (relay-only) main collector.
func (s *Store) XUIBackedPeersForNode(xuiNodeID string) ([]XUIBackedPeer, error) {
	var rows []XUIBackedPeer
	err := s.gdb.Table("client_wg_peers AS p").
		Joins("JOIN server_nodes AS n ON n.id = p.node_id").
		Joins("JOIN clients AS c ON c.id = p.client_id").
		Where("n.wg_backend = ? AND n.xui_node_id = ?", WGBackendXUI, xuiNodeID).
		Select("p.client_id AS client_id, p.node_id AS vk_turn_node_id, " +
			"p.public_key AS public_key, c.remote_control AS remote_control").
		Scan(&rows).Error
	return rows, err
}

// CountClientWGPeersForNode returns how many provisioned managed peers a node has.
// This is the panel's authoritative provisioned-peer count, backend-agnostic: it
// covers xui-backend nodes whose peers live on a 3x-ui node the relay cannot see.
func (s *Store) CountClientWGPeersForNode(nodeID string) int {
	var n int64
	s.gdb.Table("client_wg_peers").Where("node_id = ?", nodeID).Count(&n)
	return int(n)
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
