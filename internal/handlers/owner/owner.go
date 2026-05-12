// Package owner implements platform-level endpoints reserved for the owner
// role: managing admins, viewing audit log, platform-wide stats and settings.
package owner

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"v.wingsnet.org/internal/auth"
	"v.wingsnet.org/internal/guardianhub"
	"v.wingsnet.org/internal/storage"
)

type Handler struct {
	store     *storage.Store
	auth      *auth.Service
	hub       *guardianhub.Hub
	startedAt time.Time
}

func New(store *storage.Store, authSvc *auth.Service, hub *guardianhub.Hub) *Handler {
	return &Handler{store: store, auth: authSvc, hub: hub, startedAt: time.Now()}
}

// Register binds /api/owner/* routes onto the provided mux. All routes are
// gated through requireOwner.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/owner/me", h.requireOwner(h.handleMe))
	mux.HandleFunc("/api/owner/admins", h.requireOwner(h.handleAdmins))
	mux.HandleFunc("/api/owner/admins/", h.requireOwner(h.handleAdminByID))
	mux.HandleFunc("/api/owner/clients", h.requireOwner(h.handleAllClients))
	mux.HandleFunc("/api/owner/audit", h.requireOwner(h.handleAudit))
	mux.HandleFunc("/api/owner/stats", h.requireOwner(h.handleStats))
	mux.HandleFunc("/api/owner/settings", h.requireOwner(h.handleSettings))
	mux.HandleFunc("/api/owner/invites", h.requireOwner(h.handleInvites))
	mux.HandleFunc("/api/owner/invites/", h.requireOwner(h.handleInviteByToken))
}

type ownedHandler func(w http.ResponseWriter, r *http.Request, admin storage.Admin)

func (h *Handler) requireOwner(next ownedHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		admin, err := h.auth.Authenticate(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if !auth.IsOwner(admin) {
			writeError(w, http.StatusForbidden, "owner access required")
			return
		}
		next(w, r, admin)
	}
}

func (h *Handler) handleMe(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	writeJSON(w, http.StatusOK, map[string]any{
		"id":       admin.ID,
		"username": admin.Username,
		"role":     admin.Role,
	})
}

// ===== /api/owner/admins =====

func (h *Handler) handleAdmins(w http.ResponseWriter, r *http.Request, owner storage.Admin) {
	switch r.Method {
	case http.MethodGet:
		h.respondListAdmins(w)
	case http.MethodPost:
		h.respondCreateAdmin(w, r, owner)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

type adminView struct {
	ID                 int64  `json:"id"`
	Username           string `json:"username"`
	Role               string `json:"role"`
	MustChangePassword bool   `json:"must_change_password"`
	LastLoginAt        string `json:"last_login_at"`
	CreatedAt          string `json:"created_at"`
	ClientsTotal       int    `json:"clients_total"`
	ClientsOnline      int    `json:"clients_online"`
}

func (h *Handler) respondListAdmins(w http.ResponseWriter) {
	admins, err := h.store.ListAdmins()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]adminView, 0, len(admins))
	for _, a := range admins {
		view := adminView{
			ID:                 a.ID,
			Username:           a.Username,
			Role:               a.Role,
			MustChangePassword: a.MustChangePassword,
			LastLoginAt:        formatTS(a.LastLoginAt),
			CreatedAt:          formatTS(a.CreatedAt),
		}
		if cnt, err := h.store.CountClientsByOwner(a.ID); err == nil {
			view.ClientsTotal = cnt.Total
			view.ClientsOnline = cnt.Online
		}
		out = append(out, view)
	}
	writeJSON(w, http.StatusOK, map[string]any{"admins": out})
}

type createAdminRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *Handler) respondCreateAdmin(w http.ResponseWriter, r *http.Request, owner storage.Admin) {
	var req createAdminRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	username := strings.TrimSpace(req.Username)
	if len(username) < auth.MinUsernameLen {
		writeError(w, http.StatusBadRequest, "username too short")
		return
	}
	if len(req.Password) < auth.MinPasswordLen {
		writeError(w, http.StatusBadRequest, "password too short")
		return
	}
	if _, err := h.store.FindAdminByUsername(username); err == nil {
		writeError(w, http.StatusConflict, "username taken")
		return
	} else if !errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	created, err := h.store.CreateAdmin(username, hash, true, storage.RoleAdmin)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = h.store.AppendAudit(storage.AuditEntry{
		ActorAdminID: owner.ID, ActorUsername: owner.Username,
		Action:     "owner.admin_created",
		TargetType: "admin", TargetID: strconv.FormatInt(created.ID, 10),
		Message: created.Username,
		IP:      clientIP(r),
	})
	writeJSON(w, http.StatusCreated, adminView{
		ID:                 created.ID,
		Username:           created.Username,
		Role:               created.Role,
		MustChangePassword: created.MustChangePassword,
		CreatedAt:          formatTS(created.CreatedAt),
	})
}

