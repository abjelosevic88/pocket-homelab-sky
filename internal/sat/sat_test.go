package sat

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/abjelosevic88/pocket-homelab-sky/internal/astro"
)

// Vallado, Crawford, Hujsak & Kelso, "Revisiting Spacetrack Report #3"
// (AIAA 2006-6753): the "TEME example" satellite 00005 and its published
// positions (km) and velocities (km/s) from tcppver.out, with two more
// near-Earth cases from the same verification set.
var valladoCases = []struct {
	name, l1, l2 string
	rows         [][7]float64
}{
	{
		"00005 TEME example",
		"1 00005U 58002B   00179.78495062  .00000023  00000-0  28098-4 0  4753",
		"2 00005  34.2682 348.7242 1859667 331.7664  19.3264 10.82419157413667",
		[][7]float64{
			{0, 7022.46529266, -1400.08296755, 0.03995155, 1.893841015, 6.405893759, 4.534807250},
			{360, -7154.03120202, -3783.17682504, -3536.19412294, 4.741887409, -4.151817765, -2.093935425},
			{720, -7134.59340119, 6531.68641334, 3260.27186483, -4.113793027, -2.911922039, -2.557327851},
			{1080, 5568.53901181, 4492.06992591, 3863.87641983, -4.209106476, 5.159719888, 2.744852980},
			{1440, -938.55923943, -6268.18748831, -4294.02924751, 7.536105209, -0.427127707, 0.989878080},
			{1800, -9680.56121728, 2802.47771354, 124.10688038, -0.905874102, -4.659467970, -3.227347517},
			{2160, 190.19796988, 7746.96653614, 5110.00675412, -6.112325142, 1.527008184, -0.139152358},
			{2520, 5579.55640116, -3995.61396789, -1518.82108966, 4.767927483, 5.123185301, 4.276837355},
			{2880, -8650.73082219, -1914.93811525, -3007.03603443, 3.067165127, -4.828384068, -2.515322836},
			{3240, -5429.79204164, 7574.36493792, 3747.39305236, -4.999442110, -1.800561422, -2.229392830},
			{3600, 6759.04583722, 2001.58198220, 2783.55192533, -2.180993947, 6.402085603, 3.644723952},
			{3960, -3791.44531559, -5712.95617894, -4533.48630714, 6.668817493, -2.516382327, -0.082384354},
			{4320, -9060.47373569, 4658.70952502, 813.68673153, -2.232832783, -4.110453490, -3.157345433},
		},
	},
	{
		"06251 moderate drag",
		"1 06251U 62025E   06176.82412014  .00008885  00000-0  12808-3 0  3985",
		"2 06251  58.0579  54.0425 0030035 139.1568 221.1854 15.56387291  6774",
		[][7]float64{
			{0, 3988.31022699, 5498.96657235, 0.90055879, -3.290032738, 2.357652820, 6.496623475},
			{120, -3935.69800083, 409.10980837, 5471.33577327, -3.374784183, -6.635211043, -1.942056221},
			{240, -1675.12766915, -5683.30432352, -3286.21510937, 5.282496925, 1.508674259, -5.354872978},
			{2880, 1159.27802897, 5056.60175495, 4353.49418579, -5.968060341, -2.314790406, 4.230722669},
		},
	},
	{
		"28057 near-circular",
		"1 28057U 03049A   06177.78615833  .00000060  00000-0  35940-4 0  1836",
		"2 28057  98.4283 247.6961 0000884  88.1964 271.9322 14.35478080140550",
		[][7]float64{
			{0, -2715.28237486, -6619.26436889, -0.01341443, -1.008587273, 0.422782003, 7.385272942},
			{120, -1816.87920942, -1835.78762132, 6661.07926465, 2.325140071, 6.655669329, 2.463394512},
			{240, 1483.17364291, 5395.21248786, 4448.65907172, 2.560540387, 4.039025766, -5.736648561},
			{2880, 1788.42334580, 1990.50530957, -6640.59337725, -2.074169091, -6.683381288, -2.562777776},
		},
	},
}

