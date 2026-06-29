package admin

import (
	"testing"

	"v.wingsnet.org/internal/gen/guardianpb"
)

func TestParseCommandType(t *testing.T) {
	cases := []struct {
		in   string
		want guardianpb.CommandType
		ok   bool
	}{
		{"start", guardianpb.CommandType_COMMAND_TYPE_START_TUNNEL, true},
		{"START_TUNNEL", guardianpb.CommandType_COMMAND_TYPE_START_TUNNEL, true},
		{" stop ", guardianpb.CommandType_COMMAND_TYPE_STOP_TUNNEL, true},
		{"reconnect", guardianpb.CommandType_COMMAND_TYPE_RECONNECT, true},
		{"report", guardianpb.CommandType_COMMAND_TYPE_REPORT_NOW, true},
		{"refresh_subscription", guardianpb.CommandType_COMMAND_TYPE_REFRESH_SUBSCRIPTION, true},
		{"refresh_all_subscriptions", guardianpb.CommandType_COMMAND_TYPE_REFRESH_ALL_SUBSCRIPTIONS, true},
		{"generate_vk_link", guardianpb.CommandType_COMMAND_TYPE_GENERATE_VK_LINK, true},
		{"nope", 0, false},
		{"", 0, false},
	}
	for _, c := range cases {
		got, ok := parseCommandType(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("parseCommandType(%q) = (%v, %v), want (%v, %v)", c.in, got, ok, c.want, c.ok)
		}
	}
}
