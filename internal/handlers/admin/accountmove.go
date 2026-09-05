package admin

// Переезд на общий вход. Панель перестаёт быть местом, где живут пароли: свой
// пароль остаётся дверью ровно до тех пор, пока человек не заведёт учётку, и
// дальше этой двери его не пускают

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"v.wingsnet.org/internal/accountsession"
	"v.wingsnet.org/internal/storage"
)

// mustLinkAccount отвечает, обязан ли этот человек завести учётку прямо сейчас.
//
// Спрашиваем только когда общий вход настроен: у себя дома панель живёт на своём
// пароле, и гнать её админа в несуществующий сервис учёток - дурость
func (h *Handler) mustLinkAccount(admin storage.Admin) bool {
	if h.oidc == nil || !h.oidc.Enabled() {
		return false
	}
	linked, err := h.store.HasAccount(admin.ID)
	if err != nil {
		// База молчит - не запираем человека снаружи собственной панели
		log.Printf("account move: link state unreadable for %d: %v", admin.ID, err)
		return false
	}
	return !linked
}

// requireAccount пускает дальше только тех, кто уже переехал.
//
// Проверка серверная, а не в интерфейсе: экран можно объехать curl-ом, и тогда
// переезд превращается в вежливую просьбу
func (h *Handler) requireAccount(next authedHandler) http.HandlerFunc {
	return h.requireAuth(func(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
		if h.mustLinkAccount(admin) {
			writeJSON(w, http.StatusForbidden, map[string]any{
				"error":               true,
				"account_link_needed": true,
				"message":             "заведите " + h.accountName() + ", без него панель закрыта",
			})
			return
		}
		next(w, r, admin)
	})
}

func (h *Handler) accountName() string {
	if name := strings.TrimSpace(h.cfg.AccountName); name != "" {
		return name
	}
	return "WINGS Account"
}

type accountEnrollRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// handleAccountEnroll заводит человеку учётку и привязывает её к его аккаунту.
//
// Заводим за него, а не отправляем регистрироваться: сорок человек, ушедшие
// регистрироваться сами, вернутся с сорока разными именами, а половина не
// вернётся вовсе
func (h *Handler) handleAccountEnroll(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.session == nil || !h.session.Enabled() {
		writeError(w, http.StatusNotFound, "account service is not configured")
		return
	}
	linked, err := h.store.HasAccount(admin.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не смог проверить привязку")
		return
	}
	if linked {
		writeError(w, http.StatusConflict, "учётка уже привязана")
		return
	}

	var req accountEnrollRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad request")
		return
	}
	email := strings.TrimSpace(req.Email)
	if email == "" || !strings.Contains(email, "@") {
		writeError(w, http.StatusBadRequest, "нужна почта")
		return
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "пароль короче восьми знаков")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), accountTimeout)
	defer cancel()
	subject, err := h.session.CreateHuman(ctx, humanFor(admin, email, req.Password))
	if err != nil {
		log.Printf("account move: %s did not get an account: %v", admin.Username, err)
		writeError(w, http.StatusBadGateway, "сервис учёток не завёл учётку")
		return
	}
	if err := h.store.LinkAccount(admin.ID, subject, admin.Username); err != nil {
		writeError(w, http.StatusInternalServerError, "учётка заведена, но не привязалась")
		return
	}
	_ = h.store.AppendAudit(storage.AuditEntry{
		ActorAdminID: admin.ID, ActorUsername: admin.Username,
		Action: "admin.account_enrolled", TargetType: "admin",
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "login": accountLoginName(admin)})
}

// humanFor собирает заявку на учётку из того, что панель и так знает
func humanFor(admin storage.Admin, email, password string) accountsession.Human {
	return accountsession.Human{
		Username:  admin.Username,
		Email:     email,
		FirstName: admin.Username,
		LastName:  "WINGS",
		Password:  password,
	}
}

// accountLoginName - как человек будет звать себя на входе. Провайдер клеит к
// имени свою организацию, и не сказать об этом значит оставить человека гадать
func accountLoginName(admin storage.Admin) string { return admin.Username }
