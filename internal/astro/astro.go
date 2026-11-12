// Package astro works out where the Sun, the Moon, the planets and the stars
// are in the sky: positions, brightness, phase, rising and setting, twilight,
// and the events of the sky (Moon phases, seasons, conjunctions, oppositions,
// eclipses, meteor showers).
//
// The method is Paul Schlyter's "How to compute planetary positions"
// (orbital elements with the main perturbations of the Moon, Jupiter, Saturn
// and Uranus), good to a minute of arc or two: far finer than the eye, a pair
// of binoculars or a finder scope can tell. The web app has the same engine in
// TypeScript (web/src/lib/astro.ts); both answer the vectors in testdata.
//
// Angles are degrees, times are time.Time (UTC inside), right ascension is
// degrees (0–360) of the equator and equinox of date; star catalogues are
// J2000 and are precessed to the date.
package astro

import (
	"math"
	"time"
)

const (
	rad = math.Pi / 180
	deg = 180 / math.Pi
	// AU in km and the Earth's equatorial radius in km.
	auKm     = 149597870.7
	earthRKm = 6378.14
)

// Bodies the engine knows, in the order the app lists them.
var Bodies = []string{"sun", "moon", "mercury", "venus", "mars", "jupiter", "saturn", "uranus", "neptune"}

// Planets are the bodies that are planets (not the Sun or the Moon).
var Planets = []string{"mercury", "venus", "mars", "jupiter", "saturn", "uranus", "neptune"}

// Names are the bodies' display names.
var Names = map[string]string{"sun": "Sun", "moon": "Moon", "mercury": "Mercury", "venus": "Venus", "mars": "Mars",
	"jupiter": "Jupiter", "saturn": "Saturn", "uranus": "Uranus", "neptune": "Neptune"}

// radiusKm is each body's equatorial radius, for its apparent size.
var radiusKm = map[string]float64{"sun": 696000, "moon": 1737.4, "mercury": 2439.7, "venus": 6051.8, "mars": 3396.2,
	"jupiter": 71492, "saturn": 60268, "uranus": 25559, "neptune": 24764}

// JD is the Julian day of a moment.
func JD(t time.Time) float64 { return float64(t.UnixMilli())/86400000.0 + 2440587.5 }

// FromJD is the moment of a Julian day.
func FromJD(jd float64) time.Time {
	return time.UnixMilli(int64(math.Round((jd - 2440587.5) * 86400000.0))).UTC()
}

// dayNumber is Schlyter's day number: days since 1999-12-31 00:00 UT.
func dayNumber(t time.Time) float64 { return JD(t) - 2451543.5 }

// Rev brings an angle into 0–360.
func Rev(x float64) float64 {
	x = math.Mod(x, 360)
	if x < 0 {
		x += 360
	}
	return x
}

// Wrap180 brings an angle into −180–180.
func Wrap180(x float64) float64 {
	x = Rev(x)
	if x > 180 {
		x -= 360
	}
	return x
}

func sind(x float64) float64      { return math.Sin(x * rad) }
func cosd(x float64) float64      { return math.Cos(x * rad) }
func atan2d(y, x float64) float64 { return math.Atan2(y, x) * deg }
func asind(x float64) float64     { return math.Asin(clamp(x, -1, 1)) * deg }
func acosd(x float64) float64     { return math.Acos(clamp(x, -1, 1)) * deg }

func clamp(x, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, x)) }

// Obliquity of the ecliptic on a day number.
func obliquity(d float64) float64 { return 23.4393 - 3.563e-7*d }

type elements struct{ N, i, w, a, e, M float64 }

