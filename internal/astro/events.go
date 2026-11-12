package astro

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// Event is something happening in the sky.
type Event struct {
	Kind   string    `json:"kind"` // moon_phase | season | conjunction | opposition | elongation | lunar_eclipse | solar_eclipse | meteor_shower | supermoon
	T      time.Time `json:"t"`
	Title  string    `json:"title"`
	Detail string    `json:"detail"`
	Bodies []string  `json:"bodies,omitempty"`
	Value  float64   `json:"value,omitempty"` // separation, elongation, magnitude of an eclipse, ZHR…
	Sub    string    `json:"sub,omitempty"`   // new|first|full|last, march|june|…, total|partial|…, east|west
	Key    string    `json:"key"`             // stable id: moon_phase:full:2026-11-24
	// Filled for a place: whether it can be seen and from when to when.
	Visible *bool      `json:"visible,omitempty"`
	Start   *time.Time `json:"start,omitempty"`
	End     *time.Time `json:"end,omitempty"`
	Alt     *float64   `json:"alt,omitempty"`
}

func key(kind, sub string, t time.Time) string {
	if sub != "" {
		return kind + ":" + sub + ":" + t.UTC().Format("2006-01-02")
	}
	return kind + ":" + t.UTC().Format("2006-01-02")
}

// findRoot finds where g passes zero going up between t0 and t1 (bisection).
func findRoot(g func(time.Time) float64, t0, t1 time.Time) time.Time {
	a0 := g(t0)
	for t1.Sub(t0) > searchEpsilon {
		m := t0.Add(t1.Sub(t0) / 2)
		am := g(m)
		if (am < 0) == (a0 < 0) {
			t0, a0 = m, am
		} else {
			t1 = m
		}
	}
	return t0.Add(t1.Sub(t0) / 2).Truncate(time.Second)
}

// angleCrossings finds when an angle that grows over time (in degrees,
// wrapped) passes a target: g(t) = Wrap180(angle(t) − target) goes from
// negative to positive.
func angleCrossings(angle func(time.Time) float64, target float64, from, to time.Time, step time.Duration) []time.Time {
	var out []time.Time
	g := func(t time.Time) float64 { return Wrap180(angle(t) - target) }
	t0, a0 := from, g(from)
	for t0.Before(to) {
		t1 := t0.Add(step)
		a1 := g(t1)
		if a0 < 0 && a1 >= 0 && a1-a0 < 90 {
			if r := findRoot(g, t0, t1); !r.Before(from) && r.Before(to) {
				out = append(out, r)
			}
		}
		t0, a0 = t1, a1
	}
	return out
}

// moonElong is the Moon's longitude past the Sun's (0 new, 180 full).
func moonElong(t time.Time) float64 {
	return Rev(Body("moon", t).Lon - Body("sun", t).Lon)
}

// MoonPhases are the new, first quarter, full and last quarter Moons between two moments.
func MoonPhases(from, to time.Time) []Event {
	var out []Event
	names := []struct {
		sub, title string
		at         float64
	}{{"new", "New Moon", 0}, {"first", "First quarter", 90}, {"full", "Full Moon", 180}, {"last", "Last quarter", 270}}
	for _, ph := range names {
		for _, t := range angleCrossings(moonElong, ph.at, from, to, 24*time.Hour) {
			e := Event{Kind: "moon_phase", Sub: ph.sub, T: t, Title: ph.title, Bodies: []string{"moon"}, Key: key("moon_phase", ph.sub, t)}
			switch ph.sub {
			case "new":
				e.Detail = "The Moon is between the Earth and the Sun and can't be seen: the darkest nights of the month are around now."
			case "first":
				e.Detail = "Half the Moon is lit, in the evening sky. Craters along the edge of the light stand out well in binoculars."
			case "full":
				dist := Body("moon", t).Dist * auKm
				e.Value = math.Round(dist)
				e.Detail = "The whole face is lit and the Moon is up all night, so faint stars and the Milky Way are washed out."
				if dist < 361000 {
					e.Sub, e.Title, e.Key = "full", "Full Moon (a supermoon)", key("moon_phase", "full", t)
					e.Detail = fmt.Sprintf("The Moon is close to the Earth (%s km), so it looks a little bigger and brighter than usual. ", groupKm(dist)) + e.Detail
				}
			case "last":
				e.Detail = "Half the Moon is lit, rising around midnight and high in the morning sky."
			}
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].T.Before(out[j].T) })
	return out
}

