package storage

import (
	"path/filepath"
	"testing"
	"time"
)

func redeemStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(Options{Driver: DriverSQLite, DSN: filepath.Join(t.TempDir(), "test.db")})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// Погашение обязано оставлять запись: без неё код списан, а человек вне дерева
func TestRedeemInviteRecordsTheRedemption(t *testing.T) {
	st := redeemStore(t)
	inviter, err := st.CreateAdmin("inviter", "hash", false, RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	guest, err := st.CreateAccount("guest", "hash", false, RoleAdmin, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateInviteWithUses("code-1", time.Time{}, inviter.ID, 2); err != nil {
		t.Fatal(err)
	}

	if redeemed, err := st.RedeemedInvite(guest.ID); err != nil || redeemed {
		t.Fatalf("до погашения: redeemed=%v err=%v", redeemed, err)
	}
	if err := st.RedeemInvite("code-1", guest.ID); err != nil {
		t.Fatalf("RedeemInvite: %v", err)
	}
	redeemed, err := st.RedeemedInvite(guest.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !redeemed {
		t.Fatal("погашение не записано: код списан, а человек вне дерева")
	}
}

// У многоразового кода ребро получает каждый пришедший, а не только первый
func TestInviteTreeCoversEveryRedeemer(t *testing.T) {
	st := redeemStore(t)
	inviter, _ := st.CreateAdmin("inviter", "hash", false, RoleAdmin)
	first, _ := st.CreateAccount("first", "hash", false, RoleAdmin, false)
	second, _ := st.CreateAccount("second", "hash", false, RoleAdmin, false)
	if _, err := st.CreateInviteWithUses("code-2", time.Time{}, inviter.ID, 5); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{first.ID, second.ID} {
		if err := st.RedeemInvite("code-2", id); err != nil {
			t.Fatalf("RedeemInvite: %v", err)
		}
	}

	parent, err := st.inviteEdges()
	if err != nil {
		t.Fatal(err)
	}
	if parent[first.ID] != inviter.ID || parent[second.ID] != inviter.ID {
		t.Fatalf("дерево неполное: %+v", parent)
	}
}

// Исчерпанный код больше не пускает
func TestRedeemInviteStopsAtTheLimit(t *testing.T) {
	st := redeemStore(t)
	inviter, _ := st.CreateAdmin("inviter", "hash", false, RoleAdmin)
	guest, _ := st.CreateAccount("guest", "hash", false, RoleAdmin, false)
	other, _ := st.CreateAccount("other", "hash", false, RoleAdmin, false)
	if _, err := st.CreateInviteWithUses("code-3", time.Time{}, inviter.ID, 1); err != nil {
		t.Fatal(err)
	}
	if err := st.RedeemInvite("code-3", guest.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.RedeemInvite("code-3", other.ID); err == nil {
		t.Fatal("исчерпанный код пустил второго")
	}
}

// Отозванный код перестаёт существовать
func TestDeleteInviteRevokesIt(t *testing.T) {
	st := redeemStore(t)
	inviter, _ := st.CreateAdmin("inviter", "hash", false, RoleAdmin)
	if _, err := st.CreateInviteWithUses("code-4", time.Time{}, inviter.ID, 3); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteInvite("code-4"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.FindInvite("code-4"); err == nil {
		t.Fatal("отозванный код всё ещё находится")
	}
	guest, _ := st.CreateAccount("guest", "hash", false, RoleAdmin, false)
	if err := st.RedeemInvite("code-4", guest.ID); err == nil {
		t.Fatal("по отозванному коду удалось зарегистрироваться")
	}
}
