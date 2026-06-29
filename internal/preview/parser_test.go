package preview

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"testing"

	"google.golang.org/protobuf/proto"

	"v.wingsnet.org/internal/gen/wingsvpb"
)

func TestBackendLabel(t *testing.T) {
	cases := []struct {
		backend wingsvpb.BackendType
		want    string
	}{
		{wingsvpb.BackendType_BACKEND_TYPE_VK_TURN, "VK TURN + WireGuard"},
		{wingsvpb.BackendType_BACKEND_TYPE_VK_TURN_WIREGUARD, "VK TURN + WireGuard"},
		{wingsvpb.BackendType_BACKEND_TYPE_AMNEZIAWG, "VK TURN + AmneziaWG"},
		{wingsvpb.BackendType_BACKEND_TYPE_AMNEZIAWG_TL, "AmneziaWG"},
		{wingsvpb.BackendType_BACKEND_TYPE_AMNEZIAWG_PLAIN, "AmneziaWG"},
		{wingsvpb.BackendType_BACKEND_TYPE_WIREGUARD, "WireGuard"},
		{wingsvpb.BackendType_BACKEND_TYPE_XRAY, "Xray"},
		{wingsvpb.BackendType_BACKEND_TYPE_WB_STREAM, "WB Stream"},
		{wingsvpb.BackendType_BACKEND_TYPE_UNSPECIFIED, "WINGS V"},
	}
	for _, c := range cases {
		if got := backendLabel(c.backend); got != c.want {
			t.Errorf("backendLabel(%v) = %q, want %q", c.backend, got, c.want)
		}
	}
}

// backendLabelForConfig must consult Turn.tunnel_mode to disambiguate VK TURN
// WG vs AWG, since the new BACKEND_TYPE_VK_TURN carries the choice in Turn.
func TestBackendLabelForConfig(t *testing.T) {
	mk := func(b wingsvpb.BackendType, tm wingsvpb.TunnelMode) *wingsvpb.Config {
		cfg := &wingsvpb.Config{Backend: b}
		if tm != wingsvpb.TunnelMode_TUNNEL_MODE_UNSPECIFIED {
			cfg.Turn = &wingsvpb.Turn{TunnelMode: tm}
		}
		return cfg
	}
	cases := []struct {
		name string
		cfg  *wingsvpb.Config
		want string
	}{
		{"new VK TURN over WG", mk(wingsvpb.BackendType_BACKEND_TYPE_VK_TURN, wingsvpb.TunnelMode_TUNNEL_MODE_WIREGUARD), "VK TURN + WireGuard"},
		{"new VK TURN over AWG", mk(wingsvpb.BackendType_BACKEND_TYPE_VK_TURN, wingsvpb.TunnelMode_TUNNEL_MODE_AMNEZIAWG), "VK TURN + AmneziaWG"},
		{"legacy VK_TURN_WIREGUARD + AWG tunnel", mk(wingsvpb.BackendType_BACKEND_TYPE_VK_TURN_WIREGUARD, wingsvpb.TunnelMode_TUNNEL_MODE_AMNEZIAWG), "VK TURN + AmneziaWG"},
		{"standalone AmneziaWG_TL (no turn)", mk(wingsvpb.BackendType_BACKEND_TYPE_AMNEZIAWG_TL, wingsvpb.TunnelMode_TUNNEL_MODE_UNSPECIFIED), "AmneziaWG"},
		{"plain WireGuard", mk(wingsvpb.BackendType_BACKEND_TYPE_WIREGUARD, wingsvpb.TunnelMode_TUNNEL_MODE_UNSPECIFIED), "WireGuard"},
	}
	for _, c := range cases {
		if got := backendLabelForConfig(c.cfg); got != c.want {
			t.Errorf("%s: backendLabelForConfig = %q, want %q", c.name, got, c.want)
		}
	}
}

func encodeWingsLink(t *testing.T, cfg *wingsvpb.Config) string {
	t.Helper()
	raw, err := proto.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	if _, err := zw.Write(raw); err != nil {
		t.Fatalf("zlib write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zlib close: %v", err)
	}
	payload := append([]byte{FormatProtobufDeflate}, buf.Bytes()...)
	return SchemePrefix + base64.RawURLEncoding.EncodeToString(payload)
}

// End-to-end: a wingsv:// link carrying a new-model VK TURN (over AWG) config
// must decode and label correctly through parseWings -> buildPreview ->
// backendLabelForConfig.
func TestParseWingsRoundTrip(t *testing.T) {
	cfg := &wingsvpb.Config{
		Ver:     1,
		Type:    wingsvpb.ConfigType_CONFIG_TYPE_ALL,
		Backend: wingsvpb.BackendType_BACKEND_TYPE_VK_TURN,
		Turn: &wingsvpb.Turn{
			TunnelMode:      wingsvpb.TunnelMode_TUNNEL_MODE_AMNEZIAWG,
			ActiveProfileId: "p1",
			Profiles: []*wingsvpb.TurnProfile{
				{Id: "p1", Title: "Profile 1", VkAuthMode: "account", DnsMode: "doh"},
				{Id: "p2", Title: "Profile 2"},
			},
		},
	}
	got, err := Parse(encodeWingsLink(t, cfg))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.Backend != "VK TURN + AmneziaWG" {
		t.Errorf("Backend = %q, want %q", got.Backend, "VK TURN + AmneziaWG")
	}
}
