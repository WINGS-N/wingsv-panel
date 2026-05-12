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

	"v.wingsnet.org/internal/auth"
	"v.wingsnet.org/internal/config"
	"v.wingsnet.org/internal/guardianhub"
	"v.wingsnet.org/internal/storage"
)

type Handler struct {
	cfg   config.Config
	store *storage.Store
	auth  *auth.Service
	hub   *guardianhub.Hub
}

func New(cfg config.Config, store *storage.Store, authSvc *auth.Service, hub *guardianhub.Hub) *Handler {
	return &Handler{cfg: cfg, store: store, auth: authSvc, hub: hub}
}

// Register binds /api/admin/* routes onto the provided mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/admin/login", h.handleLogin)
	mux.HandleFunc("/api/admin/logout", h.handleLogout)
	mux.HandleFunc("/api/admin/register", h.handleRegister)
	mux.HandleFunc("/api/admin/registration-status", h.handleRegistrationStatus)
	mux.HandleFunc("/api/admin/me", h.requireAuth(h.handleMe))
	mux.HandleFunc("/api/admin/password", h.requireAuth(h.handleChangePassword))
	mux.HandleFunc("/api/admin/clients", h.requireAuth(h.handleClients))
	mux.HandleFunc("/api/admin/clients/", h.requireAuth(h.handleClientByID))
	mux.HandleFunc("/api/admin/decode-link", h.requireAuth(h.handleDecodeLink))
	mux.HandleFunc("/api/admin/avatars/", h.handleAvatar)
	mux.HandleFunc("/api/admin/me/avatar", h.requireAuth(h.handleMyAvatar))
	mux.HandleFunc("/api/admin/master-config", h.requireAuth(h.handleMasterConfig))
	mux.HandleFunc("/api/admin/master-config/apply", h.requireAuth(h.handleMasterConfigApply))
	mux.HandleFunc("/api/admin/master-config/seed", h.requireAuth(h.handleMasterConfigSeed))
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
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
	writeJSON(w, http.StatusOK, adminMePayload(admin))
}

func adminMePayload(admin storage.Admin) map[string]any {
	return map[string]any{
		"id":                   admin.ID,
		"username":             admin.Username,
		"must_change_password": admin.MustChangePassword,
		"role":                 admin.Role,
		"avatar_version":       admin.AvatarVersion,
		"created_at":           admin.CreatedAt.Format(timeRFC3339),
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
		http.NotFound(w, r)
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
		defer file.Close()
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

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": true, "message": message})
}

func parseInt64(value string) (int64, error) {
	return strconv.ParseInt(strings.TrimSpace(value), 10, 64)
}
