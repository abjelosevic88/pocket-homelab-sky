package astro

import (
	"math"
	"testing"
	"time"
)

func TestGalileanMoonsMeeus44a(t *testing.T) {
	// Meeus, Astronomical Algorithms, example 44.a: 1992 December 16, 0h UT.
	want := map[string][2]float64{
		"Io": {-3.44, 0.21}, "Europa": {7.44, 0.25}, "Ganymede": {1.24, 0.65}, "Callisto": {7.08, 1.10},
	}
	ms := GalileanMoons(time.Date(1992, 12, 16, 0, 0, 0, 0, time.UTC))
	if len(ms) != 4 {
		t.Fatalf("got %d moons", len(ms))
	}
	for _, m := range ms {
		w := want[m.Name]
		if math.Abs(m.X-w[0]) > 0.05 || math.Abs(m.Y-w[1]) > 0.05 {
			t.Errorf("%s: X %.3f Y %.3f, Meeus %.2f %.2f", m.Name, m.X, m.Y, w[0], w[1])
		}
	}
}

func TestGalileanMoonsHorizons(t *testing.T) {
	// JPL Horizons, 2026-Nov-12: each moon's offset from Jupiter in RA
	// (east) and Dec (north), arcseconds; Jupiter 36.987" wide with its
	// north pole at position angle 21.886°. Turned into Meeus' X (west
	// along Jupiter's equator) and Y (north along its axis), in radii.
	const pa, radius = 21.886, 36.987 / 2
	toXY := func(east, north float64) (x, y float64) {
		return (-east*cosd(pa) + north*sind(pa)) / radius, (east*sind(pa) + north*cosd(pa)) / radius
	}
	cases := []struct {
		h           int
		name        string
		east, north float64
	}{
		{0, "Io", -32.376, 13.429}, {6, "Io", -94.899, 38.296}, {12, "Io", -86.927, 34.716},
		{18, "Io", -14.844, 5.519}, {24, "Io", 68.241, -27.801},
		{0, "Europa", 60.558, -26.266}, {12, "Europa", 153.961, -62.225}, {24, "Europa", 133.075, -51.971},
		{0, "Ganymede", -106.508, 43.795}, {12, "Ganymede", -196.040, 79.778}, {24, "Ganymede", -248.380, 100.640},
		{0, "Callisto", -440.913, 175.358}, {12, "Callisto", -451.035, 178.892}, {24, "Callisto", -445.106, 176.052},
	}
	for _, c := range cases {
		tm := time.Date(2026, 11, 12, c.h, 0, 0, 0, time.UTC)
		x, y := toXY(c.east, c.north)
		for _, m := range GalileanMoons(tm) {
			if m.Name != c.name {
				continue
			}
			// The low-accuracy theory: a tenth of a radius for the inner
			// moons, a little more for Callisto far out.
			tol := 0.1
			if m.Name == "Callisto" {
				tol = 0.25
			}
			if math.Abs(m.X-x) > tol || math.Abs(m.Y-y) > tol {
				t.Errorf("%s +%dh: X %.2f Y %.2f, JPL %.2f %.2f", m.Name, c.h, m.X, m.Y, x, y)
			}
		}
	}
}

func TestGalileanMoonEvents(t *testing.T) {
	// Over 20 days Io (period 1.77 days) passes in front of Jupiter, behind
	// it and through its shadow about 11 times each; never in front and
	// behind at once.
	start := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	count := map[string]int{}
	var was [3]bool
	for tm := start; tm.Before(start.AddDate(0, 0, 20)); tm = tm.Add(5 * time.Minute) {
		for _, m := range GalileanMoons(tm) {
			if m.Transit && m.Behind {
				t.Fatalf("%s both in front and behind at %v", m.Name, tm)
			}
			if m.Name != "Io" {
				continue
			}
			now := [3]bool{m.Transit, m.Behind, m.InShadow}
			for i, k := range []string{"transit", "behind", "shadow"} {
				if now[i] && !was[i] {
					count[k]++
				}
			}
			was = now
		}
	}
	for _, k := range []string{"transit", "behind", "shadow"} {
		if count[k] < 10 || count[k] > 12 {
			t.Errorf("Io %s events in 20 days: %d, want about 11", k, count[k])
		}
	}
}

