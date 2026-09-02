package admin

import (
	"net/http"
	"strconv"
	"time"

	"v.wingsnet.org/internal/auth"
	"v.wingsnet.org/internal/storage"
)

// handleAppInvites - приглашения так, как их видит телефон.
//
// Отличие от панели одно: тут не спрашивают ни срок, ни число человек. Код
// показывают QR-кодом с экрана, его сканируют при встрече, и крутить эти ручки
// в такой момент некому.
func (h *Handler) handleAppInvites(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	switch r.Method {
	case http.MethodGet:
		invites, err := h.store.ListInvitesByAdmin(admin.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		out := make([]inviteView, 0, len(invites))
		for _, it := range invites {
			out = append(out, h.inviteView(it))
		}
		body := map[string]any{"invites": out, "may_invite": true}
		if err := h.mayInvite(r, admin); err != nil {
			body["may_invite"] = false
			body["reason"] = err.Error()
		}
		writeJSON(w, http.StatusOK, body)

	case http.MethodPost:
		if err := h.mayInvite(r, admin); err != nil {
			writeError(w, http.StatusForbidden, err.Error())
			return
		}
		token, err := auth.GenerateInviteCode()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		invite, err := h.store.CreateInviteWithUses(token, time.Time{}, admin.ID, maxInviteUses)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		_ = h.store.AppendAudit(storage.AuditEntry{
			ActorAdminID: admin.ID, ActorUsername: admin.Username,
			Action: "admin.invite_created", TargetType: "invite", TargetID: token, IP: clientIP(r),
		})
		writeJSON(w, http.StatusCreated, h.inviteView(invite))

	case http.MethodDelete:
		token := r.URL.Query().Get("token")
		if token == "" {
			writeError(w, http.StatusBadRequest, "не указан код")
			return
		}
		invite, err := h.store.FindInvite(token)
		if err != nil {
			writeError(w, http.StatusNotFound, "код не найден")
			return
		}
		if invite.CreatedByAdminID != admin.ID && admin.Role != storage.RoleOwner {
			writeError(w, http.StatusForbidden, "это не ваш код")
			return
		}
		if err := h.store.DeleteInvite(invite.Token); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		_ = h.store.AppendAudit(storage.AuditEntry{
			ActorAdminID: admin.ID, ActorUsername: admin.Username,
			Action: "admin.invite_revoked", TargetType: "invite", TargetID: invite.Token, IP: clientIP(r),
		})
		writeJSON(w, http.StatusOK, map[string]any{"revoked": true})

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

var _ = strconv.Itoa
