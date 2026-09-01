package admin

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"google.golang.org/protobuf/proto"

	"v.wingsnet.org/internal/gen/wingsvpb"
	"v.wingsnet.org/internal/preview"
	"v.wingsnet.org/internal/storage"
)

// federationSubscriptionTitle - как подписка называется в приложении
const federationSubscriptionTitle = "WINGS Federation"

// federationUserID - как участник называется голове федерации. Никакого имени,
// почты или адреса: голова про человека не знает ничего и знать не должна
func federationUserID(admin storage.Admin) string {
	return "user-" + strconv.FormatInt(admin.ID, 10)
}

// handlePanelAccess открывает участнику админ-панель.
//
// По умолчанию разрешения ни у кого спрашивать не надо: тот, кто уже в дереве,
// вправе вести собственных клиентов. Модерация включается настройкой платформы,
// и тогда просьба уходит владельцу
func (h *Handler) handlePanelAccess(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if admin.PanelAccess || admin.Role == storage.RoleOwner {
		writeError(w, http.StatusBadRequest, "панель у вас уже открыта")
		return
	}
	moderated, err := h.store.GetPlatformSetting(storage.SettingPanelByRequest, "false")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if moderated == "true" {
		if err := h.store.RequestPanelAccess(admin.ID); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		_ = h.store.AppendAudit(storage.AuditEntry{
			ActorAdminID: admin.ID, ActorUsername: admin.Username,
			Action: "admin.panel_requested", IP: clientIP(r),
		})
		writeJSON(w, http.StatusOK, map[string]any{"granted": false, "requested": true})
		return
	}
	if err := h.store.SetPanelAccess(admin.ID, true); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = h.store.ClearPanelRequest(admin.ID)
	_ = h.store.AppendAudit(storage.AuditEntry{
		ActorAdminID: admin.ID, ActorUsername: admin.Username,
		Action: "admin.panel_self_granted", IP: clientIP(r),
	})
	writeJSON(w, http.StatusOK, map[string]any{"granted": true, "requested": false})
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
		"nodes_entitled":   got.GetNodesEntitled(),
		"used_bytes":       got.GetUsedBytes(),
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
	// Ссылка для приложения: оно понимает wingsv:// и заводит подписку само,
	// без ручного копирования адреса
	if link, err := federationImportLink(got.GetSubscriptionUrl()); err == nil {
		out["import_link"] = link
	}
	writeJSON(w, http.StatusOK, out)
}

// federationImportLink собирает ссылку, по которой приложение заводит подписку
// федерации у себя
func federationImportLink(subscriptionURL string) (string, error) {
	if subscriptionURL == "" {
		return "", errors.New("нет подписки")
	}
	cfg := &wingsvpb.Config{
		Ver:  1,
		Type: wingsvpb.ConfigType_CONFIG_TYPE_XRAY,
		Xray: &wingsvpb.Xray{
			MergeOnly: proto.Bool(true),
			Subscriptions: []*wingsvpb.Subscription{{
				Id:         "wings-federation",
				Title:      federationSubscriptionTitle,
				Url:        subscriptionURL,
				FormatHint: "auto",
				AutoUpdate: proto.Bool(true),
			}},
		},
	}
	return preview.BuildWingsLink(cfg)
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
