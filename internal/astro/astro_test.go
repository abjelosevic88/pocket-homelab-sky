package astro

import (
	"encoding/json"
	"math"
	"os"
	"testing"
	"time"
)

type reference struct {
	Positions []struct {
		T     time.Time `json:"t"`
		Body  string    `json:"body"`
		RA    float64   `json:"ra"`
		Dec   float64   `json:"dec"`
		Alt   float64   `json:"alt"`
		Az    float64   `json:"az"`
		Mag   *float64  `json:"mag"`
		Phase *float64  `json:"phase"`
	} `json:"positions"`
	RiseSet []struct {
		From    time.Time  `json:"from"`
		Body    string     `json:"body"`
		Alt     *float64   `json:"alt"`
		Rise    *time.Time `json:"rise"`
		Set     *time.Time `json:"set"`
		Evening *time.Time `json:"evening"`
		Morning *time.Time `json:"morning"`
	} `json:"riseset"`
	Quarters []struct {
		Q int       `json:"q"`
		T time.Time `json:"t"`
	} `json:"quarters"`
	Seasons []struct {
		Y                  int
		Mar, Jun, Sep, Dec time.Time
	} `json:"seasons"`
	Lunar []struct {
		Kind      string    `json:"kind"`
		Peak      time.Time `json:"peak"`
		SDPartial float64   `json:"sd_partial"`
		SDPenum   float64   `json:"sd_penum"`
	} `json:"lunar"`
	Solar []struct {
		Kind        string    `json:"kind"`
		Peak        time.Time `json:"peak"`
		Obscuration float64   `json:"obscuration"`
		Alt         float64   `json:"alt"`
	} `json:"solar"`
	Global []struct {
		Kind string    `json:"kind"`
		Peak time.Time `json:"peak"`
	} `json:"global"`
}

var sarajevo = Observer{Lat: 43.8563, Lon: 18.4131}

func loadRef(t *testing.T) reference {
	t.Helper()
	b, err := os.ReadFile("testdata/reference.json")
	if err != nil {
		t.Fatal(err)
	}
	var r reference
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	return r
}

// The engine against astronomy-engine (VSOP87 and the full lunar theory):
// arc minutes for the Moon and the planets, seconds of time for the Sun.
func TestPositionsMatchReference(t *testing.T) {
	r := loadRef(t)
	tol := map[string]float64{"sun": 0.02, "moon": 0.12, "mercury": 0.06, "venus": 0.05, "mars": 0.05, "jupiter": 0.05, "saturn": 0.06, "uranus": 0.06, "neptune": 0.06}
	worst := map[string]float64{}
	for _, p := range r.Positions {
		s := At(p.Body, sarajevo, p.T)
		sep := Separation(s.RA, s.Dec, p.RA, p.Dec)
		if sep > worst[p.Body] {
			worst[p.Body] = sep
		}
		if sep > tol[p.Body] {
			t.Errorf("%s %s: off by %.3f° (ra %.3f/%.3f dec %.3f/%.3f)", p.Body, p.T.Format(time.RFC3339), sep, s.RA, p.RA, s.Dec, p.Dec)
		}
		if math.Abs(s.Alt-p.Alt) > tol[p.Body]+0.02 {
			t.Errorf("%s %s: altitude %.3f want %.3f", p.Body, p.T.Format(time.RFC3339), s.Alt, p.Alt)
		}
		if p.Mag != nil && p.Body != "moon" && math.Abs(s.Mag-*p.Mag) > 0.35 {
			t.Errorf("%s %s: magnitude %.2f want %.2f", p.Body, p.T.Format(time.RFC3339), s.Mag, *p.Mag)
		}
		if p.Phase != nil && math.Abs(s.Phase-*p.Phase) > 0.02 {
			t.Errorf("%s %s: phase %.3f want %.3f", p.Body, p.T.Format(time.RFC3339), s.Phase, *p.Phase)
		}
	}
	t.Logf("worst error (degrees): %v", worst)
}

func near(t *testing.T, what string, got time.Time, want *time.Time, tol time.Duration) {
	t.Helper()
	if want == nil {
		return
	}
	if d := got.Sub(*want); d > tol || d < -tol {
		t.Errorf("%s: got %s want %s (%s)", what, got.Format(time.RFC3339), want.Format(time.RFC3339), d)
	}
}

