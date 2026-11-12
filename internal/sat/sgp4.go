package sat

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// This file is SGP4, the model the two-line elements are made for, written
// out in Go after David Vallado's reference code ("Revisiting Spacetrack
// Report #3", AIAA 2006-6753, the sgp4unit.cpp that CelesTrak hands out),
// with the WGS-72 constants the elements are fitted with. Only the
// near-Earth branch is here (orbits shorter than 225 minutes): that is every
// satellite the eye can follow. Deep-space orbits (navigation satellites,
// geostationary ones, Molniya orbits) are refused with errDeepSpace.

// WGS-72 constants.
const (
	muKm     = 398600.8
	reKm     = 6378.135
	j2       = 0.001082616
	j3       = -0.00000253881
	j4       = -0.00000165597
	j3oj2    = j3 / j2
	x2o3     = 2.0 / 3.0
	twoPi    = 2 * math.Pi
	minPerDy = 1440.0
)

var (
	xke       = 60.0 / math.Sqrt(reKm*reKm*reKm/muKm)
	vkmpersec = reKm * xke / 60.0
)

var (
	errDeepSpace = errors.New("sat: deep-space orbit (period of 225 minutes or more) is not supported")
	errDecayed   = errors.New("sat: the satellite has come down (its orbit is below the ground)")
	errElements  = errors.New("sat: the elements no longer make sense at this time")
)

// elements is one TLE, parsed and made ready for SGP4.
type elements struct {
	epoch time.Time

	// From the TLE (radians, radians per minute, Earth radii).
	ecco, inclo, nodeo, argpo, mo, bstar, noKozai float64

	// Set up by initialise.
	noUnkozai, ao, con41, x1mth2, x7thm1                     float64
	cc1, cc4, cc5, d2, d3, d4, delmo, eta                    float64
	argpdot, omgcof, sinmao, t2cof, t3cof, t4cof, t5cof      float64
	mdot, nodedot, xlcof, xmcof, nodecf, aycof, sinio, cosio float64
	isimp                                                    bool
}

// parseElements reads the two data lines of a TLE.
func parseElements(line1, line2 string) (*elements, error) {
	line1 = strings.TrimRight(line1, " \r\n")
	line2 = strings.TrimRight(line2, " \r\n")
	if len(line1) < 64 || len(line2) < 63 || line1[0] != '1' || line2[0] != '2' {
		return nil, fmt.Errorf("sat: not a two-line element set")
	}
	f := func(s string) (float64, error) { return strconv.ParseFloat(strings.TrimSpace(s), 64) }
	var err error
	var e elements
	num := func(dst *float64, s string) {
		if err != nil {
			return
		}
		*dst, err = f(s)
	}
	// Epoch: two-digit year and day of the year with its fraction.
	var yy, day float64
	num(&yy, line1[18:20])
	num(&day, line1[20:32])
	e.bstar, err = expField(line1[53:61], err)
	var inc, node, ecc, argp, ma, mm float64
	num(&inc, line2[8:16])
	num(&node, line2[17:25])
	num(&ecc, "0."+strings.TrimSpace(line2[26:33]))
	num(&argp, line2[34:42])
	num(&ma, line2[43:51])
	end := 63
	if len(line2) < end {
		end = len(line2)
	}
	num(&mm, line2[52:end])
	if err != nil {
		return nil, fmt.Errorf("sat: bad TLE: %w", err)
	}
	year := 1900 + int(yy)
	if yy < 57 {
		year += 100
	}
	start := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
	e.epoch = start.Add(time.Duration((day - 1) * 86400 * float64(time.Second)))
	deg := math.Pi / 180
	e.inclo, e.nodeo, e.argpo, e.mo = inc*deg, node*deg, argp*deg, ma*deg
	e.ecco = ecc
	e.noKozai = mm * twoPi / minPerDy
	if e.noKozai <= 0 {
		return nil, fmt.Errorf("sat: bad TLE: mean motion %v", mm)
	}
	if err := e.initialise(); err != nil {
		return nil, err
	}
	return &e, nil
}

