// Package admin implements REST and WebSocket endpoints serving the panel UI.
//
// Authentication uses bcrypt-hashed admin passwords and HTTP-only session
// cookies issued by the auth service. Every endpoint except /login enforces
// session verification through requireAuth().
package admin

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"

	"v.wingsnet.org/internal/accountsession"
	"v.wingsnet.org/internal/auth"
	"v.wingsnet.org/internal/avatarpic"
	"v.wingsnet.org/internal/config"
	"v.wingsnet.org/internal/fedclient"
	"v.wingsnet.org/internal/guardianhub"
	"v.wingsnet.org/internal/oidcauth"
	"v.wingsnet.org/internal/pki"
	"v.wingsnet.org/internal/storage"
	"v.wingsnet.org/internal/xuiclient"
)

type Handler struct {
	cfg   config.Config
	store *storage.Store
	auth  *auth.Service
	hub   *guardianhub.Hub
	xui   *xuiclient.Client
	// fed talks to the federation head. Nil-safe: a deployment with no head
	// configured simply has no federation surface
	fed *fedclient.Client
	// appCodes - одноразовые коды, которыми приложение забирает свой токен
	appCodes *appCodes
	// redeemLimiter режет перебор кода приглашения
	redeemLimiter *attemptLimiter
	// oidc is the deployment's own account service. Nil-safe: without one the
	// panel simply never offers the button
	oidc *oidcauth.Client
	// session водит человека через наш собственный экран входа. Без настройки
	// остаётся кнопка на страницу провайдера, и это законный режим
	session *accountsession.Client
	halfway *halfwayDesk
	// SPKI pins of the deployment CA, embedded in every enrollment link.
	caPins [][]byte
}

func New(cfg config.Config, store *storage.Store, authSvc *auth.Service, hub *guardianhub.Hub) *Handler {
	h := &Handler{
		cfg: cfg, store: store, auth: authSvc, hub: hub,
		xui:           xuiclient.New(),
		fed:           fedclient.New(cfg.FederationHead, cfg.FederationSecret),
		appCodes:      newAppCodes(),
		redeemLimiter: newAttemptLimiter(),
		halfway:       newHalfwayDesk(),
		session: accountsession.New(accountsession.Config{
			Issuer: cfg.OIDCIssuer,
			Token:  cfg.AccountAPIToken,
		}),
		oidc: oidcauth.New(oidcauth.Config{
			Issuer:       cfg.OIDCIssuer,
			ClientID:     cfg.OIDCClientID,
			ClientSecret: cfg.OIDCClientSecret,
			RedirectURL:  strings.TrimRight(cfg.PublicBaseURL, "/") + "/api/oidc/callback",
		}),
	}
	// A deployment with no publicly trusted certificate - a bare-IP install, where
	// no CA will issue one - can only be verified by the device if the enrollment
	// link carries the CA's SPKI pin, so every link gets it whenever a panel CA
	// exists. With a public certificate there is no CA dir, the field stays empty,
	// and the app falls back to the system trust store.
	if ca, err := pki.LoadCADir(cfg.CADir); err == nil {
		pin := ca.Pin512()
		h.caPins = [][]byte{pin[:]}
	}
	return h
}

