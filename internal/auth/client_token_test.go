package auth

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestClientTokenRoundTrip(t *testing.T) {
	token, hash, err := GenerateClientToken()
	if err != nil {
		t.Fatalf("GenerateClientToken: %v", err)
	}
	if len(token) != 32 {
		t.Fatalf("token length = %d, want 32", len(token))
	}
	if ClientTokenHashNeedsUpgrade(hash) {
		t.Fatalf("fresh hash %q reported as legacy", hash)
	}
	if !VerifyClientToken(hash, token) {
		t.Fatal("fresh token failed to verify")
	}
	wrong := append([]byte(nil), token...)
	wrong[0] ^= 0xff
	if VerifyClientToken(hash, wrong) {
		t.Fatal("a modified token verified")
	}
}

func TestClientTokenAcceptsLegacyBcryptAndFlagsUpgrade(t *testing.T) {
	token := []byte("0123456789abcdef0123456789abcdef")
	legacy, err := bcrypt.GenerateFromPassword(token, bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	if !VerifyClientToken(string(legacy), token) {
		t.Fatal("legacy bcrypt token failed to verify")
	}
	if !ClientTokenHashNeedsUpgrade(string(legacy)) {
		t.Fatal("legacy bcrypt hash not flagged for upgrade")
	}
	if VerifyClientToken(string(legacy), []byte("wrong")) {
		t.Fatal("legacy verify accepted a wrong token")
	}
}
