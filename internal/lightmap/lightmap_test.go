package lightmap

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"path/filepath"
	"testing"
)

// A 1°-per-cell atlas: dark everywhere, a bright city at 44° N 18° E with a
// glow round it, and a truly dark patch 40 km to its south-west.
func TestBuildZoneDarkest(t *testing.T) {
	pal := color.Palette{color.RGBA{0, 0, 0, 255}, color.RGBA{251, 153, 138, 255}, color.RGBA{184, 166, 37, 255}, color.RGBA{34, 34, 34, 255}}
	img := image.NewPaletted(image.Rect(0, 0, 360, 140), pal)
	for i := range img.Pix {
		img.Pix[i] = 2 // zone 4b everywhere
	}
	set := func(lat, lon float64, idx uint8) { img.SetColorIndex(int(lon+180), int(75-lat), idx) }
	set(44, 18, 1) // 6b
	set(43, 17, 3) // 1a
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "lm.bin")
	if err := BuildFrom(&buf, dst); err != nil {
		t.Fatal(err)
	}
	m, err := Open(dst)
	if err != nil {
		t.Fatal(err)
	}
	if r, ok := m.Zone(43.5, 18.5); !ok || Zones[r] != "6b" || Bortle(r) != 7 {
		t.Fatalf("city: %d %v", r, ok)
	}
	if _, ok := m.Zone(80, 0); ok {
		t.Fatal("beyond 75° N is outside the atlas")
	}
	sp := m.Darkest(43.5, 18.5, 200, 3)
	if len(sp) == 0 || sp[0].Zone != "1a" || sp[0].Dir < 180 || sp[0].Dir > 270 {
		t.Fatalf("darkest: %+v", sp)
	}
	if _, err := Open(filepath.Join(t.TempDir(), "none")); err != ErrMissing {
		t.Fatalf("missing map: %v", err)
	}
}
