package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"v.wingsnet.org/internal/auth"
	"v.wingsnet.org/internal/gen/headpb"
	"v.wingsnet.org/internal/storage"
)

// maxInviteUses bounds a group code. Big enough for a team, small enough that a
// leaked code is a contained problem rather than an open door.
const maxInviteUses = 50

// Порог для приглашающего: сколько обещано, сколько реально прокачано и сколько
// нода прожила в федерации. Обещание проверяется вместе с расходом - иначе
// хватило бы объявить терабайт и не отдать ничего
const (
	minPledgedBytes = 5 << 30
	minDonatedBytes = 1 << 30
	minNodeAge      = 72 * time.Hour
)

type inviteView struct {
	Token     string `json:"token"`
	CreatedAt string `json:"created_at"`
	ExpiresAt string `json:"expires_at"`
	MaxUses   int64  `json:"max_uses"`
	UseCount  int64  `json:"use_count"`
	Spent     bool   `json:"spent"`
	// Link is what actually gets sent to a person, built here so the panel does
	// not have to guess at the public address it is served on
	Link string `json:"link"`
}

// handleInvites lets any administrator invite somebody.
//
// The invite tree is the whole entry price: every admin is a branch of it, so
// growing the tree cannot be an owner-only privilege. What stays owner-only is
// cutting a branch off - handing out access and revoking somebody else's are
// not the same power.
func (h *Handler) handleInvites(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
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
		var req struct {
			TTLHours int   `json:"ttl_hours"`
			MaxUses  int64 `json:"max_uses"`
		}
		// Пустое тело - обычный одноразовый код без срока
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.MaxUses < 1 {
			req.MaxUses = 1
		}
		if req.MaxUses > maxInviteUses {
			writeError(w, http.StatusBadRequest,
				"по одному коду может зарегистрироваться не больше "+strconv.Itoa(maxInviteUses)+" человек")
			return
		}
		token, err := auth.GenerateInviteToken()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		var expiresAt time.Time
		if req.TTLHours > 0 {
			expiresAt = time.Now().Add(time.Duration(req.TTLHours) * time.Hour)
		}
		invite, err := h.store.CreateInviteWithUses(token, expiresAt, admin.ID, req.MaxUses)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		_ = h.store.AppendAudit(storage.AuditEntry{
			ActorAdminID: admin.ID, ActorUsername: admin.Username,
			Action: "admin.invite_created", TargetType: "invite", TargetID: token,
		})
		writeJSON(w, http.StatusCreated, h.inviteView(invite))

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// mayInvite decides who gets to grow the tree.
//
// Порог намеренно выше, чем "у меня есть сервер": обещание ничего не стоит, а
// домашний компьютер, поднятый на вечер ради инвайта, отсеивается стажем и
// реально прокачанным трафиком.
//
// Недоступная голова - это отказ, а не пропуск: иначе привилегия, на которой
// держится вся защита от ферм, раздаётся при первом же сбое сети.
func (h *Handler) mayInvite(r *http.Request, admin storage.Admin) error {
	if admin.Role == storage.RoleOwner {
		return nil
	}
	if !h.federationOn() {
		return errors.New("приглашать могут участники федерации, а она сейчас выключена")
	}
	ctx, cancel := context.WithTimeout(r.Context(), federationTimeout)
	defer cancel()
	donor := donorID(admin)
	summary, err := h.fed.DonorSummary(ctx, donor)
	if err != nil {
		return errors.New("не удалось проверить участие в федерации: " + err.Error())
	}
	if summary.GetNodes() == 0 {
		return errors.New("приглашать могут те, кто отдал в федерацию хотя бы один сервер")
	}
	if summary.GetDeclaredBudgetBytes() < minPledgedBytes {
		return errors.New("обещанного трафика мало: приглашения открываются от 5 GB в месяц")
	}
	if summary.GetUsedBytes() < minDonatedBytes {
		return errors.New("сервер отдан, но через него ещё почти никто не прошёл: нужен хотя бы 1 GB реально отданного трафика")
	}
	nodes, err := h.fed.ListNodes(ctx, donor)
	if err != nil {
		return errors.New("не удалось проверить участие в федерации: " + err.Error())
	}
	if !hasSeasonedNode(nodes.GetNodes()) {
		return errors.New("сервер слишком свежий: нужна нода, которая держится в федерации хотя бы 3 дня и сейчас на связи")
	}
	return nil
}

// hasSeasonedNode ищет ноду со стажем, которая сейчас на связи. Стаж и есть
// проверка аптайма: нода, отваливающаяся каждый день, его не наберёт
func hasSeasonedNode(nodes []*headpb.NodeSummary) bool {
	cutoff := time.Now().Add(-minNodeAge).Unix()
	for _, n := range nodes {
		if n.GetOnline() && n.GetJoinedUnix() > 0 && n.GetJoinedUnix() <= cutoff {
			return true
		}
	}
	return false
}

func (h *Handler) inviteView(it storage.InviteToken) inviteView {
	return inviteView{
		Token:     it.Token,
		CreatedAt: it.CreatedAt.Format(time.RFC3339),
		ExpiresAt: formatOptionalTime(it.ExpiresAt),
		MaxUses:   it.MaxUses,
		UseCount:  it.UseCount,
		Spent:     it.UseCount >= it.MaxUses,
		Link:      strings.TrimRight(h.cfg.PublicBaseURL, "/") + "/register?invite=" + it.Token,
	}
}

func formatOptionalTime(t time.Time) string {
	if t.IsZero() || t.UnixMilli() == 0 {
		return ""
	}
	return t.Format(time.RFC3339)
}

// inviterView is who is doing the inviting, shown to the person opening the
// link. Deliberately just a face and a name: an invite page is a greeting, not
// a place to leak how much traffic somebody moved.
type inviterView struct {
	Username      string `json:"username"`
	AvatarVersion int64  `json:"avatar_version"`
	AdminID       int64  `json:"admin_id"`
	Valid         bool   `json:"valid"`
	Reason        string `json:"reason,omitempty"`
	Remaining     int64  `json:"remaining"`
}

// handleInviteLookup answers "who invited me, and does this code still work".
//
// Open without a session on purpose - it is read by somebody who has no account
// yet. It never says whether a token merely exists: an expired code and a
// made-up one both come back as invalid, so the endpoint cannot be used to mine
// for live invites.
func (h *Handler) handleInviteLookup(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("invite"))
	if token == "" {
		writeError(w, http.StatusBadRequest, "missing invite")
		return
	}
	invite, err := h.store.FindInvite(token)
	if err != nil {
		writeJSON(w, http.StatusOK, inviterView{Valid: false, Reason: "код не найден или уже недействителен"})
		return
	}
	if invite.UseCount >= invite.MaxUses {
		writeJSON(w, http.StatusOK, inviterView{Valid: false, Reason: "по этому коду уже зарегистрировались"})
		return
	}
	if !invite.ExpiresAt.IsZero() && invite.ExpiresAt.UnixMilli() > 0 && time.Now().After(invite.ExpiresAt) {
		writeJSON(w, http.StatusOK, inviterView{Valid: false, Reason: "срок действия кода истёк"})
		return
	}
	view := inviterView{Valid: true, Remaining: invite.MaxUses - invite.UseCount}
	if admin, err := h.store.FindAdminByID(invite.CreatedByAdminID); err == nil {
		view.Username = admin.Username
		view.AvatarVersion = admin.AvatarVersion
		view.AdminID = admin.ID
	}
	writeJSON(w, http.StatusOK, view)
}