func TestRiseSetAndTwilight(t *testing.T) {
	r := loadRef(t)
	for _, x := range r.RiseSet {
		if x.Alt != nil {
			f := BodyAlt("sun", sarajevo)
			near(t, "dusk", NextCrossing(f, *x.Alt, x.From, 36, false), x.Evening, 2*time.Minute)
			near(t, "dawn", NextCrossing(f, *x.Alt, x.From, 36, true), x.Morning, 2*time.Minute)
			continue
		}
		f := BodyAlt(x.Body, sarajevo)
		tol := time.Minute
		if x.Body == "moon" {
			tol = 4 * time.Minute
		}
		near(t, x.Body+" rise", NextCrossing(f, HorizonOf(x.Body), x.From, 36, true), x.Rise, tol)
		near(t, x.Body+" set", NextCrossing(f, HorizonOf(x.Body), x.From, 36, false), x.Set, tol)
	}
}

func TestMoonPhasesAndSeasons(t *testing.T) {
	r := loadRef(t)
	got := MoonPhases(r.Quarters[0].T.Add(-time.Hour), r.Quarters[len(r.Quarters)-1].T.Add(time.Hour))
	if len(got) != len(r.Quarters) {
		t.Fatalf("got %d phases want %d", len(got), len(r.Quarters))
	}
	subs := []string{"new", "first", "full", "last"}
	for i, q := range r.Quarters {
		if got[i].Sub != subs[q.Q] {
			t.Errorf("phase %d: %s want %s", i, got[i].Sub, subs[q.Q])
		}
		near(t, "phase "+got[i].Sub, got[i].T, &q.T, 10*time.Minute)
	}
	for _, s := range r.Seasons {
		ev := Seasons(time.Date(s.Y, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(s.Y+1, 1, 1, 0, 0, 0, 0, time.UTC))
		if len(ev) != 4 {
			t.Fatalf("%d: %d seasons", s.Y, len(ev))
		}
		for i, w := range []time.Time{s.Mar, s.Jun, s.Sep, s.Dec} {
			w := w
			near(t, ev[i].Title, ev[i].T, &w, 15*time.Minute)
		}
	}
}

func TestEclipses(t *testing.T) {
	r := loadRef(t)
	from, to := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), r.Lunar[len(r.Lunar)-1].Peak.Add(time.Hour)
	le := LunarEclipses(from, to)
	// Grazing penumbral eclipses (a few minutes in the faint outer shadow,
	// nothing to see) may be left out.
	lunar := r.Lunar[:0]
	for _, w := range r.Lunar {
		if w.Kind != "penumbral" || w.SDPenum > 30 {
			lunar = append(lunar, w)
		}
	}
	r.Lunar = lunar
	if len(le) != len(r.Lunar) {
		for _, e := range le {
			t.Logf("got %s %s", e.Sub, e.T)
		}
		t.Fatalf("got %d lunar eclipses want %d", len(le), len(r.Lunar))
	}
	for i, w := range r.Lunar {
		if le[i].Sub != w.Kind {
			t.Errorf("lunar %s: %s want %s", w.Peak.Format("2006-01-02"), le[i].Sub, w.Kind)
		}
		near(t, "lunar peak", le[i].T, &w.Peak, 8*time.Minute)
		if w.SDPartial > 0 {
			half := le[i].End.Sub(*le[i].Start).Minutes() / 2
			if math.Abs(half-w.SDPartial) > 8 {
				t.Errorf("lunar %s: partial half-length %.0f min want %.0f", w.Peak.Format("2006-01-02"), half, w.SDPartial)
			}
		}
	}
	gs := SolarEclipses(from, r.Global[len(r.Global)-1].Peak.Add(time.Hour), nil)
	if len(gs) != len(r.Global) {
		for _, e := range gs {
			t.Logf("got %s %s", e.Sub, e.T)
		}
		t.Fatalf("got %d solar eclipses want %d", len(gs), len(r.Global))
	}
	for i, w := range r.Global {
		if gs[i].Sub != w.Kind && w.Kind != "hybrid" {
			t.Errorf("solar %s: %s want %s", w.Peak.Format("2006-01-02"), gs[i].Sub, w.Kind)
		}
		if d := gs[i].T.Sub(w.Peak); d > 90*time.Minute || d < -90*time.Minute {
			t.Errorf("solar %s: peak %s", w.Peak.Format("2006-01-02"), gs[i].T)
		}
	}
	o := sarajevo
	ls := SolarEclipses(from, time.Date(2030, 12, 31, 0, 0, 0, 0, time.UTC), &o)
	byDay := map[string]Event{}
	for _, e := range ls {
		byDay[e.T.Format("2006-01-02")] = e
	}
	for _, w := range r.Solar {
		e, ok := byDay[w.Peak.Format("2006-01-02")]
		if !ok || e.Start == nil {
			t.Errorf("solar %s: not found from Sarajevo", w.Peak.Format("2006-01-02"))
			continue
		}
		if math.Abs(e.Value-w.Obscuration) > 0.06 {
			t.Errorf("solar %s: covers %.2f want %.2f", w.Peak.Format("2006-01-02"), e.Value, w.Obscuration)
		}
	}
	// 2027-08-02 is seen well from Sarajevo; 2028-01-26 happens after sunset.
	if e := byDay["2027-08-02"]; e.Visible == nil || !*e.Visible {
		t.Errorf("2027-08-02 should be visible: %+v", e)
	}
	if e := byDay["2028-01-26"]; e.Visible == nil || *e.Visible {
		t.Errorf("2028-01-26 should not be visible: %+v", e)
	}
}

