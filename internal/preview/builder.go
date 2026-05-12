package preview

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"errors"
	"strings"

	"google.golang.org/protobuf/proto"

	wingsvpb "v.wingsnet.org/internal/gen/wingsvpb"
)

// ParseWingsConfig decodes a wingsv:// link back into the underlying Config
// proto. Useful when seeding a new client from an exported link.
func ParseWingsConfig(raw string) (*wingsvpb.Config, error) {
	if !strings.HasPrefix(strings.ToLower(raw), SchemePrefix) {
		return nil, errors.New("not a wingsv link")
	}
	payload := strings.TrimPrefix(raw, SchemePrefix)
	payload = normalizeBase64(payload)
	decoded, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		decoded, err = base64.URLEncoding.DecodeString(payload)
		if err != nil {
			return nil, err
		}
	}
	if len(decoded) == 0 {
		return nil, errors.New("empty payload")
	}
	if decoded[0] != FormatProtobufDeflate {
		return nil, errors.New("unsupported payload format")
	}
	inflated, err := inflatePayload(decoded[1:])
	if err != nil {
		return nil, err
	}
	out := &wingsvpb.Config{}
	if err := proto.Unmarshal(inflated, out); err != nil {
		return nil, err
	}
	return out, nil
}

// BuildWingsLink serialises the given Config into a wingsv:// link using the
// same format the WINGS V client emits: [0x12] + zlib(proto bytes), base64-url
// encoded, prefixed with "wingsv://".
func BuildWingsLink(config *wingsvpb.Config) (string, error) {
	raw, err := proto.Marshal(config)
	if err != nil {
		return "", err
	}
	var compressed bytes.Buffer
	w := zlib.NewWriter(&compressed)
	if _, err := w.Write(raw); err != nil {
		_ = w.Close()
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	payload := append([]byte{FormatProtobufDeflate}, compressed.Bytes()...)
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	return SchemePrefix + encoded, nil
}