func groupKm(v float64) string {
	n := int64(math.Round(v))
	s := fmt.Sprintf("%d", n)
	out := ""
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out += ","
		}
		out += string(c)
	}
	return out
}

// Seasons are the equinoxes and solstices between two moments.
func Seasons(from, to time.Time) []Event {
	var out []Event
	type sd struct {
		at              float64
		sub, title, txt string
	}
	for _, s := range []sd{
		{0, "march", "March equinox", "Day and night are about equal everywhere. Spring begins in the north, autumn in the south."},
		{90, "june", "June solstice", "The longest day in the north and the shortest in the south; the shortest nights for stargazing up north."},
		{180, "september", "September equinox", "Day and night are about equal everywhere. Autumn begins in the north, spring in the south."},
		{270, "december", "December solstice", "The longest night in the north: the most dark hours of the year for the sky."},
	} {
		for _, t := range angleCrossings(func(t time.Time) float64 { return Body("sun", t).Lon }, s.at, from, to, 24*time.Hour) {
			out = append(out, Event{Kind: "season", Sub: s.sub, T: t, Title: s.title, Detail: s.txt, Bodies: []string{"sun"}, Key: key("season", s.sub, t)})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].T.Before(out[j].T) })
	return out
}

// SunAtLongitude is when the Sun reaches an ecliptic longitude of date
// near a moment (within ±200 days).
func SunAtLongitude(lon float64, near time.Time) (time.Time, bool) {
	ts := angleCrossings(func(t time.Time) float64 { return Body("sun", t).Lon }, lon, near.Add(-200*24*time.Hour), near.Add(200*24*time.Hour), 24*time.Hour)
	if len(ts) == 0 {
		return time.Time{}, false
	}
	best := ts[0]
	for _, t := range ts {
		if math.Abs(t.Sub(near).Hours()) < math.Abs(best.Sub(near).Hours()) {
			best = t
		}
	}
	return best, true
}

// minima finds the local minima of f sampled every step, refined, that are below limit.
func minima(f func(time.Time) float64, from, to time.Time, step time.Duration, limit float64) []time.Time {
	var out []time.Time
	prev2, prev1 := f(from), f(from.Add(step))
	for t := from.Add(2 * step); !t.After(to); t = t.Add(step) {
		cur := f(t)
		if prev1 <= prev2 && prev1 < cur && prev1 < limit*1.5 {
			lo, hi := t.Add(-2*step), t
			for hi.Sub(lo) > time.Minute {
				m1 := lo.Add(hi.Sub(lo) / 3)
				m2 := lo.Add(hi.Sub(lo) * 2 / 3)
				if f(m1) < f(m2) {
					hi = m2
				} else {
					lo = m1
				}
			}
			m := lo.Add(hi.Sub(lo) / 2).Truncate(time.Minute)
			if f(m) < limit {
				out = append(out, m)
			}
		}
		prev2, prev1 = prev1, cur
	}
	return out
}

func sepBodies(a, b string) func(time.Time) float64 {
	return func(t time.Time) float64 {
		pa, pb := Body(a, t), Body(b, t)
		return Separation(pa.RA, pa.Dec, pb.RA, pb.Dec)
	}
}

