package storage

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func treeStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(Options{Driver: DriverSQLite, DSN: filepath.Join(t.TempDir(), "tree.db")})
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// invite records that inviter brought invitee in
func invite(t *testing.T, st *Store, token string, inviter, invitee int64) {
	t.Helper()
	if _, err := st.CreateInvite(token, time.Now().Add(time.Hour), inviter); err != nil {
		t.Fatal(err)
	}
	if err := st.RedeemInvite(token, invitee); err != nil {
		t.Fatal(err)
	}
}

func makeAdmin(t *testing.T, st *Store, name string) int64 {
	t.Helper()
	a, err := st.CreateAdmin(name, "hash", false, RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	return a.ID
}

// Founding accounts predate invites entirely. Treating them as orphans would
// leave the tree empty on a panel with 42 admins already in it
func TestAdminsNobodyInvitedAreRoots(t *testing.T) {
	st := treeStore(t)
	root := makeAdmin(t, st, "root-one")
	child := makeAdmin(t, st, "child")
	invite(t, st, "tok-1", root, child)

	members, err := st.InviteTree()
	if err != nil {
		t.Fatal(err)
	}
	byID := map[int64]TreeMember{}
	for _, m := range members {
		byID[m.AdminID] = m
	}
	if got := byID[root]; got.Depth != 0 || got.InvitedBy != 0 {
		t.Errorf("root = %+v, want depth 0 and no inviter", got)
	}
	if got := byID[child]; got.Depth != 1 || got.InvitedBy != root {
		t.Errorf("child = %+v", got)
	}
}

// The point of the tree is cutting a branch at an arbitrary point, not removing
// one leaf at a time
func TestCuttingABranchTakesEverythingBelowIt(t *testing.T) {
	st := treeStore(t)
	root := makeAdmin(t, st, "root-one")
	bad := makeAdmin(t, st, "bad")
	kid := makeAdmin(t, st, "kid")
	grandkid := makeAdmin(t, st, "grandkid")
	bystander := makeAdmin(t, st, "bystander")
	invite(t, st, "t1", root, bad)
	invite(t, st, "t2", bad, kid)
	invite(t, st, "t3", kid, grandkid)
	invite(t, st, "t4", root, bystander)

	affected, err := st.SuspendSubtree(bad, "selling accounts")
	if err != nil {
		t.Fatal(err)
	}
	if affected != 3 {
		t.Errorf("cut %d accounts, want the branch of three", affected)
	}
	for _, id := range []int64{bad, kid, grandkid} {
		suspended, reason, err := st.IsSuspended(id)
		if err != nil {
			t.Fatal(err)
		}
		if !suspended {
			t.Errorf("admin %d survived the cut", id)
		}
		if reason != "selling accounts" {
			t.Errorf("admin %d reason = %q", id, reason)
		}
	}
	for _, id := range []int64{root, bystander} {
		if suspended, _, _ := st.IsSuspended(id); suspended {
			t.Errorf("admin %d was cut and should not have been", id)
		}
	}
}

// A cut that leaves the branch logged in until their cookie expires is not a cut
func TestCuttingABranchDropsItsSessions(t *testing.T) {
	st := treeStore(t)
	root := makeAdmin(t, st, "root-one")
	bad := makeAdmin(t, st, "bad")
	invite(t, st, "t1", root, bad)
	if _, err := st.CreateSession("sess-bad", bad, time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateSession("sess-root", root, time.Hour); err != nil {
		t.Fatal(err)
	}

	if _, err := st.SuspendSubtree(bad, "abuse"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.LookupSession("sess-bad"); err == nil {
		t.Error("the cut branch is still logged in")
	}
	if _, err := st.LookupSession("sess-root"); err != nil {
		t.Errorf("an unrelated session was dropped: %v", err)
	}
}

// An owner cutting themselves locks everybody out of the panel
func TestAnOwnerCannotBeCut(t *testing.T) {
	st := treeStore(t)
	owner, err := st.CreateAdmin("the-owner", "hash", false, RoleOwner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SuspendSubtree(owner.ID, "oops"); !errors.Is(err, ErrCannotSuspendOwner) {
		t.Errorf("err = %v, want ErrCannotSuspendOwner", err)
	}
}

// Restoring must undo only this cut. Half-undoing somebody else's decision
// because the same account happened to be under two cuts would be worse than
// leaving them suspended
func TestRestoringLiftsOnlyItsOwnCut(t *testing.T) {
	st := treeStore(t)
	root := makeAdmin(t, st, "root-one")
	upper := makeAdmin(t, st, "upper")
	lower := makeAdmin(t, st, "lower")
	invite(t, st, "t1", root, upper)
	invite(t, st, "t2", upper, lower)

	if _, err := st.SuspendSubtree(lower, "personal"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SuspendSubtree(upper, "whole branch"); err != nil {
		t.Fatal(err)
	}
	// upper was cut by its own action; lower was already cut by the earlier one
	if _, err := st.RestoreSubtree(upper); err != nil {
		t.Fatal(err)
	}
	if suspended, _, _ := st.IsSuspended(upper); suspended {
		t.Error("upper stayed cut after its own cut was lifted")
	}
	if suspended, _, _ := st.IsSuspended(lower); !suspended {
		t.Error("lifting one cut also lifted a separate one made lower down")
	}
}

// Somebody under a cut made above them is not free to act just because they are
// not personally suspended
func TestInheritedCutIsVisibleInTheTree(t *testing.T) {
	st := treeStore(t)
	root := makeAdmin(t, st, "root-one")
	bad := makeAdmin(t, st, "bad")
	kid := makeAdmin(t, st, "kid")
	invite(t, st, "t1", root, bad)
	invite(t, st, "t2", bad, kid)
	if _, err := st.SuspendSubtree(bad, "abuse"); err != nil {
		t.Fatal(err)
	}

	members, err := st.InviteTree()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range members {
		if m.AdminID == kid && !m.Cut {
			t.Error("a descendant of a cut branch does not show as cut")
		}
		if m.AdminID == root && m.Cut {
			t.Error("the root shows as cut")
		}
	}
}

// A malformed edge must cost a bounded query rather than the process
func TestACycleDoesNotHangTheWalk(t *testing.T) {
	st := treeStore(t)
	a := makeAdmin(t, st, "a")
	b := makeAdmin(t, st, "b")
	invite(t, st, "t1", a, b)
	invite(t, st, "t2", b, a)

	done := make(chan struct{})
	go func() {
		defer close(done)
		if _, err := st.InviteTree(); err != nil {
			t.Error(err)
		}
		if _, err := st.InviteSubtree(a); err != nil {
			t.Error(err)
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the walk did not terminate on a cyclic tree")
	}
}
