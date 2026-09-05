package auth

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"v.wingsnet.org/internal/storage"
)

func accountStore(t *testing.T, mode string) (*storage.Store, *Service) {
	t.Helper()
	st, err := storage.Open(storage.Options{
		Driver: storage.DriverSQLite, DSN: filepath.Join(t.TempDir(), "account.db"),
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
// this login could create an admin while registration is invite-only, it would
// be a way straight around it
func TestAccountLoginDoesNotBypassInviteOnlyRegistration(t *testing.T) {
	st, svc := accountStore(t, RegistrationModeInvite)
	_, _, err := svc.LoginWithAccount("sub-1", "newcomer", "", "")
	if !errors.Is(err, ErrRegistrationInvite) {
		t.Fatalf("err = %v, want an invite to be required", err)
	}
	if _, err := st.FindAdminByUsername("newcomer"); !errors.Is(err, storage.ErrNotFound) {
		t.Error("an admin was created without an invite")
	}
}

// And a closed panel stays closed
func TestAccountLoginRespectsClosedRegistration(t *testing.T) {
	_, svc := accountStore(t, RegistrationModeClosed)
	if _, _, err := svc.LoginWithAccount("sub-1", "newcomer", "", ""); !errors.Is(err, ErrRegistrationClosed) {
		t.Errorf("err = %v, want registration closed", err)
	}
}

// With an invite it works, the invite is spent, and the edge lands in the tree
func TestAccountLoginWithAnInviteJoinsTheTree(t *testing.T) {
	st, svc := accountStore(t, RegistrationModeInvite)
	inviter, err := st.CreateAdmin("inviter", "hash", false, storage.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateInvite("tok-1", time.Now().Add(time.Hour), inviter.ID); err != nil {
		t.Fatal(err)
	}

	admin, sess, err := svc.LoginWithAccount("sub-1", "newcomer", "", "tok-1")
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
	if _, _, err := svc.LoginWithAccount("sub-2", "second", "", "tok-1"); err == nil {
		t.Error("one invite let in two identities")
	}
}

// Signing in again finds the same admin instead of making a new one
func TestAccountLoginIsStableAcrossSessions(t *testing.T) {
	_, svc := accountStore(t, RegistrationModeOpen)
	first, _, err := svc.LoginWithAccount("sub-1", "stable", "", "")
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := svc.LoginWithAccount("sub-1", "stable", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Errorf("two logins produced admins %d and %d", first.ID, second.ID)
	}
}

// A cut branch cannot come back in through the side door
func TestASuspendedAdminCannotSignInWithAnAccount(t *testing.T) {
	st, svc := accountStore(t, RegistrationModeOpen)
	admin, _, err := svc.LoginWithAccount("sub-1", "cut", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SuspendSubtree(admin.ID, "abuse"); err != nil {
		t.Fatal(err)
	}
	_, _, err = svc.LoginWithAccount("sub-1", "cut", "", "")
	var suspended *SuspendedError
	if !errors.As(err, &suspended) {
		t.Fatalf("err = %v, want a suspension", err)
	}
}

// Taking over an existing password account with an account login of the same name
// would be an account takeover, not a convenience
func TestAccountLoginWillNotClaimAnExistingUsername(t *testing.T) {
	st, svc := accountStore(t, RegistrationModeOpen)
	if _, err := st.CreateAdmin("taken", "hash", false, storage.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.LoginWithAccount("sub-1", "taken", "", ""); !errors.Is(err, ErrUsernameTaken) {
		t.Errorf("err = %v, want the name to be refused", err)
	}
}

// One account, one admin
func TestOneAccountCannotBeLinkedTwice(t *testing.T) {
	st, svc := accountStore(t, RegistrationModeOpen)
	first, _, err := svc.LoginWithAccount("sub-1", "one", "", "")
	if err != nil {
		t.Fatal(err)
	}
	other, err := st.CreateAdmin("other", "hash", false, storage.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.LinkAccount(other.ID, "sub-1", "One"); !errors.Is(err, storage.ErrAccountTaken) {
		t.Errorf("err = %v, want the account to be taken", err)
	}
	got, err := st.FindAdminByAccount("sub-1")
	if err != nil || got.ID != first.ID {
		t.Errorf("account resolved to %+v err=%v", got, err)
	}
}

// Uniqueness on the account has to allow any number of admins with no
// account at all. An empty string instead of NULL would make the second
// unlinked admin collide with the first
func TestManyAdminsCanHaveNoAccount(t *testing.T) {
	st, _ := accountStore(t, RegistrationModeOpen)
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
		id, err := st.AccountNameFor(a.ID)
		if err != nil || id != "" {
			t.Errorf("%s has account %q err=%v", a.Username, id, err)
		}
	}
}
