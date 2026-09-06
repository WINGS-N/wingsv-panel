package storage

import (
	"testing"
	"time"
)

// Ноль в max_uses - это код без потолка, и он обязан долежать до базы таким же:
// gorm при теге default молча заменял его на дефолт колонки, и приложение
// показывало безлимит там, где панель уже считала код одноразовым
func TestUncappedInviteStaysUncappedInTheDatabase(t *testing.T) {
	st := inviteStore(t)
	if _, err := st.CreateInviteWithUses("nocap", time.Time{}, 1, 0); err != nil {
		t.Fatal(err)
	}
	got, err := st.FindInvite("nocap")
	if err != nil {
		t.Fatal(err)
	}
	if got.MaxUses != 0 {
		t.Fatalf("FindInvite отдал max_uses=%d, а клали 0", got.MaxUses)
	}
	list, err := st.ListInvitesByAdmin(1)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range list {
		if it.Token == "nocap" && it.MaxUses != 0 {
			t.Fatalf("ListInvitesByAdmin отдал max_uses=%d", it.MaxUses)
		}
	}
}
