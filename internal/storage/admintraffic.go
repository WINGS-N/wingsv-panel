package storage

import (
	"time"

	"gorm.io/gorm/clause"

	"v.wingsnet.org/internal/storage/dbmodel"
)

// ownRow is what one admin's own clients moved, before the tree is walked
type ownRow struct {
	OwnerAdminID int64
	Bytes        int64
	Clients      int64
}

// AdminTraffic is one member's usage and the usage of everybody they brought in.
type AdminTraffic struct {
	AdminID        int64
	OwnBytes       uint64
	SubtreeBytes   uint64
	OwnClients     int64
	SubtreeClients int64
	SubtreeAdmins  int64
	UpdatedAt      time.Time
}

// RollupAdminTraffic recomputes what every member of the tree has moved.
//
// One number per client already exists: client_traffic.used_bytes is fed from
// peer_traffic for both backends - the 3x-ui poll writes its clients into
// peer_traffic too - so adding the two tables together would count the same
// bytes twice.
//
// Run from a slow ticker. It reads every client and walks the whole tree, which
// is cheap at this size and hopeless to do on a page load.
func (s *Store) RollupAdminTraffic() error {
	var owned []ownRow
	err := s.gdb.
		Table("clients AS c").
		Joins("LEFT JOIN client_traffic AS ct ON ct.client_id = c.id").
		Group("c.owner_admin_id").
		Select("c.owner_admin_id AS owner_admin_id, " +
			"COALESCE(SUM(ct.used_bytes),0) AS bytes, COUNT(c.id) AS clients").
		Scan(&owned).Error
	if err != nil {
		return err
	}

	own := make(map[int64]ownRow, len(owned))
	for _, r := range owned {
		own[r.OwnerAdminID] = r
	}

	parent, err := s.inviteEdges()
	if err != nil {
		return err
	}
	admins, err := s.ListAdmins()
	if err != nil {
		return err
	}
	children := make(map[int64][]int64, len(parent))
	for child, by := range parent {
		children[by] = append(children[by], child)
	}

	now := time.Now().UTC().UnixMilli()
	rows := make([]dbmodel.AdminTrafficRollup, 0, len(admins))
	for _, a := range admins {
		bytes, clients, members := subtreeTotals(a.ID, children, own)
		rows = append(rows, dbmodel.AdminTrafficRollup{
			AdminID:        a.ID,
			OwnBytes:       uint64(own[a.ID].Bytes),
			SubtreeBytes:   bytes,
			OwnClients:     own[a.ID].Clients,
			SubtreeClients: clients,
			// The member themselves is not counted: the number answers "how many
			// people did you bring in", not "how many people are you plus you"
			SubtreeAdmins: members - 1,
			UpdatedAtUnix: now,
		})
	}
	if len(rows) == 0 {
		return nil
	}
	return s.gdb.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "admin_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"own_bytes", "subtree_bytes", "own_clients", "subtree_clients",
			"subtree_admins", "updated_at",
		}),
	}).Create(&rows).Error
}

// subtreeTotals sums an admin and everyone below them, breadth first with a seen
// set so a malformed edge cannot loop.
func subtreeTotals(root int64, children map[int64][]int64, own map[int64]ownRow) (uint64, int64, int64) {
	var bytes uint64
	var clients, members int64
	queue := []int64{root}
	seen := map[int64]bool{root: true}
	for i := 0; i < len(queue); i++ {
		at := queue[i]
		members++
		bytes += uint64(own[at].Bytes)
		clients += own[at].Clients
		for _, child := range children[at] {
			if seen[child] {
				continue
			}
			seen[child] = true
			queue = append(queue, child)
		}
	}
	return bytes, clients, members
}

// AdminTrafficMap is the rollup keyed by admin, for rendering the tree.
func (s *Store) AdminTrafficMap() (map[int64]AdminTraffic, error) {
	var rows []dbmodel.AdminTrafficRollup
	if err := s.gdb.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[int64]AdminTraffic, len(rows))
	for _, r := range rows {
		out[r.AdminID] = AdminTraffic{
			AdminID:        r.AdminID,
			OwnBytes:       r.OwnBytes,
			SubtreeBytes:   r.SubtreeBytes,
			OwnClients:     r.OwnClients,
			SubtreeClients: r.SubtreeClients,
			SubtreeAdmins:  r.SubtreeAdmins,
			UpdatedAt:      time.UnixMilli(r.UpdatedAtUnix).UTC(),
		}
	}
	return out, nil
}
