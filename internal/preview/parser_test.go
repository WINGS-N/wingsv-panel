package preview

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"testing"

	"google.golang.org/protobuf/proto"

	"v.wingsnet.org/internal/gen/wingsvpb"
)

//nolint:staticcheck // SA1019: проверяем и устаревшие значения, они в базе
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
//
//nolint:staticcheck // SA1019: устаревшие значения проверяем нарочно, они в базе
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

func TestParseConfigTypeTitles(t *testing.T) {
	tests := []struct {
		name         string
		cfg          *wingsvpb.Config
		wantTitle    string
		wantSubtitle string
	}{
		{
			name: "vk turn profile",
			cfg: &wingsvpb.Config{
				Ver:  1,
				Type: wingsvpb.ConfigType_CONFIG_TYPE_VK_TURN_PROFILE,
				Turn: &wingsvpb.Turn{Endpoint: &wingsvpb.Endpoint{Host: "relay.example", Port: 56000}},
			},
			wantTitle:    "Профиль VK TURN",
			wantSubtitle: "relay.example:56000",
		},
		{
			name: "amneziawg",
			cfg: &wingsvpb.Config{
				Ver:  1,
				Type: wingsvpb.ConfigType_CONFIG_TYPE_AMNEZIAWG,
				Awg:  &wingsvpb.AmneziaWG{Title: "My AWG"},
			},
			wantTitle:    "AmneziaWG",
			wantSubtitle: "My AWG",
		},
		{
			name: "wb stream",
			cfg: &wingsvpb.Config{
				Ver:      1,
				Type:     wingsvpb.ConfigType_CONFIG_TYPE_WB_STREAM,
				WbStream: &wingsvpb.WbStream{RoomId: "room42"},
			},
			wantTitle:    "WB Stream",
			wantSubtitle: "Room room42",
		},
		{
			name: "xposed",
			cfg: &wingsvpb.Config{
				Ver:    1,
				Type:   wingsvpb.ConfigType_CONFIG_TYPE_XPOSED,
				Xposed: &wingsvpb.Xposed{TargetPackages: []string{"a.b", "c.d"}},
			},
			wantTitle:    "Xposed модуль",
			wantSubtitle: "2 целевых приложений",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(encodeWingsLink(t, tc.cfg))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if got.Title != tc.wantTitle {
				t.Errorf("Title = %q, want %q", got.Title, tc.wantTitle)
			}
			if got.Subtitle != tc.wantSubtitle {
				t.Errorf("Subtitle = %q, want %q", got.Subtitle, tc.wantSubtitle)
			}
		})
	}
}

// Основной формат обязан читаться обратно, а старый - продолжать работать:
// ссылки, выданные вчера, никуда не делись
func TestBothLinkFormatsRoundTrip(t *testing.T) {
	config := &wingsvpb.Config{
		Ver:  1,
		Type: wingsvpb.ConfigType_CONFIG_TYPE_XRAY,
		Xray: &wingsvpb.Xray{Profiles: []*wingsvpb.VlessProfile{{Id: "p1", Title: "узел", RawLink: "vless://x@1.2.3.4:443#p1"}}},
	}
	brotliLink, err := BuildWingsLink(config)
	if err != nil {
		t.Fatal(err)
	}
	deflateLink, err := BuildWingsLinkDeflate(config)
	if err != nil {
		t.Fatal(err)
	}
	if len(brotliLink) >= len(deflateLink) {
		t.Errorf("brotli не короче: %d против %d", len(brotliLink), len(deflateLink))
	}
	for _, link := range []string{brotliLink, deflateLink} {
		got, err := ParseWingsConfig(link)
		if err != nil {
			t.Fatalf("ссылка не разобралась: %v", err)
		}
		if len(got.GetXray().GetProfiles()) != 1 || got.GetXray().GetProfiles()[0].GetRawLink() != "vless://x@1.2.3.4:443#p1" {
			t.Errorf("конфиг не совпал: %+v", got)
		}
	}
}
