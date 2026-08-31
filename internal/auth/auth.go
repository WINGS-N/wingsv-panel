package auth

import (
	"crypto/rand"
	"crypto/sha512"
	"crypto/subtle"
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
	ErrUsernameInvalid    = errors.New("auth: username must be alphanumeric (a-z, 0-9)")
	ErrRegistrationClosed = errors.New("auth: registration closed")
	ErrRegistrationInvite = errors.New("auth: invite token required")
	ErrInviteTokenInvalid = errors.New("auth: invite token invalid or expired")
)

// NormalizeUsername strips whitespace and lowercases the input. Login lookups
// run the user input through this so that mixed-case typing still finds the
// account; the storage migration backfills lowercase for every existing row.
func NormalizeUsername(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// ValidateNewUsername enforces alphanumeric-only usernames (a-z, 0-9) for new
// registrations. Existing accounts created before this constraint may contain
// other characters; they keep working for login because the lookup path only
// normalizes (lowercase+trim) without re-validating.
func ValidateNewUsername(username string) (string, error) {
	normalized := NormalizeUsername(username)
	if len(normalized) < MinUsernameLen {
		return "", ErrUsernameTooShort
	}
	for _, r := range normalized {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')) {
			return "", ErrUsernameInvalid
		}
	}
	return normalized, nil
}

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
	_, err = s.store.CreateAdmin(NormalizeUsername(username), hash, true, storage.RoleOwner)
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
	admin, err := s.store.FindAdminByUsername(NormalizeUsername(username))
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return storage.Admin{}, storage.AdminSession{}, ErrInvalidCredentials
		}
		return storage.Admin{}, storage.AdminSession{}, err
	}
	if !VerifyPassword(admin.PasswordHash, password) {
		return storage.Admin{}, storage.AdminSession{}, ErrInvalidCredentials
	}
	// A cut branch cannot log back in. Checked after the password so a suspended
	// account is not distinguishable from a wrong password by anyone guessing.
	if suspended, reason, err := s.store.IsSuspended(admin.ID); err == nil && suspended {
		return storage.Admin{}, storage.AdminSession{}, &SuspendedError{Reason: reason}
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

// VerifyCredentials проверяет логин с паролем, не заводя браузерной сессии.
// Нужен приложению: оно держит свой токен устройства, а cookie ему некуда класть
func (s *Service) VerifyCredentials(username, password string) (storage.Admin, error) {
	admin, err := s.store.FindAdminByUsername(NormalizeUsername(username))
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return storage.Admin{}, ErrInvalidCredentials
		}
		return storage.Admin{}, err
	}
	if !VerifyPassword(admin.PasswordHash, password) {
		return storage.Admin{}, ErrInvalidCredentials
	}
	if suspended, reason, err := s.store.IsSuspended(admin.ID); err == nil && suspended {
		return storage.Admin{}, &SuspendedError{Reason: reason}
	}
	_ = s.store.MarkAdminLogin(admin.ID)
	return admin, nil
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
	// Suspension is re-checked on every request, not only at login: a cut that
	// waits for a cookie to expire is not a cut.
	if suspended, reason, err := s.store.IsSuspended(admin.ID); err == nil && suspended {
		return storage.Admin{}, &SuspendedError{Reason: reason}
	}
	return admin, nil
}

// SuspendedError means the admin's branch of the invite tree was cut. Separate
// from a wrong password so the panel can explain what happened rather than
// leaving somebody retyping a password that was never the problem.
type SuspendedError struct{ Reason string }

func (e *SuspendedError) Error() string {
	if e.Reason == "" {
		return "account suspended"
	}
	return "account suspended: " + e.Reason
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
	normalized, err := ValidateNewUsername(username)
	if err != nil {
		return storage.Admin{}, storage.AdminSession{}, err
	}
	username = normalized
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
	// Пришедший по приглашению получает личный доступ к VPN, но не панель:
	// панель добавляет владелец отдельно
	admin, err := s.store.CreateAccount(username, hash, false, storage.RoleAdmin, false)
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

// clientTokenHashPrefix tags the digest form so a stored hash identifies its own
// scheme and legacy bcrypt rows keep verifying.
const clientTokenHashPrefix = "sha512:"

func GenerateClientToken() ([]byte, string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, "", err
	}
	return buf, HashClientToken(buf), nil
}

// HashClientToken digests a client token for storage.
//
// Deliberately NOT bcrypt. A work factor exists to make guessing a low-entropy
// human password expensive; a client token is 32 bytes of crypto/rand, so there
// is nothing to guess and the only thing the cost buys is CPU burned on every
// device sync - and an unauthenticated caller who knows a client id can force
// that work at will. A plain SHA-512 over 256 bits of entropy is preimage-proof
// and runs in microseconds. Admin passwords stay on bcrypt, where it belongs.
func HashClientToken(token []byte) string {
	sum := sha512.Sum512(token)
	return clientTokenHashPrefix + hex.EncodeToString(sum[:])
}

