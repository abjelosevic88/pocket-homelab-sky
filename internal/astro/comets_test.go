package astro

import (
	"math"
	"testing"
	"time"
)

// trueAnomaly is the angle from the perihelion of an in-plane position.
func trueAnomaly(x, y float64) float64 { return atan2d(y, x) }

// timeFromPerihelion is the classical (ellipse / Barker / hyperbola) time,
// in days, to reach true anomaly nu: an independent check of orbitPlane.
func timeFromPerihelion(q, e, nu float64) float64 {
	mu := gaussK * gaussK
	h := math.Tan(nu * rad / 2)
	switch {
	case e < 1:
		a := q / (1 - e)
		E := 2 * math.Atan(math.Sqrt((1-e)/(1+e))*h)
		return (E - e*math.Sin(E)) / math.Sqrt(mu/(a*a*a))
	case e == 1:
		// Barker's equation: tan(ν/2) + tan³(ν/2)/3 = √(μ/2q³)·t
		return (h + h*h*h/3) / math.Sqrt(mu/(2*q*q*q))
	default:
		a := q / (e - 1)
		H := 2 * math.Atanh(math.Sqrt((e-1)/(e+1))*h)
		return (e*math.Sinh(H) - H) / math.Sqrt(mu/(a*a*a))
	}
}

func TestOrbitSolverRoundTrip(t *testing.T) {
	for _, e := range []float64{0, 0.2, 0.5, 0.9, 0.97, 0.9999, 1, 1.0001, 1.02, 1.5, 3} {
		for _, q := range []float64{0.1, 0.59, 1, 4} {
			for _, nu := range []float64{-150, -100, -30, -1, 0, 0.5, 20, 60, 110, 170} {
				if e > 1 && math.Abs(nu) >= acosd(-1/e)-1 {
					continue // beyond the hyperbola's asymptote
				}
				dt := timeFromPerihelion(q, e, nu)
				x, y, r := orbitPlane(q, e, dt)
				wantR := q * (1 + e) / (1 + e*cosd(nu))
				if d := math.Abs(Wrap180(trueAnomaly(x, y) - nu)); d > 1e-7 {
					t.Errorf("e=%v q=%v ν=%v: got ν=%v", e, q, nu, trueAnomaly(x, y))
				}
				if math.Abs(r-wantR) > 1e-9*wantR {
					t.Errorf("e=%v q=%v ν=%v: r=%v want %v", e, q, nu, r, wantR)
				}
			}
		}
	}
	// An ellipse comes round again after one period.
	q, e := 0.5, 0.6
	a := q / (1 - e)
	P := 2 * math.Pi * math.Sqrt(a*a*a) / gaussK
	x1, y1, _ := orbitPlane(q, e, 100)
	x2, y2, _ := orbitPlane(q, e, 100+7*P)
	if math.Hypot(x1-x2, y1-y2) > 1e-9 {
		t.Errorf("ellipse not periodic: %v,%v vs %v,%v", x1, y1, x2, y2)
	}
}

func TestStumpffSeriesMatchesClosedForm(t *testing.T) {
	for _, x := range []float64{-0.1, 0.1} {
		a0, a1, a2, a3 := stumpff(x)
		b0, b1, b2, b3 := stumpff(x * (1 - 1e-12)) // the series side
		for i, d := range []float64{a0 - b0, a1 - b1, a2 - b2, a3 - b3} {
			if math.Abs(d) > 1e-12 {
				t.Errorf("x=%v c%d differs by %g", x, i, d)
			}
		}
	}
}

// Real lines from the MPC's Soft00Cmt.txt (2026-11-12), and junk.
const mpcSample = `0001P         2061 07 31.5235  0.570807  0.968038  112.1409   59.2402  162.1944  20261112   5.5  3.2  1P/Halley                                                MPC191592
0002P         2027 02 10.2278  0.338594  0.847326  187.2879  334.0196   11.3479  20261112  14.6  4.0  2P/Encke                                                 MPC xxxxx
    CK23A030  2024 09 27.8434  0.391289  1.000183  308.5896   21.6836  139.0978  20261112   6.5  3.2  C/2023 A3 (Tsuchinshan-ATLAS)                            MPEC 2026-O98
    CJ95O010  1997 03 29.0280  0.926906  0.994893  130.7391  281.8153   89.7307  20261112  -2.0  4.0  C/1995 O1 (Hale-Bopp)                                    MPC194091
    PJ96R020  2026 06 16.1211  2.587031  0.313838  333.5684   39.9881    2.5998  20261112
this line is not an orbit at all, but it is long enough to be looked at by the parser...........
`

