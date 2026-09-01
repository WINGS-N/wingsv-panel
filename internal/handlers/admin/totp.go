package admin

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	qrcode "github.com/skip2/go-qrcode"

	"v.wingsnet.org/internal/storage"
)

// backupCodeCount - сколько резервных кодов выдаётся за раз. Потерянный телефон
// не должен превращаться в потерянный аккаунт
const backupCodeCount = 10

// handleTOTPQR рисует QR по секрету этого же аккаунта
func (h *Handler) handleTOTPQR(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	state, err := h.store.TOTPFor(admin.ID)
	if err != nil {
		writeError(w, http.StatusNotFound, "2FA не настраивалась")
		return
	}
	key, err := otp.NewKeyFromURL(otpauthURL(admin.Username, state.Secret))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	png, err := qrcode.Encode(key.URL(), qrcode.Medium, 512)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store, private")
	_, _ = w.Write(png)
}

// otpauthURL собирает ссылку, которую понимают аутентификаторы
func otpauthURL(username, secret string) string {
	return fmt.Sprintf("otpauth://totp/WINGS%%20V:%s?secret=%s&issuer=WINGS%%20V&algorithm=SHA1&digits=6&period=30",
		url.PathEscape(username), secret)
}

// handleTOTP управляет 2FA своего аккаунта
func (h *Handler) handleTOTP(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	switch r.Method {
	case http.MethodGet:
		state, err := h.store.TOTPFor(admin.ID)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"enabled": false, "pending": false})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"enabled":      state.Confirmed,
			"pending":      !state.Confirmed,
			"backup_codes": h.store.BackupCodesLeft(admin.ID),
		})
	case http.MethodPost:
		h.startTOTP(w, r, admin)
	case http.MethodPut:
		h.confirmTOTP(w, r, admin)
	case http.MethodPatch:
		h.reissueBackupCodes(w, r, admin)
	case http.MethodDelete:
		h.disableTOTP(w, r, admin)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// startTOTP заводит секрет и отдаёт его вместе с ссылкой для приложения
func (h *Handler) startTOTP(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "WINGS V",
		AccountName: admin.Username,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.store.StartTOTP(admin.ID, key.Secret()); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"secret":    key.Secret(),
		"otpauth":   key.URL(),
		"confirmed": false,
	})
}

// confirmTOTP включает 2FA, когда код сошёлся, и выдаёт резервные коды
func (h *Handler) confirmTOTP(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad request body")
		return
	}
	state, err := h.store.TOTPFor(admin.ID)
	if err != nil {
		writeError(w, http.StatusNotFound, "2FA не настраивалась")
		return
	}
	if !totp.Validate(strings.TrimSpace(req.Code), state.Secret) {
		writeError(w, http.StatusForbidden, "код не подошёл")
		return
	}
	if err := h.store.ConfirmTOTP(admin.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	codes, err := generateBackupCodes()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.store.SetBackupCodes(admin.ID, codes); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = h.store.AppendAudit(storage.AuditEntry{
		ActorAdminID: admin.ID, ActorUsername: admin.Username,
		Action: "admin.totp_enabled", IP: clientIP(r),
	})
	writeJSON(w, http.StatusOK, map[string]any{"enabled": true, "backup_codes": codes})
}

// reissueBackupCodes выдаёт новый набор кодов восстановления взамен старого.
// Пароль спрашивается по той же причине, что и при снятии 2FA: иначе с чужого
// незалоченного телефона выписывается вечный обход второго фактора
func (h *Handler) reissueBackupCodes(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad request body")
		return
	}
	state, err := h.store.TOTPFor(admin.ID)
	if err != nil || !state.Confirmed {
		writeError(w, http.StatusNotFound, "2FA не включена")
		return
	}
	if _, err := h.auth.VerifyCredentials(admin.Username, req.Password); err != nil {
		writeError(w, http.StatusForbidden, "неверный пароль")
		return
	}
	codes, err := generateBackupCodes()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.store.SetBackupCodes(admin.ID, codes); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = h.store.AppendAudit(storage.AuditEntry{
		ActorAdminID: admin.ID, ActorUsername: admin.Username,
		Action: "admin.backup_codes_reissued", IP: clientIP(r),
	})
	writeJSON(w, http.StatusOK, map[string]any{"backup_codes": codes})
}

// disableTOTP снимает 2FA. Пароль спрашивается заново: снятие защиты
// не должно проходить с чужого незалоченного телефона
func (h *Handler) disableTOTP(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad request body")
		return
	}
	if _, err := h.auth.VerifyCredentials(admin.Username, req.Password); err != nil {
		writeError(w, http.StatusForbidden, "неверный пароль")
		return
	}
	if err := h.store.DisableTOTP(admin.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = h.store.AppendAudit(storage.AuditEntry{
		ActorAdminID: admin.ID, ActorUsername: admin.Username,
		Action: "admin.totp_disabled", IP: clientIP(r),
	})
	writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
}

// verifySecondFactor проверяет код или резервный код. Аккаунт без второго
// фактора проходит сразу
func (h *Handler) verifySecondFactor(admin storage.Admin, code string) bool {
	state, err := h.store.TOTPFor(admin.ID)
	if err != nil || !state.Confirmed {
		return true
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return false
	}
	if totp.Validate(code, state.Secret) {
		return true
	}
	return h.store.UseBackupCode(admin.ID, code)
}

// needsSecondFactor сообщает, спросит ли аккаунт код
func (h *Handler) needsSecondFactor(admin storage.Admin) bool {
	state, err := h.store.TOTPFor(admin.ID)
	return err == nil && state.Confirmed
}

// generateBackupCodes делает коды вида 4f2a-9c17: читаются с бумаги без путаницы
func generateBackupCodes() ([]string, error) {
	out := make([]string, 0, backupCodeCount)
	for i := 0; i < backupCodeCount; i++ {
		buf := make([]byte, 5)
		if _, err := rand.Read(buf); err != nil {
			return nil, err
		}
		raw := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf))
		out = append(out, fmt.Sprintf("%s-%s", raw[:4], raw[4:8]))
	}
	return out, nil
}
