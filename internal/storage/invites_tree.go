package storage

import (
	"errors"
	"time"

	"v.wingsnet.org/internal/storage/dbmodel"
)

// maxInviteDepth bounds the walk. An invite tree cannot legitimately contain a
// cycle, but a malformed row should cost a bounded query rather than the process.
const maxInviteDepth = 64

// TreeMember is one admin's place in the invite tree.
type TreeMember struct {
	AdminID   int64
	Username  string
	Role      string
	Depth     int
	InvitedBy int64
	// Suspended is this admin's own state. Cut is whether anybody at or above
	// them is suspended, which is what actually decides whether they may act.
	Suspended bool
	Cut       bool
	Reason    string
	CreatedAt time.Time
	// AvatarVersion нужен, чтобы дерево показывало лица, а не одинаковые
	// заглушки: ноль означает, что аватар не загружали
	AvatarVersion int64
}

// inviteEdges reads the whole edge set. The tree is small - one row per admin
// who was ever invited - so walking it in Go beats a recursive CTE that has to
// be written three times for three dialects and tested against all of them.
func (s *Store) inviteEdges() (map[int64]int64, error) {
	type edge struct {
		CreatedByAdminID *int64
		UsedByAdminID    *int64
	}
	var rows []edge
	err := s.gdb.Model(&dbmodel.InviteToken{}).
		Where("used_by_admin_id IS NOT NULL AND created_by_admin_id IS NOT NULL").
		Select("created_by_admin_id", "used_by_admin_id").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	parent := make(map[int64]int64, len(rows))
	for _, r := range rows {
		child, by := derefInt64(r.UsedByAdminID), derefInt64(r.CreatedByAdminID)
		if child == 0 || by == 0 || child == by {
			continue
		}
		// First invite wins: an admin has one inviter, and a second row for the
		// same person is a data problem rather than a second parent.
		if _, seen := parent[child]; !seen {
			parent[child] = by
		}
	}
	return parent, nil
}

// InviteTree returns every admin with their place in the tree.
//
// An admin nobody invited is a root: the founding accounts predate invites
// entirely, and treating them as orphans would leave the tree empty.
func (s *Store) InviteTree() ([]TreeMember, error) {
	admins, err := s.ListAdmins()
	if err != nil {
		return nil, err
	}
	parent, err := s.inviteEdges()
	if err != nil {
		return nil, err
	}
	suspended, err := s.suspendedSet()
	if err != nil {
		return nil, err
	}

	out := make([]TreeMember, 0, len(admins))
	for _, a := range admins {
		member := TreeMember{
			AdminID:       a.ID,
			Username:      a.Username,
			Role:          a.Role,
			InvitedBy:     parent[a.ID],
			Suspended:     suspended[a.ID].suspended,
			Reason:        suspended[a.ID].reason,
			CreatedAt:     a.CreatedAt,
			AvatarVersion: a.AvatarVersion,
		}
		member.Depth, member.Cut = walkUp(a.ID, parent, suspended)
		out = append(out, member)
	}
	return out, nil
}

type suspension struct {
	suspended bool
	reason    string
}

func (s *Store) suspendedSet() (map[int64]suspension, error) {
	// Field names have to match the columns: gorm derives suspended_at_unix from
	// SuspendedAtUnix, which is not a column that exists
	type row struct {
		ID              int64
		SuspendedAt     int64
		SuspendedReason string
	}
	var rows []row
	err := s.gdb.Model(&dbmodel.Admin{}).
		Select("id", "suspended_at", "suspended_reason").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[int64]suspension, len(rows))
	for _, r := range rows {
		out[r.ID] = suspension{suspended: r.SuspendedAt > 0, reason: r.SuspendedReason}
	}
	return out, nil
}