func TestVallado(t *testing.T) {
	for _, c := range valladoCases {
		e, err := parseElements(c.l1, c.l2)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		for _, row := range c.rows {
			r, v, err := e.propagate(row[0])
			if err != nil {
				t.Fatalf("%s t=%v: %v", c.name, row[0], err)
			}
			for k := range 3 {
				if d := math.Abs(r[k] - row[1+k]); d > 1e-5 {
					t.Errorf("%s t=%v: r[%d] = %.8f, want %.8f (off %.2g km)", c.name, row[0], k, r[k], row[1+k], d)
				}
				if d := math.Abs(v[k] - row[4+k]); d > 1e-8 {
					t.Errorf("%s t=%v: v[%d] = %.9f, want %.9f (off %.2g km/s)", c.name, row[0], k, v[k], row[4+k], d)
				}
			}
		}
	}
}

func TestDeepSpaceRefused(t *testing.T) {
	// A GPS satellite (12-hour orbit): deep space, not handled.
	_, err := parseElements(
		"1 24876U 97035A   26314.50000000  .00000000  00000-0  00000-0 0  9990",
		"2 24876  55.4523 172.1834 0047713  54.0021 306.4566  2.00563254213465")
	if err != errDeepSpace {
		t.Fatalf("err = %v, want errDeepSpace", err)
	}
}

const sampleTLE = `ISS (ZARYA)
1 25544U 98067A   26314.00000000  .00012345  00000-0  21852-3 0  9990
2 25544  51.6345 302.1520 0004713  47.8152 312.3274 15.50083712547120
CSS (TIANHE)
1 48274U 21035A   26314.00000000  .00022541  00000-0  26391-3 0  9991
2 48274  41.4651 118.6047 0005512 312.0781  47.9706 15.60519382312472
1 20580U 90037B   26314.00000000  .00004815  00000-0  22463-3 0  9993
2 20580  28.4698 141.2964 0002405  98.4119 261.6716 15.29318705768024
NOT A SATELLITE
1 99999U 26999A   garbage
2 99999 nonsense
GPS BIIR-2  (PRN 13)
1 24876U 97035A   26314.50000000  .00000000  00000-0  00000-0 0  9990
2 24876  55.4523 172.1834 0047713  54.0021 306.4566  2.00563254213465
`

func TestParseTLE(t *testing.T) {
	ss := ParseTLE(strings.ReplaceAll(sampleTLE, "\n", "\r\n"), GroupStation)
	if len(ss) != 3 {
		t.Fatalf("got %d satellites, want 3: %+v", len(ss), ss)
	}
	if s := ss[0]; s.Name != "ISS (ZARYA)" || s.NoradID != 25544 || s.StdMag != -1.8 || s.Group != "station" ||
		!strings.HasPrefix(s.Line1, "1 25544U") || !strings.HasPrefix(s.Line2, "2 25544 ") {
		t.Errorf("ISS = %+v", s)
	}
	if s := ss[1]; s.Name != "CSS (TIANHE)" || s.NoradID != 48274 || s.StdMag != -1.0 {
		t.Errorf("CSS = %+v", s)
	}
	if s := ss[2]; s.Name != "NORAD 20580" || s.StdMag != 2.0 {
		t.Errorf("unnamed = %+v", s)
	}
	if got := ss[0].Epoch(); !got.Equal(demoEpoch) {
		t.Errorf("epoch = %v, want %v", got, demoEpoch)
	}
	if v := ParseTLE(sampleTLE, GroupVisual); v[2].StdMag != 4.0 {
		t.Errorf("visual default mag = %v", v[2].StdMag)
	}
	if n := noradID("1 A0001U 26001A"); n != 100001 {
		t.Errorf("alpha-5 A0001 = %d", n)
	}
	if n := noradID("1 Z9999U 26001A"); n != 339999 {
		t.Errorf("alpha-5 Z9999 = %d", n)
	}
}

