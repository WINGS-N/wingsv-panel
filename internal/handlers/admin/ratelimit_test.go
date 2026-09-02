package admin

import (
	"testing"
	"time"
)

// Подбор короткого кода ломается лимитом попыток, поэтому лимит проверяется, а не
// принимается на веру
func TestAttemptLimiterBlocksBruteForce(t *testing.T) {
	now := time.Unix(1788300000, 0)
	limiter := newAttemptLimiter()
	limiter.now = func() time.Time { return now }

	for i := 0; i < redeemAttempts; i++ {
		if !limiter.Allow("ip:1.2.3.4") {
			t.Fatalf("попытка %d зарезана раньше лимита", i+1)
		}
	}
	if limiter.Allow("ip:1.2.3.4") {
		t.Fatal("перебор прошёл за лимит")
	}

	// Другой ключ живёт своей жизнью
	if !limiter.Allow("ip:9.9.9.9") {
		t.Fatal("чужой адрес попал под чужую блокировку")
	}

	// Блокировка отпускает по времени
	now = now.Add(redeemBlock + time.Second)
	if !limiter.Allow("ip:1.2.3.4") {
		t.Fatal("блокировка не отпустила")
	}
}

// Окно скользящее: редкие опечатки не копятся вечно
func TestAttemptLimiterForgetsOldAttempts(t *testing.T) {
	now := time.Unix(1788300000, 0)
	limiter := newAttemptLimiter()
	limiter.now = func() time.Time { return now }

	for i := 0; i < redeemAttempts; i++ {
		limiter.Allow("acc:7")
		now = now.Add(redeemWindow / 2)
	}
	if !limiter.Allow("acc:7") {
		t.Fatal("растянутые во времени попытки сложились как перебор")
	}
}

// Удачный код снимает счётчик
func TestAttemptLimiterForget(t *testing.T) {
	limiter := newAttemptLimiter()
	for i := 0; i < redeemAttempts+2; i++ {
		limiter.Allow("acc:7")
	}
	if limiter.Allow("acc:7") {
		t.Fatal("лимит не сработал")
	}
	limiter.Forget("acc:7")
	if !limiter.Allow("acc:7") {
		t.Fatal("Forget не снял блокировку")
	}
}
