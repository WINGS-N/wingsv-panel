package preview

import (
	"errors"
	"net/url"
	"strconv"
	"strings"

	"github.com/google/uuid"

	wingsvpb "v.wingsnet.org/internal/gen/wingsvpb"
)

// ParseVlessConfig builds a minimal wingsvpb.Config for a single vless://
// profile so the rest of the panel can seed a new client from one. The full
// transport params live inside raw_link and are re-parsed on the device when
// the profile is activated — server-side we only need enough to populate the
// profile-picker UI and identify the active profile.
func ParseVlessConfig(raw string) (*wingsvpb.Config, error) {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(strings.ToLower(raw), "vless://") {
		return nil, errors.New("not a vless link")
	}
	uri, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	host := uri.Hostname()
	if host == "" {
		return nil, errors.New("vless link missing host")
	}
	var port uint32
	if portStr := uri.Port(); portStr != "" {
		if p, err := strconv.ParseUint(portStr, 10, 32); err == nil {
			port = uint32(p)
		}
	}
	title := host
	if fragment := strings.TrimSpace(uri.Fragment); fragment != "" {
		if decoded, err := url.QueryUnescape(fragment); err == nil && decoded != "" {
			title = decoded
		}
	}

	profileID := uuid.NewString()
	profile := &wingsvpb.VlessProfile{
		Id:      profileID,
		Title:   title,
		RawLink: raw,
		Address: host,
	}
	if port > 0 {
		profile.Port = &port
	}

	return &wingsvpb.Config{
		Ver:     1,
		Type:    wingsvpb.ConfigType_CONFIG_TYPE_XRAY,
		Backend: wingsvpb.BackendType_BACKEND_TYPE_XRAY,
		Xray: &wingsvpb.Xray{
			ActiveProfileId: profileID,
			Profiles:        []*wingsvpb.VlessProfile{profile},
		},
	}, nil
}

// ParseLinkConfig dispatches to ParseWingsConfig or ParseVlessConfig depending
// on the scheme. Callers that previously hardcoded ParseWingsConfig for "seed
// from link" inputs should use this so users can paste either format.
func ParseLinkConfig(raw string) (*wingsvpb.Config, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("empty link")
	}
	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "vless://") {
		return ParseVlessConfig(raw)
	}
	if strings.HasPrefix(lower, SchemePrefix) {
		return ParseWingsConfig(raw)
	}
	return nil, errors.New("unsupported link scheme")
}