func TestDemoTLEsAreValid(t *testing.T) {
	cat := Demo()
	if len(cat.Sats) < 10 {
		t.Fatalf("demo has %d satellites", len(cat.Sats))
	}
	var text strings.Builder
	for _, s := range cat.Sats {
		if len(s.Line1) != 69 || len(s.Line2) != 69 {
			t.Errorf("%s: line lengths %d, %d", s.Name, len(s.Line1), len(s.Line2))
		}
		for _, l := range []string{s.Line1, s.Line2} {
			if checksum(l[:68]) != l[68:] {
				t.Errorf("%s: bad checksum in %q", s.Name, l)
			}
		}
		text.WriteString(s.Name + "\n" + s.Line1 + "\n" + s.Line2 + "\n")
	}
	if got := ParseTLE(text.String(), GroupVisual); len(got) != len(cat.Sats) {
		t.Errorf("demo reparsed: %d of %d", len(got), len(cat.Sats))
	}
	if expFormat(0.00021852) != " 21852-3" || expFormat(-0.000012345) != "-12345-4" {
		t.Errorf("expFormat: %q %q", expFormat(0.00021852), expFormat(-0.000012345))
	}
	// The real ISS line checks out the same way.
	if l := "1 25544U 98067A   08264.51782528 -.00002182  00000-0 -11606-4 0  2927"; checksum(l[:68]) != "7" {
		t.Errorf("checksum of the ISS example = %s", checksum(l[:68]))
	}
}

func TestISSAt(t *testing.T) {
	iss := Demo().Sats[0]
	if iss.NoradID != 25544 {
		t.Fatalf("first demo satellite is %s", iss.Name)
	}
	e, _ := parseElements(iss.Line1, iss.Line2)
	r, v, err := e.at(demoEpoch)
	if err != nil {
		t.Fatal(err)
	}
	rr := math.Sqrt(r[0]*r[0] + r[1]*r[1] + r[2]*r[2])
	if h := rr - earthR; h < 380 || h > 440 {
		t.Errorf("ISS height = %.1f km", h)
	}
	if vv := math.Sqrt(v[0]*v[0] + v[1]*v[1] + v[2]*v[2]); vv < 7.5 || vv > 7.8 {
		t.Errorf("ISS speed = %.3f km/s", vv)
	}

	// Seen from Sarajevo over the next day: always within sensible bounds,
	// sometimes overhead-ish, and range matching the altitude.
	const lat, lon = 43.8563, 18.4131
	best := -90.0
	for m := 0; m < 1440; m++ {
		tm := demoEpoch.Add(time.Duration(m) * time.Minute)
		l, err := iss.At(tm, lat, lon, 520)
		if err != nil {
			t.Fatal(err)
		}
		if l.Alt < -90 || l.Alt > 90 || l.Az < 0 || l.Az >= 360 || l.RA < 0 || l.RA >= 360 || l.Dec < -90 || l.Dec > 90 {
			t.Fatalf("bad look %+v", l)
		}
		// The nearest it can be is its height, the farthest across the Earth.
		if l.RangeKm < 370 || l.RangeKm > 13500 {
			t.Fatalf("range %.0f km", l.RangeKm)
		}
		if l.Alt > 0 && l.RangeKm > 2500 {
			t.Fatalf("above the horizon but %.0f km away", l.RangeKm)
		}
		if l.Alt > 80 && l.RangeKm > 450 {
			t.Fatalf("overhead but %.0f km away", l.RangeKm)
		}
		best = math.Max(best, l.Alt)
	}
	if best < 30 {
		t.Errorf("highest the ISS got in a day: %.1f°", best)
	}
}

func TestSunlitAndMagnitude(t *testing.T) {
	sun := [3]float64{auKm, 0, 0}
	if !sunlit([3]float64{7000, 0, 0}, sun) || !sunlit([3]float64{-7000, 7000, 0}, sun) {
		t.Error("a satellite on the day side is lit")
	}
	if sunlit([3]float64{-7000, 0, 0}, sun) || sunlit([3]float64{-7000, 3000, 3000}, sun) {
		t.Error("a satellite behind the Earth is in its shadow")
	}
	// Half lit at 1,000 km: the standard magnitude.
	r := [3]float64{0, 7000, 0}
	rho := [3]float64{0, 1000, 0}
	if m := magnitude(-1.8, r, rho, 1000, sun); math.Abs(m+1.8) > 0.05 {
		t.Errorf("half lit at 1000 km: %v", m)
	}
	// Farther is fainter, fully lit is brighter.
	if m := magnitude(-1.8, r, [3]float64{0, 2000, 0}, 2000, sun); math.Abs(m-(-1.8+5*math.Log10(2))) > 0.05 {
		t.Errorf("at 2000 km: %v", m)
	}
	// The watcher between the Sun and the satellite sees it fully lit.
	if m := magnitude(4, [3]float64{7000, 0, 0}, [3]float64{-1000, 0, 0}, 1000, sun); m >= 4 {
		t.Errorf("fully lit should be brighter than half lit: %v", m)
	}
}

