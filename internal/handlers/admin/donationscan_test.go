package admin

import "testing"

// Код в заметке - единственное, что связывает занос с аккаунтом, поэтому его
// разбор обязан переживать любую самодеятельность кошельков
func TestMemoIsParsedOutOfWhateverTheWalletWrote(t *testing.T) {
	for memo, want := range map[string]int64{
		"wv-42":                    42,
		" wv-7 ":                   7,
		"Donation wv-1234 thanks!": 1234,
		"перевод wv-9 на трафик":   9,
	} {
		got, ok := adminFromMemo(memo)
		if !ok || got != want {
			t.Fatalf("из %q достали %d (ok=%v), а там аккаунт %d", memo, got, ok, want)
		}
	}
}

// Заметка без кода никому не принадлежит, и присвоить её нельзя
func TestMemoWithoutCodeBelongsToNobody(t *testing.T) {
	for _, memo := range []string{"", "спасибо за впн", "wv-", "wv-abc", "vw-12"} {
		if id, ok := adminFromMemo(memo); ok {
			t.Fatalf("из %q достали аккаунт %d, а кода там нет", memo, id)
		}
	}
}

// Код показывается человеку, и он же ищется обратно: разъедутся - занос
// потеряется
func TestMemoRoundTrips(t *testing.T) {
	id, ok := adminFromMemo(MemoFor(77))
	if !ok || id != 77 {
		t.Fatalf("свой же код не разобрался: %d %v", id, ok)
	}
}