func TestParseMPCComets(t *testing.T) {
	cs := ParseMPCComets(mpcSample)
	if len(cs) != 5 {
		t.Fatalf("got %d comets, want 5: %+v", len(cs), cs)
	}
	h := cs[0]
	if h.Name != "1P/Halley" || h.Desig != "1P" || h.Q != 0.570807 || h.E != 0.968038 ||
		h.Peri != 112.1409 || h.Node != 59.2402 || h.Incl != 162.1944 || h.H != 5.5 || h.K != 3.2 {
		t.Errorf("Halley parsed wrong: %+v", h)
	}
	if want := time.Date(2061, 7, 31, 12, 33, 50, 0, time.UTC); h.Tp.Sub(want).Abs() > time.Second {
		t.Errorf("Halley perihelion %v, want %v", h.Tp, want)
	}
	if c := cs[2]; c.Desig != "C/2023 A3" || c.Name != "C/2023 A3 (Tsuchinshan-ATLAS)" || c.E != 1.000183 {
		t.Errorf("Tsuchinshan-ATLAS parsed wrong: %+v", c)
	}
	if c := cs[3]; c.H != -2 || c.Desig != "C/1995 O1" {
		t.Errorf("Hale-Bopp parsed wrong: %+v", c)
	}
	if c := cs[4]; c.Name != "P J96R020" || c.H != 0 || c.K != 0 {
		t.Errorf("unnamed comet parsed wrong: %+v", c)
	}
	if m := cs[4].At(time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)).Mag; m != NoMag {
		t.Errorf("comet without a magnitude law: Mag %v, want NoMag", m)
	}
}

// checkJ2000 compares a position of date with an astrometric J2000 one
// from JPL Horizons.
func checkJ2000(t *testing.T, what string, p Position, tm time.Time, ra, dec, delta, r, tolDeg, tolAU float64) {
	t.Helper()
	pra, pdec := ToJ2000(p.RA, p.Dec, tm)
	if sep := Separation(pra, pdec, ra, dec); sep > tolDeg {
		t.Errorf("%s: RA %.3f Dec %.3f (J2000), JPL %.3f %.3f: %.3f° off", what, pra, pdec, ra, dec, sep)
	}
	if math.Abs(p.Dist-delta) > tolAU || math.Abs(p.Helio-r) > tolAU {
		t.Errorf("%s: Δ %.4f r %.4f, JPL Δ %.4f r %.4f", what, p.Dist, p.Helio, delta, r)
	}
}

func TestHalley1986(t *testing.T) {
	tm := time.Date(1986, 3, 15, 0, 0, 0, 0, time.UTC)
	// JPL Horizons osculating elements for 1986-Mar-15 (record
	// "DES=1P;CAP<1990", J2000 ecliptic, perihelion 1986 Feb 9.46).
	jpl := Comet{Name: "1P/Halley", Q: 0.5871038226038944, E: 0.9672763422854721, Incl: 162.2422040514738,
		Peri: 111.8656485904534, Node: 58.86005765203967, Tp: FromJD(2446470.958947423380), H: 5.5, K: 3.2}
	// Horizons, 1986-Mar-15 00:00 UT: RA 300.21805 Dec −22.45737 (ICRF),
	// r 0.917132, Δ 0.937243, elongation 56.6°.
	p := jpl.At(tm)
	checkJ2000(t, "Halley (JPL elements)", p, tm, 300.21805, -22.45737, 0.937243, 0.917132, 0.01, 0.0005)
	if math.Abs(p.Elong-56.6) > 0.3 {
		t.Errorf("Halley elongation %.2f, JPL 56.6", p.Elong)
	}
	if p.Mag < 3 || p.Mag > 6 {
		t.Errorf("Halley magnitude %.1f, expected about 4–5", p.Mag)
	}
	// The textbook elements (q 0.5860, e 0.96714, ω 111.332, Ω 58.420,
	// i 162.262, perihelion 1986 Feb 9.46) are a rounder set: within a
	// quarter of a degree. (Elements osculating far from the date, like
	// Horizons' 1968 set, are 2° off by 1986: Jupiter and the jets of gas
	// move Halley that much.)
	book := Comet{Q: 0.5860, E: 0.96714, Incl: 162.262, Peri: 111.332, Node: 58.420,
		Tp: time.Date(1986, 2, 9, 0, 0, 0, 0, time.UTC).Add(time.Duration(0.46 * 24 * float64(time.Hour)))}
	checkJ2000(t, "Halley (textbook elements)", book.At(tm), tm, 300.21805, -22.45737, 0.937243, 0.917132, 0.5, 0.01)
}