// Conjunctions are close meetings: the Moon passing a bright planet (within
// 4°) and two planets passing each other (within 2°), not too near the Sun.
func Conjunctions(from, to time.Time) []Event {
	var out []Event
	bright := []string{"mercury", "venus", "mars", "jupiter", "saturn"}
	for _, p := range bright {
		for _, t := range minima(sepBodies("moon", p), from, to, 6*time.Hour, 4) {
			pp := Body(p, t)
			if pp.Elong < 15 {
				continue
			}
			sep := sepBodies("moon", p)(t)
			out = append(out, Event{Kind: "conjunction", T: t, Title: "The Moon and " + Names[p], Bodies: []string{"moon", p}, Value: round1(sep),
				Detail: fmt.Sprintf("The Moon passes %s from %s. A lovely pair to look for, no telescope needed.", degWords(sep), Names[p]), Key: key("conjunction", "moon-"+p, t)})
		}
	}
	all := append([]string{}, bright...)
	all = append(all, "uranus", "neptune")
	for i := 0; i < len(all); i++ {
		for j := i + 1; j < len(all); j++ {
			a, b := all[i], all[j]
			if (a == "uranus" || a == "neptune") && (b == "uranus" || b == "neptune") {
				continue
			}
			for _, t := range minima(sepBodies(a, b), from, to, 24*time.Hour, 2) {
				pa, pb := Body(a, t), Body(b, t)
				if pa.Elong < 15 || pb.Elong < 15 {
					continue
				}
				sep := sepBodies(a, b)(t)
				out = append(out, Event{Kind: "conjunction", T: t, Title: Names[a] + " and " + Names[b], Bodies: []string{a, b}, Value: round1(sep),
					Detail: fmt.Sprintf("%s and %s are %s apart: in the same view of binoculars when they're this close.", Names[a], Names[b], degWords(sep)), Key: key("conjunction", a+"-"+b, t)})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].T.Before(out[j].T) })
	return out
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }

func degWords(v float64) string {
	if v < 1 {
		return fmt.Sprintf("%.1f°", v)
	}
	return fmt.Sprintf("%.1f°", v)
}

// Oppositions are when an outer planet is opposite the Sun: closest, brightest and up all night.
func Oppositions(from, to time.Time) []Event {
	var out []Event
	for _, p := range []string{"mars", "jupiter", "saturn", "uranus", "neptune"} {
		// The anti-Sun point moves east while the planet moves west (it is
		// going backwards then), so their difference grows through zero.
		g := func(t time.Time) float64 { return Rev(Body("sun", t).Lon + 180 - Body(p, t).Lon) }
		for _, t := range angleCrossings(g, 0, from, to, 24*time.Hour) {
			b := Body(p, t)
			out = append(out, Event{Kind: "opposition", T: t, Title: Names[p] + " at opposition", Bodies: []string{p}, Value: round1(b.Mag),
				Detail: fmt.Sprintf("%s is opposite the Sun: up all night, closest to us and at its brightest (magnitude %.1f). The best time of the year to look at it.", Names[p], b.Mag),
				Key:    key("opposition", p, t)})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].T.Before(out[j].T) })
	return out
}

// Elongations are when Mercury or Venus stand furthest from the Sun, the
// best evenings (east) or mornings (west) to see them.
func Elongations(from, to time.Time) []Event {
	var out []Event
	for _, p := range []string{"mercury", "venus"} {
		neg := func(t time.Time) float64 { return -Body(p, t).Elong }
		for _, t := range minima(neg, from, to, 24*time.Hour, 0) {
			b, s := Body(p, t), Body("sun", t)
			east := Wrap180(b.Lon-s.Lon) > 0
			sub, when := "west", "in the morning before sunrise, low in the east"
			if east {
				sub, when = "east", "in the evening after sunset, low in the west"
			}
			out = append(out, Event{Kind: "elongation", Sub: sub, T: t, Title: Names[p] + " furthest from the Sun", Bodies: []string{p}, Value: round1(b.Elong),
				Detail: fmt.Sprintf("%s is %.0f° from the Sun: the best time to see it %s.", Names[p], b.Elong, when), Key: key("elongation", p, t)})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].T.Before(out[j].T) })
	return out
}

