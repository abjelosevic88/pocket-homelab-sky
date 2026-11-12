package astro

import (
	"math"
	"testing"
	"time"
)

func TestOpticalLibrationMeeus53a(t *testing.T) {
	// Meeus example 53.a, 1992 April 12 0h TD: with the Moon at λ 133.162655
	// (geometric, i.e. apparent less nutation), β −3.229126, the optical
	// libration is l' = −1.206°, b' = +4.194°.
	T := (2448724.5 - 2451545.0) / 36525
	b, l := selenographic(133.162655, -3.229126, T)
	if math.Abs(l+1.206) > 0.002 || math.Abs(b-4.194) > 0.002 {
		t.Errorf("l' %.4f b' %.4f, Meeus −1.206 +4.194", l, b)
	}
	// The same moment from the engine's own Moon and Sun: libration close
	// to Meeus' total (−1.23, +4.20) and the Sun's colongitude near the
	// book's 22.1° (subsolar point l0 67.9°, b0 1.46°).
	tm := time.Date(1992, 4, 12, 0, 0, 0, 0, time.UTC)
	if la, lo := Libration(tm); math.Abs(lo+1.23) > 0.1 || math.Abs(la-4.20) > 0.1 {
		t.Errorf("Libration %.3f, %.3f", la, lo)
	}
	if c := SunColongitude(tm); math.Abs(c-22.1) > 0.2 {
		t.Errorf("colongitude %.2f, Meeus about 22.1", c)
	}
}

func TestTerminatorAtPhases(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, e := range MoonPhases(from, from.AddDate(1, 0, 0)) {
		lon, morning := Terminator(e.T)
		_, lib := Libration(e.T)
		switch e.Sub {
		case "first", "last":
			// The terminator runs down the middle of the face we see,
			// which is the libration's longitude.
			if math.Abs(Wrap180(lon-lib)) > 1.5 || math.Abs(lon) > 10 {
				t.Errorf("%s %v: terminator %.2f, libration %.2f", e.Sub, e.T, lon, lib)
			}
			if morning != (e.Sub == "first") {
				t.Errorf("%s %v: morning %v", e.Sub, e.T, morning)
			}
		case "full":
			if d := 90 - math.Abs(lon); math.Abs(d) > 10 {
				t.Errorf("full %v: terminator %.2f, want near ±90", e.T, lon)
			}
		}
	}
	// A day after first quarter the sunrise line has moved west (to
	// negative longitudes) by about 12°.
	fq := MoonPhases(from, from.AddDate(0, 1, 0))
	for _, e := range fq {
		if e.Sub != "first" {
			continue
		}
		l0, _ := Terminator(e.T)
		l1, m := Terminator(e.T.Add(24 * time.Hour))
		if d := Wrap180(l0 - l1); d < 10 || d > 14 || !m {
			t.Errorf("terminator moved %.2f° in a day (morning %v)", d, m)
		}
	}
}

func TestBestOnMoon(t *testing.T) {
	from := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	var first time.Time
	for _, e := range MoonPhases(from, from.AddDate(0, 1, 0)) {
		if e.Sub == "first" {
			first = e.T
		}
	}
	// A day after first quarter: the Sun is rising over the middle of the
	// face, on the Apennines, Plato, Eratosthenes, Clavius…
	tm := first.Add(24 * time.Hour)
	best := BestOnMoon(tm, 8)
	if len(best) < 4 {
		t.Fatalf("only %d features: %v", len(best), best)
	}
	sLat, sLon := subsolar(tm)
	names := map[string]bool{}
	for _, f := range best {
		names[f.Name] = true
		if f.Kind == "landing" {
			t.Errorf("landing site %s listed", f.Name)
		}
		h := sunAltOnMoon(f.Lat, f.Lon, sLat, sLon)
		half := math.Min(15, f.SizeKm/2/moonRadiusKm*deg)
		if h+half <= 0 || h-half >= 12 {
			t.Errorf("%s: Sun %.1f° high there, not near the terminator", f.Name, h)
		}
	}
	if !names["Montes Apenninus"] && !names["Eratosthenes"] {
		t.Errorf("expected the Apennines or Eratosthenes a day after first quarter: %v", names)
	}
	if n := len(BestOnMoon(tm, 2)); n != 2 {
		t.Errorf("asked for 2, got %d", n)
	}
}

func TestMoonFeatures(t *testing.T) {
	seen := map[string]bool{}
	kinds := map[string]int{}
	for _, f := range MoonFeatures {
		if seen[f.Name] {
			t.Errorf("duplicate %s", f.Name)
		}
		seen[f.Name] = true
		kinds[f.Kind]++
		if f.About == "" || f.Lat < -90 || f.Lat > 90 || f.Lon < -90 || f.Lon > 90 {
			t.Errorf("bad feature %+v", f)
		}
	}
	if len(MoonFeatures) < 40 || kinds["sea"] < 12 || kinds["crater"] < 20 || kinds["mountains"] < 3 || kinds["landing"] < 1 {
		t.Errorf("features by kind: %v (total %d)", kinds, len(MoonFeatures))
	}
	if !seen["Apollo 11"] || !seen["Tycho"] || !seen["Rupes Recta"] || !seen["Sinus Iridum"] {
		t.Error("a must-have feature is missing")
	}
}