func TestSaturnRingsMeeus45a(t *testing.T) {
	// Meeus example 45.a: 1992 December 16, 0h TD (59 s before is 0h UT).
	B, P := SaturnRings(time.Date(1992, 12, 16, 0, 0, 0, 0, time.UTC).Add(-59 * time.Second))
	if math.Abs(B-16.442) > 0.03 || math.Abs(P-6.741) > 0.03 {
		t.Errorf("B %.3f P %.3f, Meeus 16.442 6.741", B, P)
	}
	// The rings were edge-on to the Earth on 2025 March 23.
	if B, _ := SaturnRings(time.Date(2025, 3, 23, 0, 0, 0, 0, time.UTC)); math.Abs(B) > 0.2 {
		t.Errorf("rings not edge-on at the 2025 crossing: B %.3f", B)
	}
}

func TestJupiterCMIIMeeus43a(t *testing.T) {
	// Meeus example 43.a (1992 December 16, 0h UT) prints ω2 = 72.74°, a
	// figure that includes his correction for the phase, 57.3·sin²(ψ/2);
	// ours is the geometric central meridian, so that is added back here.
	tm := time.Date(1992, 12, 16, 0, 0, 0, 0, time.UTC)
	_, _, psi, _, _, _, _ := jupiterBasics(tm)
	got := JupiterCMII(tm) + 57.3*sind(psi/2)*sind(psi/2)
	if math.Abs(Wrap180(got-72.74)) > 0.15 {
		t.Errorf("ω2 %.2f (with phase), Meeus 72.74", got)
	}
}

func TestRedSpotTransits(t *testing.T) {
	from := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	ts := RedSpotTransits(from, from.AddDate(0, 0, 3), 60)
	if len(ts) < 7 || len(ts) > 8 {
		t.Fatalf("%d transits in 3 days, want 7 or 8", len(ts))
	}
	for i, tm := range ts {
		if d := math.Abs(Wrap180(JupiterCMII(tm) - 60)); d > 0.05 {
			t.Errorf("transit %v: CM II %.3f", tm, JupiterCMII(tm))
		}
		if i > 0 {
			// One turn of System II: 9 h 55 min 41 s (and a little for the
			// changing distance and angle).
			gap := tm.Sub(ts[i-1])
			if (gap - (9*time.Hour + 55*time.Minute + 41*time.Second)).Abs() > time.Minute {
				t.Errorf("gap %v", gap)
			}
		}
	}
}

// ioEdges are the moments Io's flag f turns on and off between from and to
// (one-minute steps).
func ioEdges(from, to time.Time, f func(JovianMoon) bool) (on, off []time.Time) {
	was := false
	for tm := from; tm.Before(to); tm = tm.Add(time.Minute) {
		now := f(GalileanMoons(tm)[0])
		if now && !was {
			on = append(on, tm)
		}
		if !now && was {
			off = append(off, tm)
		}
		was = now
	}
	return
}

func TestIoEventsHorizons(t *testing.T) {
	// JPL Horizons for Io, 2026-Nov-12/13 (10-minute steps, Jupiter's
	// radius 18.49"): partly in the shadow ("u") from 16:40, fully ("U")
	// from 17:50; its distance from Jupiter's centre drops below the radius
	// between 17:40 and 17:50 and rises above it again between 20:00 and
	// 20:10 (Horizons shows "U" rather than "O" while it is both eclipsed
	// and hidden); in front ("t") 15:10–17:20 on the 13th. Our edges must
	// fall inside those 10-minute brackets, give or take five minutes for
	// the low-accuracy theory and the shadow taken as a cylinder.
	day := func(d, h, m int) time.Time { return time.Date(2026, 11, d, h, m, 0, 0, time.UTC) }
	from, to := day(12, 12, 0), day(13, 22, 0)
	in := func(what string, got []time.Time, lo, hi time.Time) {
		t.Helper()
		if len(got) != 1 {
			t.Errorf("%s: %d edges %v", what, len(got), got)
			return
		}
		if got[0].Before(lo.Add(-5*time.Minute)) || got[0].After(hi.Add(5*time.Minute)) {
			t.Errorf("%s at %v, JPL between %v and %v", what, got[0].Format("15:04"), lo.Format("15:04"), hi.Format("15:04"))
		}
	}
	on, _ := ioEdges(from, to, func(m JovianMoon) bool { return m.InShadow })
	in("into the shadow", on, day(12, 16, 40), day(12, 17, 50))
	on, off := ioEdges(from, to, func(m JovianMoon) bool { return m.Behind })
	in("behind Jupiter", on, day(12, 17, 40), day(12, 17, 50))
	in("out from behind", off, day(12, 20, 0), day(12, 20, 10))
	on, off = ioEdges(from, to, func(m JovianMoon) bool { return m.Transit })
	in("transit starts", on, day(13, 15, 0), day(13, 15, 10))
	in("transit ends", off, day(13, 17, 20), day(13, 17, 30))
}