func orbit(body string, d float64) elements {
	switch body {
	case "sun":
		return elements{0, 0, 282.9404 + 4.70935e-5*d, 1, 0.016709 - 1.151e-9*d, 356.0470 + 0.9856002585*d}
	case "moon":
		return elements{125.1228 - 0.0529538083*d, 5.1454, 318.0634 + 0.1643573223*d, 60.2666, 0.054900, 115.3654 + 13.0649929509*d}
	case "mercury":
		return elements{48.3313 + 3.24587e-5*d, 7.0047 + 5.00e-8*d, 29.1241 + 1.01444e-5*d, 0.387098, 0.205635 + 5.59e-10*d, 168.6562 + 4.0923344368*d}
	case "venus":
		return elements{76.6799 + 2.46590e-5*d, 3.3946 + 2.75e-8*d, 54.8910 + 1.38374e-5*d, 0.723330, 0.006773 - 1.302e-9*d, 48.0052 + 1.6021302244*d}
	case "mars":
		return elements{49.5574 + 2.11081e-5*d, 1.8497 - 1.78e-8*d, 286.5016 + 2.92961e-5*d, 1.523688, 0.093405 + 2.516e-9*d, 18.6021 + 0.5240207766*d}
	case "jupiter":
		return elements{100.4542 + 2.76854e-5*d, 1.3030 - 1.557e-7*d, 273.8777 + 1.64505e-5*d, 5.20256, 0.048498 + 4.469e-9*d, 19.8950 + 0.0830853001*d}
	case "saturn":
		return elements{113.6634 + 2.38980e-5*d, 2.4886 - 1.081e-7*d, 339.3939 + 2.97661e-5*d, 9.55475, 0.055546 - 9.499e-9*d, 316.9670 + 0.0334442282*d}
	case "uranus":
		return elements{74.0005 + 1.3978e-5*d, 0.7733 + 1.9e-8*d, 96.6612 + 3.0565e-5*d, 19.18171 - 1.55e-8*d, 0.047318 + 7.45e-9*d, 142.5905 + 0.011725806*d}
	case "neptune":
		return elements{131.7806 + 3.0173e-5*d, 1.7700 - 2.55e-7*d, 272.8461 - 6.027e-6*d, 30.05826 + 3.313e-8*d, 0.008606 + 2.15e-9*d, 260.2471 + 0.005995147*d}
	}
	return elements{}
}

// kepler solves Kepler's equation; M and the result are degrees.
func kepler(M, e float64) float64 {
	M = Rev(M)
	E := M + e*deg*sind(M)*(1+e*cosd(M))
	for k := 0; k < 30; k++ {
		dE := (E - e*deg*sind(E) - M) / (1 - e*cosd(E))
		E -= dE
		if math.Abs(dE) < 1e-9 {
			break
		}
	}
	return E
}

// inOrbit is the true anomaly and the distance in the orbit's plane.
func inOrbit(el elements) (v, r float64) {
	E := kepler(el.M, el.e)
	xv := el.a * (cosd(E) - el.e)
	yv := el.a * (math.Sqrt(1-el.e*el.e) * sind(E))
	return atan2d(yv, xv), math.Hypot(xv, yv)
}

// eclipticOf turns an orbit into ecliptic longitude, latitude and distance.
func eclipticOf(el elements) (lon, lat, r float64) {
	v, r := inOrbit(el)
	vw := v + el.w
	x := r * (cosd(el.N)*cosd(vw) - sind(el.N)*sind(vw)*cosd(el.i))
	y := r * (sind(el.N)*cosd(vw) + cosd(el.N)*sind(vw)*cosd(el.i))
	z := r * (sind(vw) * sind(el.i))
	return Rev(atan2d(y, x)), atan2d(z, math.Hypot(x, y)), r
}

// sunEcliptic is the Sun's geocentric ecliptic longitude and distance (AU).
func sunEcliptic(d float64) (lon, r float64) {
	el := orbit("sun", d)
	v, r := inOrbit(el)
	return Rev(v + el.w), r
}

// moonEcliptic is the Moon's geocentric ecliptic longitude, latitude and
// distance (Earth radii) with its main perturbations.
func moonEcliptic(d float64) (lon, lat, r float64) {
	m := orbit("moon", d)
	s := orbit("sun", d)
	lon, lat, r = eclipticOf(m)
	Ms, Mm := Rev(s.M), Rev(m.M)
	Ls := Rev(s.M + s.w)
	Lm := Rev(m.M + m.w + m.N)
	D := Lm - Ls
	F := Lm - m.N
	lon += -1.274*sind(Mm-2*D) + 0.658*sind(2*D) - 0.186*sind(Ms) - 0.059*sind(2*Mm-2*D) - 0.057*sind(Mm-2*D+Ms) +
		0.053*sind(Mm+2*D) + 0.046*sind(2*D-Ms) + 0.041*sind(Mm-Ms) - 0.035*sind(D) - 0.031*sind(Mm+Ms) -
		0.015*sind(2*F-2*D) + 0.011*sind(Mm-4*D)
	lat += -0.173*sind(F-2*D) - 0.055*sind(Mm-F-2*D) - 0.046*sind(Mm+F-2*D) + 0.033*sind(F+2*D) + 0.017*sind(2*Mm+F)
	r += -0.58*cosd(Mm-2*D) - 0.46*cosd(2*D)
	return Rev(lon), lat, r
}

