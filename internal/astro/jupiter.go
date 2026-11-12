package astro

import (
	"math"
	"time"
)

// Jupiter's four big moons, its Great Red Spot and Saturn's rings, after
// Jean Meeus, "Astronomical Algorithms" (2nd ed., 1998), chapters 43–45.

// jupiterBasics are the shared quantities of Meeus' low-accuracy method
// (ch. 43 and 44) for d days from J2000: Jupiter's equation of centre B,
// the phase angle ψ, Jupiter's distances from the Sun (r) and Earth (Δ),
// and the light-time-corrected day number dl = d − Δ/173.
func jupiterBasics(t time.Time) (d, B, psi, r, delta, dl, V float64) {
	// Meeus counts in Dynamical Time; a minute of difference is half a
	// degree of Jupiter's turn, so it is added here.
	d = JD(t) + deltaT(t)/86400 - 2451545.0
	V = 172.74 + 0.00111588*d
	M := 357.529 + 0.9856003*d
	N := 20.020 + 0.0830853*d + 0.329*sind(V)
	J := 66.115 + 0.9025179*d - 0.329*sind(V)
	A := 1.915*sind(M) + 0.020*sind(2*M)
	B = 5.555*sind(N) + 0.168*sind(2*N)
	K := J + A - B
	R := 1.00014 - 0.01671*cosd(M) - 0.00014*cosd(2*M)
	r = 5.20872 - 0.25208*cosd(N) - 0.00611*cosd(2*N)
	delta = math.Sqrt(r*r + R*R - 2*r*R*cosd(K))
	psi = asind(R / delta * sind(K))
	dl = d - delta/173
	return
}

// deltaT is TT − UT in seconds, by Espenak and Meeus' polynomials (NASA
// eclipse web site): a minute or so now; good to a few seconds 1961–2050,
// rougher outside.
func deltaT(t time.Time) float64 {
	y := float64(t.Year()) + (float64(t.YearDay())-0.5)/365.25
	switch {
	case y < 1986:
		u := y - 1975
		return 45.45 + 1.067*u - u*u/260 - u*u*u/718
	case y < 2005:
		u := y - 2000
		return 63.86 + 0.3345*u - 0.060374*u*u + 0.0017275*u*u*u + 0.000651814*u*u*u*u + 0.00002373599*u*u*u*u*u
	case y < 2050:
		u := y - 2000
		return 62.92 + 0.32217*u + 0.005589*u*u
	case y < 2150:
		u := (y - 1820) / 100
		return -20 + 32*u*u - 0.5628*(2150-y)
	}
	u := (y - 1820) / 100
	return -20 + 32*u*u
}

// jupiterTilts are the planetocentric declinations of the Sun (DS) and of
// the Earth (DE): how far north of Jupiter's equator each one stands, so
// how much the moons' orbits are seen opened out (Meeus 43, low accuracy).
func jupiterTilts(d, B, psi, r, delta, V float64) (DS, DE float64) {
	lambda := 34.35 + 0.083091*d + 0.329*sind(V) + B
	DS = 3.12 * sind(lambda+42.8)
	DE = DS - 2.22*sind(psi)*cosd(lambda+22) - 1.30*(r-delta)/delta*sind(lambda-100.5)
	return
}

// JupiterCMII is Jupiter's System II central meridian longitude (degrees):
// which of Jupiter's longitudes, in the system used for its belts and the
// Great Red Spot, faces the Earth right now. Meeus' low-accuracy method
// (ch. 43), good to about a degree, which is two minutes of Jupiter's turn.
// It is the geometric central meridian (the line through the middle of the
// disc), not corrected for the half-degree that the phase shifts the middle
// of the lit part.
func JupiterCMII(t time.Time) float64 {
	_, B, psi, _, _, dl, _ := jupiterBasics(t)
	return Rev(187.23 + 870.1869088*dl + psi - B)
}

// RedSpotTransits are the moments between from and to when the given
// System II longitude (that of the Great Red Spot, which drifts slowly and
// is published by amateur Jupiter observers, e.g. the ALPO and the BAA)
// crosses the middle of Jupiter's disc, best for seeing it. Jupiter turns
// in about 9 h 56 min, so there are two or three a day; whether Jupiter is
// up at the place is for the caller to check.
func RedSpotTransits(from, to time.Time, spotLonII float64) []time.Time {
	return angleCrossings(JupiterCMII, Rev(spotLonII), from, to, time.Hour)
}