// walkUp returns the depth and whether anybody at or above this admin is cut.
func walkUp(id int64, parent map[int64]int64, suspended map[int64]suspension) (int, bool) {
	depth, cut := 0, suspended[id].suspended
	seen := map[int64]bool{id: true}
	for at := id; depth < maxInviteDepth; depth++ {
		up, ok := parent[at]
		if !ok || seen[up] {
			break
		}
		if suspended[up].suspended {
			cut = true
		}
		seen[up] = true
		at = up
	}
	return depth, cut
}

// InviteSubtree lists an admin and everyone below them.
//
// This is what a branch cut operates on: the whole point is being able to take
// off a branch at an arbitrary point rather than only a leaf.
func (s *Store) InviteSubtree(rootAdminID int64) ([]int64, error) {
	parent, err := s.inviteEdges()
	if err != nil {
		return nil, err
	}
	children := make(map[int64][]int64, len(parent))
	for child, by := range parent {
		children[by] = append(children[by], child)
	}

	out := []int64{rootAdminID}
	seen := map[int64]bool{rootAdminID: true}
	for i := 0; i < len(out) && i < maxInviteDepth*len(parent)+1; i++ {
		for _, child := range children[out[i]] {
			if seen[child] {
				continue
			}
			seen[child] = true
			out = append(out, child)
		}
	}
	return out, nil
}

// ErrCannotSuspendOwner guards the obvious foot-gun: an owner who cuts their own
// branch locks everybody out of the panel including themselves.
var ErrCannotSuspendOwner = errors.New("storage: an owner cannot be suspended")

// SuspendSubtree cuts an admin and everyone they brought in.
//
// Existing sessions are dropped as well: a cut that leaves the branch logged in
// until their cookie expires is not a cut.
func (s *Store) SuspendSubtree(rootAdminID int64, reason string) (int, error) {
	root, err := s.FindAdminByID(rootAdminID)
	if err != nil {
		return 0, err
	}
	if root.Role == RoleOwner {
		return 0, ErrCannotSuspendOwner
	}
	ids, err := s.InviteSubtree(rootAdminID)
	if err != nil {
		return 0, err
	}
	now := time.Now().UTC().UnixMilli()
	res := s.gdb.Model(&dbmodel.Admin{}).
		Where("id IN ? AND suspended_at = 0 AND role <> ?", ids, RoleOwner).
		Updates(map[string]any{
			"suspended_at":      now,
			"suspended_reason":  reason,
			"suspended_by_root": rootAdminID,
			"updated_at":        now,
		})
	if res.Error != nil {
		return 0, res.Error
	}
	if err := s.gdb.Where("admin_id IN ?", ids).Delete(&dbmodel.AdminSession{}).Error; err != nil {
		return int(res.RowsAffected), err
	}
	return int(res.RowsAffected), nil
}

// RestoreSubtree lifts a cut, but only the one made at this point.
//
// Restoring somebody who is also under a separate, higher cut would quietly
// half-undo that other decision, so only the rows this branch cut are touched.
func (s *Store) RestoreSubtree(rootAdminID int64) (int, error) {
	now := time.Now().UTC().UnixMilli()
	res := s.gdb.Model(&dbmodel.Admin{}).
		Where("suspended_by_root = ?", rootAdminID).
		Updates(map[string]any{
			"suspended_at":      0,
			"suspended_reason":  "",
			"suspended_by_root": 0,
			"updated_at":        now,
		})
	if res.Error != nil {
		return 0, res.Error
	}
	return int(res.RowsAffected), nil
}

// IsSuspended reports whether this admin may act at all.
func (s *Store) IsSuspended(adminID int64) (bool, string, error) {
	type row struct {
		SuspendedAt     int64
		SuspendedReason string
	}
	var got row
	err := s.gdb.Model(&dbmodel.Admin{}).
		Select("suspended_at", "suspended_reason").
		Where("id = ?", adminID).Take(&got).Error
	if err != nil {
		return false, "", err
	}
	return got.SuspendedAt > 0, got.SuspendedReason, nil
}
