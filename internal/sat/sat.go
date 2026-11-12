// Package sat says where the satellites are and when they pass over a place:
// the International Space Station, the Chinese station Tiangong, about a
// hundred of the brightest satellites and rocket bodies, and the strings of
// new Starlink satellites that cross the sky one behind the other ("trains")
// in the weeks after a launch.
//
// The orbits come from CelesTrak as two-line elements (TLEs), refreshed by
// the caller every few hours (Fetch). Positions are worked out with SGP4, the
// model the elements are made for, in sgp4.go (checked against Vallado's
// published test cases). A satellite is only seen when the Sun lights it
// while the sky around the watcher is dark: Passes finds every time a
// satellite climbs above a chosen altitude, when it is highest, where it
// rises and sets, whether and when it can be seen and how bright it gets.
//
// Angles are degrees, azimuth from north through east, heights in metres,
// distances in kilometres; magnitudes are the usual "smaller is brighter".
package sat

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-sky/internal/astro"
)

// Groups a satellite can be in.
const (
	GroupStation  = "station"
	GroupVisual   = "visual"
	GroupStarlink = "starlink"
)

// Standard magnitudes (brightness at 1,000 km, half lit) used when nothing
// better is known.
const (
	magISS         = -1.8
	magCSS         = -1.0
	magStation     = 2.0
	magVisual      = 4.0
	magStarlink    = 4.5
	magNewStarlink = 3.0
)

// NORAD catalogue numbers of the two crewed stations.
const (
	noradISS = 25544
	noradCSS = 48274
)

// Sat is one satellite with its orbit (a two-line element set).
type Sat struct {
	Name    string  `json:"name"`
	NoradID int     `json:"norad"`
	Line1   string  `json:"line1"`
	Line2   string  `json:"line2"`
	StdMag  float64 `json:"stdMag"` // brightness at 1,000 km, half lit
	Group   string  `json:"group"`  // "station" | "visual" | "starlink"
}

// Catalog is the satellites known at one moment.
type Catalog struct {
	Fetched time.Time `json:"fetched"`
	Sats    []Sat     `json:"sats"`
}

// Epoch is the moment the satellite's elements were taken (they are good for
// a few days either side).
func (s Sat) Epoch() time.Time {
	e, err := parseElements(s.Line1, s.Line2)
	if err != nil {
		return time.Time{}
	}
	return e.epoch
}

// ParseTLE reads a list of two-line elements (with or without a name line
// above each pair) into satellites of a group. Entries that are not valid,
// and deep-space orbits SGP4 here does not handle (periods of 225 minutes or
// more: none of them can be seen with the eye), are left out. StdMag is the
// group's default (ISS −1.8, Tiangong −1.0, other stations 2.0, Starlink 4.5,
// the rest 4.0).
func ParseTLE(text, group string) []Sat {
	var out []Sat
	var name string
	lines := strings.Split(strings.ReplaceAll(text, "\r", ""), "\n")
	for i := 0; i < len(lines); i++ {
		l := strings.TrimRight(lines[i], " \t")
		if l == "" {
			continue
		}
		if strings.HasPrefix(l, "1 ") && i+1 < len(lines) && strings.HasPrefix(lines[i+1], "2 ") {
			l2 := strings.TrimRight(lines[i+1], " \t")
			i++
			if _, err := parseElements(l, l2); err != nil {
				name = ""
				continue
			}
			id := noradID(l)
			n := name
			if n == "" {
				n = "NORAD " + strconv.Itoa(id)
			}
			out = append(out, Sat{Name: n, NoradID: id, Line1: l, Line2: l2, StdMag: defaultMag(n, id, group), Group: group})
			name = ""
			continue
		}
		// A name line ("0 NAME" in the three-line form with numbers).
		name = strings.TrimSpace(strings.TrimPrefix(l, "0 "))
	}
	return out
}

// noradID reads the catalogue number of line 1 (the "Alpha-5" form for
// numbers above 99,999: a letter for the first two digits, I and O skipped).
func noradID(line1 string) int {
	f := strings.TrimSpace(line1[2:7])
	if f == "" {
		return 0
	}
	if c := f[0]; c >= 'A' && c <= 'Z' {
		v := int(c-'A') + 10
		if c > 'I' {
			v--
		}
		if c > 'O' {
			v--
		}
		n, _ := strconv.Atoi(f[1:])
		return v*10000 + n
	}
	n, _ := strconv.Atoi(f)
	return n
}

