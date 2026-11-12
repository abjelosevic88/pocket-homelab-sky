package astro

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Comets and minor planets (asteroids).
//
// Their orbits come as six numbers (orbital elements) measured on the
// ecliptic and equinox of J2000, the standard frame of the Minor Planet
// Center and of JPL. From them we work out where the body is around the Sun
// at a moment, then where the Earth is, and the difference is what we see.
//
// One orbit solver serves every shape of orbit: the "universal variable"
// form of Kepler's problem (Danby, "Fundamentals of Celestial Mechanics",
// §6.9), which runs smoothly through ellipses (e < 1), parabolas (e = 1) and
// hyperbolas (e > 1). The old split into three formulas breaks down exactly
// where comets live, with e within a hair of 1. Only the Sun's pull is used,
// so the planets' tugs since the elements' date are left out: for the MPC's
// current elements that is far finer than a finder scope needs.

const (
	// gaussK is the Gaussian gravitational constant (AU^1.5 per day); its
	// square is the Sun's pull in AU³/day².
	gaussK = 0.01720209895
	// lightDaysPerAU is the time light takes to cross 1 AU, in days.
	lightDaysPerAU = 0.0057755183
	// oblJ2000 is the tilt of the ecliptic to the equator at J2000 (degrees).
	oblJ2000 = 23.4392911
	// NoMag is the Mag of a comet whose brightness is unknown.
	NoMag = 99.0
)

// Comet is a comet's orbit and brightness law, as the Minor Planet Center
// publishes them. Angles are degrees on the ecliptic and equinox of J2000.
type Comet struct {
	Name  string    `json:"name"`  // as printed, e.g. "1P/Halley", "C/2023 A3 (Tsuchinshan-ATLAS)"
	Desig string    `json:"desig"` // the bare designation, e.g. "1P", "C/2023 A3"
	Q     float64   `json:"q"`     // closest distance to the Sun (perihelion), AU
	E     float64   `json:"e"`     // eccentricity: 0 circle, <1 ellipse, 1 parabola, >1 hyperbola
	Incl  float64   `json:"incl"`  // tilt of the orbit to the ecliptic
	Peri  float64   `json:"peri"`  // argument of perihelion ω
	Node  float64   `json:"node"`  // longitude of the ascending node Ω
	Tp    time.Time `json:"tp"`    // moment of perihelion
	H     float64   `json:"h"`     // absolute magnitude M1 (at 1 AU from both Sun and Earth)
	K     float64   `json:"k"`     // brightening with nearness to the Sun: mag = H + 5 log Δ + 2.5 K log r
}

// MinorPlanet is an asteroid's orbit (JPL's elements) and brightness law
// (the H, G system). Angles are degrees on the ecliptic and equinox of J2000.
type MinorPlanet struct {
	Name  string    `json:"name"`
	A     float64   `json:"a"`    // semi-major axis, AU
	E     float64   `json:"e"`    // eccentricity
	Incl  float64   `json:"incl"` // tilt of the orbit to the ecliptic
	Peri  float64   `json:"peri"` // argument of perihelion ω
	Node  float64   `json:"node"` // longitude of the ascending node Ω
	M     float64   `json:"m"`    // mean anomaly at Epoch
	Epoch time.Time `json:"epoch"`
	H     float64   `json:"h"` // absolute magnitude
	G     float64   `json:"g"` // slope parameter (0.15 if not known)
}

// ---- the orbit solver ----

// stumpff are Stumpff's functions c0…c3 of x, the building blocks of the
// universal-variable solution (for x > 0 they are cos and sin in disguise,
// for x < 0 cosh and sinh, and near 0 a power series keeps them exact).
func stumpff(x float64) (c0, c1, c2, c3 float64) {
	switch {
	case math.Abs(x) < 0.1:
		// c2 = Σ (−x)^n/(2n+2)!, c3 = Σ (−x)^n/(2n+3)!
		t2, t3 := 0.5, 1.0/6
		for n := 1; n < 12; n++ {
			c2 += t2
			c3 += t3
			t2 *= -x / float64((2*n+1)*(2*n+2))
			t3 *= -x / float64((2*n+2)*(2*n+3))
		}
		c0, c1 = 1-x*c2, 1-x*c3
	case x > 0:
		sx := math.Sqrt(x)
		c0, c1 = math.Cos(sx), math.Sin(sx)/sx
		c2, c3 = (1-c0)/x, (1-c1)/x
	default:
		sx := math.Sqrt(-x)
		c0, c1 = math.Cosh(sx), math.Sinh(sx)/sx
		c2, c3 = (1-c0)/x, (1-c1)/x
	}
	return
}