// helioPlanet is a planet's heliocentric ecliptic longitude, latitude and
// distance (AU), with the perturbations of Jupiter, Saturn and Uranus.
func helioPlanet(body string, d float64) (lon, lat, r float64) {
	lon, lat, r = eclipticOf(orbit(body, d))
	Mj := Rev(orbit("jupiter", d).M)
	Msat := Rev(orbit("saturn", d).M)
	Mu := Rev(orbit("uranus", d).M)
	switch body {
	case "jupiter":
		lon += -0.332*sind(2*Mj-5*Msat-67.6) - 0.056*sind(2*Mj-2*Msat+21) + 0.042*sind(3*Mj-5*Msat+21) -
			0.036*sind(Mj-2*Msat) + 0.022*cosd(Mj-Msat) + 0.023*sind(2*Mj-3*Msat+52) - 0.016*sind(Mj-5*Msat-69)
	case "saturn":
		lon += 0.812*sind(2*Mj-5*Msat-67.6) - 0.229*cosd(2*Mj-4*Msat-2) + 0.119*sind(Mj-2*Msat-3) +
			0.046*sind(2*Mj-6*Msat-69) + 0.014*sind(Mj-3*Msat+32)
		lat += -0.020*cosd(2*Mj-4*Msat-2) + 0.018*sind(2*Mj-6*Msat-49)
	case "uranus":
		lon += 0.040*sind(Msat-2*Mu+6) + 0.035*sind(Msat-3*Mu+33) - 0.015*sind(Mj-Mu+20)
	}
	return Rev(lon), lat, r
}

// Position is where a body is, seen from the centre of the Earth (or from a
// place, after Topocentric), on the equator and equinox of date.
type Position struct {
	Body  string  `json:"body"`
	RA    float64 `json:"ra"`    // degrees
	Dec   float64 `json:"dec"`   // degrees
	Lon   float64 `json:"lon"`   // ecliptic longitude, degrees
	Lat   float64 `json:"lat"`   // ecliptic latitude, degrees
	Dist  float64 `json:"dist"`  // AU from the Earth
	Helio float64 `json:"helio"` // AU from the Sun (0 for the Sun)
	Mag   float64 `json:"mag"`   // brightness (smaller = brighter)
	Phase float64 `json:"phase"` // lit fraction 0–1
	Elong float64 `json:"elong"` // degrees from the Sun
	Size  float64 `json:"size"`  // apparent diameter, arcseconds
}

// eclToEqu turns ecliptic coordinates of date into right ascension and declination.
func eclToEqu(lon, lat, ecl float64) (ra, dec float64) {
	x := cosd(lon) * cosd(lat)
	y := sind(lon) * cosd(lat)
	z := sind(lat)
	ye := y*cosd(ecl) - z*sind(ecl)
	ze := y*sind(ecl) + z*cosd(ecl)
	return Rev(atan2d(ye, x)), atan2d(ze, math.Hypot(x, ye))
}

// Body is where a body is at a moment, from the centre of the Earth.
func Body(name string, t time.Time) Position {
	d := dayNumber(t)
	ecl := obliquity(d)
	slon, sr := sunEcliptic(d)
	p := Position{Body: name}
	switch name {
	case "sun":
		p.Lon, p.Lat, p.Dist, p.Mag, p.Phase = slon, 0, sr, -26.74, 1
	case "moon":
		lon, lat, rer := moonEcliptic(d)
		p.Lon, p.Lat, p.Dist = lon, lat, rer*earthRKm/auKm
		p.Elong = acosd(cosd(lon-slon) * cosd(lat))
		// The phase angle (Sun–Moon–Earth), with the Sun's distance taken in.
		fv := atan2d(sr*sind(p.Elong), p.Dist-sr*cosd(p.Elong))
		fv = math.Abs(fv)
		p.Phase = (1 + cosd(fv)) / 2
		p.Helio = sr
		p.Mag = -12.73 + 0.026*fv + 4e-9*math.Pow(fv, 4)
	default:
		lon, lat, r := helioPlanet(name, d)
		xh := r * cosd(lon) * cosd(lat)
		yh := r * sind(lon) * cosd(lat)
		zh := r * sind(lat)
		xg := xh + sr*cosd(slon)
		yg := yh + sr*sind(slon)
		R := math.Sqrt(xg*xg + yg*yg + zh*zh)
		p.Lon, p.Lat, p.Dist, p.Helio = Rev(atan2d(yg, xg)), atan2d(zh, math.Hypot(xg, yg)), R, r
		p.Elong = acosd((sr*sr + R*R - r*r) / (2 * sr * R))
		fv := acosd((r*r + R*R - sr*sr) / (2 * r * R))
		p.Phase = (1 + cosd(fv)) / 2
		p.Mag = planetMag(name, r, R, fv, p.Lon, p.Lat, d)
	}
	p.RA, p.Dec = eclToEqu(p.Lon, p.Lat, ecl)
	if rk := radiusKm[name]; rk > 0 && p.Dist > 0 {
		p.Size = 2 * math.Atan(rk/(p.Dist*auKm)) * deg * 3600
	}
	return p
}