func defaultMag(name string, id int, group string) float64 {
	up := strings.ToUpper(name)
	switch {
	case id == noradISS || strings.HasPrefix(up, "ISS (ZARYA)"):
		return magISS
	case id == noradCSS || strings.HasPrefix(up, "CSS (TIANHE)"):
		return magCSS
	case group == GroupStation:
		return magStation
	case group == GroupStarlink || strings.HasPrefix(up, "STARLINK"):
		return magStarlink
	}
	return magVisual
}

// Look is a satellite seen from a place at a moment.
type Look struct {
	Alt     float64 `json:"alt"`     // degrees above the horizon (no refraction)
	Az      float64 `json:"az"`      // degrees from north through east
	RangeKm float64 `json:"rangeKm"` // distance from the place
	RA      float64 `json:"ra"`      // degrees, topocentric, equator of date
	Dec     float64 `json:"dec"`     // degrees
	Sunlit  bool    `json:"sunlit"`  // the Sun shines on it (it is not in the Earth's shadow)
	Mag     float64 `json:"mag"`     // brightness if it is lit
}

// At is where the satellite is seen from a place (latitude and longitude in
// degrees, east positive; height in metres) at a moment.
func (s Sat) At(t time.Time, lat, lon, heightM float64) (Look, error) {
	e, err := parseElements(s.Line1, s.Line2)
	if err != nil {
		return Look{}, err
	}
	o := newSite(lat, lon, heightM)
	sun := sunVector(t)
	return o.look(e, t, s.StdMag, &sun)
}

const (
	deg    = 180 / math.Pi
	rad    = math.Pi / 180
	auKm   = 149597870.7
	earthR = 6378.137 // the Earth's equatorial radius (WGS-84), km
)

// site is a place on the Earth made ready for many looks.
type site struct {
	lat, lon       float64 // radians
	sinLat, cosLat float64
	ecef           [3]float64 // km, Earth-fixed
	geocentricR    float64    // km from the Earth's centre
	astroObs       astro.Observer
}

func newSite(lat, lon, heightM float64) *site {
	const f = 1 / 298.257223563
	e2 := f * (2 - f)
	p := lat * rad
	sp, cp := math.Sincos(p)
	n := earthR / math.Sqrt(1-e2*sp*sp)
	h := heightM / 1000
	l := lon * rad
	sl, cl := math.Sincos(l)
	x := (n + h) * cp * cl
	y := (n + h) * cp * sl
	z := (n*(1-e2) + h) * sp
	return &site{
		lat: p, lon: l, sinLat: sp, cosLat: cp,
		ecef:        [3]float64{x, y, z},
		geocentricR: math.Sqrt(x*x + y*y + z*z),
		astroObs:    astro.Observer{Lat: lat, Lon: lon, Height: heightM},
	}
}

// gmst is the Greenwich mean sidereal time in radians (IAU 1982, as SGP4's
// TEME frame wants it; UT1 is taken as UTC).
func gmst(t time.Time) float64 {
	jd := float64(t.UnixNano())/86400e9 + 2440587.5
	tut1 := (jd - 2451545.0) / 36525.0
	temp := -6.2e-6*tut1*tut1*tut1 + 0.093104*tut1*tut1 +
		(876600.0*3600+8640184.812866)*tut1 + 67310.54841
	temp = math.Mod(temp*rad/240.0, twoPi)
	if temp < 0 {
		temp += twoPi
	}
	return temp
}

// sunVector is the Sun's place from the Earth's centre in km, on the
// equator of date (close enough to TEME for shadows and phases).
func sunVector(t time.Time) [3]float64 {
	p := astro.Body("sun", t)
	sr, cr := math.Sincos(p.RA * rad)
	sd, cd := math.Sincos(p.Dec * rad)
	d := p.Dist * auKm
	return [3]float64{d * cd * cr, d * cd * sr, d * sd}
}