// Register binds /api/admin/* routes onto the provided mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/admin/login", h.handleLogin)
	mux.HandleFunc("/api/admin/logout", h.handleLogout)
	mux.HandleFunc("/api/admin/register", h.handleRegister)
	mux.HandleFunc("/api/admin/registration/status", h.handleRegistrationStatus)
	// Без сессии: страницу приглашения открывает тот, у кого аккаунта ещё нет
	mux.HandleFunc("/api/invite", h.handleInviteLookup)
	mux.HandleFunc("/api/admin/me", h.requireAuth(h.handleMe))
	mux.HandleFunc("/api/admin/password", h.requireAuth(h.handleChangePassword))
	mux.HandleFunc("/api/admin/clients", h.requirePanel(h.handleClients))
	mux.HandleFunc("/api/admin/clients/", h.requirePanel(h.handleClientByID))
	mux.HandleFunc("/api/admin/link/decode", h.requirePanel(h.handleDecodeLink))
	mux.HandleFunc("/api/admin/stats/traffic", h.requirePanel(h.handleStatsTraffic))
	mux.HandleFunc("/api/admin/stats/flows", h.requirePanel(h.handleStatsFlows))
	mux.HandleFunc("/api/admin/stats/flowhistory", h.requirePanel(h.handleStatsFlowHistory))
	mux.HandleFunc("/api/admin/stats/xrayflows", h.requirePanel(h.handleStatsXrayFlows))
	mux.HandleFunc("/api/admin/stats/connections", h.requirePanel(h.handleStatsConnections))
	mux.HandleFunc("/api/admin/nodes", h.requirePanel(h.handleNodes))
	mux.HandleFunc("/api/admin/nodes/", h.requirePanel(h.handleNodeByID))
	mux.HandleFunc("/api/admin/wgpeers", h.requirePanel(h.handleWGPeers))
	mux.HandleFunc("/api/admin/wgpeers/", h.requirePanel(h.handleWGPeerByID))
	mux.HandleFunc("/api/admin/vk-links", h.requirePanel(h.handleVKLinks))
	mux.HandleFunc("/api/admin/avatars/", h.handleAvatar)
	mux.HandleFunc("/api/admin/me/avatar", h.requireAuth(h.handleMyAvatar))
	// Кабинет участника: собственный доступ есть у любого аккаунта, включая
	// администратора - одно другого не отменяет
	mux.HandleFunc("/api/admin/me/access", h.requireAuth(h.handleMyAccess))
	mux.HandleFunc("/api/admin/me/panel-access", h.requireAuth(h.handlePanelAccess))
	mux.HandleFunc("/api/admin/me/totp", h.requireAuth(h.handleTOTP))
	mux.HandleFunc("/api/admin/me/totp/qr", h.requireAuth(h.handleTOTPQR))
	// Аккаунт в приложении: браузер уводит человека обратно с одноразовым кодом,
	// приложение меняет его на токен устройства и дальше ходит только по нему
	mux.HandleFunc("/app/link", h.handleAppLink)
	mux.HandleFunc("/api/app/login", h.handleAppLogin)
	mux.HandleFunc("/api/app/session", h.handleAppSession)
	mux.HandleFunc("/api/app/me", h.requireApp(h.handleAppMe))
	mux.HandleFunc("/api/app/access", h.requireApp(h.handleMyAccess))
	// Те же действия над своим аккаунтом, но по токену устройства: приложение
	// ходит с ним, а не с сессионной кукой
	mux.HandleFunc("/api/app/password", h.requireApp(h.handleChangePassword))
	mux.HandleFunc("/api/app/avatar", h.requireApp(h.handleMyAvatar))
	mux.HandleFunc("/api/app/invites", h.requireApp(h.handleAppInvites))
	// Донорская часть админа: свои отданные серверы, их лимиты и токен подключения
	mux.HandleFunc("/api/app/federation/summary", h.requireApp(h.handleFederationSummary))
	mux.HandleFunc("/api/app/federation/key", h.requireApp(h.handleAppReceiptKey))
	mux.HandleFunc("/api/app/federation/receipts", h.requireApp(h.handleAppReceipts))
	mux.HandleFunc("/api/app/federation/enroll", h.requireApp(h.handleFederationEnrollToken))
	mux.HandleFunc("/api/app/federation/nodes/", h.requireApp(h.handleFederationNodeState))
	mux.HandleFunc("/api/app/invites/redeem", h.requireApp(h.handleRedeemInvite))
	mux.HandleFunc("/api/app/logout", h.requireApp(h.handleAppLogout))
	// Не под /api/admin: этим входом пользуются не только администраторы.
	// Бесплатный пользователь федерации заходит тем же WINGS V ID, и путь,
	// названный админским, пришлось бы ломать ровно в тот день, когда до этого
	// дойдут руки. Привязка требует сессии, остальное открыто по определению -
	// логинится тот, у кого её ещё нет.
	mux.HandleFunc("/api/oidc/status", h.handleOIDCStatus)
	mux.HandleFunc("/api/oidc/start", h.handleOIDCStart)
	mux.HandleFunc("/api/oidc/callback", h.handleOIDCCallback)
	mux.HandleFunc("/api/oidc/link", h.requireAuth(h.handleOIDCLink))
	// Свой экран входа: форму рисует панель, проверяет сервис учёток
	mux.HandleFunc("/api/account/login", h.handleAccountLogin)
	mux.HandleFunc("/api/account/factor", h.handleAccountFactor)
	mux.HandleFunc("/api/account/enroll", h.requireAuth(h.handleAccountEnroll))
	// Приглашать может любой администратор: дерево инвайтов и есть цена входа,
	// и растить его - не привилегия владельца. Владельцу остаётся обрезка ветви:
	// выдать доступ и отобрать чужой - разные права.
	// Приглашения не про панель: кого пускать в дерево, решает mayInvite по
	// вкладу в федерацию, а сам список и так только свой
	mux.HandleFunc("/api/admin/invites", h.requireAuth(h.handleInvites))
	// Код вводит любой аккаунт: он ставит человека в дерево, а не открывает панель
	mux.HandleFunc("/api/admin/invites/redeem", h.requireAuth(h.handleRedeemInvite))
	mux.HandleFunc("/api/admin/invites/", h.requireAuth(h.handleInviteByToken))
	mux.HandleFunc("/api/admin/fleet", h.requirePanel(h.handleFleetSettings))
	mux.HandleFunc("/api/admin/fleet/nodes", h.requirePanel(h.handleFleetNodes))
	mux.HandleFunc("/api/admin/fleet/releases", h.requirePanel(h.handleFleetReleases))
	mux.HandleFunc("/api/admin/fleet/restart", h.requirePanel(h.handleFleetRestart))
	mux.HandleFunc("/api/admin/federation/summary", h.requirePanel(h.handleFederationSummary))
	mux.HandleFunc("/api/admin/federation/enroll-token", h.requirePanel(h.handleFederationEnrollToken))
	mux.HandleFunc("/api/admin/federation/live", h.requirePanel(h.handleFederationLive))
	mux.HandleFunc("/api/admin/donations", h.requireAuth(h.handleDonations))
	mux.HandleFunc("/api/admin/donations/claim", h.requireAuth(h.handleClaimDonation))
	mux.HandleFunc("/api/admin/federation/payouts", h.requirePanel(h.handlePayoutStatement))
	mux.HandleFunc("/api/admin/federation/payouts/address", h.requirePanel(h.handlePayoutAddress))
	mux.HandleFunc("/api/admin/federation/nodes/", h.requirePanel(h.handleFederationNodeState))
	mux.HandleFunc("/api/admin/master/config", h.requirePanel(h.handleMasterConfig))
	mux.HandleFunc("/api/admin/master/config/apply", h.requirePanel(h.handleMasterConfigApply))
	mux.HandleFunc("/api/admin/master/config/seed", h.requirePanel(h.handleMasterConfigSeed))
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	// Code - код 2FA или резервный код
	Code string `json:"code"`
}