func TestOppositionsAndShowers(t *testing.T) {
	from, to := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 12, 31, 0, 0, 0, 0, time.UTC)
	var jup, mars bool
	for _, e := range Oppositions(from, to) {
		if e.Bodies[0] == "jupiter" && e.T.Format("2006-01-02") == "2027-02-11" {
			jup = true
		}
		if e.Bodies[0] == "mars" && e.T.Format("2006-01") == "2027-02" {
			mars = true
		}
	}
	if !jup || !mars {
		t.Errorf("want Jupiter opposition 2027-02-11 (%v) and Mars Feb 2027 (%v): %+v", jup, mars, Oppositions(from, to))
	}
	ms := MeteorShowers(time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC))
	want := map[string]string{"quadrantids": "2027-01-0[34]", "perseids": "2027-08-1[23]", "geminids": "2027-12-1[45]"}
	for _, e := range ms {
		if pat, ok := want[e.Sub]; ok {
			if ok2, _ := pathMatch(pat, e.T.Format("2006-01-02")); !ok2 {
				t.Errorf("%s peak %s, want %s", e.Sub, e.T.Format(time.RFC3339), pat)
			}
			delete(want, e.Sub)
		}
	}
	if len(want) > 0 {
		t.Errorf("missing showers %v", want)
	}
}

func pathMatch(pat, s string) (bool, error) {
	// tiny glob: only [ab] classes and literals
	i := 0
	for j := 0; j < len(pat); j++ {
		if i >= len(s) {
			return false, nil
		}
		if pat[j] == '[' {
			k := j + 1
			ok := false
			for ; pat[k] != ']'; k++ {
				if pat[k] == s[i] {
					ok = true
				}
			}
			if !ok {
				return false, nil
			}
			j = k
		} else if pat[j] != s[i] {
			return false, nil
		}
		i++
	}
	return i == len(s), nil
}

func TestNight(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Sarajevo")
	n, err := NightAt(sarajevo, "2026-11-12", loc)
	if err != nil {
		t.Fatal(err)
	}
	if n.DarkKind != "astro" || n.DarkHours < 10 || n.DarkHours > 12.5 {
		t.Errorf("November night: %s %.1f h", n.DarkKind, n.DarkHours)
	}
	if n.Sunset == nil || n.Sunset.In(loc).Hour() != 16 {
		t.Errorf("sunset %v", n.Sunset)
	}
	// Midsummer in Oslo: no astronomical night.
	oslo := Observer{Lat: 59.91, Lon: 10.75}
	ol, _ := time.LoadLocation("Europe/Oslo")
	n2, _ := NightAt(oslo, "2026-06-21", ol)
	if n2.DarkKind == "astro" {
		t.Errorf("Oslo midsummer should have no astronomical night: %+v", n2)
	}
}

func TestPrecessionRoundTrip(t *testing.T) {
	at := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	ra, dec := OfDate(88.79, 7.41, at) // Betelgeuse
	if d := Separation(ra, dec, 88.79, 7.41); d < 0.3 || d > 0.5 {
		t.Errorf("30 years of precession should move it ~0.4°, got %.3f", d)
	}
	r2, d2 := ToJ2000(ra, dec, at)
	if Separation(r2, d2, 88.79, 7.41) > 1e-9 {
		t.Errorf("round trip off")
	}
}