// The horizon frame agrees with astro's: a point far out along a direction
// of the sky comes out where astro.Horizontal puts it.
func TestViewMatchesAstro(t *testing.T) {
	o := newSite(43.8563, 18.4131, 0)
	tm := time.Date(2026, 11, 12, 19, 30, 0, 0, time.UTC)
	for _, c := range [][2]float64{{10, 20}, {101.3, -16.7}, {250, 60}, {300, 5}, {37.95, 89.26}} {
		ra, dec := c[0], c[1]
		d := 1e12
		r := [3]float64{d * math.Cos(dec*rad) * math.Cos(ra*rad), d * math.Cos(dec*rad) * math.Sin(ra*rad), d * math.Sin(dec*rad)}
		l := o.view(r, tm, 0, nil)
		alt, az := astro.Horizontal(ra, dec, o.astroObs, tm)
		if math.Abs(l.Alt-alt) > 0.01 || (alt < 89 && angleDiff(l.Az, az) > 0.01) {
			t.Errorf("ra %v dec %v: alt/az %.4f/%.4f, astro %.4f/%.4f", ra, dec, l.Alt, l.Az, alt, az)
		}
		if astro.Separation(l.RA, l.Dec, ra, dec) > 1e-5 {
			t.Errorf("ra/dec %v/%v back as %v/%v", ra, dec, l.RA, l.Dec)
		}
	}
	// And the Sun's altitude from the same frame matches astro.At.
	sun := sunVector(tm)
	l := o.view(sun, tm, 0, nil)
	if want := astro.At("sun", o.astroObs, tm).Alt; math.Abs(l.Alt-want) > 0.02 {
		t.Errorf("sun altitude %.3f, astro %.3f", l.Alt, want)
	}
}

func sarajevoPasses(t *testing.T, days int) []Pass {
	t.Helper()
	from := demoEpoch
	return Passes(Demo().Sats, 43.8563, 18.4131, 520, from, from.Add(time.Duration(days)*24*time.Hour), 10)
}

