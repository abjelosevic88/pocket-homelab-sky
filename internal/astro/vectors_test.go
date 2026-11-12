package astro

import (
	"encoding/json"
	"math"
	"os"
	"testing"
	"time"
)

// The vectors the web engine (web/src/lib/astro.ts) must answer the same as
// this one: positions, sidereal time, precession and sunrise. UPDATE_VECTORS=1
// rewrites the file; otherwise this test checks the Go engine still gives it.

type vector struct {
	T        int64   `json:"t"` // ms
	Lat      float64 `json:"lat"`
	Lon      float64 `json:"lon"`
	Body     string  `json:"body,omitempty"`
	RA       float64 `json:"ra,omitempty"`
	Dec      float64 `json:"dec,omitempty"`
	Alt      float64 `json:"alt,omitempty"`
	Az       float64 `json:"az,omitempty"`
	Mag      float64 `json:"mag,omitempty"`
	Phase    float64 `json:"phase,omitempty"`
	Dist     float64 `json:"dist,omitempty"`
	LST      float64 `json:"lst,omitempty"`
	StarRA   float64 `json:"star_ra,omitempty"`
	StarDec  float64 `json:"star_dec,omitempty"`
	Sunrise  int64   `json:"sunrise,omitempty"`
	MoonRise int64   `json:"moonrise,omitempty"`
}

func makeVectors() []vector {
	places := [][2]float64{{43.8563, 18.4131}, {-33.87, 151.21}, {64.15, -21.94}, {0.5, -78.5}}
	var out []vector
	for i, iso := range []string{"2001-03-04T05:06:07Z", "2019-12-31T23:59:00Z", "2026-11-12T19:00:00Z", "2031-07-15T02:30:00Z", "2042-01-01T12:00:00Z"} {
		t, _ := time.Parse(time.RFC3339, iso)
		p := places[i%len(places)]
		o := Observer{Lat: p[0], Lon: p[1]}
		for _, b := range Bodies {
			s := At(b, o, t)
			out = append(out, vector{T: t.UnixMilli(), Lat: o.Lat, Lon: o.Lon, Body: b, RA: s.RA, Dec: s.Dec, Alt: s.Alt, Az: s.Az, Mag: s.Mag, Phase: s.Phase, Dist: s.Dist})
		}
		ra, dec := OfDate(101.2875, -16.7161, t) // Sirius
		v := vector{T: t.UnixMilli(), Lat: o.Lat, Lon: o.Lon, LST: SiderealTime(t, o.Lon), StarRA: ra, StarDec: dec}
		ms := func(t time.Time) int64 {
			if t.IsZero() {
				return 0
			}
			return t.UnixMilli()
		}
		v.Sunrise = ms(NextCrossing(BodyAlt("sun", o), HorizonSun, t, 36, true))
		v.MoonRise = ms(NextCrossing(BodyAlt("moon", o), HorizonMoon, t, 36, true))
		out = append(out, v)
	}
	return out
}

func TestVectors(t *testing.T) {
	got := makeVectors()
	if os.Getenv("UPDATE_VECTORS") == "1" {
		b, _ := json.MarshalIndent(got, "", " ")
		if err := os.WriteFile("testdata/vectors.json", b, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	b, err := os.ReadFile("testdata/vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var want []vector
	if err := json.Unmarshal(b, &want); err != nil {
		t.Fatal(err)
	}
	if len(want) != len(got) {
		t.Fatalf("%d vectors, file has %d (UPDATE_VECTORS=1 to rewrite)", len(got), len(want))
	}
	for i := range got {
		g, w := got[i], want[i]
		for _, d := range []float64{g.RA - w.RA, g.Dec - w.Dec, g.Alt - w.Alt, g.Az - w.Az, g.Mag - w.Mag, g.Phase - w.Phase, g.LST - w.LST, g.StarRA - w.StarRA} {
			if math.Abs(d) > 1e-9 {
				t.Fatalf("vector %d changed: %+v vs %+v", i, g, w)
			}
		}
		if g.Sunrise != w.Sunrise || g.MoonRise != w.MoonRise {
			t.Fatalf("vector %d rise changed", i)
		}
	}
}
