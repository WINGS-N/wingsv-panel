package oidcauth

import (
	"testing"
)

func testClient() *Client {
	return New(Config{
		Issuer:   "https://id.wings.example",
		ClientID: "panel", RedirectURL: "https://v.wings.example/cb",
	})
}

// Личность - это subject. Без него связывать вход не с чем, и молча заводить
// нового админа на пустоте нельзя
func TestAnIdentityWithoutASubjectIsRefused(t *testing.T) {
	c := testClient()
	if _, err := c.identityFrom("  ", "alice", ""); err == nil {
		t.Error("an identity with no subject was accepted")
	}
}

// Провайдер зовёт человека вида admin@org.домен, а имена админов у нас короткие
func TestTheUsernameLosesTheProviderDomain(t *testing.T) {
	c := testClient()
	got, err := c.identityFrom("sub-1", "Admin@wings.id.wings.example", "WINGS Owner")
	if err != nil {
		t.Fatal(err)
	}
	if got.Username != "admin" {
		t.Errorf("username = %q, want the local part alone", got.Username)
	}
	if got.Subject != "sub-1" || got.DisplayName != "WINGS Owner" {
		t.Errorf("identity = %+v", got)
	}
}

// Имя у провайдера человек меняет когда захочет, поэтому решает subject
func TestARenamedAccountKeepsItsSubject(t *testing.T) {
	c := testClient()
	before, err := c.identityFrom("sub-1", "alice", "Alice")
	if err != nil {
		t.Fatal(err)
	}
	after, err := c.identityFrom("sub-1", "bob", "Bob")
	if err != nil {
		t.Fatal(err)
	}
	if before.Subject != after.Subject {
		t.Errorf("subject moved from %q to %q", before.Subject, after.Subject)
	}
}

// Без настройки панель обязана не предлагать ничего, а не половину потока
func TestAnUnconfiguredClientIsDisabled(t *testing.T) {
	for _, cfg := range []Config{
		{},
		{Issuer: "https://id.example"},
		{ClientID: "panel"},
	} {
		if New(cfg).Enabled() {
			t.Errorf("config %+v reported itself ready", cfg)
		}
	}
	if !testClient().Enabled() {
		t.Error("a complete config reported itself disabled")
	}
}

// Колбэк, не совпавший с начатым здесь входом, принадлежит кому-то другому
func TestAnUnknownStateIsRefused(t *testing.T) {
	c := testClient()
	c.states["known"] = pending{verifier: "v"}
	if _, _, _, _, err := c.Complete(t.Context(), "not-known", "code"); err == nil {
		t.Error("an unknown state was accepted")
	}
}

// Слеш на конце issuer'а значащий. go-oidc сверяет то, что дали ему, с тем, что
// вернул провайдер в discovery, посимвольно. Любая "нормализация" по дороге
// ломает вход целиком
func TestIssuerIsPassedThroughUntouched(t *testing.T) {
	const withSlash = "https://id.example.org/"
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
