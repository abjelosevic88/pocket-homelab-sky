package astro

import (
	"math"
	"time"
)

// Altitudes at which a body counts as rising or setting: the top edge of the
// Sun and the Moon on the horizon with the air's lift, a point for the rest.
const (
	HorizonSun    = -0.833
	HorizonMoon   = -0.833 // topocentric: the Moon's half-width plus the air's lift
	HorizonPoint  = -0.5667
	CivilTwi      = -6.0
	NauticalTwi   = -12.0
	AstroTwi      = -18.0
	searchStep    = 10 * time.Minute
	searchEpsilon = time.Second
)

// HorizonOf is the rising altitude for a body.
func HorizonOf(body string) float64 {
	switch body {
	case "sun":
		return HorizonSun
	case "moon":
		return HorizonMoon
	}
	return HorizonPoint
}

// Crossing is a moment a body's altitude passes a level.
type Crossing struct {
	T      time.Time
	Rising bool
}

// AltFunc is an altitude at a moment.
type AltFunc func(t time.Time) float64

// BodyAlt is a body's altitude above a place.
func BodyAlt(body string, o Observer) AltFunc {
	return func(t time.Time) float64 { return At(body, o, t).Alt }
}

// FixedAlt is the altitude of a fixed point of the sky (of date) above a place.
func FixedAlt(ra, dec float64, o Observer) AltFunc {
	return func(t time.Time) float64 { a, _ := Horizontal(ra, dec, o, t); return a }
}

// Crossings finds every time between from and to when f passes level.
func Crossings(f AltFunc, level float64, from, to time.Time) []Crossing {
	var out []Crossing
	t0 := from
	a0 := f(t0) - level
	for t0.Before(to) {
		t1 := t0.Add(searchStep)
		if t1.After(to) {
			t1 = to
		}
		a1 := f(t1) - level
		if (a0 < 0) != (a1 < 0) {
			lo, hi, alo := t0, t1, a0
			for hi.Sub(lo) > searchEpsilon {
				mid := lo.Add(hi.Sub(lo) / 2)
				am := f(mid) - level
				if (am < 0) == (alo < 0) {
					lo, alo = mid, am
				} else {
					hi = mid
				}
			}
			out = append(out, Crossing{T: lo.Add(hi.Sub(lo) / 2).Truncate(time.Second), Rising: a0 < 0})
		}
		t0, a0 = t1, a1
	}
	return out
}

// NextCrossing is the first rise (rising = true) or set after from, within
// the hours given; zero time when there is none.
func NextCrossing(f AltFunc, level float64, from time.Time, hours float64, rising bool) time.Time {
	for _, c := range Crossings(f, level, from, from.Add(time.Duration(hours*float64(time.Hour)))) {
		if c.Rising == rising {
			return c.T
		}
	}
	return time.Time{}
}

// Culmination is the moment a body is highest between from and to (it may
// be an end of the window when it is rising or setting all along).
func Culmination(f AltFunc, from, to time.Time) (time.Time, float64) {
	best, bestA := from, f(from)
	for t := from; !t.After(to); t = t.Add(searchStep) {
		if a := f(t); a > bestA {
			best, bestA = t, a
		}
	}
	if a := f(to); a > bestA {
		best, bestA = to, a
	}
	// Refine around the best sample (golden section).
	lo, hi := best.Add(-searchStep), best.Add(searchStep)
	if lo.Before(from) {
		lo = from
	}
	if hi.After(to) {
		hi = to
	}
	for hi.Sub(lo) > searchEpsilon {
		m1 := lo.Add(hi.Sub(lo) / 3)
		m2 := lo.Add(hi.Sub(lo) * 2 / 3)
		if f(m1) < f(m2) {
			lo = m1
		} else {
			hi = m2
		}
	}
	t := lo.Add(hi.Sub(lo) / 2).Truncate(time.Second)
	if a := f(t); a >= bestA {
		return t, a
	}
	return best, bestA
}

// RiseSet is a body's day above a place: when it rises, is highest and sets.
type RiseSet struct {
	Rise    *time.Time `json:"rise"`
	Transit *time.Time `json:"transit"`
	Set     *time.Time `json:"set"`
	MaxAlt  float64    `json:"max_alt"`
	Always  string     `json:"always,omitempty"` // "up" (never sets) | "down" (never rises) in the window
}

// RiseSetIn is the rise, highest point and set of a body within a window.
func RiseSetIn(body string, o Observer, from, to time.Time) RiseSet {
	f := BodyAlt(body, o)
	return riseSetFn(f, HorizonOf(body), from, to)
}

// RiseSetFixed is the same for a point of the sky (a star, a galaxy).
func RiseSetFixed(ra, dec float64, o Observer, from, to time.Time) RiseSet {
	return riseSetFn(FixedAlt(ra, dec, o), HorizonPoint, from, to)
}

func riseSetFn(f AltFunc, level float64, from, to time.Time) RiseSet {
	var rs RiseSet
	cs := Crossings(f, level, from, to)
	for _, c := range cs {
		c := c
		if c.Rising && rs.Rise == nil {
			rs.Rise = &c.T
		}
		if !c.Rising && rs.Set == nil {
			rs.Set = &c.T
		}
	}
	tt, a := Culmination(f, from, to)
	if tt.After(from) && tt.Before(to) {
		rs.Transit = &tt
	}
	rs.MaxAlt = a
	if len(cs) == 0 {
		if f(from) > level {
			rs.Always = "up"
		} else {
			rs.Always = "down"
		}
	}
	return rs
}

