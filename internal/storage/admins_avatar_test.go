package storage

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"path/filepath"
	"testing"
)

// Аватар есть у аккаунта с первой минуты, а не появляется когда-нибудь потом
func TestNewAccountGetsTheDefaultAvatar(t *testing.T) {
	st, err := Open(Options{Driver: DriverSQLite, DSN: filepath.Join(t.TempDir(), "a.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()

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

// Картинка, легшая до появления уменьшенных копий, не должна остаться в стороне:
// копии делаются при первом же запросе размера
func TestOldAvatarGetsItsSizesOnDemand(t *testing.T) {
	st, err := Open(Options{Driver: DriverSQLite, DSN: filepath.Join(t.TempDir(), "a.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()

	admin, err := st.CreateAccount("olduser", "hash", false, RoleAdmin, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetAdminAvatar(admin.ID, "image/png", bigPNG(t, 600)); err != nil {
		t.Fatal(err)
	}
	// Стираем набор копий, изображая аккаунт из прошлого
	if err := st.gdb.Exec("UPDATE admins SET avatar_sizes = '' WHERE id = ?", admin.ID).Error; err != nil {
		t.Fatal(err)
	}
	_, small, _, err := st.GetAdminAvatarSized(admin.ID, 64)
	if err != nil {
		t.Fatal(err)
	}
	_, full, _, err := st.GetAdminAvatar(admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(small) == 0 || len(small) >= len(full) {
		t.Fatalf("копия не уменьшилась: %d против %d", len(small), len(full))
	}
	var stored string
	if err := st.gdb.Raw("SELECT avatar_sizes FROM admins WHERE id = ?", admin.ID).Scan(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored == "" {
		t.Fatal("копии не сохранились, следующий запрос будет резать заново")
	}
}

// Запрошенный размер подбирается не меньше нужного, а большой отдаётся как есть
func TestSizedAvatarPicksTheRightCopy(t *testing.T) {
	st, err := Open(Options{Driver: DriverSQLite, DSN: filepath.Join(t.TempDir(), "a.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()

	admin, err := st.CreateAccount("sizeduser", "hash", false, RoleAdmin, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetAdminAvatar(admin.ID, "image/png", bigPNG(t, 600)); err != nil {
		t.Fatal(err)
	}
	_, small, _, err := st.GetAdminAvatarSized(admin.ID, 64)
	if err != nil {
		t.Fatal(err)
	}
	_, large, _, err := st.GetAdminAvatarSized(admin.ID, 460)
	if err != nil {
		t.Fatal(err)
	}
	if len(small) >= len(large) {
		t.Fatalf("мелкая копия не легче крупной: %d против %d", len(small), len(large))
	}
}

func bigPNG(t *testing.T, side int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, side, side))
	for y := 0; y < side; y++ {
		for x := 0; x < side; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 64, A: 255})
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
