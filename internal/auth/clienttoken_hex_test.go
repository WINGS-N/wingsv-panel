package auth

import (
	"encoding/hex"
	"testing"
)

// Ссылка несёт токен hex-строкой, а хранится он сырыми байтами: принимать надо
// оба вида, иначе телефон со свежей ссылкой не подтвердит провижн
func TestVerifyClientTokenTakesBothShapes(t *testing.T) {
	token := []byte{0xde, 0xad, 0xbe, 0xef, 0x00, 0x11, 0x22, 0x33}
	hash := HashClientToken(token)
	if !VerifyClientToken(hash, token) {
		t.Fatal("сырые байты отбиты")
	}
	if !VerifyClientToken(hash, []byte(hex.EncodeToString(token))) {
		t.Fatal("hex-строка отбита")
	}
	if VerifyClientToken(hash, []byte("deadbeef")) {
		t.Fatal("чужой токен принят")
	}
	if VerifyClientToken(hash, []byte("не hex и не токен")) {
		t.Fatal("мусор принят")
	}
}