// orbitPlane is where a body is in its own orbit's plane dt days after
// perihelion: x towards the perihelion, y along the motion there, r the
// distance from the Sun (AU). q is the perihelion distance, e the
// eccentricity; any e ≥ 0 works.
//
// Kepler's equation in universal form (Danby 6.9, starting at perihelion
// where the radial speed is zero):
//
//	dt = q·s·c1(βs²) + μ·s³·c3(βs²),  r = q·c0(βs²) + μ·s²·c2(βs²)
//
// with μ = k² and β = μ(1−e)/q, solved for s by Laguerre's method (which
// converges from almost any start). The first guess is the exact parabola.
func orbitPlane(q, e, dt float64) (x, y, r float64) {
	mu := gaussK * gaussK
	beta := mu * (1 - e) / q
	if e < 1 {
		// Ellipse: fold the time into the orbit around the nearest perihelion.
		a := q / (1 - e)
		period := 2 * math.Pi * math.Sqrt(a*a*a/mu)
		dt -= period * math.Round(dt/period)
	}
	// Parabola guess: μs³/6 + q s = dt (Cardano).
	p, qq := 6*q/mu, 6*dt/mu
	disc := math.Sqrt(qq*qq/4 + p*p*p/27)
	s := math.Cbrt(qq/2+disc) + math.Cbrt(qq/2-disc)
	if beta > 0 {
		lim := math.Pi / math.Sqrt(beta)
		s = clamp(s, -lim, lim)
	}
	const n = 5.0
	for k := 0; k < 100; k++ {
		c0, c1, c2, c3 := stumpff(beta * s * s)
		F := q*s*c1 + mu*s*s*s*c3 - dt
		F1 := q*c0 + mu*s*s*c2
		F2 := mu * e * s * c1
		root := math.Sqrt(math.Abs((n-1)*(n-1)*F1*F1 - n*(n-1)*F*F2))
		den := F1 + math.Copysign(root, F1)
		if den == 0 {
			break
		}
		ds := n * F / den
		s -= ds
		if math.Abs(ds) <= 1e-13*(1+math.Abs(s)) {
			break
		}
	}
	_, _, c2, c3 := stumpff(beta * s * s)
	f := 1 - mu*s*s*c2/q
	g := dt - mu*s*s*s*c3
	v0 := math.Sqrt(mu * (1 + e) / q) // speed at perihelion
	x, y = q*f, v0*g
	return x, y, math.Hypot(x, y)
}

// helioJ2000 turns a place in the orbit's plane into heliocentric
// rectangular coordinates on the ecliptic of J2000 (AU).
func helioJ2000(x, y, incl, peri, node float64) [3]float64 {
	cw, sw := cosd(peri), sind(peri)
	cn, sn := cosd(node), sind(node)
	ci, si := cosd(incl), sind(incl)
	return [3]float64{
		x*(cn*cw-sn*sw*ci) - y*(cn*sw+sn*cw*ci),
		x*(sn*cw+cn*sw*ci) - y*(sn*sw-cn*cw*ci),
		x*(sw*si) + y*(cw*si),
	}
}

// eclToEquJ2000 turns J2000 ecliptic rectangular coordinates into J2000
// equatorial ones.
func eclToEquJ2000(v [3]float64) [3]float64 {
	ce, se := cosd(oblJ2000), sind(oblJ2000)
	return [3]float64{v[0], v[1]*ce - v[2]*se, v[1]*se + v[2]*ce}
}