func VerifyClientToken(hash string, token []byte) bool {
	if strings.HasPrefix(hash, clientTokenHashPrefix) {
		return subtle.ConstantTimeCompare([]byte(hash), []byte(HashClientToken(token))) == 1
	}
	// Legacy bcrypt row from before the scheme change; still valid, and the
	// caller rewrites it after a successful verify.
	return bcrypt.CompareHashAndPassword([]byte(hash), token) == nil
}

// ClientTokenHashNeedsUpgrade reports whether a stored hash is still in the old
// bcrypt form and should be rewritten once the token has verified.
func ClientTokenHashNeedsUpgrade(hash string) bool {
	return !strings.HasPrefix(hash, clientTokenHashPrefix)
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

// ErrMatrixAccountUnknown means nobody has attached this Matrix account, and the
// deployment is not accepting new registrations from it.
var ErrMatrixAccountUnknown = errors.New("auth: this matrix account is not linked to an admin")

// LoginWithMatrix signs somebody in with an account on our own homeserver.
//
// A first-time account is subject to exactly the same registration rules as a
// password signup. That matters more than it looks: if Matrix login could create
// an admin while registration is invite-only, it would be a way straight around
// the invite tree, and the tree is the only thing making an identity cost
// anything at all.
func (s *Service) LoginWithMatrix(matrixID, localpart, subject, inviteToken string) (storage.Admin, storage.AdminSession, error) {
	matrixID = strings.ToLower(strings.TrimSpace(matrixID))
	if matrixID == "" {
		return storage.Admin{}, storage.AdminSession{}, ErrInvalidCredentials
	}

	admin, err := s.store.FindAdminByMatrixID(matrixID)
	switch {
	case err == nil:
		if suspended, reason, sErr := s.store.IsSuspended(admin.ID); sErr == nil && suspended {
			return storage.Admin{}, storage.AdminSession{}, &SuspendedError{Reason: reason}
		}
		sess, sErr := s.newSession(admin.ID)
		if sErr != nil {
			return storage.Admin{}, storage.AdminSession{}, sErr
		}
		_ = s.store.MarkAdminLogin(admin.ID)
		return admin, sess, nil
	case !errors.Is(err, storage.ErrNotFound):
		return storage.Admin{}, storage.AdminSession{}, err
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

	username, err := ValidateNewUsername(localpart)
	if err != nil {
		return storage.Admin{}, storage.AdminSession{}, err
	}
	if _, err := s.store.FindAdminByUsername(username); err == nil {
		// Somebody already holds the name. Refusing beats silently taking over an
		// existing password account with a Matrix login
		return storage.Admin{}, storage.AdminSession{}, ErrUsernameTaken
	} else if !errors.Is(err, storage.ErrNotFound) {
		return storage.Admin{}, storage.AdminSession{}, err
	}

	// No password: this account signs in through the homeserver and nothing else
	hash, err := HashPassword(mustRandomPassword())
	if err != nil {
		return storage.Admin{}, storage.AdminSession{}, err
	}
	created, err := s.store.CreateAccount(username, hash, false, storage.RoleAdmin, false)
	if err != nil {
		return storage.Admin{}, storage.AdminSession{}, err
	}
	if mode == RegistrationModeInvite {
		if err := s.store.RedeemInvite(inviteToken, created.ID); err != nil {
			_ = s.store.DeleteAdmin(created.ID)
			return storage.Admin{}, storage.AdminSession{}, ErrInviteTokenInvalid
		}
	}
	if err := s.store.LinkMatrixID(created.ID, matrixID, subject); err != nil {
		_ = s.store.DeleteAdmin(created.ID)
		return storage.Admin{}, storage.AdminSession{}, err
	}
	sess, err := s.newSession(created.ID)
	if err != nil {
		return storage.Admin{}, storage.AdminSession{}, err
	}
	_ = s.store.MarkAdminLogin(created.ID)
	return created, sess, nil
}

// newSession mints a session for an admin who has already been authenticated.
func (s *Service) newSession(adminID int64) (storage.AdminSession, error) {
	id, err := newToken(32)
	if err != nil {
		return storage.AdminSession{}, err
	}
	return s.store.CreateSession(id, adminID, SessionTTL)
}

// mustRandomPassword fills the password column for an account that never uses
// one. It is unguessable and nobody is ever told it, so the only way in is the
// homeserver.
func mustRandomPassword() string {
	token, err := newToken(32)
	if err != nil {
		// newToken only fails if the system entropy source does, at which point
		// nothing here is safe to continue with
		panic("auth: no entropy for a placeholder password: " + err.Error())
	}
	return token
}
