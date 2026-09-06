package admin

// Вход учёткой прямо из приложения. Браузер тут не нужен: пароль проверяет
// сервис учёток, а панель по хозяину сессии находит, кому выдавать токен
// устройства

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"v.wingsnet.org/internal/accountsession"
	"v.wingsnet.org/internal/storage"
)

type appAccountRequest struct {
	Login      string `json:"login"`
	Password   string `json:"password"`
	Code       string `json:"code"`
	Ticket     string `json:"ticket"`
	DeviceName string `json:"device_name"`
}

// handleAppAccountLogin впускает в приложение по учётке.
//
// Отдельная дверь от панельной: там всё крутится вокруг запроса авторизации и
// возврата в браузер, а приложению нужен только ответ "это он" и токен
func (h *Handler) handleAppAccountLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.session == nil || !h.session.Enabled() {
		writeError(w, http.StatusNotFound, "account login is not configured")
		return
	}
	var req appAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad request body")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), accountTimeout)
	defer cancel()

	// Второй фактор приезжает отдельным заходом с квитком: пароль уже принят, и
	// гонять его снова незачем
	if strings.TrimSpace(req.Ticket) != "" {
		h.appAccountSecondFactor(ctx, w, r, req)
		return
	}

	login := strings.TrimSpace(req.Login)
	if login == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "нужны логин и пароль")
		return
	}
	if !h.limitAttempt(w, r, "login-ip:"+clientIP(r), "login-account:"+strings.ToLower(login)) {
		return
	}

	session, err := h.session.Password(ctx, login, req.Password)
	if err != nil {
		if errors.Is(err, accountsession.ErrBadPassword) {
			writeError(w, http.StatusUnauthorized, "неверный логин или пароль")
			return
		}
		log.Printf("app account login: the session was not created: %v", err)
		writeError(w, http.StatusBadGateway, "сервис учёток не ответил")
		return
	}
	h.finishAppAccountLogin(ctx, w, r, session, req)
}

// appAccountSecondFactor дозаканчивает вход кодом из аутентификатора
func (h *Handler) appAccountSecondFactor(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	req appAccountRequest,
) {
	row, ok := h.halfway.take(strings.TrimSpace(req.Ticket))
	if !ok {
		writeError(w, http.StatusUnauthorized, "вход просрочен, начните заново")
		return
	}
	session, err := h.session.SecondFactor(ctx, row.session, strings.TrimSpace(req.Code))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "код не подошёл")
		return
	}
	h.finishAppAccountLogin(ctx, w, r, session, req)
}

// finishAppAccountLogin находит человека по хозяину сессии и выдаёт токен
func (h *Handler) finishAppAccountLogin(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	session accountsession.Session,
	req appAccountRequest,
) {
	subject, err := h.session.Owner(ctx, session)
	if err != nil {
		// Пароль принят, но сессию не дали дочитать - почти всегда это значит,
		// что провайдер ждёт второй фактор
		ticket, ticketErr := h.halfway.put(halfway{session: session})
		if ticketErr != nil {
			writeError(w, http.StatusInternalServerError, "не смог удержать вход")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"second_factor": true, "ticket": ticket})
		return
	}
	admin, err := h.store.FindAdminByAccount(subject)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			// Учётка есть, а к аккаунту панели не привязана: заводить его втихую
			// нельзя, инвайт-дерево держится на том, что личность чего-то стоит
			writeJSON(w, http.StatusForbidden, map[string]any{
				"error": true, "account_unlinked": true,
				"message": "эта учётка ни к кому не привязана - войдите паролем и привяжите её",
			})
			return
		}
		writeError(w, http.StatusInternalServerError, "не смог поднять аккаунт")
		return
	}
	h.issueAppSession(w, r, admin, req.DeviceName)
}
