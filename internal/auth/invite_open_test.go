package auth

import (
	"path/filepath"
	"testing"
	"time"

	"v.wingsnet.org/internal/storage"
)

// Режим регистрации на федеративные коды влиять не должен: человек приходит по
// ссылке, и код обязан погаситься, иначе он остаётся вне дерева и без
// бесплатного доступа
func TestКодГаситсяВЛюбомРежимеРегистрации(t *testing.T) {
	for _, mode := range []string{RegistrationModeOpen, RegistrationModeInvite} {
		t.Run(mode, func(t *testing.T) {
			st, err := storage.Open(storage.Options{
				Driver: storage.DriverSQLite, DSN: filepath.Join(t.TempDir(), "invite.db"),
			})
			if err != nil {
				t.Fatal(err)
			}
			svc := New(st, false)
			if err := st.SetPlatformSetting(storage.SettingRegistrationMode, mode); err != nil {
				t.Fatal(err)
			}
			// Код ссылается на выписавшего, поэтому сперва нужен он сам
			hash, err := HashPassword("owner-pass-1")
			if err != nil {
				t.Fatal(err)
			}
			owner, err := st.CreateAdmin("inviter"+mode, hash, false, storage.RoleOwner)
			if err != nil {
				t.Fatal(err)
			}
			invite, err := st.CreateInvite("OPEN"+mode, time.Time{}, owner.ID)
			if err != nil {
				t.Fatal(err)
			}

			admin, _, err := svc.Register("newcomer", "password123", invite.Token)
			if err != nil {
				t.Fatalf("регистрация не прошла: %v", err)
			}
			redeemed, err := st.RedeemedInvite(admin.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !redeemed {
				t.Fatal("пришедший по ссылке остался вне дерева")
			}
		})
	}
}
