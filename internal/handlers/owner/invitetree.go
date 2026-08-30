package owner

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"v.wingsnet.org/internal/storage"
)

type treeMemberView struct {
	AdminID   int64  `json:"admin_id"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	Depth     int    `json:"depth"`
	InvitedBy int64  `json:"invited_by"`
	// Suspended is this admin's own cut; Cut is whether anybody at or above them
	// is suspended, which is what actually decides whether they may act.
	Suspended bool   `json:"suspended"`
	Cut       bool   `json:"cut"`
	Reason    string `json:"reason"`
	CreatedAt int64  `json:"created_at"`
	// Usage is monitoring only. A subtree total never cuts anybody off: a
	// personal limit does that, and holding somebody responsible for what people
	// they invited a year ago are doing is not a rule anybody could live with.
	OwnBytes       uint64 `json:"own_bytes"`
	SubtreeBytes   uint64 `json:"subtree_bytes"`
	OwnClients     int64  `json:"own_clients"`
	SubtreeClients int64  `json:"subtree_clients"`
	SubtreeAdmins  int64  `json:"subtree_admins"`
}

// handleInviteTree answers who brought whom in.
//
// An admin nobody invited is a root: the founding accounts predate invites
// entirely, and treating them as orphans would show an empty tree.
func (h *Handler) handleInviteTree(w http.ResponseWriter, r *http.Request, _ storage.Admin) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	members, err := h.store.InviteTree()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// A rollup that has not run yet is not an error: the tree is still worth
	// showing, just without the numbers
	usage, err := h.store.AdminTrafficMap()
	if err != nil {
		usage = map[int64]storage.AdminTraffic{}
	}
	out := make([]treeMemberView, 0, len(members))
	for _, m := range members {
		out = append(out, treeMemberView{
			AdminID:   m.AdminID,
			Username:  m.Username,
			Role:      m.Role,
			Depth:     m.Depth,
			InvitedBy: m.InvitedBy,
			Suspended: m.Suspended,
			Cut:       m.Cut,
			Reason:    m.Reason,
			CreatedAt: m.CreatedAt.Unix(),

			OwnBytes:       usage[m.AdminID].OwnBytes,
			SubtreeBytes:   usage[m.AdminID].SubtreeBytes,
			OwnClients:     usage[m.AdminID].OwnClients,
			SubtreeClients: usage[m.AdminID].SubtreeClients,
			SubtreeAdmins:  usage[m.AdminID].SubtreeAdmins,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"members": out})
}

// handleInviteBranch cuts or restores a branch at an arbitrary point.
//
// Arbitrary point is the whole idea: an abusive admin who invited a dozen more
// is not dealt with by removing one leaf.
func (h *Handler) handleInviteBranch(w http.ResponseWriter, r *http.Request, owner storage.Admin) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/owner/invite-tree/")
	idPart, action, found := strings.Cut(rest, "/")
	if !found || (action != "cut" && action != "restore") {
		writeError(w, http.StatusNotFound, "unknown action")
		return
	}
	adminID, err := strconv.ParseInt(idPart, 10, 64)
	if err != nil || adminID <= 0 {
		writeError(w, http.StatusBadRequest, "bad admin id")
		return
	}
	if adminID == owner.ID {
		writeError(w, http.StatusBadRequest, "cutting your own branch would lock you out")
		return
	}

	var req struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	var affected int
	reason := strings.TrimSpace(req.Reason)
	if action == "cut" {
		affected, err = h.store.SuspendSubtree(adminID, reason)
	} else {
		affected, err = h.store.RestoreSubtree(adminID)
	}
	switch {
	case errors.Is(err, storage.ErrCannotSuspendOwner):
		writeError(w, http.StatusBadRequest, "an owner cannot be cut out of the tree")
		return
	case errors.Is(err, storage.ErrNotFound):
		writeError(w, http.StatusNotFound, "no such admin")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Cutting a branch takes accounts away from people. It is exactly the kind of
	// decision that has to be answerable for afterwards
	_ = h.store.AppendAudit(storage.AuditEntry{
		ActorAdminID: owner.ID, ActorUsername: owner.Username,
		Action:     "owner.invite_branch_" + action,
		TargetType: "admin", TargetID: strconv.FormatInt(adminID, 10),
		Message: strconv.Itoa(affected) + " accounts: " + reason,
	})
	writeJSON(w, http.StatusOK, map[string]any{"affected": affected})
}
