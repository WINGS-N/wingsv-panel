package storage

import (
	"path/filepath"
	"testing"
	"time"
)

func inviteStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(Options{Driver: DriverSQLite, DSN: filepath.Join(t.TempDir(), "invites.db")})
	if err != nil {
		t.Fatal(err)
	}
	// Инвайт ссылается на того, кто его выписал, так что автор должен
	// существовать: без него падает внешний ключ, а не проверяемая логика
	if _, err := st.CreateAdmin("inviter", "hash", false, "owner"); err != nil {
		t.Fatal(err)
	}
	// И те, кто по инвайтам придёт: погашение записывает первого из них
	for _, name := range []string{"joiner1", "joiner2", "joiner3", "joiner4"} {
		if _, err := st.CreateAdmin(name, "hash", false, "admin"); err != nil {
			t.Fatal(err)
		}
	}
	return st
}

// Код на нескольких человек должен пускать ровно столько, сколько заявлено, и
// закрываться на последнем.
func TestAGroupInviteIsSpentExactlyMaxUsesTimes(t *testing.T) {
	st := inviteStore(t)
	if _, err := st.CreateInviteWithUses("team", time.Time{}, 1, 3); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 3; i++ {
		if err := st.RedeemInvite("team", int64(1+i)); err != nil {
			t.Fatalf("регистрация %d отвалилась: %v", i, err)
		}
	}
	if err := st.RedeemInvite("team", 5); err == nil {
		t.Fatal("четвёртая регистрация прошла по коду на троих")
	}
}

// Обычный инвайт остаётся одноразовым: значение по умолчанию не должно втихую
// превратить каждый старый код в многоразовый.
func TestADefaultInviteStaysSingleUse(t *testing.T) {
	st := inviteStore(t)
	if _, err := st.CreateInvite("solo", time.Time{}, 1); err != nil {
		t.Fatal(err)
	}
	if err := st.RedeemInvite("solo", 2); err != nil {
		t.Fatal(err)
	}
	if err := st.RedeemInvite("solo", 3); err == nil {
		t.Fatal("одноразовый код сработал дважды")
	}
}

// Срок жизни важнее остатка: у кода могут быть непотраченные места, но если он
// протух, пускать по нему нельзя.
func TestAnExpiredInviteIsRefusedEvenWithUsesLeft(t *testing.T) {
	st := inviteStore(t)
	if _, err := st.CreateInviteWithUses("stale", time.Now().Add(-time.Hour), 1, 5); err != nil {
		t.Fatal(err)
	}
	if err := st.RedeemInvite("stale", 2); err == nil {
		t.Fatal("протухший код пустил, потому что у него остались места")
	}
}

// Свежевыписанный код должен вернуться с тем лимитом, который заказали: панель
// показывает ответ как есть, и ноль в нём выглядит как сломанный код.
func TestANewInviteReportsItsLimit(t *testing.T) {
	st := inviteStore(t)
	got, err := st.CreateInviteWithUses("group", time.Time{}, 1, 4)
	if err != nil {
		t.Fatal(err)
	}
	if got.MaxUses != 4 {
		t.Errorf("MaxUses = %d, want 4", got.MaxUses)
	}
	if got.UseCount != 0 {
		t.Errorf("UseCount = %d, want 0 у нового кода", got.UseCount)
	}
}
