package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"v.wingsnet.org/internal/storage"
)

const (
	SessionCookieName = "wingsv_admin_session"
	SessionTTL        = 14 * 24 * time.Hour
	BcryptCost        = 12
	MinPasswordLen    = 8
	MinUsernameLen    = 3
)

const (
	RegistrationModeOpen   = "open"
	RegistrationModeInvite = "invite"
	RegistrationModeClosed = "closed"
)

var (
	ErrInvalidCredentials = errors.New("auth: invalid credentials")
	ErrSessionExpired     = errors.New("auth: session expired")
	ErrUsernameTaken      = errors.New("auth: username taken")
	ErrPasswordTooShort   = errors.New("auth: password too short")
	ErrUsernameTooShort   = errors.New("auth: username too short")
	ErrRegistrationClosed = errors.New("auth: registration closed")
	ErrRegistrationInvite = errors.New("auth: invite token required")
	ErrInviteTokenInvalid = errors.New("auth: invite token invalid or expired")
)

type Service struct {
	store        *storage.Store
	cookieSecure bool
}

func New(store *storage.Store, cookieSecure bool) *Service {
	return &Service{store: store, cookieSecure: cookieSecure}
}

// Bootstrap creates the very first admin (role=owner) when the admins table is
// empty. On non-empty databases it's a no-op; the caller should follow it with
// EnsureAtLeastOneOwner so legacy admins still get an owner promoted.
func (s *Service) Bootstrap(username, password string) error {
	count, err := s.store.CountAdmins()
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	_, err = s.store.CreateAdmin(username, hash, true, storage.RoleOwner)
	return err
}

func (s *Service) EnsureAtLeastOneOwner() error {
	return s.store.EnsureAtLeastOneOwner()
}

func HashPassword(password string) (string, error) {
	if len(password) < 1 {
		return "", errors.New("auth: empty password")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), BcryptCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func VerifyPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func (s *Service) Login(username, password string) (storage.Admin, storage.AdminSession, error) {
	admin, err := s.store.FindAdminByUsername(strings.TrimSpace(username))
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return storage.Admin{}, storage.AdminSession{}, ErrInvalidCredentials
		}
		return storage.Admin{}, storage.AdminSession{}, err
	}
	if !VerifyPassword(admin.PasswordHash, password) {
		return storage.Admin{}, storage.AdminSession{}, ErrInvalidCredentials
	}
	id, err := newToken(32)
	if err != nil {
		return storage.Admin{}, storage.AdminSession{}, err
	}
	sess, err := s.store.CreateSession(id, admin.ID, SessionTTL)
	if err != nil {
		return storage.Admin{}, storage.AdminSession{}, err
	}
	_ = s.store.MarkAdminLogin(admin.ID)
	return admin, sess, nil
}

func (s *Service) Logout(sessionID string) error {
	if sessionID == "" {
		return nil
	}
	return s.store.DeleteSession(sessionID)
}

func (s *Service) Authenticate(r *http.Request) (storage.Admin, error) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return storage.Admin{}, ErrSessionExpired
	}
	sess, err := s.store.LookupSession(cookie.Value)
	if err != nil {
		return storage.Admin{}, ErrSessionExpired
	}
	admin, err := s.store.FindAdminByID(sess.AdminID)
	if err != nil {
		return storage.Admin{}, ErrSessionExpired
	}
	return admin, nil
}

func (s *Service) ChangePassword(adminID int64, oldPassword, newPassword string) error {
	admin, err := s.store.FindAdminByID(adminID)
	if err != nil {
		return err
	}
	if !VerifyPassword(admin.PasswordHash, oldPassword) {
		return ErrInvalidCredentials
	}
	if len(newPassword) < MinPasswordLen {
		return ErrPasswordTooShort
	}
	hash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	return s.store.UpdateAdminPassword(adminID, hash, false)
}

// Register creates a new admin honouring the platform's registration mode.
// inviteToken may be empty for open mode; required for invite mode.
func (s *Service) Register(username, password, inviteToken string) (storage.Admin, storage.AdminSession, error) {
	username = strings.TrimSpace(username)
	if len(username) < MinUsernameLen {
		return storage.Admin{}, storage.AdminSession{}, ErrUsernameTooShort
	}
	if len(password) < MinPasswordLen {
		return storage.Admin{}, storage.AdminSession{}, ErrPasswordTooShort
	}
	mode, err := s.store.GetPlatformSetting(storage.SettingRegistrationMode, RegistrationModeOpen)
	if err != nil {
		return storage.Admin{}, storage.AdminSession{}, err
	}
	switch mode {
	case RegistrationModeClosed:
		return storage.Admin{}, storage.AdminSession{}, ErrRegistrationClosed
	case RegistrationModeInvite:
		if strings.TrimSpace(inviteToken) == "" {
			return storage.Admin{}, storage.AdminSession{}, ErrRegistrationInvite
		}
	}
	if _, err := s.store.FindAdminByUsername(username); err == nil {
		return storage.Admin{}, storage.AdminSession{}, ErrUsernameTaken
	} else if !errors.Is(err, storage.ErrNotFound) {
		return storage.Admin{}, storage.AdminSession{}, err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return storage.Admin{}, storage.AdminSession{}, err
	}
	admin, err := s.store.CreateAdmin(username, hash, false, storage.RoleAdmin)
	if err != nil {
		return storage.Admin{}, storage.AdminSession{}, err
	}
	if mode == RegistrationModeInvite {
		if err := s.store.RedeemInvite(inviteToken, admin.ID); err != nil {
			_ = s.store.DeleteAdmin(admin.ID)
			return storage.Admin{}, storage.AdminSession{}, ErrInviteTokenInvalid
		}
	}
	id, err := newToken(32)
	if err != nil {
		return storage.Admin{}, storage.AdminSession{}, err
	}
	sess, err := s.store.CreateSession(id, admin.ID, SessionTTL)
	if err != nil {
		return storage.Admin{}, storage.AdminSession{}, err
	}
	_ = s.store.MarkAdminLogin(admin.ID)
	return admin, sess, nil
}

// ResetPasswordTo force-replaces an admin's password (owner-only flow).
// Sets must_change_password=true so the user changes it on next login.
func (s *Service) ResetPasswordTo(adminID int64, newPassword string) error {
	if len(newPassword) < MinPasswordLen {
		return ErrPasswordTooShort
	}
	hash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	return s.store.UpdateAdminPassword(adminID, hash, true)
}

func IsOwner(admin storage.Admin) bool {
	return admin.Role == storage.RoleOwner
}

func (s *Service) WriteSessionCookie(w http.ResponseWriter, sess storage.AdminSession) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    sess.ID,
		Path:     "/",
		Expires:  sess.ExpiresAt,
		HttpOnly: true,
		Secure:   s.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Service) ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func newToken(numBytes int) (string, error) {
	buf := make([]byte, numBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func GenerateClientToken() ([]byte, string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, "", err
	}
	hash, err := bcrypt.GenerateFromPassword(buf, BcryptCost)
	if err != nil {
		return nil, "", err
	}
	return buf, string(hash), nil
}

func VerifyClientToken(hash string, token []byte) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), token) == nil
}

func GenerateClientID() (string, error) {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// GenerateInviteToken returns a hex-encoded 16-byte token suitable for an
// invite link path.
func GenerateInviteToken() (string, error) {
	return newToken(16)
}
