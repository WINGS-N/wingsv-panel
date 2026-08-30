package auth

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"v.wingsnet.org/internal/storage"
)

func suspendStore(t *testing.T) (*storage.Store, *Service) {
	t.Helper()
	st, err := storage.Open(storage.Options{
		Driver: storage.DriverSQLite, DSN: filepath.Join(t.TempDir(), "suspend.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return st, New(st, false)
}

// A cut branch cannot log back in, and the panel has to be able to say why
// rather than leave somebody retyping a password that was never the problem
func TestASuspendedAdminCannotLogIn(t *testing.T) {
	st, svc := suspendStore(t)
	hash, err := HashPassword("correct-horse")
	if err != nil {
		t.Fatal(err)
	}
	admin, err := st.CreateAdmin("cut-admin", hash, false, storage.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := svc.Login("cut-admin", "correct-horse"); err != nil {
		t.Fatalf("login before the cut: %v", err)
	}
	if _, err := st.SuspendSubtree(admin.ID, "abuse"); err != nil {
		t.Fatal(err)
	}

	_, _, err = svc.Login("cut-admin", "correct-horse")
	var suspended *SuspendedError
	if !asSuspended(err, &suspended) {
		t.Fatalf("err = %v, want a suspension", err)
	}
	if suspended.Reason != "abuse" {
		t.Errorf("reason = %q", suspended.Reason)
	}
	// A wrong password still looks like a wrong password: a suspended account
	// must not be distinguishable to somebody guessing
	if _, _, err := svc.Login("cut-admin", "wrong"); !isInvalidCredentials(err) {
		t.Errorf("a wrong password on a suspended account reported %v", err)
	}
}

// A cut that waits for a cookie to expire is not a cut, so every request
// re-checks rather than trusting the session
func TestASuspensionTakesEffectMidSession(t *testing.T) {
	st, svc := suspendStore(t)
	hash, err := HashPassword("correct-horse")
	if err != nil {
		t.Fatal(err)
	}
	admin, err := st.CreateAdmin("mid-session", hash, false, storage.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	_, sess, err := svc.Login("mid-session", "correct-horse")
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/admin/me", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: sess.ID})
	if _, err := svc.Authenticate(req); err != nil {
		t.Fatalf("authenticate before the cut: %v", err)
	}

	if _, err := st.SuspendSubtree(admin.ID, "abuse"); err != nil {
		t.Fatal(err)
	}
	// The session row is gone as well, but the check must not depend on that
	if _, err := svc.Authenticate(req); err == nil {
		t.Error("a cut admin is still authenticated on their old cookie")
	}
}

func asSuspended(err error, target **SuspendedError) bool {
	if err == nil {
		return false
	}
	s, ok := err.(*SuspendedError)
	if ok {
		*target = s
	}
	return ok
}

func isInvalidCredentials(err error) bool { return err == ErrInvalidCredentials }
