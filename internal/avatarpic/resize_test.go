package avatarpic

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func drawRect(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 128, A: 255})
		}
	}
	var out bytes.Buffer
	_ = png.Encode(&out, img)
	return out.Bytes()
}

func TestResizeMakesASquare(t *testing.T) {
	scaled, err := Resize(drawRect(400, 200), 128)
	if err != nil {
		t.Fatalf("ресайз обосрался: %v", err)
	}
	got, err := png.Decode(bytes.NewReader(scaled))
	if err != nil {
		t.Fatalf("на выходе не png: %v", err)
	}
	if b := got.Bounds(); b.Dx() != 128 || b.Dy() != 128 {
		t.Fatalf("размер %dx%d, хотели 128x128", b.Dx(), b.Dy())
	}
}

// Уменьшенная копия обязана весить меньше оригинала, иначе смысла в ней нет
func TestResizeShrinksTheBytes(t *testing.T) {
	source := drawRect(600, 600)
	scaled, err := Resize(source, 64)
	if err != nil {
		t.Fatalf("ресайз обосрался: %v", err)
	}
	if len(scaled) >= len(source) {
		t.Fatalf("копия не легче: было %d, стало %d", len(source), len(scaled))
	}
}

func TestResizeRejectsGarbage(t *testing.T) {
	if _, err := Resize([]byte("не картинка нахуй"), 64); err == nil {
		t.Fatal("мусор прошёл за картинку")
	}
}

func TestNearestSizePicksNotSmaller(t *testing.T) {
	for _, c := range []struct{ want, expect int }{
		{1, 64}, {64, 64}, {65, 128}, {128, 128}, {300, 460}, {460, 460}, {4000, 460},
	} {
		if got := NearestSize(c.want); got != c.expect {
			t.Errorf("для %d получили %d, хотели %d", c.want, got, c.expect)
		}
	}
}