// sunGeoJ2000 is the Sun seen from the Earth's centre as a J2000 equatorial
// vector (AU). Schlyter's Sun is on the ecliptic and equinox of DATE (his
// elements carry the precession in them), so it is first put on the equator
// of date with the obliquity of date, then turned back to J2000 with the
// inverse precession. The comet's orbit is J2000 from the start, so the two
// meet in one frame (J2000) before they are subtracted; only the finished
// direction is precessed to the date.
func sunGeoJ2000(t time.Time) [3]float64 {
	d := dayNumber(t)
	lon, r := sunEcliptic(d)
	ra, dec := eclToEqu(lon, 0, obliquity(d))
	v := Precession(t).T().Apply(Vec(ra, dec))
	return [3]float64{v[0] * r, v[1] * r, v[2] * r}
}

// smallBodyAt is the shared geometry of comets and asteroids. helio gives
// the body's heliocentric J2000 ecliptic position at a moment; mag its
// brightness from the distances to the Sun (r) and Earth (Δ) and the phase
// angle (degrees, Sun–body–Earth).
func smallBodyAt(kind string, t time.Time, helio func(time.Time) [3]float64, mag func(r, delta, phase float64) float64) Position {
	sun := sunGeoJ2000(t)
	// Light time: we see the body where it was when the light left it.
	var g, h [3]float64
	tau := 0.0
	for k := 0; k < 3; k++ {
		h = eclToEquJ2000(helio(t.Add(-time.Duration(tau * 86400e9))))
		g = [3]float64{h[0] + sun[0], h[1] + sun[1], h[2] + sun[2]}
		tau = math.Sqrt(g[0]*g[0]+g[1]*g[1]+g[2]*g[2]) * lightDaysPerAU
	}
	delta := math.Sqrt(g[0]*g[0] + g[1]*g[1] + g[2]*g[2])
	r := math.Sqrt(h[0]*h[0] + h[1]*h[1] + h[2]*h[2])
	R := math.Sqrt(sun[0]*sun[0] + sun[1]*sun[1] + sun[2]*sun[2])
	p := Position{Body: kind, Dist: delta, Helio: r}
	p.RA, p.Dec = RADec(Precession(t).Apply([3]float64{g[0] / delta, g[1] / delta, g[2] / delta}))
	// Ecliptic longitude and latitude of date, from the RA/Dec of date.
	ecl := obliquity(dayNumber(t))
	v := Vec(p.RA, p.Dec)
	p.Lon = Rev(atan2d(v[1]*cosd(ecl)+v[2]*sind(ecl), v[0]))
	p.Lat = asind(v[2]*cosd(ecl) - v[1]*sind(ecl))
	p.Elong = acosd((R*R + delta*delta - r*r) / (2 * R * delta))
	phase := acosd((r*r + delta*delta - R*R) / (2 * r * delta))
	p.Phase = (1 + cosd(phase)) / 2
	p.Mag = mag(r, delta, phase)
	return p
}

// At is where the comet is in the sky at a moment, seen from the centre of
// the Earth: right ascension and declination of date, distance from the
// Earth (Dist) and the Sun (Helio) in AU, brightness and angle from the
// Sun. Mag is the total brightness of the head by the comet's own law
// (NoMag when unknown); comets often surprise, so it is a guide only.
func (c Comet) At(t time.Time) Position {
	q, e := c.Q, c.E
	helio := func(t time.Time) [3]float64 {
		dt := t.Sub(c.Tp).Hours() / 24
		x, y, _ := orbitPlane(q, e, dt)
		return helioJ2000(x, y, c.Incl, c.Peri, c.Node)
	}
	mag := func(r, delta, _ float64) float64 {
		if c.H == 0 && c.K == 0 {
			return NoMag
		}
		return c.H + 5*math.Log10(delta) + 2.5*c.K*math.Log10(r)
	}
	return smallBodyAt("comet", t, helio, mag)
}

