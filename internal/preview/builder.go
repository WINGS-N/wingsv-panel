package preview

import (
	"bytes"

	"compress/zlib"
	"encoding/base64"
	"errors"
	"github.com/andybalholm/brotli"
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
	inflated, err := decompressPayload(decoded)
	if err != nil {
		return nil, err
	}
	out := &wingsvpb.Config{}
	if err := proto.Unmarshal(inflated, out); err != nil {
		return nil, err
	}
	return out, nil
}

// brotliQuality - пятый уровень. Одиннадцатый на этих данных даёт те же байты
// за время в полсотни раз большее: словарь кончается на первых сотнях байт
const brotliQuality = 5

// BuildWingsLink собирает ссылку wingsv:// в основном формате: байт 0x13,
// дальше protobuf, сжатый brotli, всё в алфавите без подчёркиваний
func BuildWingsLink(config *wingsvpb.Config) (string, error) {
	framed, err := brotliFrame(config)
	if err != nil {
		return "", err
	}
	return SchemePrefix + encodeLinkPayload(framed), nil
}

// BuildWingsLinkLegacyAlphabet собирает ту же brotli-ссылку прежним base64url.
// Нужна сборкам, которые brotli читают, а точку в теле ссылки ещё нет
func BuildWingsLinkLegacyAlphabet(config *wingsvpb.Config) (string, error) {
	framed, err := brotliFrame(config)
	if err != nil {
		return "", err
	}
	return SchemePrefix + base64.RawURLEncoding.EncodeToString(framed), nil
}

func brotliFrame(config *wingsvpb.Config) ([]byte, error) {
	raw, err := proto.Marshal(config)
	if err != nil {
		return nil, err
	}
	var compressed bytes.Buffer
	w := brotli.NewWriterLevel(&compressed, brotliQuality)
	if _, err := w.Write(raw); err != nil {
		_ = w.Close()
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return append([]byte{FormatProtobufBrotli}, compressed.Bytes()...), nil
}

// encodeLinkPayload кодирует кадр в тело ссылки. Отличие от base64url одно: 63-й
// символ алфавита не подчёркивание, а точка. Пара подчёркиваний подряд выходила
// сама собой, а мессенджеры читают её как разметку и съедают кусок ссылки.
//
// Завершающую точку автолинковка обрезает как знак конца предложения, поэтому
// такой хвост закрывается знаком равенства - его декодер снимает вместе с padding
func encodeLinkPayload(framed []byte) string {
	encoded := strings.ReplaceAll(base64.RawURLEncoding.EncodeToString(framed), "_", ".")
	if strings.HasSuffix(encoded, ".") {
		encoded += "="
	}
	return encoded
}

// BuildWingsLinkDeflate собирает ссылку в старом формате, с прежним алфавитом: на
// той стороне заведомо старая сборка, а точку она не прочитает
func BuildWingsLinkDeflate(config *wingsvpb.Config) (string, error) {
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