// Night is one night at a place: from the evening of a date to the next morning.
type Night struct {
	Date        string     `json:"date"` // the evening's date, local
	Sunset      *time.Time `json:"sunset"`
	CivilDusk   *time.Time `json:"civil_dusk"`
	NauticDusk  *time.Time `json:"nautical_dusk"`
	AstroDusk   *time.Time `json:"astro_dusk"`
	AstroDawn   *time.Time `json:"astro_dawn"`
	NauticDawn  *time.Time `json:"nautical_dawn"`
	CivilDawn   *time.Time `json:"civil_dawn"`
	Sunrise     *time.Time `json:"sunrise"`
	DarkFrom    time.Time  `json:"dark_from"` // the darkest part: astronomical night, else nautical
	DarkTo      time.Time  `json:"dark_to"`
	DarkKind    string     `json:"dark_kind"` // astro | nautical | civil | none
	DarkHours   float64    `json:"dark_hours"`
	MoonRise    *time.Time `json:"moon_rise"`
	MoonSet     *time.Time `json:"moon_set"`
	MoonPhase   float64    `json:"moon_phase"`    // lit fraction at the middle of the night
	MoonAge     float64    `json:"moon_age"`      // degrees past new (0–360)
	MoonUpDark  float64    `json:"moon_up_dark"`  // share of the dark hours the Moon is up (0–1)
	MoonDarkHrs float64    `json:"moon_dark_hrs"` // dark hours with the Moon down
}

// NightAt works the night out from local noon of the date to noon the next day.
func NightAt(o Observer, date string, loc *time.Location) (Night, error) {
	day, err := time.ParseInLocation("2006-01-02", date, loc)
	if err != nil {
		return Night{}, err
	}
	from := day.Add(12 * time.Hour)
	to := from.Add(24 * time.Hour)
	n := Night{Date: date}
	sun := BodyAlt("sun", o)
	pick := func(level float64) (dusk, dawn *time.Time) {
		for _, c := range Crossings(sun, level, from, to) {
			c := c
			if !c.Rising && dusk == nil {
				dusk = &c.T
			}
			if c.Rising && dusk != nil && dawn == nil {
				dawn = &c.T
			}
			if c.Rising && dusk == nil && dawn == nil {
				// rising before any setting in the window: polar oddities
				dawn = &c.T
			}
		}
		return
	}
	n.Sunset, n.Sunrise = pick(HorizonSun)
	n.CivilDusk, n.CivilDawn = pick(CivilTwi)
	n.NauticDusk, n.NauticDawn = pick(NauticalTwi)
	n.AstroDusk, n.AstroDawn = pick(AstroTwi)
	switch {
	case n.AstroDusk != nil && n.AstroDawn != nil && n.AstroDawn.After(*n.AstroDusk):
		n.DarkFrom, n.DarkTo, n.DarkKind = *n.AstroDusk, *n.AstroDawn, "astro"
	case n.NauticDusk != nil && n.NauticDawn != nil && n.NauticDawn.After(*n.NauticDusk):
		n.DarkFrom, n.DarkTo, n.DarkKind = *n.NauticDusk, *n.NauticDawn, "nautical"
	case n.CivilDusk != nil && n.CivilDawn != nil && n.CivilDawn.After(*n.CivilDusk):
		n.DarkFrom, n.DarkTo, n.DarkKind = *n.CivilDusk, *n.CivilDawn, "civil"
	default:
		// No dusk in the window: the Sun stays down (polar night) or up.
		if sun(from.Add(12*time.Hour)) < AstroTwi {
			n.DarkFrom, n.DarkTo, n.DarkKind = from, to, "astro"
		} else {
			n.DarkFrom, n.DarkTo, n.DarkKind = from.Add(12*time.Hour), from.Add(12*time.Hour), "none"
		}
	}
	n.DarkHours = n.DarkTo.Sub(n.DarkFrom).Hours()
	moon := BodyAlt("moon", o)
	for _, c := range Crossings(moon, HorizonMoon, from, to) {
		c := c
		if c.Rising && n.MoonRise == nil {
			n.MoonRise = &c.T
		}
		if !c.Rising && n.MoonSet == nil {
			n.MoonSet = &c.T
		}
	}
	mid := n.DarkFrom.Add(n.DarkTo.Sub(n.DarkFrom) / 2)
	if n.DarkKind == "none" {
		mid = from.Add(12 * time.Hour)
	}
	m := Body("moon", mid)
	s := Body("sun", mid)
	n.MoonPhase = m.Phase
	n.MoonAge = Rev(m.Lon - s.Lon)
	if n.DarkHours > 0 {
		up, steps := 0, 0
		for t := n.DarkFrom; t.Before(n.DarkTo); t = t.Add(10 * time.Minute) {
			steps++
			if moon(t) > 0 {
				up++
			}
		}
		if steps > 0 {
			n.MoonUpDark = float64(up) / float64(steps)
		}
		n.MoonDarkHrs = math.Round(n.DarkHours*(1-n.MoonUpDark)*10) / 10
	}
	return n, nil
}