func TestTsuchinshanATLASNearParabolic(t *testing.T) {
	// Horizons: 2024-Oct-12 00:00 RA 208.21463 Dec −1.47073, r 0.544940, Δ 0.473436;
	// 2024-Oct-20 00:00 RA 246.12494 Dec +2.31482, r 0.693893, Δ 0.586283.
	t1 := time.Date(2024, 10, 12, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 10, 20, 0, 0, 0, 0, time.UTC)
	// Horizons osculating elements for 2024-Oct-12: a hyperbola, e = 1.000063.
	osc := Comet{Q: 0.3914170324442775, E: 1.000063031043158, Incl: 139.1105154592885,
		Peri: 308.4883591192136, Node: 21.55947235764483, Tp: FromJD(2460581.241716984659)}
	checkJ2000(t, "C/2023 A3 Oct 12 (osculating)", osc.At(t1), t1, 208.21463, -1.47073, 0.473436, 0.544940, 0.01, 0.0005)
	checkJ2000(t, "C/2023 A3 Oct 20 (osculating)", osc.At(t2), t2, 246.12494, 2.31482, 0.586283, 0.693893, 0.01, 0.0005)
	// The MPC file's elements (e = 1.000183) are for an epoch two years
	// later, so they drift a little from the 2024 path.
	c := ParseMPCComets(mpcSample)[2]
	checkJ2000(t, "C/2023 A3 Oct 12 (MPC)", c.At(t1), t1, 208.21463, -1.47073, 0.473436, 0.544940, 0.6, 0.003)
	checkJ2000(t, "C/2023 A3 Oct 20 (MPC)", c.At(t2), t2, 246.12494, 2.31482, 0.586283, 0.693893, 0.6, 0.003)
}

// Trimmed answer of https://ssd-api.jpl.nasa.gov/sbdb.api?sstr=4&full-prec=1&phys-par=1 (2026-11-12).
const vestaSBDB = `{"object":{"orbit_id":"36","fullname":"4 Vesta (A807 FA)","kind":"an","shortname":"4 Vesta","des":"4","prefix":null,"neo":false,"spkid":"20000004","pha":false},
"phys_par":[{"name":"H","value":"3.25","sigma":null},{"name":"G","value":"0.32"},{"name":"diameter","value":"522.77","units":"km"}],
"orbit":{"epoch":"2461200.5","equinox":"J2000","source":"JPL","elements":[
{"name":"e","value":".09020374382834395"},{"name":"a","value":"2.361365965127599","units":"au"},
{"name":"q","value":"2.148361914524259"},{"name":"i","value":"7.143925545058711"},
{"name":"om","value":"103.701293265032"},{"name":"w","value":"151.4686478221564"},
{"name":"ma","value":"81.19015607686903"},{"name":"tp","value":"2460901.587379842988"},
{"name":"per","value":"1325.389042911101"},{"name":"n","value":".2716183613599909"},{"name":"ad","value":"2.574370015730939"}]}}`

func TestVestaFromSBDB(t *testing.T) {
	v, err := ParseSBDB([]byte(vestaSBDB))
	if err != nil {
		t.Fatal(err)
	}
	if v.Name != "4 Vesta" || v.A != 2.361365965127599 || v.H != 3.25 || v.G != 0.32 || math.Abs(JD(v.Epoch)-2461200.5) > 1e-6 {
		t.Errorf("Vesta parsed wrong: %+v", v)
	}
	// Horizons (JPL's own magnitude law, APmag):
	for _, c := range []struct {
		t                      time.Time
		ra, dec, r, delta, mag float64
	}{
		{time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC), 16.40317, -4.56957, 2.491484, 1.630323, 6.927},
		{time.Date(2027, 3, 12, 0, 0, 0, 0, time.UTC), 37.60314, 10.18945, 2.558876, 3.116508, 8.472},
		{time.Date(2027, 11, 7, 0, 0, 0, 0, time.UTC), 136.00314, 17.68665, 2.528144, 2.313862, 7.966},
	} {
		p := v.At(c.t)
		checkJ2000(t, "Vesta "+c.t.Format("2006-01-02"), p, c.t, c.ra, c.dec, c.delta, c.r, 0.03, 0.002) // two-body only: the planets' tugs add up over a year
		if p.Mag < 5.5 || p.Mag > 8.5 || math.Abs(p.Mag-c.mag) > 0.3 {
			t.Errorf("Vesta %s: mag %.2f, JPL %.2f", c.t.Format("2006-01-02"), p.Mag, c.mag)
		}
		if p.Body != "asteroid" {
			t.Errorf("body %q", p.Body)
		}
	}
}

func TestParseSBDBErrors(t *testing.T) {
	for _, body := range []string{
		`not json`,
		`{"message":"specified object was not found"}`,
		`{"list":[{"pdes":"1","name":"Ceres"}],"code":"300"}`,
		`{"object":{},"orbit":{"epoch":"2461200.5","elements":[{"name":"a","value":"2"}]}}`,
	} {
		if _, err := ParseSBDB([]byte(body)); err == nil {
			t.Errorf("no error for %s", body)
		}
	}
}
