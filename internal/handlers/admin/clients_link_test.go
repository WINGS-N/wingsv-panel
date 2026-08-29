package admin

import (
	"bytes"
	"testing"

	"v.wingsnet.org/internal/config"
	wingsvpb "v.wingsnet.org/internal/gen/wingsvpb"
	"v.wingsnet.org/internal/preview"
	"v.wingsnet.org/internal/storage"
)

func TestBuildClientLinkRemoteControlOn(t *testing.T) {
	h := &Handler{cfg: config.Config{PublicBaseURL: "https://panel.example"}}
	token := []byte("tok-bytes")
	link, err := h.buildClientLink("c1", "Dev", token, "always", 30, storage.Admin{ID: 1, Username: "adm"}, true, "relay.example.com:56000")
	if err != nil {
		t.Fatalf("buildClientLink on: %v", err)
	}
	cfg, err := preview.ParseWingsConfig(link)
	if err != nil {
		t.Fatalf("ParseWingsConfig: %v", err)
	}
	if cfg.GetType() != wingsvpb.ConfigType_CONFIG_TYPE_GUARDIAN {
		t.Fatalf("want GUARDIAN type, got %v", cfg.GetType())
	}
	if cfg.GetGuardian().GetClientId() != "c1" || !bytes.Equal(cfg.GetGuardian().GetClientToken(), token) {
		t.Fatalf("guardian block wrong: %+v", cfg.GetGuardian())
	}
	assertManagedProfile(t, cfg, token)
}

func TestBuildClientLinkRemoteControlOff(t *testing.T) {
	h := &Handler{cfg: config.Config{}}
	token := []byte("tok-bytes")
	link, err := h.buildClientLink("c1", "Dev", token, "always", 30, storage.Admin{ID: 1}, false, "relay.example.com:56000")
	if err != nil {
		t.Fatalf("buildClientLink off: %v", err)
	}
	cfg, err := preview.ParseWingsConfig(link)
	if err != nil {
		t.Fatalf("ParseWingsConfig: %v", err)
	}
	if cfg.GetType() != wingsvpb.ConfigType_CONFIG_TYPE_VK_TURN_PROFILE {
		t.Fatalf("want VK_TURN_PROFILE type, got %v", cfg.GetType())
	}
	if cfg.GetGuardian() != nil {
		t.Fatalf("plain profile link must not carry a guardian block")
	}
	assertManagedProfile(t, cfg, token)
}

func TestBuildClientLinkOffWithoutEndpoint(t *testing.T) {
	h := &Handler{cfg: config.Config{}}
	if _, err := h.buildClientLink("c1", "Dev", []byte("t"), "always", 30, storage.Admin{}, false, ""); err == nil {
		t.Fatal("expected an error issuing a profile link with no vk-turn endpoint")
	}
}

func assertManagedProfile(t *testing.T, cfg *wingsvpb.Config, token []byte) {
	t.Helper()
	profiles := cfg.GetTurn().GetProfiles()
	if len(profiles) != 1 {
		t.Fatalf("want 1 managed profile, got %d", len(profiles))
	}
	p := profiles[0]
	if !p.GetWgProvisioned() || p.GetProvisionClientId() != "c1" {
		t.Fatalf("managed profile not marked provisioned: %+v", p)
	}
	if !bytes.Equal(p.GetProvisionToken(), token) {
		t.Fatalf("managed profile token mismatch: %x", p.GetProvisionToken())
	}
	if p.GetVkTurnEndpoint() != "relay.example.com:56000" {
		t.Fatalf("managed profile endpoint wrong: %q", p.GetVkTurnEndpoint())
	}
}
