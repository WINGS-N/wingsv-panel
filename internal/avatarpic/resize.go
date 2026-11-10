package avatarpic

import (
	"bytes"
	"errors"
	"image"
	"image/draw"
	_ "image/jpeg" // Загрузка принимает jpeg, значит декодер обязан быть зарегистрирован
	"image/png"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // webp тоже принимается на загрузке, но кодировать в него нечем
)

// Sizes - во что режем картинку на загрузке.
//
// Список короткий нарочно: каждый размер это ещё одна копия в хранилище. 64 под
// список, 128 под карточку, 460 под экран профиля и ретину
var Sizes = []int{64, 128, 460}

// ErrNoImage - в байтах не картинка, которую мы умеем читать
var ErrNoImage = errors.New("avatarpic: unsupported image")

// Resize вписывает картинку в квадрат и отдаёт PNG.
//
// Режем по центру: аватарка круглая почти везде, и влезшая целиком панорама
// выглядит полоской в кружке. PNG на выходе всегда, потому что кодировать в
// webp нечем, а жать фото в jpeg второй раз - терять на ровном месте
func Resize(data []byte, size int) ([]byte, error) {
	if size <= 0 {
		return nil, ErrNoImage
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, ErrNoImage
	}
	square := centerSquare(src)
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	// CatmullRom, а не ApproxBiLinear: аватарку уменьшают в разы, и дешёвый
	// фильтр на таком масштабе мылит лицо в кашу
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, square, draw.Src, nil)
	var out bytes.Buffer
	if err := png.Encode(&out, dst); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// centerSquare - самый большой квадрат по центру кадра
func centerSquare(src image.Image) image.Rectangle {
	b := src.Bounds()
	side := b.Dx()
	if b.Dy() < side {
		side = b.Dy()
	}
	offsetX := b.Min.X + (b.Dx()-side)/2
	offsetY := b.Min.Y + (b.Dy()-side)/2
	return image.Rect(offsetX, offsetY, offsetX+side, offsetY+side)
}

// NearestSize подбирает готовый размер под запрошенный.
//
// Берём ближайший НЕ МЕНЬШЕ запрошенного, иначе браузер растянет мелкую копию и
// получится мыло. Больше самого крупного - отдаём крупный
func NearestSize(want int) int {
	if want <= 0 {
		return 0
	}
	for _, size := range Sizes {
		if size >= want {
			return size
		}
	}
	return Sizes[len(Sizes)-1]
}