// expField reads a TLE number with an assumed decimal point and a power of
// ten, like " 28098-4" (0.28098e-4).
func expField(s string, prev error) (float64, error) {
	if prev != nil {
		return 0, prev
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	sign := 1.0
	switch s[0] {
	case '-':
		sign, s = -1, s[1:]
	case '+':
		s = s[1:]
	}
	i := strings.LastIndexAny(s, "+-")
	if i <= 0 {
		v, err := strconv.ParseFloat("0."+s, 64)
		return sign * v, err
	}
	m, err := strconv.ParseFloat("0."+strings.TrimSpace(s[:i]), 64)
	if err != nil {
		return 0, err
	}
	x, err := strconv.Atoi(s[i:])
	if err != nil {
		return 0, err
	}
	return sign * m * math.Pow(10, float64(x)), nil
}

// initialise is Vallado's initl and sgp4init for the near-Earth branch.
func (e *elements) initialise() error {
	ss := 78.0/reKm + 1.0
	qzms2t := math.Pow((120.0-78.0)/reKm, 4)

	// initl: recover the original mean motion and semi-major axis.
	eccsq := e.ecco * e.ecco
	omeosq := 1 - eccsq
	rteosq := math.Sqrt(omeosq)
	cosio := math.Cos(e.inclo)
	cosio2 := cosio * cosio
	ak := math.Pow(xke/e.noKozai, x2o3)
	d1 := 0.75 * j2 * (3*cosio2 - 1) / (rteosq * omeosq)
	del := d1 / (ak * ak)
	adel := ak * (1 - del*del - del*(1.0/3.0+134*del*del/81))
	del = d1 / (adel * adel)
	e.noUnkozai = e.noKozai / (1 + del)
	e.ao = math.Pow(xke/e.noUnkozai, x2o3)
	e.sinio = math.Sin(e.inclo)
	e.cosio = cosio
	po := e.ao * omeosq
	con42 := 1 - 5*cosio2
	e.con41 = -con42 - cosio2 - cosio2
	posq := po * po
	rp := e.ao * (1 - e.ecco)

	if twoPi/e.noUnkozai >= 225 {
		return errDeepSpace
	}
	if e.ecco >= 1 || e.ecco < 0 {
		return errElements
	}

	// sgp4init.
	e.isimp = rp < 220.0/reKm+1.0
	sfour := ss
	qzms24 := qzms2t
	perige := (rp - 1) * reKm
	if perige < 156 {
		sfour = perige - 78
		if perige < 98 {
			sfour = 20
		}
		qzms24 = math.Pow((120-sfour)/reKm, 4)
		sfour = sfour/reKm + 1
	}
	pinvsq := 1 / posq
	tsi := 1 / (e.ao - sfour)
	e.eta = e.ao * e.ecco * tsi
	etasq := e.eta * e.eta
	eeta := e.ecco * e.eta
	psisq := math.Abs(1 - etasq)
	coef := qzms24 * math.Pow(tsi, 4)
	coef1 := coef / math.Pow(psisq, 3.5)
	cc2 := coef1 * e.noUnkozai * (e.ao*(1+1.5*etasq+eeta*(4+etasq)) +
		0.375*j2*tsi/psisq*e.con41*(8+3*etasq*(8+etasq)))
	e.cc1 = e.bstar * cc2
	cc3 := 0.0
	if e.ecco > 1e-4 {
		cc3 = -2 * coef * tsi * j3oj2 * e.noUnkozai * e.sinio / e.ecco
	}
	e.x1mth2 = 1 - cosio2
	e.cc4 = 2 * e.noUnkozai * coef1 * e.ao * omeosq *
		(e.eta*(2+0.5*etasq) + e.ecco*(0.5+2*etasq) -
			j2*tsi/(e.ao*psisq)*(-3*e.con41*(1-2*eeta+etasq*(1.5-0.5*eeta))+
				0.75*e.x1mth2*(2*etasq-eeta*(1+etasq))*math.Cos(2*e.argpo)))
	e.cc5 = 2 * coef1 * e.ao * omeosq * (1 + 2.75*(etasq+eeta) + eeta*etasq)
	cosio4 := cosio2 * cosio2
	temp1 := 1.5 * j2 * pinvsq * e.noUnkozai
	temp2 := 0.5 * temp1 * j2 * pinvsq
	temp3 := -0.46875 * j4 * pinvsq * pinvsq * e.noUnkozai
	e.mdot = e.noUnkozai + 0.5*temp1*rteosq*e.con41 + 0.0625*temp2*rteosq*(13-78*cosio2+137*cosio4)
	e.argpdot = -0.5*temp1*con42 + 0.0625*temp2*(7-114*cosio2+395*cosio4) +
		temp3*(3-36*cosio2+49*cosio4)
	xhdot1 := -temp1 * cosio
	e.nodedot = xhdot1 + (0.5*temp2*(4-19*cosio2)+2*temp3*(3-7*cosio2))*cosio
	e.omgcof = e.bstar * cc3 * math.Cos(e.argpo)
	e.xmcof = 0
	if e.ecco > 1e-4 {
		e.xmcof = -x2o3 * coef * e.bstar / eeta
	}
	e.nodecf = 3.5 * omeosq * xhdot1 * e.cc1
	e.t2cof = 1.5 * e.cc1
	if math.Abs(cosio+1) > 1.5e-12 {
		e.xlcof = -0.25 * j3oj2 * e.sinio * (3 + 5*cosio) / (1 + cosio)
	} else {
		e.xlcof = -0.25 * j3oj2 * e.sinio * (3 + 5*cosio) / 1.5e-12
	}
	e.aycof = -0.5 * j3oj2 * e.sinio
	e.delmo = math.Pow(1+e.eta*math.Cos(e.mo), 3)
	e.sinmao = math.Sin(e.mo)
	e.x7thm1 = 7*cosio2 - 1

	if !e.isimp {
		cc1sq := e.cc1 * e.cc1
		e.d2 = 4 * e.ao * tsi * cc1sq
		temp := e.d2 * tsi * e.cc1 / 3
		e.d3 = (17*e.ao + sfour) * temp
		e.d4 = 0.5 * temp * e.ao * tsi * (221*e.ao + 31*sfour) * e.cc1
		e.t3cof = e.d2 + 2*cc1sq
		e.t4cof = 0.25 * (3*e.d3 + e.cc1*(12*e.d2+10*cc1sq))
		e.t5cof = 0.2 * (3*e.d4 + 12*e.cc1*e.d3 + 6*e.d2*e.d2 + 15*cc1sq*(2*e.d2+cc1sq))
	}
	return nil
}

// propagate is SGP4: position (km) and velocity (km/s) in the TEME frame
// (true equator, mean equinox of date) tsince minutes after the epoch.
func (e *elements) propagate(tsince float64) (r, v [3]float64, err error) {
	t := tsince
	xmdf := e.mo + e.mdot*t
	argpdf := e.argpo + e.argpdot*t
	nodedf := e.nodeo + e.nodedot*t
	argpm := argpdf
	mm := xmdf
	t2 := t * t
	nodem := nodedf + e.nodecf*t2
	tempa := 1 - e.cc1*t
	tempe := e.bstar * e.cc4 * t
	templ := e.t2cof * t2

	if !e.isimp {
		delomg := e.omgcof * t
		delmtemp := 1 + e.eta*math.Cos(xmdf)
		delm := e.xmcof * (delmtemp*delmtemp*delmtemp - e.delmo)
		temp := delomg + delm
		mm = xmdf + temp
		argpm = argpdf - temp
		t3 := t2 * t
		t4 := t3 * t
		tempa = tempa - e.d2*t2 - e.d3*t3 - e.d4*t4
		tempe = tempe + e.bstar*e.cc5*(math.Sin(mm)-e.sinmao)
		templ = templ + e.t3cof*t3 + t4*(e.t4cof+t*e.t5cof)
	}

	nm := e.noUnkozai
	em := e.ecco
	if nm <= 0 {
		return r, v, errElements
	}
	am := math.Pow(xke/nm, x2o3) * tempa * tempa
	nm = xke / math.Pow(am, 1.5)
	em -= tempe
	if em >= 1 || em < -0.001 {
		return r, v, errElements
	}
	if em < 1e-6 {
		em = 1e-6
	}
	mm += e.noUnkozai * templ
	xlm := mm + argpm + nodem
	nodem = math.Mod(nodem, twoPi)
	argpm = math.Mod(argpm, twoPi)
	xlm = math.Mod(xlm, twoPi)
	mm = math.Mod(xlm-argpm-nodem, twoPi)

	// Long-period periodics.
	ep, argpp, nodep, mp := em, argpm, nodem, mm
	sinip, cosip := e.sinio, e.cosio
	axnl := ep * math.Cos(argpp)
	temp := 1 / (am * (1 - ep*ep))
	aynl := ep*math.Sin(argpp) + temp*e.aycof
	xl := mp + argpp + nodep + temp*e.xlcof*axnl

	// Kepler's equation.
	u := math.Mod(xl-nodep, twoPi)
	eo1 := u
	tem5 := 9999.9
	var sineo1, coseo1 float64
	for ktr := 1; math.Abs(tem5) >= 1e-12 && ktr <= 10; ktr++ {
		sineo1, coseo1 = math.Sincos(eo1)
		tem5 = 1 - coseo1*axnl - sineo1*aynl
		tem5 = (u - aynl*coseo1 + axnl*sineo1 - eo1) / tem5
		if math.Abs(tem5) >= 0.95 {
			if tem5 > 0 {
				tem5 = 0.95
			} else {
				tem5 = -0.95
			}
		}
		eo1 += tem5
	}

	// Short-period periodics.
	ecose := axnl*coseo1 + aynl*sineo1
	esine := axnl*sineo1 - aynl*coseo1
	el2 := axnl*axnl + aynl*aynl
	pl := am * (1 - el2)
	if pl < 0 {
		return r, v, errElements
	}
	rl := am * (1 - ecose)
	rdotl := math.Sqrt(am) * esine / rl
	rvdotl := math.Sqrt(pl) / rl
	betal := math.Sqrt(1 - el2)
	temp = esine / (1 + betal)
	sinu := am / rl * (sineo1 - aynl - axnl*temp)
	cosu := am / rl * (coseo1 - axnl + aynl*temp)
	su := math.Atan2(sinu, cosu)
	sin2u := (cosu + cosu) * sinu
	cos2u := 1 - 2*sinu*sinu
	temp = 1 / pl
	temp1 := 0.5 * j2 * temp
	temp2 := temp1 * temp

	mrt := rl*(1-1.5*temp2*betal*e.con41) + 0.5*temp1*e.x1mth2*cos2u
	su -= 0.25 * temp2 * e.x7thm1 * sin2u
	xnode := nodep + 1.5*temp2*cosip*sin2u
	xinc := e.inclo + 1.5*temp2*cosip*sinip*cos2u
	mvt := rdotl - nm*temp1*e.x1mth2*sin2u/xke
	rvdot := rvdotl + nm*temp1*(e.x1mth2*cos2u+1.5*e.con41)/xke

	sinsu, cossu := math.Sincos(su)
	snod, cnod := math.Sincos(xnode)
	sini, cosi := math.Sincos(xinc)
	xmx := -snod * cosi
	xmy := cnod * cosi
	ux := xmx*sinsu + cnod*cossu
	uy := xmy*sinsu + snod*cossu
	uz := sini * sinsu
	vx := xmx*cossu - cnod*sinsu
	vy := xmy*cossu - snod*sinsu
	vz := sini * cossu

	r = [3]float64{mrt * ux * reKm, mrt * uy * reKm, mrt * uz * reKm}
	v = [3]float64{
		(mvt*ux + rvdot*vx) * vkmpersec,
		(mvt*uy + rvdot*vy) * vkmpersec,
		(mvt*uz + rvdot*vz) * vkmpersec,
	}
	if mrt < 1 {
		return r, v, errDecayed
	}
	return r, v, nil
}

// at is the position and velocity at a moment.
func (e *elements) at(t time.Time) (r, v [3]float64, err error) {
	return e.propagate(float64(t.Sub(e.epoch)) / float64(time.Minute))
}