func planetMag(name string, r, R, fv, glon, glat, d float64) float64 {
	l := 5 * math.Log10(r*R)
	switch name {
	case "mercury":
		return -0.36 + l + 0.027*fv + 2.2e-13*math.Pow(fv, 6)
	case "venus":
		return -4.34 + l + 0.013*fv + 4.2e-7*math.Pow(fv, 3)
	case "mars":
		return -1.51 + l + 0.016*fv
	case "jupiter":
		return -9.25 + l + 0.014*fv
	case "saturn":
		// The rings: their tilt towards the Earth adds light.
		ir := 28.06
		Nr := 169.51 + 3.82e-5*d
		B := asind(sind(glat)*cosd(ir) - cosd(glat)*sind(ir)*sind(glon-Nr))
		return -9.0 + l + 0.044*fv - 2.6*sind(math.Abs(B)) + 1.2*sind(B)*sind(B)
	case "uranus":
		return -7.15 + l + 0.001*fv
	case "neptune":
		return -6.90 + l + 0.001*fv
	}
	return 0
}

// Observer is a place on the Earth.
type Observer struct {
	Lat, Lon float64 // degrees, east positive
	Height   float64 // metres (only used for nothing yet; kept for callers)
}

// SiderealTime is the local mean sidereal time in degrees.
func SiderealTime(t time.Time, lon float64) float64 {
	jd := JD(t)
	T := (jd - 2451545.0) / 36525
	g := 280.46061837 + 360.98564736629*(jd-2451545.0) + 0.000387933*T*T - T*T*T/38710000
	return Rev(g + lon)
}

// Topocentric moves a geocentric position to where it is seen from a place
// (the parallax: up to a degree for the Moon, nothing to see for the rest).
func Topocentric(p Position, o Observer, t time.Time) Position {
	if p.Dist <= 0 {
		return p
	}
	par := asind(earthRKm / (p.Dist * auKm))
	if par < 1e-6 {
		return p
	}
	gclat := o.Lat - 0.1924*sind(2*o.Lat)
	rho := 0.99833 + 0.00167*cosd(2*o.Lat)
	ha := SiderealTime(t, o.Lon) - p.RA
	// Rigorous form (Meeus 40): no division by zero at the equator.
	rcos := rho * cosd(gclat)
	rsin := rho * sind(gclat)
	sp := sind(par)
	dA := atan2d(-rcos*sp*sind(ha), cosd(p.Dec)-rcos*sp*cosd(ha))
	q := p
	q.RA = Rev(p.RA + dA)
	q.Dec = atan2d((sind(p.Dec)-rsin*sp)*cosd(dA), cosd(p.Dec)-rcos*sp*cosd(ha))
	// Distance from the place, by the same vector.
	x := p.Dist*cosd(p.Dec)*cosd(ha) - rcos*earthRKm/auKm
	y := p.Dist * cosd(p.Dec) * sind(ha)
	z := p.Dist*sind(p.Dec) - rsin*earthRKm/auKm
	q.Dist = math.Sqrt(x*x + y*y + z*z)
	if rk := radiusKm[p.Body]; rk > 0 {
		q.Size = 2 * math.Atan(rk/(q.Dist*auKm)) * deg * 3600
	}
	return q
}

// Horizontal is where a point of the sky stands above a place: altitude
// above the horizon and azimuth from north through east (degrees, no
// refraction).
func Horizontal(ra, dec float64, o Observer, t time.Time) (alt, az float64) {
	ha := SiderealTime(t, o.Lon) - ra
	sa := sind(o.Lat)*sind(dec) + cosd(o.Lat)*cosd(dec)*cosd(ha)
	alt = asind(sa)
	az = Rev(atan2d(-cosd(dec)*sind(ha), sind(dec)*cosd(o.Lat)-cosd(dec)*sind(o.Lat)*cosd(ha)))
	return
}