// At is where the asteroid is in the sky at a moment, seen from the centre
// of the Earth (see Comet.At). Mag is by the H, G system (Bowell et al.
// 1989, as the MPC uses it).
func (m MinorPlanet) At(t time.Time) Position {
	q := m.A * (1 - m.E)
	// Mean motion in degrees a day; perihelion was M/n days before Epoch.
	n := gaussK * deg / math.Pow(m.A, 1.5)
	tp := m.Epoch.Add(-time.Duration(m.M / n * 86400e9))
	helio := func(t time.Time) [3]float64 {
		dt := t.Sub(tp).Hours() / 24
		x, y, _ := orbitPlane(q, m.E, dt)
		return helioJ2000(x, y, m.Incl, m.Peri, m.Node)
	}
	return smallBodyAt("asteroid", t, helio, func(r, delta, phase float64) float64 {
		return hgMag(m.H, m.G, r, delta, phase)
	})
}

// hgMag is an asteroid's magnitude by the H, G system.
func hgMag(H, G, r, delta, phase float64) float64 {
	ta := math.Tan(phase * rad / 2)
	phi1 := math.Exp(-3.33 * math.Pow(ta, 0.63))
	phi2 := math.Exp(-1.87 * math.Pow(ta, 1.22))
	return H + 5*math.Log10(r*delta) - 2.5*math.Log10((1-G)*phi1+G*phi2)
}

// ---- the Minor Planet Center's comet file ----

// col is a 1-based, inclusive column range of a fixed-width line, trimmed
// ("" when the line is too short).
func col(line string, from, to int) string {
	if len(line) < from {
		return ""
	}
	if len(line) < to {
		to = len(line)
	}
	return strings.TrimSpace(line[from-1 : to])
}

// ParseMPCComets reads the Minor Planet Center's one-line comet orbits
// (Soft00Cmt.txt, the "MPC 1-line comet format" of
// minorplanetcenter.net/iau/info/CometOrbitFormat.html). Columns, 1-based:
//
//	1–4 periodic number, 5 orbit type (C, P, D, X, I, A), 6–12 provisional
//	designation (packed), 15–18 year, 20–21 month, 23–29 day (TT) of
//	perihelion, 31–39 q (AU), 42–49 e, 52–59 ω, 62–69 Ω, 72–79 i (J2000),
//	82–89 epoch (YYYYMMDD), 92–95 H, 97–100 slope K, 103–158 name,
//	160–168 reference.
//
// Lines that don't parse are skipped. A comet with no magnitude law gets
// H = K = 0 (its Mag is then NoMag). The perihelion moment is TT, read as
// UTC: the minute between them is nothing on a comet's path.
func ParseMPCComets(text string) []Comet {
	var out []Comet
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, "\r")
		if len(line) < 79 {
			continue
		}
		year, err1 := strconv.Atoi(col(line, 15, 18))
		month, err2 := strconv.Atoi(col(line, 20, 21))
		day, err3 := strconv.ParseFloat(col(line, 23, 29), 64)
		var nums [5]float64
		bad := err1 != nil || err2 != nil || err3 != nil || month < 1 || month > 12
		for i, c := range [][2]int{{31, 39}, {42, 49}, {52, 59}, {62, 69}, {72, 79}} {
			v, err := strconv.ParseFloat(col(line, c[0], c[1]), 64)
			if err != nil {
				bad = true
			}
			nums[i] = v
		}
		if bad || nums[0] <= 0 || nums[1] < 0 {
			continue
		}
		c := Comet{Q: nums[0], E: nums[1], Peri: nums[2], Node: nums[3], Incl: nums[4]}
		c.Tp = time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC).Add(time.Duration((day - 1) * 86400e9))
		hs, ks := col(line, 92, 95), col(line, 97, 100)
		if h, err := strconv.ParseFloat(hs, 64); err == nil {
			c.H = h
			if k, err := strconv.ParseFloat(ks, 64); err == nil {
				c.K = k
			} else {
				c.K = 4 // the usual law when only H is given
			}
		}
		c.Name = col(line, 103, 158)
		if c.Name == "" {
			c.Name = strings.TrimLeft(col(line, 1, 4), "0") + col(line, 5, 5) + " " + col(line, 6, 12)
			c.Name = strings.TrimSpace(c.Name)
		}
		c.Desig = c.Name
		if i := strings.Index(c.Name, " ("); i > 0 {
			c.Desig = c.Name[:i]
		} else if i := strings.Index(c.Name, "/"); i > 0 && c.Name[i-1] >= 'A' && c.Name[i-1] <= 'Z' && isDigits(c.Name[:i-1]) {
			c.Desig = c.Name[:i] // "1P/Halley" → "1P"
		}
		out = append(out, c)
	}
	return out
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ---- JPL's Small-Body Database ----