// ---- eclipses ----

// shadow sizes for a moment, in degrees (Meeus 54, enlarged by 2% as usual).
func shadow(t time.Time) (moonPar, moonR, sunR, umbra, penumbra float64) {
	m, s := Body("moon", t), Body("sun", t)
	moonPar = asind(earthRKm / (m.Dist * auKm))
	sunPar := 8.794 / 3600 / s.Dist
	sunR = 959.63 / 3600 / s.Dist
	moonR = 0.272481 * moonPar
	umbra = 1.02 * (0.99834*moonPar - sunR + sunPar)
	penumbra = 1.02 * (0.99834*moonPar + sunR + sunPar)
	return
}

// moonFromShadow is the Moon's distance from the centre of the Earth's shadow (degrees).
func moonFromShadow(t time.Time) float64 {
	m, s := Body("moon", t), Body("sun", t)
	return acosd(cosd(m.Lat) * cosd(m.Lon-(s.Lon+180)))
}

// LunarEclipses are the eclipses of the Moon between two moments, with their
// phases' times. Seen from a place, Visible says whether the Moon is up at the
// middle and Start/End are the partial (or penumbral) phase.
func LunarEclipses(from, to time.Time) []Event {
	var out []Event
	for _, full := range angleCrossings(moonElong, 180, from, to, 24*time.Hour) {
		if math.Abs(Body("moon", full).Lat) > 1.6 {
			continue
		}
		// The deepest moment, within three hours of the full Moon.
		peak, d := full, moonFromShadow(full)
		for t := full.Add(-3 * time.Hour); !t.After(full.Add(3 * time.Hour)); t = t.Add(2 * time.Minute) {
			if v := moonFromShadow(t); v < d {
				peak, d = t, v
			}
		}
		_, moonR, _, umbra, penumbra := shadow(peak)
		var sub, title string
		switch {
		case d < umbra-moonR:
			sub, title = "total", "Total eclipse of the Moon"
		case d < umbra+moonR:
			sub, title = "partial", "Partial eclipse of the Moon"
		case d < penumbra+moonR:
			sub, title = "penumbral", "Penumbral eclipse of the Moon"
		default:
			continue
		}
		edge := umbra + moonR
		if sub == "penumbral" {
			edge = penumbra + moonR
		}
		g := func(t time.Time) float64 { return edge - moonFromShadow(t) } // > 0 inside
		start := findRoot(g, peak.Add(-5*time.Hour), peak)
		end := findRoot(func(t time.Time) float64 { return -g(t) }, peak, peak.Add(5*time.Hour))
		mag := (umbra + moonR - d) / (2 * moonR)
		if sub == "penumbral" {
			mag = (penumbra + moonR - d) / (2 * moonR)
		}
		e := Event{Kind: "lunar_eclipse", Sub: sub, T: peak, Title: title, Bodies: []string{"moon"}, Value: math.Round(mag*100) / 100, Key: key("lunar_eclipse", sub, peak), Start: &start, End: &end}
		switch sub {
		case "total":
			e.Detail = "The whole Moon passes into the Earth's shadow and turns a coppery red. Safe to watch with your eyes; binoculars show the colours best."
		case "partial":
			e.Detail = fmt.Sprintf("Part of the Moon (%.0f%% across) passes into the Earth's dark shadow. Safe to watch with your eyes.", math.Max(0, mag)*100)
		default:
			e.Detail = "The Moon passes through the Earth's faint outer shadow: a subtle dimming, hard to notice except near the middle."
		}
		out = append(out, e)
	}
	return out
}