func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	// Подбор пароля и кода 2FA упирается только в скорость сети, если попытки не
	// считать. Ключей два: адрес меняется через прокси, логин - нет
	if !h.limitAttempt(w, r, "login-ip:"+clientIP(r), "login-user:"+strings.ToLower(strings.TrimSpace(req.Username))) {
		return
	}
	// 2FA проверяется до выдачи сессии: пароль сам по себе входом уже
	// не является
	if checked, err := h.auth.VerifyCredentials(req.Username, req.Password); err == nil {
		if !h.verifySecondFactor(checked, req.Code) {
			writeJSON(w, http.StatusUnauthorized, map[string]any{
				"error": true, "totp_required": true, "message": "нужен код 2FA",
			})
			return
		}
	}
	admin, sess, err := h.auth.Login(req.Username, req.Password)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			_ = h.store.AppendAudit(storage.AuditEntry{
				ActorUsername: req.Username, Action: "auth.login_failed", IP: clientIP(r),
			})
			writeError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.auth.WriteSessionCookie(w, sess)
	_ = h.store.AppendAudit(storage.AuditEntry{
		ActorAdminID: admin.ID, ActorUsername: admin.Username,
		Action: "auth.login", IP: clientIP(r),
	})
	writeJSON(w, http.StatusOK, adminMePayload(admin))
}

type registerRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	InviteToken string `json:"invite_token"`
}

