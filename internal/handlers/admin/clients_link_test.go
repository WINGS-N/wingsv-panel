package admin

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	"v.wingsnet.org/internal/config"
	wingsvpb "v.wingsnet.org/internal/gen/wingsvpb"
	"v.wingsnet.org/internal/preview"
	"v.wingsnet.org/internal/storage"
	"v.wingsnet.org/internal/storage/dbmodel"
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

// The link must name the relay the client's own stored config records, not whatever
// relay happens to be the admin's default. Handing back the default meant a link
// could contradict the config the panel had just saved for that client.
func TestClientVkTurnEndpointPrefersTheStoredConfig(t *testing.T) {
	h := &Handler{cfg: config.Config{}, store: newLinkTestStore(t)}
	adm, err := h.store.CreateAdmin("owner", "hash", false, "owner")
	if err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	if _, err := h.store.CreateClient("c1", adm.ID, "Dev", "hash", []byte("tok")); err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	client := storage.Client{ID: "c1", OwnerAdminID: adm.ID}
	cfg := &wingsvpb.Config{Ver: 1}
	cfg.Turn = h.managedTurn("c1", "Dev", []byte("tok"), "chosen.example:56000")
	blob, mErr := proto.Marshal(cfg)
	if mErr != nil {
		t.Fatal(mErr)
	}
	if _, err := h.store.UpsertClientConfig("c1", blob, "r1"); err != nil {
		t.Fatal(err)
	}

	got, err := h.clientVkTurnEndpoint(storage.Admin{ID: adm.ID}, client, "")
	if err != nil {
		t.Fatalf("clientVkTurnEndpoint: %v", err)
	}
	if got != "chosen.example:56000" {
		t.Errorf("endpoint = %q, want the one stored on the client", got)
	}
}

// A client with no managed profile yet has nothing to follow, so it falls back to
// the admin's default relay - here, none, which must not be an error.
func TestClientVkTurnEndpointFallsBackWithoutStoredConfig(t *testing.T) {
	h := &Handler{cfg: config.Config{}, store: newLinkTestStore(t)}
	got, err := h.clientVkTurnEndpoint(storage.Admin{ID: 1}, storage.Client{ID: "fresh"}, "")
	if err != nil {
		t.Fatalf("clientVkTurnEndpoint: %v", err)
	}
	if got != "" {
		t.Errorf("endpoint = %q, want empty when no relay is registered", got)
	}
}

func newLinkTestStore(t *testing.T) *storage.Store {
	t.Helper()
	st, err := storage.Open(storage.Options{Driver: storage.DriverSQLite, DSN: filepath.Join(t.TempDir(), "link.db")})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// Enabling provisioning without naming a relay must leave the client with no managed
// profile rather than failing the save or quietly picking a relay for the admin.
// Choosing one implicitly is what handed clients a relay nobody selected.
func TestApplyProvisionWithoutANodeTurnsProvisioningOff(t *testing.T) {
	h := &Handler{cfg: config.Config{}, store: newLinkTestStore(t)}
	adm, err := h.store.CreateAdmin("owner", "hash", false, "owner")
	if err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	if _, err := h.store.CreateClient("c1", adm.ID, "Dev", "hash", []byte("tok")); err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	client := storage.Client{ID: "c1", OwnerAdminID: adm.ID, Name: "Dev"}

	cfg := &wingsvpb.Config{Ver: 1}
	if err := h.applyProvisionToConfig(storage.Admin{ID: adm.ID}, client, cfg, true, ""); err != nil {
		t.Fatalf("applyProvisionToConfig: %v", err)
	}
	if ep := existingManagedEndpoint(cfg.Turn); ep != "" {
		t.Errorf("a client with no relay chosen got endpoint %q", ep)
	}
	for _, p := range cfg.GetTurn().GetProfiles() {
		if isManagedProfile(p) {
			t.Errorf("managed profile survived with no relay: %+v", p)
		}
	}
}

// A relay named explicitly still wins and produces a managed profile.
func TestApplyProvisionWithANodeKeepsTheProfile(t *testing.T) {
	h := &Handler{cfg: config.Config{}, store: newLinkTestStore(t)}
	adm, err := h.store.CreateAdmin("owner", "hash", false, "owner")
	if err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	if _, err := h.store.CreateClient("c1", adm.ID, "Dev", "hash", []byte("tok")); err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	node, err := h.store.CreateServerNode(dbmodel.ServerNode{
		ID: "n1", Kind: storage.ServerNodeVKTurnProxy, Name: "relay",
		GRPCEndpoint: "relay.example:25612", OwnerAdminID: adm.ID, WGBackend: storage.WGBackendOwn,
	})
	if err != nil {
		t.Fatalf("CreateServerNode: %v", err)
	}
	client := storage.Client{ID: "c1", OwnerAdminID: adm.ID, Name: "Dev"}

	cfg := &wingsvpb.Config{Ver: 1}
	if err := h.applyProvisionToConfig(storage.Admin{ID: adm.ID}, client, cfg, true, node.ID); err != nil {
		t.Fatalf("applyProvisionToConfig: %v", err)
	}
	if ep := existingManagedEndpoint(cfg.Turn); ep == "" {
		t.Error("an explicitly chosen relay produced no managed endpoint")
	} else if !strings.HasPrefix(ep, "relay.example:") {
		t.Errorf("endpoint = %q, want the chosen node's host", ep)
	}
}