// sunMoonSep is the Sun–Moon distance seen from a place (degrees), and their radii.
func sunMoonSep(o Observer, t time.Time) (sep, sunR, moonR float64) {
	s := Topocentric(Body("sun", t), o, t)
	m := Topocentric(Body("moon", t), o, t)
	return Separation(s.RA, s.Dec, m.RA, m.Dec), s.Size / 7200, m.Size / 7200
}

// overlap is the share of the Sun's disc the Moon covers.
func overlap(sep, rs, rm float64) float64 {
	if sep >= rs+rm {
		return 0
	}
	if sep <= math.Abs(rm-rs) {
		if rm >= rs {
			return 1
		}
		return (rm * rm) / (rs * rs)
	}
	a := acosd((sep*sep+rs*rs-rm*rm)/(2*sep*rs)) * rad
	b := acosd((sep*sep+rm*rm-rs*rs)/(2*sep*rm)) * rad
	area := rs*rs*(a-math.Sin(2*a)/2) + rm*rm*(b-math.Sin(2*b)/2)
	return area / (math.Pi * rs * rs)
}

// SolarEclipses are the eclipses of the Sun between two moments: the kind
// somewhere on the Earth, and how it looks from the place (nil place = only
// the global ones).
func SolarEclipses(from, to time.Time, o *Observer) []Event {
	var out []Event
	for _, nm := range angleCrossings(moonElong, 0, from, to, 24*time.Hour) {
		m := Body("moon", nm)
		if math.Abs(m.Lat) > 1.6 {
			continue
		}
		// Global: the smallest geocentric distance around the new Moon.
		peak, d := nm, 99.0
		for t := nm.Add(-4 * time.Hour); !t.After(nm.Add(4 * time.Hour)); t = t.Add(2 * time.Minute) {
			mm, ss := Body("moon", t), Body("sun", t)
			if v := Separation(mm.RA, mm.Dec, ss.RA, ss.Dec); v < d {
				peak, d = t, v
			}
		}
		moonPar, moonR, sunR, _, _ := shadow(peak)
		sunPar := 8.794 / 3600 / Body("sun", peak).Dist
		if d > moonPar-sunPar+sunR+moonR {
			continue // the penumbra misses the Earth
		}
		sub, title := "partial", "Partial eclipse of the Sun"
		if d < moonPar-sunPar {
			// The shadow's axis meets the Earth: central.
			if moonR > sunR {
				sub, title = "total", "Total eclipse of the Sun"
			} else {
				sub, title = "annular", "Annular eclipse of the Sun"
			}
		}
		e := Event{Kind: "solar_eclipse", Sub: sub, T: peak, Title: title, Bodies: []string{"sun", "moon"}, Key: key("solar_eclipse", sub, peak)}
		e.Detail = "Seen somewhere else on the Earth, not from your place."
		if o != nil {
			localEclipse(&e, *o, peak)
		}
		out = append(out, e)
	}
	return out
}