func (h *Handler) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	admin, sess, err := h.auth.Register(req.Username, req.Password, req.InviteToken)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrUsernameTooShort):
			writeError(w, http.StatusBadRequest, "username too short")
		case errors.Is(err, auth.ErrUsernameInvalid):
			writeError(w, http.StatusBadRequest, "username must be alphanumeric (a-z, 0-9)")
		case errors.Is(err, auth.ErrPasswordTooShort):
			writeError(w, http.StatusBadRequest, "password too short")
		case errors.Is(err, auth.ErrUsernameTaken):
			writeError(w, http.StatusConflict, "username taken")
		case errors.Is(err, auth.ErrRegistrationClosed):
			writeError(w, http.StatusForbidden, "registration disabled")
		case errors.Is(err, auth.ErrRegistrationInvite):
			writeError(w, http.StatusBadRequest, "invite token required")
		case errors.Is(err, auth.ErrInviteTokenInvalid):
			writeError(w, http.StatusForbidden, "invite token invalid")
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	h.auth.WriteSessionCookie(w, sess)
	_ = h.store.AppendAudit(storage.AuditEntry{
		ActorAdminID: admin.ID, ActorUsername: admin.Username,
		Action: "auth.register", IP: clientIP(r),
	})
	writeJSON(w, http.StatusCreated, adminMePayload(admin))
}

func (h *Handler) handleRegistrationStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	mode, err := h.store.GetPlatformSetting(storage.SettingRegistrationMode, auth.RegistrationModeOpen)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"mode": mode})
}

func (h *Handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	cookie, err := r.Cookie(auth.SessionCookieName)
	if err == nil {
		_ = h.auth.Logout(cookie.Value)
	}
	if admin, err := h.auth.Authenticate(r); err == nil {
		_ = h.store.AppendAudit(storage.AuditEntry{
			ActorAdminID: admin.ID, ActorUsername: admin.Username,
			Action: "auth.logout", IP: clientIP(r),
		})
	}
	h.auth.ClearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handler) handleMe(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	payload := adminMePayload(admin)
	// self_provisioning tells the client-create form whether an autonomous
	// VK-TURN-profile link is possible: it needs a vk-turn relay the app can
	// self-enroll against. Without one the only valid link shape is Guardian, so
	// the remote-control toggle is hidden and remote control is forced on. This
	// asks whether the admin actually has a relay registered rather than whether a
	// panel-global endpoint env happens to be set.
	payload["self_provisioning"] = h.hasVkTurnNode(admin)
	// Пока учётки нет, интерфейс обязан вести на переезд, а не рисовать разделы,
	// которые всё равно ответят отказом
	payload["account_link_needed"] = h.mustLinkAccount(admin)
	payload["account_name"] = h.accountName()
	writeJSON(w, http.StatusOK, payload)
}

func adminMePayload(admin storage.Admin) map[string]any {
	return map[string]any{
		"id":                   admin.ID,
		"username":             admin.Username,
		"must_change_password": admin.MustChangePassword,
		"role":                 admin.Role,
		"panel_access":         admin.PanelAccess || admin.Role == storage.RoleOwner,
		"avatar_version":       admin.AvatarVersion,
		"created_at":           admin.CreatedAt.Format(timeRFC3339),
		"panel_requested":      !admin.PanelRequestedAt.IsZero(),
		// Как участника зовут в федерации. Приложение подписывает этим именем
		// расписки, и выдумывать его на своей стороне ему нечего: схема имён
		// принадлежит панели и однажды поменяется
		"federation_id": federationUserID(admin),
	}
}

const timeRFC3339 = "2006-01-02T15:04:05Z07:00"

func clientIP(r *http.Request) string {
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		if i := strings.IndexByte(ip, ','); i > 0 {
			return strings.TrimSpace(ip[:i])
		}
		return strings.TrimSpace(ip)
	}
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

type changePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

