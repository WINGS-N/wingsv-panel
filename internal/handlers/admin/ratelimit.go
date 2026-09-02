package admin

import (
	"net/http"
	"sync"
	"time"
)

// Код приглашения короткий, и подбор его - тупой цикл по HEX. Лимит попыток и
// есть то, обо что этот перебор разбивается: восемь байт при десятке попыток в
// минуту не переберёшь никогда.
const (
	redeemAttempts = 10
	redeemWindow   = time.Minute
	redeemBlock    = 10 * time.Minute
)

// attemptLimiter считает попытки по ключу в скользящем окне.
//
// Живёт в памяти: панель крутится одним процессом, а переживать рестарт счётчику
// нахуй не сдалось - перебор всё равно упрётся в новое окно.
type attemptLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	blocked  map[string]time.Time
	now      func() time.Time
}

func newAttemptLimiter() *attemptLimiter {
	return &attemptLimiter{
		attempts: map[string][]time.Time{},
		blocked:  map[string]time.Time{},
		now:      time.Now,
	}
}

// Allow говорит, можно ли пробовать ещё, и записывает попытку
func (l *attemptLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()

	if until, ok := l.blocked[key]; ok {
		if now.Before(until) {
			return false
		}
		delete(l.blocked, key)
	}

	kept := l.attempts[key][:0]
	for _, at := range l.attempts[key] {
		if now.Sub(at) < redeemWindow {
			kept = append(kept, at)
		}
	}
	kept = append(kept, now)
	l.attempts[key] = kept

	if len(kept) > redeemAttempts {
		l.blocked[key] = now.Add(redeemBlock)
		delete(l.attempts, key)
		return false
	}
	return true
}

// Forget снимает счётчик: код подошёл, и наказывать за прошлые опечатки не за что
func (l *attemptLimiter) Forget(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, key)
	delete(l.blocked, key)
}

// limitAttempt режет перебор по своему ключу и говорит, можно ли продолжать.
//
// Вешается на всё, где подбирают секрет: пароль, код 2FA, одноразовый код
// приложения, код приглашения. Без него перебор упирается только в скорость сети,
// а это вообще ни разу не преграда
func (h *Handler) limitAttempt(w http.ResponseWriter, r *http.Request, keys ...string) bool {
	if h.redeemLimiter == nil {
		return true
	}
	for _, key := range keys {
		if key == "" {
			continue
		}
		if !h.redeemLimiter.Allow(key) {
			writeError(w, http.StatusTooManyRequests, "слишком много попыток, попробуйте позже")
			return false
		}
	}
	return true
}

// limitRedeem режет перебор кода. Ключ - и адрес, и аккаунт: адрес меняется через
// любой прокси, а аккаунт стоит инвайта, и жечь его на перебор дорого
func (h *Handler) limitRedeem(w http.ResponseWriter, r *http.Request, accountKey string) bool {
	if h.redeemLimiter == nil {
		return true
	}
	for _, key := range []string{"ip:" + clientIP(r), "acc:" + accountKey} {
		if !h.redeemLimiter.Allow(key) {
			writeError(w, http.StatusTooManyRequests, "слишком много попыток, попробуйте позже")
			return false
		}
	}
	return true
}
