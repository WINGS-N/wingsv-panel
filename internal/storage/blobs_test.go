package storage

import (
	"bytes"
	"path/filepath"
	"testing"
)

func blobStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(Options{Driver: DriverSQLite, DSN: filepath.Join(t.TempDir(), "b.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// Одинаковые байты дают один блоб: копий в базе быть не должно
func TestPutBlobDeduplicates(t *testing.T) {
	st := blobStore(t)
	data := []byte("одна и та же картинка")

	first, err := st.PutBlob("image/png", data)
	if err != nil {
		t.Fatal(err)
	}
	second, err := st.PutBlob("image/png", append([]byte{}, data...))
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("хеши разошлись: %s против %s", first, second)
	}

	var rows int64
	if err := st.gdb.Table("blobs").Count(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("в базе %d строк, а картинка одна", rows)
	}

	mime, stored, err := st.GetBlob(first)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored, data) || mime != "image/png" {
		t.Fatal("содержимое не то, что клали")
	}
}

// Два аккаунта с одинаковой аватаркой держат один блоб, и он живёт, пока на него
// ссылается хоть кто-то
func TestAvatarBlobIsSharedAndCleanedUp(t *testing.T) {
	st := blobStore(t)
	first, err := st.CreateAccount("one", "h", false, RoleAdmin, false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := st.CreateAccount("two", "h", false, RoleAdmin, false)
	if err != nil {
		t.Fatal(err)
	}

	same := []byte("общая картинка")
	if _, err := st.SetAdminAvatar(first.ID, "image/png", same); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetAdminAvatar(second.ID, "image/png", same); err != nil {
		t.Fatal(err)
	}

	hash := BlobHash(same)
	if _, _, err := st.GetBlob(hash); err != nil {
		t.Fatal("общий блоб не найден")
	}

	// Первый убрал аватар - блоб живёт, на него ещё ссылается второй
	if err := st.ClearAdminAvatar(first.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.GetBlob(hash); err != nil {
		t.Fatal("блоб снесли, хотя на него ещё ссылаются")
	}

	// Ушёл и второй - теперь на него не ссылается никто
	if err := st.ClearAdminAvatar(second.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.GetBlob(hash); err == nil {
		t.Fatal("осиротевший блоб остался в базе")
	}
}
