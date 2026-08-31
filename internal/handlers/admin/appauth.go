package admin

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"v.wingsnet.org/internal/auth"
	"v.wingsnet.org/internal/storage"
)

const (
	// appSessionTTL - сколько живёт сессия приложения. Долго: телефон не браузер,
	// и разлогинивать человека раз в неделю нечестно
	appSessionTTL = 180 * 24 * time.Hour
	// appCodeTTL - жизнь одноразового кода. Секунды, потому что он едет через
	// адресную строку браузера и оседает в его истории
	appCodeTTL = 2 * time.Minute
	// appCallbackScheme - схема, которой приложение ловит ответ
	appCallbackScheme = "wingsv://account"
)

// appCode - выданный код обмена. Живёт в памяти: он одноразовый и с временем
// жизни в минуты, а переживать перезапуск ему незачем
type appCode struct {
	adminID   int64
	expiresAt time.Time
}

type appCodes struct {
	mu     sync.Mutex
	byCode map[string]appCode
}

func newAppCodes() *appCodes { return &appCodes{byCode: map[string]appCode{}} }

func (a *appCodes) issue(code string, adminID int64, now time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for value, entry := range a.byCode {
		if now.After(entry.expiresAt) {
			delete(a.byCode, value)
		}
	}
	a.byCode[code] = appCode{adminID: adminID, expiresAt: now.Add(appCodeTTL)}
}

// redeem отдаёт аккаунт и сразу забывает код: второй обмен по нему невозможен
func (a *appCodes) redeem(code string, now time.Time) (int64, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	entry, ok := a.byCode[code]
	delete(a.byCode, code)
	if !ok || now.After(entry.expiresAt) {
		return 0, false
	}
	return entry.adminID, true
}

// handleAppLink выдаёт код уже вошедшему человеку и уводит его обратно в
// приложение. Открывается в браузере, поэтому в адрес попадает только код
func (h *Handler) handleAppLink(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	code, err := auth.GenerateInviteToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.appCodes.issue(code, admin.ID, time.Now())
	_ = h.store.AppendAudit(storage.AuditEntry{
		ActorAdminID: admin.ID, ActorUsername: admin.Username,
		Action: "app.link_started", IP: clientIP(r),
	})
	http.Redirect(w, r, appCallbackScheme+"?code="+code, http.StatusFound)
}

// handleAppSession меняет одноразовый код на токен устройства
func (h *Handler) handleAppSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Code       string `json:"code"`
		DeviceName string `json:"device_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad request body")
		return
	}
	adminID, ok := h.appCodes.redeem(strings.TrimSpace(req.Code), time.Now())
	if !ok {
		writeError(w, http.StatusForbidden, "код недействителен или уже использован")
		return
	}
	admin, err := h.store.FindAdminByID(adminID)
	if err != nil {
		writeError(w, http.StatusForbidden, "аккаунт не найден")
		return
	}
	h.issueAppSession(w, r, admin, req.DeviceName)
}

// handleAppLogin - вход логином и паролём прямо из приложения. Браузер нужен
// только тем, кто заходит через Matrix
func (h *Handler) handleAppLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Username   string `json:"username"`
		Password   string `json:"password"`
		DeviceName string `json:"device_name"`
		Code       string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad request body")
		return
	}
	admin, err := h.auth.VerifyCredentials(req.Username, req.Password)
	if err != nil {
		var suspended *auth.SuspendedError
		if errors.As(err, &suspended) {
			writeError(w, http.StatusForbidden, suspended.Error())
			return
		}
		writeError(w, http.StatusUnauthorized, "неверный логин или пароль")
		return
	}
	if !h.verifySecondFactor(admin, req.Code) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"error": true, "totp_required": true, "message": "нужен код второго фактора",
		})
		return
	}
	h.issueAppSession(w, r, admin, req.DeviceName)
}

// issueAppSession заводит токен устройства и отдаёт его вместе с аккаунтом
func (h *Handler) issueAppSession(w http.ResponseWriter, r *http.Request, admin storage.Admin, deviceName string) {
	id, err := auth.GenerateInviteToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	token, err := auth.GenerateInviteToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	device := strings.TrimSpace(deviceName)
	if device == "" {
		device = "устройство"
	}
	if _, err := h.store.CreateAppSession(id, token, admin.ID, device, appSessionTTL); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = h.store.AppendAudit(storage.AuditEntry{
		ActorAdminID: admin.ID, ActorUsername: admin.Username,
		Action: "app.session_created", Message: device, IP: clientIP(r),
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"token":   token,
		"account": adminMePayload(admin),
	})
}

// handleAppMe - профиль для экрана аккаунта
func (h *Handler) handleAppMe(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	payload := adminMePayload(admin)
	if redeemed, err := h.store.RedeemedInvite(admin.ID); err == nil {
		payload["in_tree"] = redeemed
	}
	writeJSON(w, http.StatusOK, payload)
}

// requireApp пускает по токену устройства
func (h *Handler) requireApp(next authedHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		admin, err := h.authenticateApp(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next(w, r, admin)
	}
}

func (h *Handler) authenticateApp(r *http.Request) (storage.Admin, error) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return storage.Admin{}, errors.New("no bearer token")
	}
	session, err := h.store.LookupAppSession(strings.TrimPrefix(header, "Bearer "))
	if err != nil {
		return storage.Admin{}, err
	}
	admin, err := h.store.FindAdminByID(session.AdminID)
	if err != nil {
		return storage.Admin{}, err
	}
	return admin, nil
}

// handleAppLogout отзывает сессию этого устройства
func (h *Handler) handleAppLogout(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	header := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	session, err := h.store.LookupAppSession(header)
	if err == nil {
		_ = h.store.DeleteAppSession(session.ID)
		_ = h.store.AppendAudit(storage.AuditEntry{
			ActorAdminID: admin.ID, ActorUsername: admin.Username,
			Action: "app.session_revoked", Message: session.DeviceName, IP: clientIP(r),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
