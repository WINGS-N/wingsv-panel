package admin

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"v.wingsnet.org/internal/auth"
	"v.wingsnet.org/internal/oidcauth"
	"v.wingsnet.org/internal/storage"
)

// accountTimeout bounds a round trip to the account service. Short: somebody is
// waiting on a redirect, and a hung provider must not hang the panel.
const accountTimeout = 10 * time.Second

// handleOIDCStatus tells the login page whether to offer the button at all.
func (h *Handler) handleOIDCStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled": h.oidc != nil && h.oidc.Enabled(),
		"name":    h.cfg.AccountName,
	})
}

// handleOIDCLink reports and removes the attached account.
//
// Removing is allowed only when a password still works, or an admin unlinks
// themselves out of the panel entirely with no way back in.
func (h *Handler) handleOIDCLink(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	switch r.Method {
	case http.MethodGet:
		account, err := h.store.AccountNameFor(admin.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"enabled": h.oidc != nil && h.oidc.Enabled(),
			"name":    h.cfg.AccountName,
			"account": account,
		})
	case http.MethodDelete:
		if err := h.store.UnlinkAccount(admin.ID); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		_ = h.store.AppendAudit(storage.AuditEntry{
			ActorAdminID: admin.ID, ActorUsername: admin.Username,
			Action: "admin.account_unlinked", TargetType: "admin",
		})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleOIDCStart sends the browser off to the account service.
func (h *Handler) handleOIDCStart(w http.ResponseWriter, r *http.Request) {
	if h.oidc == nil || !h.oidc.Enabled() {
		writeError(w, http.StatusNotFound, "account login is not configured")
		return
	}
	// An admin already signed in is attaching an account rather than signing in
	var linkAdminID int64
	if admin, err := h.auth.Authenticate(r); err == nil {
		linkAdminID = admin.ID
	}

	ctx, cancel := context.WithTimeout(r.Context(), accountTimeout)
	defer cancel()
	// Код приглашения запоминается на нашей стороне: обратно провайдер отдаст
	// только state и code, и без этого первый вход зарегистрировать некого
	target, err := h.oidc.Start(ctx, safeReturnTo(r.URL.Query().Get("return_to")), linkAdminID,
		strings.TrimSpace(r.URL.Query().Get("invite")))
	if err != nil {
		writeError(w, http.StatusBadGateway, "account service unreachable: "+err.Error())
		return
	}
	http.Redirect(w, r, target, http.StatusFound)
}

// handleOIDCCallback finishes the login and drops the browser back in the panel.
func (h *Handler) handleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	if h.oidc == nil || !h.oidc.Enabled() {
		writeError(w, http.StatusNotFound, "account login is not configured")
		return
	}
	if reason := r.URL.Query().Get("error"); reason != "" {
		h.oidcFail(w, r, reason)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), accountTimeout)
	defer cancel()

	identity, returnTo, linkAdminID, invite, err := h.oidc.Complete(ctx,
		r.URL.Query().Get("state"), r.URL.Query().Get("code"))
	if err != nil {
		switch {
		case errors.Is(err, oidcauth.ErrUnknownState):
			h.oidcFail(w, r, "expired")
		default:
			h.oidcFail(w, r, "exchange_failed")
		}
		return
	}

	// Attaching an account to the admin who is already signed in
	if linkAdminID != 0 {
		if err := h.store.LinkAccount(linkAdminID, identity.Subject, identity.DisplayName); err != nil {
			if errors.Is(err, storage.ErrAccountTaken) {
				h.oidcFail(w, r, "already_linked")
				return
			}
			h.oidcFail(w, r, "link_failed")
			return
		}
		_ = h.store.AppendAudit(storage.AuditEntry{
			ActorAdminID: linkAdminID, Action: "admin.account_linked",
			TargetType: "admin", Message: identity.DisplayName,
		})
		http.Redirect(w, r, redirectTarget(returnTo, "/admin/account"), http.StatusFound)
		return
	}

	admin, sess, err := h.auth.LoginWithAccount(
		identity.Subject, identity.Username, identity.DisplayName, invite,
	)
	if err != nil {
		var suspended *auth.SuspendedError
		switch {
		case errors.As(err, &suspended):
			h.oidcFail(w, r, "suspended")
		case errors.Is(err, auth.ErrRegistrationClosed):
			h.oidcFail(w, r, "registration_closed")
		case errors.Is(err, auth.ErrRegistrationInvite):
			h.oidcFail(w, r, "invite_required")
		case errors.Is(err, auth.ErrUsernameTaken):
			h.oidcFail(w, r, "username_taken")
		default:
			h.oidcFail(w, r, "login_failed")
		}
		return
	}
	h.auth.WriteSessionCookie(w, sess)
	_ = h.store.AppendAudit(storage.AuditEntry{
		ActorAdminID: admin.ID, ActorUsername: admin.Username,
		Action: "admin.account_login", Message: identity.DisplayName,
	})
	http.Redirect(w, r, redirectTarget(returnTo, "/admin/clients"), http.StatusFound)
}

// oidcFail sends the browser back with a reason the page can explain, rather
// than dumping an OAuth error on somebody who only clicked a button.
func (h *Handler) oidcFail(w http.ResponseWriter, r *http.Request, reason string) {
	http.Redirect(w, r, "/login?account_error="+url.QueryEscape(reason), http.StatusFound)
}

// safeReturnTo keeps a redirect inside this panel. An open redirect on a login
// callback is a phishing primitive.
func safeReturnTo(candidate string) string {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" || !strings.HasPrefix(candidate, "/") || strings.HasPrefix(candidate, "//") {
		return ""
	}
	return candidate
}

func redirectTarget(returnTo, fallback string) string {
	if got := safeReturnTo(returnTo); got != "" {
		return got
	}
	return fallback
}