func TestPassesSarajevo(t *testing.T) {
	ps := sarajevoPasses(t, 3)
	if len(ps) == 0 {
		t.Fatal("no passes")
	}
	if !sort.SliceIsSorted(ps, func(i, j int) bool { return ps[i].Start.Before(ps[j].Start) }) {
		t.Error("passes are not sorted by start")
	}
	var iss, issVisible, css int
	issBest := 99.0
	for _, p := range ps {
		if !(p.Start.Before(p.Peak) || p.Start.Equal(p.Peak)) || !(p.Peak.Before(p.End) || p.Peak.Equal(p.End)) {
			t.Errorf("%s: start %v peak %v end %v out of order", p.Sat, p.Start, p.Peak, p.End)
		}
		if p.Start.Nanosecond() != 0 || p.Peak.Nanosecond() != 0 || p.End.Nanosecond() != 0 {
			t.Errorf("%s: times not whole seconds", p.Sat)
		}
		if p.PeakAlt < 10 || p.PeakAlt > 90 {
			t.Errorf("%s: peak altitude %.1f", p.Sat, p.PeakAlt)
		}
		if p.Visible && (p.VisibleFrom.Before(p.Start) || p.VisibleTo.After(p.End) || p.VisibleTo.Before(p.VisibleFrom)) {
			t.Errorf("%s: visible %v–%v outside %v–%v", p.Sat, p.VisibleFrom, p.VisibleTo, p.Start, p.End)
		}
		if p.Visible && (p.Mag < -8 || p.Mag > 10) {
			t.Errorf("%s: magnitude %.1f", p.Sat, p.Mag)
		}
		switch p.NoradID {
		case 25544:
			iss++
			// Above 10°: up to 12 minutes; a pass that climbs past 15°
			// takes at least two (only a graze is shorter).
			d := p.End.Sub(p.Start)
			if d > 12*time.Minute || (p.PeakAlt >= 15 && d < 2*time.Minute) {
				t.Errorf("ISS pass of %v peaking at %.1f°", d, p.PeakAlt)
			}
			if p.Visible {
				issVisible++
				if p.Mag > 1.5 {
					t.Errorf("a visible ISS pass of magnitude %.1f", p.Mag)
				}
				issBest = math.Min(issBest, p.Mag)
			}
		case 48274:
			css++
		case 20580:
			t.Errorf("HST (28.5° orbit) never climbs 10° over Sarajevo, got %+v", p)
		}
	}
	if iss < 4 {
		t.Errorf("%d ISS passes in 3 days", iss)
	}
	if issVisible == 0 {
		t.Error("no visible ISS pass")
	}
	if issBest > -1 {
		t.Errorf("the brightest visible ISS pass is only magnitude %.1f", issBest)
	}
	if css == 0 {
		t.Error("no Tiangong pass")
	}
	if testing.Verbose() {
		for _, p := range ps {
			t.Logf("%-16s %s %s %s  az %3.0f→%3.0f→%3.0f  alt %4.1f  mag %4.1f vis=%v train=%v×%d",
				p.Sat, p.Start.Format("01-02 15:04:05"), p.Peak.Format("15:04:05"), p.End.Format("15:04:05"),
				p.StartAz, p.PeakAz, p.EndAz, p.PeakAlt, p.Mag, p.Visible, p.Train, p.Count)
		}
	}
}

func TestStarlinkTrain(t *testing.T) {
	ps := sarajevoPasses(t, 3)
	trains := 0
	for _, p := range ps {
		if p.Train {
			trains++
			if p.Count < 2 || p.Group != GroupStarlink {
				t.Errorf("train %+v", p)
			}
			if p.End.Sub(p.Start) > 20*time.Minute {
				t.Errorf("train lasts %v", p.End.Sub(p.Start))
			}
		} else if p.Group == GroupStarlink && p.Count != 1 {
			t.Errorf("single Starlink with count %d", p.Count)
		}
	}
	if trains == 0 {
		t.Fatal("no Starlink train merged")
	}
	// Each train stands for eight satellites; nothing is left over as a
	// loose Starlink passing together with it.
	for _, p := range ps {
		if p.Group == GroupStarlink && !p.Train {
			t.Errorf("loose Starlink pass %s at %v", p.Sat, p.Start)
		}
	}
}

func TestPassUnderWayAtStart(t *testing.T) {
	ps := sarajevoPasses(t, 1)
	var first Pass
	for _, p := range ps {
		if p.NoradID == 25544 {
			first = p
			break
		}
	}
	if first.Sat == "" {
		t.Fatal("no ISS pass")
	}
	// Ask from the middle of that pass: it is still there, whole.
	mid := first.Start.Add(first.End.Sub(first.Start) / 2)
	again := Passes(Demo().Sats[:1], 43.8563, 18.4131, 520, mid, mid.Add(time.Hour), 10)
	near := func(a, b time.Time) bool { return a.Sub(b).Abs() <= time.Second }
	if len(again) == 0 || !near(again[0].Start, first.Start) || !near(again[0].End, first.End) {
		t.Errorf("pass under way: got %+v, want start %v end %v", again, first.Start, first.End)
	}
}

func TestPassesSpeed(t *testing.T) {
	// About 250 satellites for three days: the demo set over and over.
	var sats []Sat
	for len(sats) < 250 {
		sats = append(sats, Demo().Sats...)
	}
	start := time.Now()
	ps := Passes(sats, 43.8563, 18.4131, 520, demoEpoch, demoEpoch.Add(72*time.Hour), 10)
	took := time.Since(start)
	t.Logf("%d satellites, 3 days: %d passes in %v", len(sats), len(ps), took)
	if took > 2*time.Second {
		t.Errorf("too slow: %v", took)
	}
}