func (h *Handler) handleChangePassword(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req changePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if len(strings.TrimSpace(req.NewPassword)) < 4 {
		writeError(w, http.StatusBadRequest, "new password too short")
		return
	}
	if err := h.auth.ChangePassword(admin.ID, req.OldPassword, req.NewPassword); err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			writeError(w, http.StatusUnauthorized, "invalid old password")
			return
		}
		if errors.Is(err, auth.ErrPasswordTooShort) {
			writeError(w, http.StatusBadRequest, "password too short")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = h.store.AppendAudit(storage.AuditEntry{
		ActorAdminID: admin.ID, ActorUsername: admin.Username,
		Action: "auth.password_changed", IP: clientIP(r),
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

const maxAvatarBytes = 2 * 1024 * 1024

func (h *Handler) handleAvatar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/admin/avatars/")
	rest = strings.TrimSuffix(rest, ".png")
	rest = strings.TrimSuffix(rest, ".jpg")
	rest = strings.TrimSuffix(rest, ".webp")
	id, err := strconv.ParseInt(rest, 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	mime, data, _, err := h.store.GetAdminAvatar(id)
	if err != nil || len(data) == 0 {
		// Своей картинки нет - отдаём кружок с буквой имени. На 404 каждый клиент
		// лепил бы свою затычку, а их дохуя и все разные
		admin, lookupErr := h.store.FindAdminByID(id)
		if lookupErr != nil {
			http.NotFound(w, r)
			return
		}
		generated, drawErr := avatarpic.Generate(admin.Username)
		if drawErr != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "private, max-age=86400")
		_, _ = w.Write(generated)
		return
	}
	if mime == "" {
		mime = "image/png"
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	_, _ = w.Write(data)
}

func (h *Handler) handleMyAvatar(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	switch r.Method {
	case http.MethodPost:
		if err := r.ParseMultipartForm(maxAvatarBytes + 4096); err != nil {
			writeError(w, http.StatusBadRequest, "could not parse form")
			return
		}
		file, header, err := r.FormFile("avatar")
		if err != nil {
			writeError(w, http.StatusBadRequest, "missing avatar file")
			return
		}
		defer func() { _ = file.Close() }()
		if header.Size > maxAvatarBytes {
			writeError(w, http.StatusRequestEntityTooLarge, "avatar too large (max 2 MiB)")
			return
		}
		mime := header.Header.Get("Content-Type")
		switch mime {
		case "image/png", "image/jpeg", "image/webp":
			// ok
		default:
			writeError(w, http.StatusBadRequest, "unsupported image type")
			return
		}
		buf := make([]byte, header.Size)
		n, err := io.ReadFull(file, buf)
		if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		buf = buf[:n]
		version, err := h.store.SetAdminAvatar(admin.ID, mime, buf)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		_ = h.store.AppendAudit(storage.AuditEntry{
			ActorAdminID: admin.ID, ActorUsername: admin.Username,
			Action: "auth.avatar_changed", IP: clientIP(r),
		})
		writeJSON(w, http.StatusOK, map[string]any{"avatar_version": version})
	case http.MethodDelete:
		if err := h.store.ClearAdminAvatar(admin.ID); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		// Аккаунт возвращается к аватару, который был у него с регистрации
		if picture, drawErr := avatarpic.Generate(admin.Username); drawErr == nil {
			_, _ = h.store.SetAdminAvatar(admin.ID, "image/png", picture)
		}
		_ = h.store.AppendAudit(storage.AuditEntry{
			ActorAdminID: admin.ID, ActorUsername: admin.Username,
			Action: "auth.avatar_cleared", IP: clientIP(r),
		})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

type authedHandler func(w http.ResponseWriter, r *http.Request, admin storage.Admin)

func (h *Handler) requireAuth(next authedHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		admin, err := h.auth.Authenticate(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next(w, r, admin)
	}
}

// requirePanel пускает только тех, кому открыта админ-панель. Аккаунт без неё -
// обычный участник: у него есть свой доступ к VPN и свой кабинет, но чужими
// нодами и клиентами он не распоряжается
func (h *Handler) requirePanel(next authedHandler) http.HandlerFunc {
	// Сначала переезд, потом всё остальное: панель перестаёт быть местом, где
	// живут пароли, и свой пароль остаётся дверью только до заведения учётки
	return h.requireAccount(func(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
		if !admin.PanelAccess && admin.Role != storage.RoleOwner {
			writeError(w, http.StatusForbidden, "админ-панель для этого аккаунта закрыта")
			return
		}
		next(w, r, admin)
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": true, "message": message})
}
