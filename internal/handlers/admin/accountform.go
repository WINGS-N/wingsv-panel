package admin

// Свой экран входа: логин и второй фактор панель показывает сама, а проверяет
// их сервис учёток. Чужая страница логина выглядит чужой админкой, и человек
// посреди входа в WINGS уезжает на незнакомый домен

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"v.wingsnet.org/internal/accountsession"
)

// halfwayTTL - сколько живёт вход, застрявший на втором факторе. Хватает
// достать телефон, и не хватает уйти пить чай
const halfwayTTL = 5 * time.Minute

// halfway - вход, дошедший до пароля и ждущий кода
type halfway struct {
	authRequestID string
	session       accountsession.Session
	at            time.Time
}

// halfwayDesk держит такие входы. В памяти намеренно: они живут минуты, а
// переживать перезапуск панели им незачем
type halfwayDesk struct {
	mu   sync.Mutex
	rows map[string]halfway
}

func newHalfwayDesk() *halfwayDesk { return &halfwayDesk{rows: map[string]halfway{}} }

func (d *halfwayDesk) put(row halfway) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	ticket := base64.RawURLEncoding.EncodeToString(buf)
	d.mu.Lock()
	defer d.mu.Unlock()
	cutoff := time.Now().Add(-halfwayTTL)
	for key, got := range d.rows {
		if got.at.Before(cutoff) {
			delete(d.rows, key)
		}
	}
	row.at = time.Now()
	d.rows[ticket] = row
	return ticket, nil
}

func (d *halfwayDesk) take(ticket string) (halfway, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	row, ok := d.rows[ticket]
	delete(d.rows, ticket)
	if !ok || time.Since(row.at) > halfwayTTL {
		return halfway{}, false
	}
	return row, true
}

type accountLoginRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
	ReturnTo string `json:"return_to"`
	Invite   string `json:"invite"`
}

// handleAccountLogin проверяет пароль и, если больше ничего не нужно, отдаёт
// адрес, по которому браузер возвращается в панель уже с кодом
func (h *Handler) handleAccountLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.session == nil || !h.session.Enabled() || h.oidc == nil || !h.oidc.Enabled() {
		writeError(w, http.StatusNotFound, "account login is not configured")
		return
	}
	var req accountLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad request")
		return
	}
	login, password := strings.TrimSpace(req.Login), req.Password
	if login == "" || password == "" {
		writeError(w, http.StatusBadRequest, "login and password are required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), accountTimeout)
	defer cancel()

	// Тот же старт, что и у кнопки: state с проверкой PKCE заводится здесь, и
	// колбэк потом узнаёт свой вход
	var linkAdminID int64
	if admin, err := h.auth.Authenticate(r); err == nil {
		linkAdminID = admin.ID
	}
	authURL, err := h.oidc.Start(ctx, safeReturnTo(req.ReturnTo), linkAdminID, strings.TrimSpace(req.Invite))
	if err != nil {
		writeError(w, http.StatusBadGateway, "account service unreachable")
		return
	}
	authRequestID, err := h.session.Begin(ctx, authURL)
	if err != nil {
		writeError(w, http.StatusBadGateway, "account service unreachable")
		return
	}

	session, err := h.session.Password(ctx, login, password)
	if err != nil {
		if errors.Is(err, accountsession.ErrBadPassword) {
			writeError(w, http.StatusUnauthorized, "неверный логин или пароль")
			return
		}
		writeError(w, http.StatusBadGateway, "account service unreachable")
		return
	}

	callback, err := h.session.Finish(ctx, authRequestID, session)
	if err == nil {
		writeJSON(w, http.StatusOK, map[string]any{"redirect": callback})
		return
	}
	// Провайдер не принял одну лишь проверку пароля - значит просит второй
	// фактор. Пароль при этом уже принят, поэтому вход не начинаем заново
	ticket, ticketErr := h.halfway.put(halfway{authRequestID: authRequestID, session: session})
	if ticketErr != nil {
		writeError(w, http.StatusInternalServerError, "не смог удержать вход")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"second_factor": true, "ticket": ticket})
}

type accountFactorRequest struct {
	Ticket string `json:"ticket"`
	Code   string `json:"code"`
}

// handleAccountFactor принимает код из аутентификатора и дозаканчивает вход
func (h *Handler) handleAccountFactor(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.session == nil || !h.session.Enabled() {
		writeError(w, http.StatusNotFound, "account login is not configured")
		return
	}
	var req accountFactorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad request")
		return
	}
	row, ok := h.halfway.take(strings.TrimSpace(req.Ticket))
	if !ok {
		writeError(w, http.StatusUnauthorized, "вход просрочен, начните заново")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), accountTimeout)
	defer cancel()
	session, err := h.session.SecondFactor(ctx, row.session, strings.TrimSpace(req.Code))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "код не подошёл")
		return
	}
	callback, err := h.session.Finish(ctx, row.authRequestID, session)
	if err != nil {
		writeError(w, http.StatusBadGateway, "сервис учёток не завершил вход")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"redirect": callback})
}
