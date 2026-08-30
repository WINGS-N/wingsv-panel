package auth

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"v.wingsnet.org/internal/storage"
)

func matrixStore(t *testing.T, mode string) (*storage.Store, *Service) {
	t.Helper()
	st, err := storage.Open(storage.Options{
		Driver: storage.DriverSQLite, DSN: filepath.Join(t.TempDir(), "matrix.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetPlatformSetting(storage.SettingRegistrationMode, mode); err != nil {
		t.Fatal(err)
	}
	return st, New(st, false)
}

// The invite tree is the only thing making an identity cost anything. If a
// Matrix login could create an admin while registration is invite-only, it would
// be a way straight around it
func TestMatrixLoginDoesNotBypassInviteOnlyRegistration(t *testing.T) {
	st, svc := matrixStore(t, RegistrationModeInvite)
	_, _, err := svc.LoginWithMatrix("@newcomer:wings.example", "newcomer", "sub-1", "")
	if !errors.Is(err, ErrRegistrationInvite) {
		t.Fatalf("err = %v, want an invite to be required", err)
	}
	if _, err := st.FindAdminByUsername("newcomer"); !errors.Is(err, storage.ErrNotFound) {
		t.Error("an admin was created without an invite")
	}
}

// And a closed panel stays closed
func TestMatrixLoginRespectsClosedRegistration(t *testing.T) {
	_, svc := matrixStore(t, RegistrationModeClosed)
	if _, _, err := svc.LoginWithMatrix("@newcomer:wings.example", "newcomer", "sub-1", ""); !errors.Is(err, ErrRegistrationClosed) {
		t.Errorf("err = %v, want registration closed", err)
	}
}

// With an invite it works, the invite is spent, and the edge lands in the tree
func TestMatrixLoginWithAnInviteJoinsTheTree(t *testing.T) {
	st, svc := matrixStore(t, RegistrationModeInvite)
	inviter, err := st.CreateAdmin("inviter", "hash", false, storage.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateInvite("tok-1", time.Now().Add(time.Hour), inviter.ID); err != nil {
		t.Fatal(err)
	}

	admin, sess, err := svc.LoginWithMatrix("@newcomer:wings.example", "newcomer", "sub-1", "tok-1")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if admin.Username != "newcomer" || sess.ID == "" {
		t.Errorf("admin = %+v sess = %+v", admin, sess)
	}
	members, err := st.InviteTree()
	if err != nil {
		t.Fatal(err)
	}
	var linked bool
	for _, m := range members {
		if m.AdminID == admin.ID && m.InvitedBy == inviter.ID {
			linked = true
		}
	}
	if !linked {
		t.Error("the new admin is not attached under the inviter")
	}
	// The invite is spent, so the same token cannot bring in a second identity
	if _, _, err := svc.LoginWithMatrix("@second:wings.example", "second", "sub-2", "tok-1"); err == nil {
		t.Error("one invite let in two identities")
	}
}

// Signing in again finds the same admin instead of making a new one
func TestMatrixLoginIsStableAcrossSessions(t *testing.T) {
	_, svc := matrixStore(t, RegistrationModeOpen)
	first, _, err := svc.LoginWithMatrix("@stable:wings.example", "stable", "sub-1", "")
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := svc.LoginWithMatrix("@stable:wings.example", "stable", "sub-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Errorf("two logins produced admins %d and %d", first.ID, second.ID)
	}
}

// A cut branch cannot come back in through the side door
func TestASuspendedAdminCannotSignInWithMatrix(t *testing.T) {
	st, svc := matrixStore(t, RegistrationModeOpen)
	admin, _, err := svc.LoginWithMatrix("@cut:wings.example", "cut", "sub-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SuspendSubtree(admin.ID, "abuse"); err != nil {
		t.Fatal(err)
	}
	_, _, err = svc.LoginWithMatrix("@cut:wings.example", "cut", "sub-1", "")
	var suspended *SuspendedError
	if !errors.As(err, &suspended) {
		t.Fatalf("err = %v, want a suspension", err)
	}
}

// Taking over an existing password account with a Matrix login of the same name
// would be an account takeover, not a convenience
func TestMatrixLoginWillNotClaimAnExistingUsername(t *testing.T) {
	st, svc := matrixStore(t, RegistrationModeOpen)
	if _, err := st.CreateAdmin("taken", "hash", false, storage.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.LoginWithMatrix("@taken:wings.example", "taken", "sub-1", ""); !errors.Is(err, ErrUsernameTaken) {
		t.Errorf("err = %v, want the name to be refused", err)
	}
}

// One Matrix account, one admin
func TestOneMatrixAccountCannotBeLinkedTwice(t *testing.T) {
	st, svc := matrixStore(t, RegistrationModeOpen)
	first, _, err := svc.LoginWithMatrix("@one:wings.example", "one", "sub-1", "")
	if err != nil {
		t.Fatal(err)
	}
	other, err := st.CreateAdmin("other", "hash", false, storage.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.LinkMatrixID(other.ID, "@one:wings.example", "sub-1"); !errors.Is(err, storage.ErrMatrixIDTaken) {
		t.Errorf("err = %v, want the account to be taken", err)
	}
	got, err := st.FindAdminByMatrixID("@one:wings.example")
	if err != nil || got.ID != first.ID {
		t.Errorf("account resolved to %+v err=%v", got, err)
	}
}

// Uniqueness on the Matrix account has to allow any number of admins with no
// account at all. An empty string instead of NULL would make the second
// unlinked admin collide with the first
func TestManyAdminsCanHaveNoMatrixAccount(t *testing.T) {
	st, _ := matrixStore(t, RegistrationModeOpen)
	for _, name := range []string{"alpha", "bravo", "charlie"} {
		if _, err := st.CreateAdmin(name, "hash", false, storage.RoleAdmin); err != nil {
			t.Fatalf("creating %s: %v", name, err)
		}
	}
	admins, err := st.ListAdmins()
	if err != nil {
		t.Fatal(err)
	}
	if len(admins) != 3 {
		t.Fatalf("got %d admins, want all three", len(admins))
	}
	for _, a := range admins {
		id, err := st.MatrixIDFor(a.ID)
		if err != nil || id != "" {
			t.Errorf("%s has matrix id %q err=%v", a.Username, id, err)
		}
	}
}
