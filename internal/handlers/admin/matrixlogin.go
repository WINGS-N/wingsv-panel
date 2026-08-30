package admin

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"v.wingsnet.org/internal/auth"
	"v.wingsnet.org/internal/matrixauth"
	"v.wingsnet.org/internal/storage"
)

// matrixTimeout bounds a round trip to the account service. Short: somebody is
// waiting on a redirect, and a hung homeserver must not hang the panel.
const matrixTimeout = 10 * time.Second

// handleMatrixStatus tells the login page whether to offer the button at all.
func (h *Handler) handleMatrixStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":    h.matrix != nil && h.matrix.Enabled(),
		"homeserver": h.cfg.MatrixHomeserver,
	})
}

// handleMatrixLink reports and removes the attached account.
//
// Removing is allowed only when a password still works, or an admin unlinks
// themselves out of the panel entirely with no way back in.
func (h *Handler) handleMatrixLink(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	switch r.Method {
	case http.MethodGet:
		matrixID, err := h.store.MatrixIDFor(admin.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"enabled":    h.matrix != nil && h.matrix.Enabled(),
			"homeserver": h.cfg.MatrixHomeserver,
			"matrix_id":  matrixID,
		})
	case http.MethodDelete:
		if err := h.store.UnlinkMatrixID(admin.ID); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		_ = h.store.AppendAudit(storage.AuditEntry{
			ActorAdminID: admin.ID, ActorUsername: admin.Username,
			Action: "admin.matrix_unlinked", TargetType: "admin",
		})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleMatrixStart sends the browser off to the account service.
func (h *Handler) handleMatrixStart(w http.ResponseWriter, r *http.Request) {
	if h.matrix == nil || !h.matrix.Enabled() {
		writeError(w, http.StatusNotFound, "matrix login is not configured")
		return
	}
	// An admin already signed in is attaching an account rather than signing in
	var linkAdminID int64
	if admin, err := h.auth.Authenticate(r); err == nil {
		linkAdminID = admin.ID
	}

	ctx, cancel := context.WithTimeout(r.Context(), matrixTimeout)
	defer cancel()
	target, err := h.matrix.Start(ctx, safeReturnTo(r.URL.Query().Get("return_to")), linkAdminID)
	if err != nil {
		writeError(w, http.StatusBadGateway, "account service unreachable: "+err.Error())
		return
	}
	http.Redirect(w, r, target, http.StatusFound)
}

// handleMatrixCallback finishes the login and drops the browser back in the panel.
func (h *Handler) handleMatrixCallback(w http.ResponseWriter, r *http.Request) {
	if h.matrix == nil || !h.matrix.Enabled() {
		writeError(w, http.StatusNotFound, "matrix login is not configured")
		return
	}
	if reason := r.URL.Query().Get("error"); reason != "" {
		h.matrixFail(w, r, reason)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), matrixTimeout)
	defer cancel()

	identity, returnTo, linkAdminID, err := h.matrix.Complete(ctx,
		r.URL.Query().Get("state"), r.URL.Query().Get("code"))
	if err != nil {
		switch {
		case errors.Is(err, matrixauth.ErrForeignHomeserver):
			h.matrixFail(w, r, "foreign_homeserver")
		case errors.Is(err, matrixauth.ErrUnknownState):
			h.matrixFail(w, r, "expired")
		default:
			h.matrixFail(w, r, "exchange_failed")
		}
		return
	}

	// Attaching an account to the admin who is already signed in
	if linkAdminID != 0 {
		if err := h.store.LinkMatrixID(linkAdminID, identity.MatrixID, identity.Subject); err != nil {
			if errors.Is(err, storage.ErrMatrixIDTaken) {
				h.matrixFail(w, r, "already_linked")
				return
			}
			h.matrixFail(w, r, "link_failed")
			return
		}
		h.importAvatar(ctx, linkAdminID, identity.AvatarURL)
		_ = h.store.AppendAudit(storage.AuditEntry{
			ActorAdminID: linkAdminID, Action: "admin.matrix_linked",
			TargetType: "admin", Message: identity.MatrixID,
		})
		http.Redirect(w, r, redirectTarget(returnTo, "/admin/account"), http.StatusFound)
		return
	}

	knownBefore := h.matrixAccountKnown(identity.MatrixID)
	admin, sess, err := h.auth.LoginWithMatrix(
		identity.MatrixID, identity.Localpart, identity.Subject,
		r.URL.Query().Get("invite"),
	)
	if err != nil {
		var suspended *auth.SuspendedError
		switch {
		case errors.As(err, &suspended):
			h.matrixFail(w, r, "suspended")
		case errors.Is(err, auth.ErrRegistrationClosed):
			h.matrixFail(w, r, "registration_closed")
		case errors.Is(err, auth.ErrRegistrationInvite):
			h.matrixFail(w, r, "invite_required")
		case errors.Is(err, auth.ErrUsernameTaken):
			h.matrixFail(w, r, "username_taken")
		default:
			h.matrixFail(w, r, "login_failed")
		}
		return
	}
	// Only on the login that created the account. An ordinary repeat login has no
	// business restyling somebody's profile behind their back
	if !knownBefore {
		h.importAvatar(ctx, admin.ID, identity.AvatarURL)
	}
	h.auth.WriteSessionCookie(w, sess)
	_ = h.store.AppendAudit(storage.AuditEntry{
		ActorAdminID: admin.ID, ActorUsername: admin.Username,
		Action: "admin.matrix_login", Message: identity.MatrixID,
	})
	http.Redirect(w, r, redirectTarget(returnTo, "/admin/clients"), http.StatusFound)
}

// matrixFail sends the browser back with a reason the page can explain, rather
// than dumping an OAuth error on somebody who only clicked a button.
func (h *Handler) matrixFail(w http.ResponseWriter, r *http.Request, reason string) {
	http.Redirect(w, r, "/login?matrix_error="+url.QueryEscape(reason), http.StatusFound)
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

// matrixAccountKnown reports whether this account already had an admin before
// the login, which is what separates a first sign-in from a repeat one
func (h *Handler) matrixAccountKnown(matrixID string) bool {
	_, err := h.store.FindAdminByMatrixID(matrixID)
	return err == nil
}

// importAvatar takes the Matrix picture, and only when there is one to take and
// nothing here to lose.
//
// Two conditions, both of them about not making a decision for somebody: an
// avatar uploaded here is their choice and is never overwritten, and a Matrix
// account with no picture has nothing worth pulling - replacing a real avatar
// with a default would be worse than doing nothing. Best effort throughout: a
// homeserver that will not serve a picture is not a reason to fail a login that
// already worked.
func (h *Handler) importAvatar(ctx context.Context, adminID int64, pictureURL string) {
	if strings.TrimSpace(pictureURL) == "" {
		return
	}
	has, err := h.store.HasAvatar(adminID)
	if err != nil || has {
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pictureURL, nil)
	if err != nil {
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return
	}
	mime := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(mime, "image/") {
		return
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxAvatarBytes+1))
	if err != nil || len(body) == 0 || len(body) > maxAvatarBytes {
		return
	}
	_, _ = h.store.SetAdminAvatar(adminID, mime, body)
}