func localEclipse(e *Event, o Observer, near time.Time) {
	f := false
	e.Visible = &f
	best, bsep := near, 99.0
	for t := near.Add(-5 * time.Hour); !t.After(near.Add(5 * time.Hour)); t = t.Add(time.Minute) {
		if s, _, _ := sunMoonSep(o, t); s < bsep {
			best, bsep = t, s
		}
	}
	_, rs, rm := sunMoonSep(o, best)
	if bsep >= rs+rm {
		return
	}
	inside := func(t time.Time) float64 { s, a, b := sunMoonSep(o, t); return a + b - s }
	start := findRoot(inside, best.Add(-4*time.Hour), best)
	end := findRoot(func(t time.Time) float64 { return -inside(t) }, best, best.Add(4*time.Hour))
	cover := overlap(bsep, rs, rm)
	// Is the Sun up for any of it?
	sun := BodyAlt("sun", o)
	upFrom, upTo := time.Time{}, time.Time{}
	for t := start; !t.After(end); t = t.Add(time.Minute) {
		if sun(t) > HorizonSun {
			if upFrom.IsZero() {
				upFrom = t
			}
			upTo = t
		}
	}
	altPeak := sun(best)
	e.Start, e.End, e.Alt = &start, &end, &altPeak
	e.Value = math.Round(cover*100) / 100
	local := "partial"
	if bsep <= rm-rs {
		local = "total"
	} else if bsep <= rs-rm {
		local = "annular"
	}
	if upFrom.IsZero() {
		e.Detail = "It happens while the Sun is below your horizon, so it can't be seen from your place."
		return
	}
	t := true
	e.Visible = &t
	what := fmt.Sprintf("The Moon covers %.0f%% of the Sun", cover*100)
	if local == "total" {
		what = "The Sun is completely covered: a total eclipse from your place"
	} else if local == "annular" {
		what = "A ring of Sun stays around the Moon: annular from your place"
	}
	switch {
	case altPeak < HorizonSun:
		e.Detail = what + " at its deepest, but the Sun is below the horizon then; you see the start or the end low in the sky. Never look at the Sun without eclipse glasses."
	default:
		e.Detail = what + ". Never look at the Sun without eclipse glasses or a proper solar filter."
	}
	if upFrom.After(start) || upTo.Before(end) {
		e.Start, e.End = &upFrom, &upTo
		e.Detail += " Part of it happens with the Sun below the horizon."
	}
}

// ---- meteor showers ----

// Shower is a yearly meteor shower (International Meteor Organization figures).
type Shower struct {
	Key     string  `json:"key"`
	Name    string  `json:"name"`
	SolLon  float64 `json:"sol_lon"` // the Sun's longitude (J2000) at the peak
	ZHR     int     `json:"zhr"`     // meteors an hour in a perfect dark sky with the radiant overhead
	RA      float64 `json:"ra"`      // radiant, J2000
	Dec     float64 `json:"dec"`
	Speed   int     `json:"speed"` // km/s
	Active  string  `json:"active"`
	Parent  string  `json:"parent"`
	Comment string  `json:"comment"`
}

// Showers are the main yearly meteor showers.
var Showers = []Shower{
	{"quadrantids", "Quadrantids", 283.15, 110, 230, 49, 41, "Dec 28 – Jan 12", "asteroid 2003 EH1", "A short, sharp peak of a few hours; bright fireballs. Best after midnight."},
	{"lyrids", "Lyrids", 32.32, 18, 271, 34, 49, "Apr 14 – Apr 30", "comet Thatcher", "A modest shower with the odd bright one; best before dawn."},
	{"eta-aquariids", "Eta Aquariids", 45.5, 50, 338, -1, 66, "Apr 19 – May 28", "comet Halley", "Dust of Halley's comet; the radiant rises late, so look in the hour before dawn."},
	{"delta-aquariids", "Southern Delta Aquariids", 127, 25, 340, -16, 41, "Jul 12 – Aug 23", "comet 96P/Machholz", "Faint, steady meteors on warm summer nights, best from the south."},
	{"perseids", "Perseids", 140.0, 100, 48, 58, 59, "Jul 17 – Aug 24", "comet Swift–Tuttle", "The summer favourite: fast, bright meteors, many with trails. Best from midnight to dawn."},
	{"draconids", "Draconids", 195.4, 10, 262, 54, 20, "Oct 6 – Oct 10", "comet Giacobini–Zinner", "Slow meteors in the early evening; usually few, now and then a burst."},
	{"orionids", "Orionids", 208, 20, 95, 16, 66, "Oct 2 – Nov 7", "comet Halley", "Halley's dust again: fast meteors from near Orion, best after midnight."},
	{"leonids", "Leonids", 235.27, 15, 152, 22, 71, "Nov 6 – Nov 30", "comet Tempel–Tuttle", "Very fast meteors from Leo after midnight; famous for great storms in some years."},
	{"geminids", "Geminids", 262.2, 150, 112, 33, 35, "Dec 4 – Dec 20", "asteroid 3200 Phaethon", "The richest shower of the year: bright, slow meteors from mid-evening on."},
	{"ursids", "Ursids", 270.7, 10, 217, 76, 33, "Dec 17 – Dec 26", "comet Tuttle", "A small shower near the solstice; the radiant is up all night in the north."},
}

