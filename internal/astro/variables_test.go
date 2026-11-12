package astro

import (
	"encoding/json"
	"math"
	"os"
	"testing"
	"time"
)

func TestAlgolMinima(t *testing.T) {
	nov := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	ms := AlgolMinima(nov, nov.AddDate(0, 1, 0))
	// 30 days / 2.867 days: 10 or 11; November 2026 has 11 (the first on
	// the 1st at 05:44, the last on the 29th at 21:54).
	if len(ms) != 11 {
		t.Fatalf("%d minima in November 2026, want 11: %v", len(ms), ms)
	}
	// By hand: cycle E = 5481 → 2445641.5135 + 5481 × 2.8673043
	// = 2445641.5135 + 15715.6948683 = JD 2461357.2083683, and JD
	// 2461356.5 is 2026-11-12 0h UT, so 0.7083683 d = 17 h 00 min 03 s.
	want := time.Date(2026, 11, 12, 17, 0, 3, 0, time.UTC)
	found := false
	for i, m := range ms {
		if m.Sub(want).Abs() < time.Second {
			found = true
		}
		if i > 0 {
			if gap := m.Sub(ms[i-1]).Hours() / 24; math.Abs(gap-2.8673043) > 1e-6 {
				t.Errorf("gap %.7f days", gap)
			}
		}
		if m.Before(nov) || !m.Before(nov.AddDate(0, 1, 0)) {
			t.Errorf("minimum %v outside the month", m)
		}
	}
	if !found {
		t.Errorf("no minimum at %v: %v", want, ms)
	}
	// Edges: a window starting exactly at a minimum includes it.
	if got := AlgolMinima(ms[3], ms[3].Add(time.Hour)); len(got) != 1 || !got[0].Equal(ms[3]) {
		t.Errorf("window from a minimum: %v", got)
	}
}

func TestDeltaCepheiMaxima(t *testing.T) {
	nov := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	ms := DeltaCepheiMaxima(nov, nov.AddDate(0, 1, 0))
	// 30 days / 5.366 days: 5 or 6; November 2026 has 5 (5th 23:28 … 27th 10:38).
	if len(ms) != 5 {
		t.Fatalf("%d maxima, want 5: %v", len(ms), ms)
	}
	// Each is a whole number of periods from the GCVS epoch.
	for _, m := range ms {
		e := (JD(m) - 2436075.445) / 5.366249
		if math.Abs(e-math.Round(e)) > 1e-6 {
			t.Errorf("%v is cycle %.6f", m, e)
		}
	}
	if want := time.Date(2026, 11, 5, 23, 28, 1, 0, time.UTC); ms[0].Sub(want).Abs() > time.Second {
		t.Errorf("first maximum %v, want %v", ms[0], want)
	}
}

func TestDoubleStarsAgainstCatalogue(t *testing.T) {
	// The app's own star catalogue (Hipparcos, J2000): every double's
	// position must sit on a catalogued star within 0.01°.
	raw, err := os.ReadFile("../catalog/data/stars.json")
	if err != nil {
		t.Skip("star catalogue not found:", err)
	}
	var stars [][]any
	if err := json.Unmarshal(raw, &stars); err != nil {
		t.Fatal(err)
	}
	if len(DoubleStars) < 20 {
		t.Errorf("only %d doubles", len(DoubleStars))
	}
	for _, d := range DoubleStars {
		best := 180.0
		for _, s := range stars {
			ra, _ := s[0].(float64)
			dec, _ := s[1].(float64)
			best = math.Min(best, Separation(d.RA, d.Dec, ra, dec))
		}
		if best > 0.01 {
			t.Errorf("%s: nearest catalogue star %.3f° away", d.Name, best)
		}
		if d.Mag2 < d.Mag1 || d.SepArcsec <= 0 || d.Kit == "" || d.About == "" || d.Con == "" {
			t.Errorf("%s: incomplete %+v", d.Name, d)
		}
	}
}
