package admin

// Пароль, второй фактор и аватар живут там, где живёт вход. Настроен общий
// сервис учёток - правим в нём; не настроен - панель остаётся сама себе
// хозяйкой, и админ, поднявший её у себя, ничего этого даже не заметит

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"v.wingsnet.org/internal/storage"
)

// accountOf говорит, ведёт ли этот человек свои дела через общий сервис учёток
func (h *Handler) accountOf(admin storage.Admin) (string, bool) {
	if h.session == nil || !h.session.Enabled() {
		return "", false
	}
	subject, err := h.store.AccountSubjectOf(admin.ID)
	if err != nil || strings.TrimSpace(subject) == "" {
		return "", false
	}
	return subject, true
}

// freshAccountToken отдаёт живой ключ человека, обновив протухший.
//
// Ключ выдают на часы, а аватар меняют когда захотят: без обновления панель
// упиралась бы в "срок вышел" уже к вечеру того же дня
func (h *Handler) freshAccountToken(ctx context.Context, admin storage.Admin) (string, error) {
	tokens, err := h.store.AccountTokensOf(admin.ID)
	if err != nil {
		return "", err
	}
	if tokens.Access != "" && time.Now().Before(tokens.ExpiresAt.Add(-time.Minute)) {
		return tokens.Access, nil
	}
	if h.oidc == nil || strings.TrimSpace(tokens.Refresh) == "" {
		return "", errors.New("account: no usable token")
	}
	fresh, err := h.oidc.Refresh(ctx, tokens.Refresh)
	if err != nil {
		return "", err
	}
	if err := h.store.SaveAccountTokens(admin.ID, storage.AccountTokens{
		Access: fresh.AccessToken, Refresh: fresh.RefreshToken, ExpiresAt: fresh.ExpiresAt,
	}); err != nil {
		log.Printf("account: fresh token for %s was not stored: %v", admin.Username, err)
	}
	return fresh.AccessToken, nil
}

// pushAvatarToAccount уносит картинку в учётку.
//
// Молча и на своих ногах: панель свою копию уже сохранила, и валить смену
// аватара из-за недоступного провайдера было бы через край
func (h *Handler) pushAvatarToAccount(ctx context.Context, admin storage.Admin, png []byte) {
	if _, ok := h.accountOf(admin); !ok || len(png) == 0 {
		return
	}
	token, err := h.freshAccountToken(ctx, admin)
	if err != nil {
		log.Printf("account: avatar of %s stayed at home: %v", admin.Username, err)
		return
	}
	if err := h.session.UploadAvatar(ctx, token, png); err != nil {
		log.Printf("account: avatar of %s was not accepted: %v", admin.Username, err)
	}
}

// accountSecurity - что показывать во вкладке аккаунта
func (h *Handler) handleAccountSecurity(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	subject, managed := h.accountOf(admin)
	out := map[string]any{
		"managed": managed,
		"name":    h.accountName(),
	}
	if !managed {
		writeJSON(w, http.StatusOK, out)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), accountTimeout)
	defer cancel()
	if has, err := h.session.HasTOTP(ctx, subject); err == nil {
		out["totp"] = has
	} else {
		log.Printf("account: second factor state of %s is unknown: %v", admin.Username, err)
		out["totp"] = false
	}
	writeJSON(w, http.StatusOK, out)
}

// changeAccountPassword меняет пароль в учётке.
//
// Старый проверяем сами, той же дверью, что и вход: служебному ключу провайдер
// разрешает менять пароль вообще без проверки, и довериться его verification
// значит не проверить нихуя
func (h *Handler) changeAccountPassword(
	w http.ResponseWriter,
	r *http.Request,
	admin storage.Admin,
	subject string,
	req changePasswordRequest,
) {
	ctx, cancel := context.WithTimeout(r.Context(), accountTimeout)
	defer cancel()

	name, err := h.store.AccountNameFor(admin.ID)
	if err != nil || strings.TrimSpace(name) == "" {
		name = admin.Username
	}
	if _, err := h.session.Password(ctx, name, req.OldPassword); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid old password")
		return
	}
	if err := h.session.ChangePassword(ctx, subject, req.NewPassword); err != nil {
		log.Printf("account: password of %s was not changed: %v", admin.Username, err)
		writeError(w, http.StatusBadGateway, "сервис учёток не сменил пароль")
		return
	}
	_ = h.store.AppendAudit(storage.AuditEntry{
		ActorAdminID: admin.ID, ActorUsername: admin.Username,
		Action: "auth.password_changed", IP: clientIP(r),
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// accountTOTP - второй фактор в общей учётке
func (h *Handler) accountTOTP(w http.ResponseWriter, r *http.Request, admin storage.Admin, subject string) {
	ctx, cancel := context.WithTimeout(r.Context(), accountTimeout)
	defer cancel()

	switch r.Method {
	case http.MethodGet:
		has, err := h.session.HasTOTP(ctx, subject)
		if err != nil {
			writeError(w, http.StatusBadGateway, "сервис учёток не ответил")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"enabled": has, "pending": false, "managed": true})
	case http.MethodPost:
		secret, err := h.session.StartTOTP(ctx, subject)
		if err != nil {
			log.Printf("account: second factor of %s did not start: %v", admin.Username, err)
			writeError(w, http.StatusBadGateway, "сервис учёток не завёл второй фактор")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"secret": secret.Secret, "otpauth": secret.URI, "confirmed": false, "managed": true,
		})
	case http.MethodPut:
		var req struct {
			Code string `json:"code"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "bad request body")
			return
		}
		if err := h.session.VerifyTOTP(ctx, subject, strings.TrimSpace(req.Code)); err != nil {
			writeError(w, http.StatusUnauthorized, "код не подошёл")
			return
		}
		_ = h.store.AppendAudit(storage.AuditEntry{
			ActorAdminID: admin.ID, ActorUsername: admin.Username,
			Action: "auth.totp_enabled", IP: clientIP(r),
		})
		writeJSON(w, http.StatusOK, map[string]any{"enabled": true, "managed": true})
	case http.MethodDelete:
		if err := h.session.DropTOTP(ctx, subject); err != nil {
			writeError(w, http.StatusBadGateway, "сервис учёток не выключил второй фактор")
			return
		}
		_ = h.store.AppendAudit(storage.AuditEntry{
			ActorAdminID: admin.ID, ActorUsername: admin.Username,
			Action: "auth.totp_disabled", IP: clientIP(r),
		})
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false, "managed": true})
	default:
		// Резервных кодов у провайдера нет: их место занимают ключи входа
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