func BenchmarkPasses(b *testing.B) {
	var sats []Sat
	for len(sats) < 250 {
		sats = append(sats, Demo().Sats...)
	}
	for b.Loop() {
		Passes(sats, 43.8563, 18.4131, 520, demoEpoch, demoEpoch.Add(72*time.Hour), 10)
	}
}

func TestFetch(t *testing.T) {
	demo := Demo()
	block := func(ss []Sat) string {
		var b strings.Builder
		for _, s := range ss {
			b.WriteString(s.Name + "\n" + s.Line1 + "\n" + s.Line2 + "\n")
		}
		return b.String()
	}
	extraStation := Sat{Name: "SOYUZ-MS 28", Line1: demo.Sats[0].Line1, Line2: demo.Sats[0].Line2}
	extraStation.Line1 = "1 99001" + extraStation.Line1[7:]
	extraStation.Line2 = "2 99001" + extraStation.Line2[7:]
	var gotUA []string
	var groups []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = append(gotUA, r.UserAgent())
		g := r.URL.Query().Get("GROUP")
		groups = append(groups, g)
		if r.URL.Query().Get("FORMAT") != "tle" {
			http.Error(w, "format", http.StatusBadRequest)
			return
		}
		switch g {
		case "stations":
			w.Write([]byte(block([]Sat{demo.Sats[0], extraStation, demo.Sats[1]})))
		case "visual":
			// The visual list has the ISS too: it must not come twice.
			w.Write([]byte(block(append([]Sat{demo.Sats[0]}, demo.Sats[2:7]...))))
		case "last-30-days":
			other := Sat{Name: "ONEWEB-0999", Line1: "1 99002" + demo.Sats[2].Line1[7:], Line2: "2 99002" + demo.Sats[2].Line2[7:]}
			w.Write([]byte(block(append([]Sat{other}, demo.Sats[7:]...))))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	old := celestrakURL
	celestrakURL = srv.URL + "/NORAD/elements/gp.php"
	defer func() { celestrakURL = old }()

	cat, err := Fetch(context.Background(), srv.Client(), "PocketSky/test")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(groups, ",") != "stations,visual,last-30-days" {
		t.Errorf("groups asked for: %v", groups)
	}
	for _, ua := range gotUA {
		if ua != "PocketSky/test" {
			t.Errorf("user agent %q", ua)
		}
	}
	count := map[string]int{}
	ids := map[int]int{}
	for _, s := range cat.Sats {
		count[s.Group]++
		ids[s.NoradID]++
		switch {
		case s.NoradID == 25544 && (s.Group != GroupStation || s.StdMag != -1.8):
			t.Errorf("ISS %+v", s)
		case s.NoradID == 48274 && s.StdMag != -1.0:
			t.Errorf("CSS %+v", s)
		case s.Group == GroupStarlink && s.StdMag != 3.0:
			t.Errorf("new Starlink %+v", s)
		case s.Group == GroupVisual && s.StdMag != 4.0:
			t.Errorf("visual %+v", s)
		}
	}
	if count[GroupStation] != 2 || count[GroupVisual] != 5 || count[GroupStarlink] != 8 {
		t.Errorf("groups: %v", count)
	}
	for id, n := range ids {
		if n > 1 {
			t.Errorf("NORAD %d %d times", id, n)
		}
	}
	if cat.Fetched.IsZero() {
		t.Error("no fetch time")
	}

	// A refusal in plain words is an error, not an empty list.
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("GROUP") == "visual" {
			w.Write([]byte("GP data has not updated since your last successful download"))
			return
		}
		http.Error(w, "busy", http.StatusServiceUnavailable)
	}))
	defer srv2.Close()
	celestrakURL = srv2.URL
	if cat, err := Fetch(context.Background(), srv2.Client(), "x"); err == nil || cat != nil {
		t.Errorf("all failed: cat %v err %v", cat, err)
	}
}
