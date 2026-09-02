package storage

import (
	"path/filepath"
	"testing"
)

// Аватар есть у аккаунта с первой минуты, а не появляется когда-нибудь потом
func TestNewAccountGetsTheDefaultAvatar(t *testing.T) {
	st, err := Open(Options{Driver: DriverSQLite, DSN: filepath.Join(t.TempDir(), "a.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	admin, err := st.CreateAccount("freshuser", "hash", false, RoleAdmin, false)
	if err != nil {
		t.Fatal(err)
	}
	mime, data, version, err := st.GetAdminAvatar(admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("аккаунт родился без аватара")
	}
	if mime != "image/png" {
		t.Fatalf("mime = %q, want image/png", mime)
	}
	if version == 0 {
		t.Fatal("версия нулевая - клиент решит, что картинки нет")
	}
}