// MeteorShowers are the shower peaks between two moments.
func MeteorShowers(from, to time.Time) []Event {
	var out []Event
	for y := from.Year() - 1; y <= to.Year()+1; y++ {
		for _, s := range Showers {
			// J2000 longitude to the equinox of date (precession, ~50″ a year).
			guess := time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration((Rev(s.SolLon-280)/360)*365.25*24) * time.Hour)
			lon := Rev(s.SolLon + 50.29/3600*(float64(guess.Year())-2000))
			t, ok := SunAtLongitude(lon, guess)
			if !ok || t.Before(from) || !t.Before(to) {
				continue
			}
			m := Body("moon", t)
			e := Event{Kind: "meteor_shower", T: t, Title: s.Name + " meteor shower", Bodies: []string{}, Value: float64(s.ZHR), Sub: s.Key,
				Key: key("meteor_shower", s.Key, t)}
			moon := "The Moon is " + moonWord(m.Phase) + "."
			if m.Phase > 0.6 {
				moon += " Its light hides the fainter meteors this year."
			} else if m.Phase < 0.25 {
				moon += " A dark sky: a good year for it."
			}
			e.Detail = fmt.Sprintf("Up to about %d meteors an hour at best in a dark sky (fewer from town). %s %s", s.ZHR, s.Comment, moon)
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].T.Before(out[j].T) })
	return out
}

func moonWord(phase float64) string {
	switch {
	case phase < 0.05:
		return "new"
	case phase < 0.35:
		return "a thin crescent"
	case phase < 0.65:
		return "about half lit"
	case phase < 0.95:
		return "mostly lit"
	}
	return "full"
}

// MoonPhaseName is the plain name of the Moon's phase from its age (degrees past new).
func MoonPhaseName(age float64) string {
	age = Rev(age)
	switch {
	case age < 10 || age >= 350:
		return "New Moon"
	case age < 80:
		return "Waxing crescent"
	case age < 100:
		return "First quarter"
	case age < 170:
		return "Waxing gibbous"
	case age < 190:
		return "Full Moon"
	case age < 260:
		return "Waning gibbous"
	case age < 280:
		return "Last quarter"
	}
	return "Waning crescent"
}

// AllEvents are every kind of event between two moments; a place adds how
// the eclipses look from it.
func AllEvents(from, to time.Time, o *Observer) []Event {
	var out []Event
	out = append(out, MoonPhases(from, to)...)
	out = append(out, Seasons(from, to)...)
	out = append(out, Conjunctions(from, to)...)
	out = append(out, Oppositions(from, to)...)
	out = append(out, Elongations(from, to)...)
	out = append(out, MeteorShowers(from, to)...)
	le := LunarEclipses(from, to)
	if o != nil {
		for i := range le {
			a := At("moon", *o, le[i].T).Alt
			v := a > 0
			le[i].Visible, le[i].Alt = &v, &a
			if !v {
				// Up for part of it?
				moon := BodyAlt("moon", *o)
				for t := *le[i].Start; !t.After(*le[i].End); t = t.Add(5 * time.Minute) {
					if moon(t) > 0 {
						v = true
						le[i].Visible = &v
						le[i].Detail += " The Moon is below your horizon at the middle; you see part of it."
						break
					}
				}
				if !v {
					le[i].Detail += " The Moon is below your horizon then, so it can't be seen from your place."
				}
			}
		}
	}
	out = append(out, le...)
	out = append(out, SolarEclipses(from, to, o)...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].T.Before(out[j].T) })
	return out
}