func (h *Handler) handleAdminByID(w http.ResponseWriter, r *http.Request, owner storage.Admin) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/owner/admins/")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, http.StatusNotFound, "admin id missing")
		return
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	subpath := ""
	if len(parts) == 2 {
		subpath = parts[1]
	}
	switch {
	case subpath == "" && r.Method == http.MethodDelete:
		h.respondDeleteAdmin(w, r, owner, id)
	case subpath == "reset-password" && r.Method == http.MethodPost:
		h.respondResetPassword(w, r, owner, id)
	default:
		writeError(w, http.StatusNotFound, "unknown route")
	}
}

func (h *Handler) respondDeleteAdmin(w http.ResponseWriter, r *http.Request, owner storage.Admin, id int64) {
	if id == owner.ID {
		writeError(w, http.StatusForbidden, "cannot delete yourself")
		return
	}
	target, err := h.store.FindAdminByID(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "admin not found")
		return
	}
	if target.Role == storage.RoleOwner {
		writeError(w, http.StatusForbidden, "cannot delete another owner")
		return
	}
	if err := h.store.DeleteAdmin(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = h.store.AppendAudit(storage.AuditEntry{
		ActorAdminID: owner.ID, ActorUsername: owner.Username,
		Action:     "owner.admin_deleted",
		TargetType: "admin", TargetID: strconv.FormatInt(id, 10),
		Message: target.Username,
		IP:      clientIP(r),
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

type resetPasswordRequest struct {
	NewPassword string `json:"new_password"`
}

func (h *Handler) respondResetPassword(w http.ResponseWriter, r *http.Request, owner storage.Admin, id int64) {
	var req resetPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	target, err := h.store.FindAdminByID(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "admin not found")
		return
	}
	if err := h.auth.ResetPasswordTo(id, req.NewPassword); err != nil {
		if errors.Is(err, auth.ErrPasswordTooShort) {
			writeError(w, http.StatusBadRequest, "password too short")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = h.store.AppendAudit(storage.AuditEntry{
		ActorAdminID: owner.ID, ActorUsername: owner.Username,
		Action:     "owner.password_reset",
		TargetType: "admin", TargetID: strconv.FormatInt(id, 10),
		Message: target.Username,
		IP:      clientIP(r),
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ===== /api/owner/clients =====

type clientView struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	OwnerID     int64  `json:"owner_admin_id"`
	OwnerName   string `json:"owner_username"`
	Online      bool   `json:"online"`
	LastSeenAt  string `json:"last_seen_at"`
	CreatedAt   string `json:"created_at"`
	DeviceModel string `json:"device_model"`
	OSVersion   string `json:"os_version"`
	AppVersion  string `json:"app_version"`
}

func (h *Handler) handleAllClients(w http.ResponseWriter, r *http.Request, _ storage.Admin) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	admins, err := h.store.ListAdmins()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	nameByID := make(map[int64]string, len(admins))
	for _, a := range admins {
		nameByID[a.ID] = a.Username
	}
	clients, err := h.store.ListAllClients()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]clientView, 0, len(clients))
	for _, c := range clients {
		out = append(out, clientView{
			ID:          c.ID,
			Name:        c.Name,
			OwnerID:     c.OwnerAdminID,
			OwnerName:   nameByID[c.OwnerAdminID],
			Online:      c.Online,
			LastSeenAt:  formatTS(c.LastSeenAt),
			CreatedAt:   formatTS(c.CreatedAt),
			DeviceModel: c.DeviceModel,
			OSVersion:   c.OSVersion,
			AppVersion:  c.AppVersion,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"clients": out})
}

// ===== /api/owner/audit =====

func (h *Handler) handleAudit(w http.ResponseWriter, r *http.Request, _ storage.Admin) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	q := r.URL.Query()
	filter := storage.AuditFilter{}
	if v := q.Get("actor"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			filter.ActorAdminID = id
		}
	}
	if v := q.Get("action"); v != "" {
		filter.Action = v
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			filter.Limit = n
		}
	}
	if v := q.Get("since"); v != "" {
		if ts, err := time.Parse(time.RFC3339, v); err == nil {
			filter.Since = ts
		}
	}
	entries, err := h.store.ListAudit(filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		out = append(out, map[string]any{
			"id":             e.ID,
			"ts":             formatTS(e.TS),
			"actor_admin_id": e.ActorAdminID,
			"actor_username": e.ActorUsername,
			"action":         e.Action,
			"target_type":    e.TargetType,
			"target_id":      e.TargetID,
			"message":        e.Message,
			"ip":             e.IP,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": out})
}

// ===== /api/owner/stats =====

func (h *Handler) handleStats(w http.ResponseWriter, r *http.Request, _ storage.Admin) {
	adminsCount, _ := h.store.CountAdmins()
	clientCounts, _ := h.store.CountClients()
	dbOK := true
	if err := h.store.DB().Ping(); err != nil {
		dbOK = false
	}
	version := "dev"
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, kv := range info.Settings {
			if kv.Key == "vcs.revision" && kv.Value != "" {
				version = kv.Value
				if len(version) > 8 {
					version = version[:8]
				}
				break
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"admins_count":   adminsCount,
		"clients_count":  clientCounts.Total,
		"clients_online": clientCounts.Online,
		"ws_clients":     h.hub.ClientCount(),
		"ws_admins":      h.hub.AdminCount(),
		"db_ok":          dbOK,
		"version":        version,
		"uptime_seconds": int64(time.Since(h.startedAt).Seconds()),
		"started_at":     formatTS(h.startedAt),
	})
}

// ===== /api/owner/settings =====

type settingsRequest struct {
	RegistrationMode string `json:"registration_mode"`
}

func (h *Handler) handleSettings(w http.ResponseWriter, r *http.Request, owner storage.Admin) {
	switch r.Method {
	case http.MethodGet:
		mode, err := h.store.GetPlatformSetting(storage.SettingRegistrationMode, auth.RegistrationModeOpen)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"registration_mode": mode})
	case http.MethodPut:
		var req settingsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid body")
			return
		}
		mode := strings.TrimSpace(req.RegistrationMode)
		if mode != auth.RegistrationModeOpen &&
			mode != auth.RegistrationModeInvite &&
			mode != auth.RegistrationModeClosed {
			writeError(w, http.StatusBadRequest, "unknown registration_mode")
			return
		}
		if err := h.store.SetPlatformSetting(storage.SettingRegistrationMode, mode); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		_ = h.store.AppendAudit(storage.AuditEntry{
			ActorAdminID: owner.ID, ActorUsername: owner.Username,
			Action: "owner.registration_mode_changed", Message: mode, IP: clientIP(r),
		})
		writeJSON(w, http.StatusOK, map[string]any{"registration_mode": mode})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// ===== /api/owner/invites =====

func (h *Handler) handleInvites(w http.ResponseWriter, r *http.Request, owner storage.Admin) {
	switch r.Method {
	case http.MethodGet:
		invites, err := h.store.ListInvites(true)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		out := make([]map[string]any, 0, len(invites))
		for _, it := range invites {
			out = append(out, map[string]any{
				"token":      it.Token,
				"created_at": formatTS(it.CreatedAt),
				"expires_at": formatTS(it.ExpiresAt),
				"used_at":    formatTS(it.UsedAt),
				"used":       it.UsedAt.UnixMilli() > 0,
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"invites": out})
	case http.MethodPost:
		token, err := auth.GenerateInviteToken()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		var ttlReq struct {
			TTLHours int `json:"ttl_hours"`
		}
		_ = json.NewDecoder(r.Body).Decode(&ttlReq)
		var expiresAt time.Time
		if ttlReq.TTLHours > 0 {
			expiresAt = time.Now().Add(time.Duration(ttlReq.TTLHours) * time.Hour)
		}
		invite, err := h.store.CreateInvite(token, expiresAt, owner.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		_ = h.store.AppendAudit(storage.AuditEntry{
			ActorAdminID: owner.ID, ActorUsername: owner.Username,
			Action: "owner.invite_created", TargetType: "invite", TargetID: token, IP: clientIP(r),
		})
		writeJSON(w, http.StatusCreated, map[string]any{
			"token":      invite.Token,
			"created_at": formatTS(invite.CreatedAt),
			"expires_at": formatTS(invite.ExpiresAt),
		})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *Handler) handleInviteByToken(w http.ResponseWriter, r *http.Request, owner storage.Admin) {
	token := strings.TrimPrefix(r.URL.Path, "/api/owner/invites/")
	if token == "" {
		writeError(w, http.StatusNotFound, "missing token")
		return
	}
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err := h.store.DeleteInvite(token); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = h.store.AppendAudit(storage.AuditEntry{
		ActorAdminID: owner.ID, ActorUsername: owner.Username,
		Action: "owner.invite_deleted", TargetType: "invite", TargetID: token, IP: clientIP(r),
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ===== helpers =====

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": true, "message": message})
}

func formatTS(t time.Time) string {
	if t.IsZero() || t.UnixMilli() <= 0 {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

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
