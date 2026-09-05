package admin

import (
	"testing"
	"time"
)

// Второй заход по тому же коду - это тот, кто сфотографировал чужой экран
func TestAnApprovedCodeWorksExactlyOnce(t *testing.T) {
	desk := newQRDesk()
	code, _, err := desk.open("1.2.3.4", "Firefox")
	if err != nil {
		t.Fatal(err)
	}
	if !desk.approve(code, 7) {
		t.Fatal("одобрение не прошло")
	}
	if id, ok := desk.take(code); !ok || id != 7 {
		t.Fatalf("take = %d %v, want 7 true", id, ok)
	}
	if _, ok := desk.take(code); ok {
		t.Error("код сработал второй раз")
	}
}

// Пока с телефона не подтвердили, машине отдавать нечего
func TestAPendingCodeGivesNothing(t *testing.T) {
	desk := newQRDesk()
	code, _, err := desk.open("1.2.3.4", "Firefox")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := desk.take(code); ok {
		t.Error("неподтверждённый код пустил внутрь")
	}
}

// Одобрять дважды нельзя: иначе второй подтверждающий переписывает, кого впустят
func TestASecondApprovalIsRefused(t *testing.T) {
	desk := newQRDesk()
	code, _, _ := desk.open("1.2.3.4", "Firefox")
	if !desk.approve(code, 7) {
		t.Fatal("первое одобрение не прошло")
	}
	if desk.approve(code, 9) {
		t.Error("код переодобрили на другого человека")
	}
	if id, _ := desk.take(code); id != 7 {
		t.Errorf("впустили %d, а одобрял 7", id)
	}
}

// Просроченный код мёртв, даже если его успели одобрить
func TestAnExpiredCodeIsDead(t *testing.T) {
	desk := newQRDesk()
	code, _, _ := desk.open("1.2.3.4", "Firefox")
	desk.approve(code, 7)

	desk.mu.Lock()
	row := desk.rows[code]
	row.expiresAt = time.Now().Add(-time.Second)
	desk.rows[code] = row
	desk.mu.Unlock()

	if _, ok := desk.get(code); ok {
		t.Error("протухший код всё ещё виден")
	}
	if _, ok := desk.take(code); ok {
		t.Error("протухший код пустил внутрь")
	}
}

// Чужой код не угадывается: он длинный и случайный
func TestCodesAreUnique(t *testing.T) {
	desk := newQRDesk()
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		code, _, err := desk.open("1.2.3.4", "Firefox")
		if err != nil {
			t.Fatal(err)
		}
		if len(code) < 30 {
			t.Fatalf("код %q короче некуда", code)
		}
		if seen[code] {
			t.Fatal("код повторился")
		}
		seen[code] = true
	}
}
