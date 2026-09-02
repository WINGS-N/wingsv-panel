package admin

import (
	"bytes"
	"image"
	"image/png"
	"testing"
)

// Аккаунт без своей картинки всё равно получает свою: буква имени на цветном круге
func TestGeneratedAvatarIsAPngWithColour(t *testing.T) {
	data, err := generatedAvatar("admin")
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("отдали не png: %v", err)
	}
	if img.Bounds().Dx() != generatedAvatarSize || img.Bounds().Dy() != generatedAvatarSize {
		t.Fatalf("размер %v, ждали %d", img.Bounds(), generatedAvatarSize)
	}

	// Центр круга закрашен, а угол за его пределами прозрачный
	centre := img.At(generatedAvatarSize/2, generatedAvatarSize/2)
	if _, _, _, alpha := centre.RGBA(); alpha == 0 {
		t.Fatal("середина прозрачная - круг не нарисовался")
	}
	if _, _, _, alpha := img.At(1, 1).RGBA(); alpha != 0 {
		t.Fatal("угол закрашен - это квадрат, а не круг")
	}

	// Белые точки буквы должны быть: без них круг пустой
	if !hasWhite(img) {
		t.Fatal("буквы не нарисовались")
	}
}

// Цвет берётся из имени, поэтому он постоянный и у разных имён разный
func TestAvatarColourFollowsTheName(t *testing.T) {
	if avatarColor("admin") != avatarColor("ADMIN ") {
		t.Fatal("регистр и пробелы поменяли цвет")
	}
	same := 0
	names := []string{"admin", "killrdf", "debar", "august", "nikita", "test2"}
	for i := 1; i < len(names); i++ {
		if avatarColor(names[0]) == avatarColor(names[i]) {
			same++
		}
	}
	if same == len(names)-1 {
		t.Fatal("все имена получили один цвет")
	}
}

// Две буквы для составного имени, одна для простого
func TestAvatarLetters(t *testing.T) {
	cases := map[string]string{
		"admin":       "A",
		"nikita kim":  "NK",
		"litt.garic":  "LG",
		"kill_rdf":    "KR",
		"  ":          "",
		"ярослав пёс": "ЯП",
	}
	for name, want := range cases {
		if got := avatarLetters(name); got != want {
			t.Fatalf("avatarLetters(%q) = %q, want %q", name, got, want)
		}
	}
}

func hasWhite(img image.Image) bool {
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y += 3 {
		for x := bounds.Min.X; x < bounds.Max.X; x += 3 {
			r, g, b, a := img.At(x, y).RGBA()
			if a > 0 && r > 0xF000 && g > 0xF000 && b > 0xF000 {
				return true
			}
		}
	}
	return false
}