// position is the site in the TEME frame at a sidereal angle.
func (o *site) position(theta float64) [3]float64 {
	st, ct := math.Sincos(theta)
	return [3]float64{
		o.ecef[0]*ct - o.ecef[1]*st,
		o.ecef[0]*st + o.ecef[1]*ct,
		o.ecef[2],
	}
}

// look works out the view of a satellite at a moment.
func (o *site) look(e *elements, t time.Time, stdMag float64, sun *[3]float64) (Look, error) {
	r, _, err := e.at(t)
	if err != nil {
		return Look{}, err
	}
	return o.view(r, t, stdMag, sun), nil
}

// view is a point r (km from the Earth's centre, TEME) seen from the place.
// With sun nil only the altitude, azimuth, range and RA/Dec are filled in.
func (o *site) view(r [3]float64, t time.Time, stdMag float64, sun *[3]float64) Look {
	th := gmst(t)
	op := o.position(th)
	rho := [3]float64{r[0] - op[0], r[1] - op[1], r[2] - op[2]}
	rng := math.Sqrt(rho[0]*rho[0] + rho[1]*rho[1] + rho[2]*rho[2])

	// South, east, zenith components at the place.
	sl, cl := math.Sincos(th + o.lon)
	south := o.sinLat*cl*rho[0] + o.sinLat*sl*rho[1] - o.cosLat*rho[2]
	east := -sl*rho[0] + cl*rho[1]
	zen := o.cosLat*cl*rho[0] + o.cosLat*sl*rho[1] + o.sinLat*rho[2]

	var l Look
	l.RangeKm = rng
	l.Alt = math.Asin(clamp(zen/rng, -1, 1)) * deg
	l.Az = rev(math.Atan2(east, -south) * deg)
	l.RA = rev(math.Atan2(rho[1], rho[0]) * deg)
	l.Dec = math.Asin(clamp(rho[2]/rng, -1, 1)) * deg
	if sun != nil {
		l.Sunlit = sunlit(r, *sun)
		l.Mag = magnitude(stdMag, r, rho, rng, *sun)
	}
	return l
}

// sunlit says whether the Sun shines on a point, with the Earth's shadow
// taken as a cylinder (the penumbra is a few seconds of a pass).
func sunlit(r, sun [3]float64) bool {
	sd := math.Sqrt(sun[0]*sun[0] + sun[1]*sun[1] + sun[2]*sun[2])
	u := [3]float64{sun[0] / sd, sun[1] / sd, sun[2] / sd}
	along := r[0]*u[0] + r[1]*u[1] + r[2]*u[2]
	if along >= 0 {
		return true
	}
	px := r[0] - along*u[0]
	py := r[1] - along*u[1]
	pz := r[2] - along*u[2]
	return px*px+py*py+pz*pz > earthR*earthR
}

// magnitude is how bright a satellite looks: its standard magnitude (at
// 1,000 km and half lit), dimmed by distance, and by the phase of a diffusely
// reflecting sphere (normalised to 1 at 90°, the half-lit standard).
func magnitude(std float64, r, rho [3]float64, rng float64, sun [3]float64) float64 {
	toSun := [3]float64{sun[0] - r[0], sun[1] - r[1], sun[2] - r[2]}
	toObs := [3]float64{-rho[0], -rho[1], -rho[2]}
	ns := math.Sqrt(toSun[0]*toSun[0] + toSun[1]*toSun[1] + toSun[2]*toSun[2])
	c := (toSun[0]*toObs[0] + toSun[1]*toObs[1] + toSun[2]*toObs[2]) / (ns * rng)
	beta := math.Acos(clamp(c, -1, 1)) // phase angle: 0 = fully lit
	f := math.Sin(beta) + (math.Pi-beta)*math.Cos(beta)
	f = math.Max(f, 0.01) // a thin crescent is about five magnitudes down
	m := std + 5*math.Log10(rng/1000) - 2.5*math.Log10(f)
	return clamp(math.Round(m*10)/10, -8, 15)
}

func clamp(x, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, x)) }

func rev(x float64) float64 {
	x = math.Mod(x, 360)
	if x < 0 {
		x += 360
	}
	return x
}
