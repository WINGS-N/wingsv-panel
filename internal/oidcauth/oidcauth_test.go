package oidcauth

import (
	"errors"
	"testing"
)

func testClient() *Client {
	return New(Config{
		Issuer: "https://mas.wings.example", Homeserver: "wings.example",
		ClientID: "panel", RedirectURL: "https://v.wings.example/cb",
	})
}

// The whole security property: an identity from somewhere else costs nothing to
// mint, so the invite tree would mean nothing if one were accepted
func TestAnIdentityFromAnotherHomeserverIsRefused(t *testing.T) {
	c := testClient()
	if _, err := c.identityFrom("sub", "@evil:attacker.example", ""); !errors.Is(err, ErrForeignHomeserver) {
		t.Errorf("err = %v, want the foreign homeserver refused", err)
	}
}

// MAS hands back either a bare localpart or a full id depending on how it is
// set up, and both have to land on the same account
func TestBothFormsOfUsernameProduceTheSameID(t *testing.T) {
	c := testClient()
	fromLocalpart, err := c.identityFrom("sub", "alice", "Alice")
	if err != nil {
		t.Fatal(err)
	}
	fromFull, err := c.identityFrom("sub", "@alice:wings.example", "Alice")
	if err != nil {
		t.Fatal(err)
	}
	if fromLocalpart.MatrixID != "@alice:wings.example" || fromFull.MatrixID != fromLocalpart.MatrixID {
		t.Errorf("%q vs %q", fromLocalpart.MatrixID, fromFull.MatrixID)
	}
	if fromLocalpart.Localpart != "alice" {
		t.Errorf("localpart = %q", fromLocalpart.Localpart)
	}
}

// A domain differing only in case is the same homeserver, and treating it as
// foreign would lock people out for no reason
func TestTheHomeserverCheckIsCaseInsensitive(t *testing.T) {
	c := testClient()
	got, err := c.identityFrom("sub", "@alice:WINGS.EXAMPLE", "")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got.MatrixID != "@alice:wings.example" {
		t.Errorf("id = %q, want it normalised", got.MatrixID)
	}
}

func TestAnEmptyUsernameIsRefused(t *testing.T) {
	c := testClient()
	if _, err := c.identityFrom("sub", "   ", ""); err == nil {
		t.Error("an account with no username was accepted")
	}
}

// Without configuration the panel must offer nothing rather than half a flow
func TestAnUnconfiguredClientIsDisabled(t *testing.T) {
	for _, cfg := range []Config{
		{},
		{Issuer: "https://mas.example"},
		{Issuer: "https://mas.example", ClientID: "panel"},
	} {
		if New(cfg).Enabled() {
			t.Errorf("config %+v reported itself ready", cfg)
		}
	}
	if !testClient().Enabled() {
		t.Error("a complete config reported itself disabled")
	}
}

// A callback that does not match a login this panel started is somebody else's
func TestAnUnknownStateIsRefused(t *testing.T) {
	c := testClient()
	c.states["known"] = pending{verifier: "v"}
	if _, _, _, _, err := c.Complete(t.Context(), "not-known", "code"); err == nil {
		t.Error("an unknown state was accepted")
	}
}

// Слеш на конце issuer'а значащий. go-oidc сверяет то, что дали ему, с тем, что
// вернул провайдер в discovery, посимвольно - а MAS отдаёт issuer со слешем.
// Любая "нормализация" по дороге ломает вход целиком.
func TestIssuerIsPassedThroughUntouched(t *testing.T) {
	const withSlash = "https://mxaccount.example.org/"
	c := New(Config{
		Issuer:       withSlash,
		ClientID:     "panel",
		ClientSecret: "s",
		RedirectURL:  "https://panel.example/api/oidc/callback",
	})
	if got := c.cfg.Issuer; got != withSlash {
		t.Errorf("Issuer = %q, want %q: трогать его нельзя", got, withSlash)
	}
}