// HourAngle is a point's local hour angle in −180–180 degrees (0 = due south
// in the north, its highest).
func HourAngle(ra float64, o Observer, t time.Time) float64 {
	return Wrap180(SiderealTime(t, o.Lon) - ra)
}

// Refraction is how much the air lifts a point at this true altitude
// (degrees; Sæmundsson).
func Refraction(alt float64) float64 {
	if alt < -1.9 {
		return 0
	}
	return 1.02 / math.Tan((alt+10.3/(alt+5.11))*rad) / 60
}

// Seen is a body as seen from a place: topocentric position, altitude and azimuth.
type Seen struct {
	Position
	Alt float64 `json:"alt"`
	Az  float64 `json:"az"`
}

// At is a body seen from a place at a moment.
func At(name string, o Observer, t time.Time) Seen {
	p := Topocentric(Body(name, t), o, t)
	alt, az := Horizontal(p.RA, p.Dec, o, t)
	return Seen{Position: p, Alt: alt, Az: az}
}

// Separation is the angle between two points of the sky (degrees).
func Separation(ra1, dec1, ra2, dec2 float64) float64 {
	// Vincenty's form: exact for tiny and for large angles alike.
	dra := (ra2 - ra1) * rad
	p1, p2 := dec1*rad, dec2*rad
	a := math.Cos(p2) * math.Sin(dra)
	b := math.Cos(p1)*math.Sin(p2) - math.Sin(p1)*math.Cos(p2)*math.Cos(dra)
	c := math.Sin(p1)*math.Sin(p2) + math.Cos(p1)*math.Cos(p2)*math.Cos(dra)
	return math.Atan2(math.Hypot(a, b), c) * deg
}

// ---- precession (catalogues are J2000) ----

// Mat is a 3×3 rotation.
type Mat [3][3]float64

// Apply rotates a vector.
func (m Mat) Apply(v [3]float64) [3]float64 {
	return [3]float64{
		m[0][0]*v[0] + m[0][1]*v[1] + m[0][2]*v[2],
		m[1][0]*v[0] + m[1][1]*v[1] + m[1][2]*v[2],
		m[2][0]*v[0] + m[2][1]*v[1] + m[2][2]*v[2],
	}
}

// T is the transpose (the inverse rotation).
func (m Mat) T() Mat {
	var o Mat
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			o[i][j] = m[j][i]
		}
	}
	return o
}

// Vec is the unit vector of a right ascension and declination.
func Vec(ra, dec float64) [3]float64 {
	return [3]float64{cosd(dec) * cosd(ra), cosd(dec) * sind(ra), sind(dec)}
}

// RADec is the right ascension and declination of a vector.
func RADec(v [3]float64) (ra, dec float64) {
	return Rev(atan2d(v[1], v[0])), atan2d(v[2], math.Hypot(v[0], v[1]))
}

// Precession is the rotation from J2000 to the equator and equinox of date (Meeus 21).
func Precession(t time.Time) Mat {
	T := (JD(t) - 2451545.0) / 36525
	zeta := (2306.2181*T + 0.30188*T*T + 0.017998*T*T*T) / 3600
	z := (2306.2181*T + 1.09468*T*T + 0.018203*T*T*T) / 3600
	th := (2004.3109*T - 0.42665*T*T - 0.041833*T*T*T) / 3600
	cz, sz := cosd(zeta), sind(zeta)
	cZ, sZ := cosd(z), sind(z)
	ct, st := cosd(th), sind(th)
	return Mat{
		{cZ*ct*cz - sZ*sz, -cZ*ct*sz - sZ*cz, -cZ * st},
		{sZ*ct*cz + cZ*sz, -sZ*ct*sz + cZ*cz, -sZ * st},
		{st * cz, -st * sz, ct},
	}
}

// OfDate precesses a J2000 position to the date.
func OfDate(ra, dec float64, t time.Time) (float64, float64) {
	return RADec(Precession(t).Apply(Vec(ra, dec)))
}

// ToJ2000 precesses a position of date back to J2000.
func ToJ2000(ra, dec float64, t time.Time) (float64, float64) {
	return RADec(Precession(t).T().Apply(Vec(ra, dec)))
}