// jsonNum reads a JSON value that JPL sends as a string ("2.36") or a number.
func jsonNum(raw json.RawMessage) (float64, bool) {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return 0, false
	}
	if strings.HasPrefix(s, `"`) {
		if err := json.Unmarshal(raw, &s); err != nil {
			return 0, false
		}
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return v, err == nil
}

// ParseSBDB reads one object from JPL's Small-Body Database API
// (https://ssd-api.jpl.nasa.gov/sbdb.api?sstr=4&full-prec=1&phys-par=1):
// the "orbit.elements" a, e, i, om, w, ma at "orbit.epoch" (a Julian day)
// and, from "phys_par", H and G. Ask with phys-par=1 or H is missing (and
// left 0); a missing G is the usual 0.15. full-prec=1 gives every digit
// (without it JPL rounds to three).
func ParseSBDB(body []byte) (MinorPlanet, error) {
	var raw struct {
		Message string `json:"message"`
		List    any    `json:"list"`
		Object  struct {
			Fullname  string `json:"fullname"`
			Shortname string `json:"shortname"`
		} `json:"object"`
		Orbit *struct {
			Epoch    json.RawMessage `json:"epoch"`
			Equinox  string          `json:"equinox"`
			Elements []struct {
				Name  string          `json:"name"`
				Value json.RawMessage `json:"value"`
			} `json:"elements"`
		} `json:"orbit"`
		PhysPar []struct {
			Name  string          `json:"name"`
			Value json.RawMessage `json:"value"`
		} `json:"phys_par"`
	}
	var m MinorPlanet
	if err := json.Unmarshal(body, &raw); err != nil {
		return m, fmt.Errorf("sbdb: %w", err)
	}
	if raw.List != nil {
		return m, errors.New("sbdb: more than one object matches; be more precise")
	}
	if raw.Orbit == nil {
		if raw.Message != "" {
			return m, errors.New("sbdb: " + raw.Message)
		}
		return m, errors.New("sbdb: no orbit in the answer")
	}
	if eq := raw.Orbit.Equinox; eq != "" && eq != "J2000" {
		return m, fmt.Errorf("sbdb: elements on equinox %s, not J2000", eq)
	}
	m.Name = raw.Object.Shortname
	if m.Name == "" {
		m.Name = raw.Object.Fullname
	}
	epoch, ok := jsonNum(raw.Orbit.Epoch)
	if !ok {
		return m, errors.New("sbdb: no epoch")
	}
	m.Epoch = FromJD(epoch)
	have := map[string]bool{}
	for _, el := range raw.Orbit.Elements {
		v, ok := jsonNum(el.Value)
		if !ok {
			continue
		}
		switch el.Name {
		case "a":
			m.A = v
		case "e":
			m.E = v
		case "i":
			m.Incl = v
		case "om":
			m.Node = v
		case "w":
			m.Peri = v
		case "ma":
			m.M = v
		default:
			continue
		}
		have[el.Name] = true
	}
	for _, k := range []string{"a", "e", "i", "om", "w", "ma"} {
		if !have[k] {
			return m, fmt.Errorf("sbdb: element %q missing", k)
		}
	}
	if m.E >= 1 || m.A <= 0 {
		return m, errors.New("sbdb: not an elliptic orbit (use a Comet for e ≥ 1)")
	}
	m.G = 0.15
	for _, p := range raw.PhysPar {
		if v, ok := jsonNum(p.Value); ok {
			switch p.Name {
			case "H":
				m.H = v
			case "G":
				m.G = v
			}
		}
	}
	return m, nil
}
