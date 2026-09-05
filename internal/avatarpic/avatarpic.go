// Package avatarpic рисует аватар из имени: его ставят при регистрации и к нему
// же аккаунт возвращается, когда свою картинку убирают
package avatarpic

import (
	"bytes"
	_ "embed"
	"hash/fnv"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Шрифт вордмарка: буква на заглушке пишется им же. Системным шрифтом выходит
// хуйня, которая с продуктом рядом не лежала
//
//go:embed fonts/samsungsharpsans_bold.otf
var sharpSansBold []byte

// Аватар, который стоит у аккаунта с рождения. Буква приходит на его место
// только когда человек убирает свою картинку - до тех пор все выглядят одинаково
//
//go:embed assets/avatar-default.png
var defaultAvatar []byte

// Default отдаёт стартовый аватар нового аккаунта
func Default() []byte {
	out := make([]byte, len(defaultAvatar))
	copy(out, defaultAvatar)
	return out
}

// Size - сторона картинки. Берётся с запасом от любого места, где
// её показывают: клиент пусть ужимает, апскейл мылит всё нахуй
const Size = 460

var (
	sharpSansOnce sync.Once
	sharpSansFace *opentype.Font
	sharpSansErr  error
)

// avatarPalette - цвета заглушки. Цвет считается из имени, иначе он скакал бы от
// запроса к запросу и аккаунт мигал бы разными кружками
var avatarPalette = []color.RGBA{
	{0x2C, 0x7D, 0xF0, 0xFF},
	{0x5A, 0x63, 0xE8, 0xFF},
	{0x8A, 0x63, 0xE8, 0xFF},
	{0x1F, 0x9E, 0x6E, 0xFF},
	{0xD9, 0x82, 0x2B, 0xFF},
	{0xCC, 0x4B, 0x5A, 0xFF},
	{0x0E, 0x9F, 0xA8, 0xFF},
	{0x68, 0x68, 0xA3, 0xFF},
}

// generatedAvatar рисует заглушку из имени: буквы на цветном круге.
//
// Рисуется тут, а не в клиенте: их дохуя (панель, андроид, DeX), и каждый
// нарисовал бы свою хуйню - три разных лица на один аккаунт
func Generate(username string) ([]byte, error) {
	size := Size
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	drawCircle(img, avatarColor(username))

	letters := avatarLetters(username)
	if letters != "" {
		if err := drawLetters(img, letters, size); err != nil {
			return nil, err
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func drawCircle(img *image.RGBA, fill color.RGBA) {
	bounds := img.Bounds()
	radius := float64(bounds.Dx()) / 2
	center := radius
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			dx, dy := float64(x)+0.5-center, float64(y)+0.5-center
			if dx*dx+dy*dy <= radius*radius {
				img.SetRGBA(x, y, fill)
			}
		}
	}
}

func drawLetters(img *image.RGBA, letters string, size int) error {
	sharpSansOnce.Do(func() { sharpSansFace, sharpSansErr = opentype.Parse(sharpSansBold) })
	if sharpSansErr != nil {
		return sharpSansErr
	}
	scale := 0.46
	if len(letters) > 1 {
		scale = 0.38
	}
	face, err := opentype.NewFace(sharpSansFace, &opentype.FaceOptions{
		Size: float64(size) * scale,
		DPI:  72,
	})
	if err != nil {
		return err
	}
	defer func() { _ = face.Close() }()

	drawer := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(color.White),
		Face: face,
	}
	width := drawer.MeasureString(letters)
	metrics := face.Metrics()
	// Базовая линия считается от метрик: по центру строки буква сидит криво и
	// сползает вниз, потому что у прописных нижних выносных нет
	baseline := (fixed.I(size) + metrics.Ascent - metrics.Descent) / 2
	drawer.Dot = fixed.Point26_6{X: (fixed.I(size) - width) / 2, Y: baseline}
	drawer.DrawString(letters)
	return nil
}

// avatarLetters - две буквы, когда имя из двух слов, иначе одна
func avatarLetters(username string) string {
	trimmed := strings.TrimSpace(username)
	if trimmed == "" {
		return ""
	}
	parts := strings.FieldsFunc(trimmed, func(r rune) bool {
		return r == ' ' || r == '.' || r == '_' || r == '-'
	})
	if len(parts) > 1 {
		return strings.ToUpper(firstRune(parts[0]) + firstRune(parts[1]))
	}
	return strings.ToUpper(firstRune(trimmed))
}

func firstRune(value string) string {
	for _, r := range value {
		return string(r)
	}
	return ""
}

func avatarColor(username string) color.RGBA {
	sum := fnv.New32a()
	_, _ = sum.Write([]byte(strings.ToLower(strings.TrimSpace(username))))
	return avatarPalette[int(sum.Sum32())%len(avatarPalette)]
}

var _ = draw.Draw