// JovianMoon is one of Jupiter's four big moons as seen from the Earth.
//
// X and Y are in Jupiter's equatorial radii from the planet's centre,
// Meeus' convention: X positive to the WEST in the sky (the side where the
// planet sets, right in an eye's or binoculars' upright view from the
// northern hemisphere), Y positive to the NORTH. Behind: hidden behind
// Jupiter's disc. Transit: crossing in front of it. InShadow: in Jupiter's
// shadow (eclipsed), so not seen even if beside the disc.
type JovianMoon struct {
	Name     string  `json:"name"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Behind   bool    `json:"behind"`
	InShadow bool    `json:"in_shadow"`
	Transit  bool    `json:"transit"`
}

// jupiterPolar is Jupiter's polar radius in equatorial radii (it is
// flattened by its fast spin).
const jupiterPolar = 0.935

// onDisc is whether a point (equatorial radii) falls on Jupiter's flattened disc.
func onDisc(x, y float64) bool {
	return x*x+(y/jupiterPolar)*(y/jupiterPolar) < 1
}

// GalileanMoons are Io, Europa, Ganymede and Callisto at a moment, by
// Meeus' low-accuracy method (ch. 44: good to a few hundredths of Jupiter's
// radius for Io, more for the outer moons; a fine match for any small
// telescope's view). The angle u of each moon counts from the line
// Jupiter–Earth on the near side, so cos u > 0 means the moon is between
// Jupiter and us. For the shadow the same moons are looked at from the Sun
// (the phase angle ψ taken back out, the Sun's tilt DS instead of the
// Earth's); the shadow is taken as a cylinder the size of the disc.
func GalileanMoons(t time.Time) []JovianMoon {
	d, B, psi, r, delta, dl, V := jupiterBasics(t)
	DS, DE := jupiterTilts(d, B, psi, r, delta, V)
	u1 := 163.8069 + 203.4058646*dl + psi - B
	u2 := 358.4140 + 101.2916335*dl + psi - B
	u3 := 5.7176 + 50.2345180*dl + psi - B
	u4 := 224.8092 + 21.4879800*dl + psi - B
	G := 331.18 + 50.310482*dl
	H := 87.45 + 21.569231*dl
	// The moons' pulls on one another.
	c1, c2 := 2*(u1-u2), 2*(u2-u3)
	us := [4]float64{u1 + 0.473*sind(c1), u2 + 1.065*sind(c2), u3 + 0.165*sind(G), u4 + 0.843*sind(H)}
	rs := [4]float64{5.9057 - 0.0244*cosd(c1), 9.3966 - 0.0882*cosd(c2), 14.9883 - 0.0216*cosd(G), 26.3627 - 0.1939*cosd(H)}
	names := [4]string{"Io", "Europa", "Ganymede", "Callisto"}
	out := make([]JovianMoon, 4)
	for k := range out {
		u, rr := us[k], rs[k]
		m := JovianMoon{Name: names[k], X: rr * sind(u), Y: -rr * cosd(u) * sind(DE)}
		on := onDisc(m.X, m.Y)
		m.Transit = on && cosd(u) > 0
		m.Behind = on && cosd(u) < 0
		usun := u - psi
		xs, ys := rr*sind(usun), -rr*cosd(usun)*sind(DS)
		m.InShadow = cosd(usun) < 0 && onDisc(xs, ys)
		out[k] = m
	}
	return out
}

// SaturnRings are how Saturn's rings are turned towards us (Meeus 45):
// B is the tilt of the ring plane to the line of sight in degrees (positive
// = we see the north face; 0 = edge-on and the rings all but vanish, about
// 27 at most = wide open), P the position angle of Saturn's north pole
// (and the rings' minor axis), degrees from north towards east.
//
// Saturn's place is the engine's own (Schlyter, ecliptic and equinox of
// date, good to an arcminute or two); nutation and aberration are left
// out, which moves P by less than a hundredth of a degree.
func SaturnRings(t time.Time) (B, P float64) {
	T := (JD(t) - 2451545.0) / 36525
	// The ring plane on the ecliptic and equinox of date.
	i := 28.075216 - 0.012998*T + 0.000004*T*T
	node := 169.508470 + 1.394681*T + 0.000412*T*T
	s := Body("saturn", t)
	B = asind(sind(i)*cosd(s.Lat)*sind(s.Lon-node) - cosd(i)*sind(s.Lat))
	// The ring plane's north pole, then the angle at Saturn between it and
	// the north celestial pole.
	ecl := obliquity(dayNumber(t))
	a0, d0 := eclToEqu(node-90, 90-i, ecl)
	P = atan2d(cosd(d0)*sind(a0-s.RA), sind(d0)*cosd(s.Dec)-cosd(d0)*sind(s.Dec)*cosd(a0-s.RA))
	return B, P
}
