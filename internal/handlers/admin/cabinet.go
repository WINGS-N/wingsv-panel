package admin

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"v.wingsnet.org/internal/storage"
)

// federationUserID - как участник называется голове федерации. Никакого имени,
// почты или адреса: голова про человека не знает ничего и знать не должна
func federationUserID(admin storage.Admin) string {
	return "user-" + strconv.FormatInt(admin.ID, 10)
}

// handleMyAccess выдаёт участнику его собственный доступ.
//
// Вызов идемпотентен: голова возвращает уже выданное, если оно есть, поэтому
// экран можно открывать сколько угодно раз, никого при этом не переселяя
func (h *Handler) handleMyAccess(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !h.federationOn() {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), federationTimeout)
	defer cancel()
	if err := h.mayUseFederation(ctx, admin); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	got, err := h.fed.EnsureUser(ctx, federationUserID(admin))
	if err != nil {
		writeError(w, http.StatusBadGateway, "голова федерации недоступна: "+err.Error())
		return
	}
	out := map[string]any{
		"enabled":          true,
		"subscription_url": got.GetSubscriptionUrl(),
		"nodes":            got.GetNodes(),
		"sticky_until":     got.GetStickyUntilUnix(),
	}
	// Уровень доверия объясняет, почему серверов столько, а не иначе. Голова без
	// Oracle отвечает Unimplemented, и экран просто остаётся без этой части
	if subject, err := h.fed.OracleSubject(ctx, federationUserID(admin)); err == nil {
		s := subject.GetSubject()
		classes := make([]map[string]any, 0, len(s.GetClasses()))
		for _, c := range s.GetClasses() {
			classes = append(classes, map[string]any{"kind": c.GetKind(), "weight": c.GetWeight()})
		}
		out["trust"] = map[string]any{
			"confidence": s.GetConfidence(),
			"band":       s.GetBand(),
			"classes":    classes,
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// mayUseFederation решает, кому вообще положен бесплатный доступ.
//
// Дерево - это и есть цена входа: аккаунт, которого никто не приглашал и
// который ничего не отдал, в федерации не участвует
func (h *Handler) mayUseFederation(ctx context.Context, admin storage.Admin) error {
	if admin.Role == storage.RoleOwner {
		return nil
	}
	if redeemed, err := h.store.RedeemedInvite(admin.ID); err == nil && redeemed {
		return nil
	}
	// Донор - корень собственной ветки: его никто не приглашал, но он отдал
	// сервер, и это тот же вклад
	summary, err := h.fed.DonorSummary(ctx, donorID(admin))
	if err == nil && summary.GetNodes() > 0 {
		return nil
	}
	return errors.New("бесплатный доступ выдаётся по приглашению: введите код на экране приглашений")
}
