package sat

import (
	"math"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/abjelosevic88/pocket-homelab-sky/internal/astro"
)

// Pass is one time a satellite (or a Starlink train) climbs above the
// chosen altitude at a place.
type Pass struct {
	Sat     string `json:"sat"`
	NoradID int    `json:"norad"`
	Group   string `json:"group"`

	// Above the chosen lowest altitude, refined to the second.
	Start time.Time `json:"start"`
	Peak  time.Time `json:"peak"`
	End   time.Time `json:"end"`

	StartAz float64 `json:"startAz"`
	PeakAz  float64 `json:"peakAz"`
	EndAz   float64 `json:"endAz"`
	PeakAlt float64 `json:"peakAlt"`

	// Mag is the brightest it gets while it can be seen; for a pass that
	// cannot be seen, how bright it would be at its highest if it were lit.
	Mag float64 `json:"mag"`

	// Visible: the Sun lights the satellite while the Sun is at least 6°
	// below the horizon at the place, for part of the pass (from
	// VisibleFrom to VisibleTo).
	Visible     bool      `json:"visible"`
	VisibleFrom time.Time `json:"visibleFrom,omitzero"`
	VisibleTo   time.Time `json:"visibleTo,omitzero"`

	// Train: several new Starlinks crossing on the same track within a few
	// minutes of each other, merged into one pass from the first one's rise
	// to the last one's setting; Count is how many (1 for a single
	// satellite).
	Train bool `json:"train"`
	Count int  `json:"count"`
}

// darkSun is how far the Sun must be below the horizon for a satellite to
// stand out (civil twilight over).
const darkSun = -6.0

// Search steps.
const (
	coarseStep = 10 * time.Second // while a satellite could be up
	visStep    = 10 * time.Second // when looking for the lit and dark part
	margin     = 30 * time.Minute // look this far either side for passes under way
)

// Passes finds every pass of the satellites above minAlt degrees at a place
// between from and to (passes under way at either end are included whole),
// sorted by Start. New Starlinks crossing together are merged into trains.
func Passes(sats []Sat, lat, lon, heightM float64, from, to time.Time, minAlt float64) []Pass {
	if !to.After(from) || len(sats) == 0 {
		return nil
	}
	o := newSite(lat, lon, heightM)
	sunCache := newSunCache(o)

	workers := runtime.GOMAXPROCS(0)
	if workers > len(sats) {
		workers = len(sats)
	}
	jobs := make(chan int)
	results := make([][]Pass, len(sats))
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				results[i] = satPasses(sats[i], o, sunCache, from, to, minAlt)
			}
		}()
	}
	for i := range sats {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	var all []Pass
	for _, r := range results {
		all = append(all, r...)
	}
	all = mergeTrains(all)
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].Start.Equal(all[j].Start) {
			return all[i].NoradID < all[j].NoradID
		}
		return all[i].Start.Before(all[j].Start)
	})
	return all
}

// satPasses is the passes of one satellite.
func satPasses(s Sat, o *site, sc *sunCache, from, to time.Time, minAlt float64) []Pass {
	e, err := parseElements(s.Line1, s.Line2)
	if err != nil {
		return nil
	}
	// The farthest the satellite can be (its apogee, with room for the
	// short-period terms) gives the widest circle around the place it can be
	// seen above minAlt from.
	apogee := e.ao * (1 + e.ecco) * reKm * 1.01
	lambda := reachAngle(o.geocentricR, apogee, minAlt)
	if lambda <= 0 {
		return nil
	}
	// It never comes near enough to the place's latitude.
	incl := e.inclo * deg
	if incl > 90 {
		incl = 180 - incl
	}
	geocLat := math.Abs(math.Asin(o.ecef[2]/o.geocentricR) * deg)
	if geocLat > incl+lambda*deg+0.5 {
		return nil
	}
	// The fastest the satellite can move around the Earth's centre (at
	// perigee), plus the Earth's turning under it, radians a minute.
	n := e.noUnkozai
	ec := e.ecco
	rate := (n*(1+ec)*(1+ec)/math.Pow(1-ec*ec, 1.5) + 0.0044) * 1.1

	altAt := func(t time.Time) (float64, float64, bool) {
		r, _, err := e.at(t)
		if err != nil {
			return 0, 0, false
		}
		th := gmst(t)
		op := o.position(th)
		rr := math.Sqrt(r[0]*r[0] + r[1]*r[1] + r[2]*r[2])
		cosAng := (r[0]*op[0] + r[1]*op[1] + r[2]*op[2]) / (rr * o.geocentricR)
		ang := math.Acos(clamp(cosAng, -1, 1))
		rho := [3]float64{r[0] - op[0], r[1] - op[1], r[2] - op[2]}
		rng := math.Sqrt(rho[0]*rho[0] + rho[1]*rho[1] + rho[2]*rho[2])
		sl, cl := math.Sincos(th + o.lon)
		zen := o.cosLat*cl*rho[0] + o.cosLat*sl*rho[1] + o.sinLat*rho[2]
		return math.Asin(clamp(zen/rng, -1, 1)) * deg, ang, true
	}
	above := func(t time.Time) bool {
		a, _, ok := altAt(t)
		return ok && a >= minAlt
	}

	var out []Pass
	start, stop := from.Add(-margin), to.Add(margin)
	var (
		up          bool
		rise, peakT time.Time
		peakAlt     = -90.0
		prevT       time.Time
		havePrev    bool
	)
	finish := func(set time.Time) {
		up = false
		if !set.After(from) || !rise.Before(to) {
			return
		}
		if p, ok := describe(s, e, o, sc, rise, peakT, set, minAlt, altAt); ok {
			out = append(out, p)
		}
	}
	for t := start; t.Before(stop); {
		alt, ang, ok := altAt(t)
		if !ok {
			break // decayed or the elements have run out
		}
		next := coarseStep
		if !up && ang > lambda {
			// Out of reach: skip the time it needs at least to come into reach.
			if skip := time.Duration((ang - lambda) / rate * float64(time.Minute)); skip > next {
				next = skip
			}
		}
		switch {
		case !up && alt >= minAlt:
			up = true
			if havePrev {
				rise = bisect(prevT, t, above)
			} else {
				rise = t.Round(time.Second) // already up when the search begins
			}
			peakT, peakAlt = t, alt
		case up && alt >= minAlt:
			if alt > peakAlt {
				peakT, peakAlt = t, alt
			}
		case up && alt < minAlt:
			set := bisect(prevT, t, func(x time.Time) bool { return !above(x) })
			finish(set)
			peakAlt = -90
		}
		prevT, havePrev = t, true
		t = t.Add(next)
	}
	if up {
		finish(prevT.Round(time.Second))
	}
	return out
}

// reachAngle is the angle at the Earth's centre between a place (r0 km from
// the centre) and a satellite r km from the centre seen at altitude minAlt
// (radians); the satellite is above minAlt only nearer than that.
func reachAngle(r0, r, minAlt float64) float64 {
	el := minAlt * rad
	x := r0 * math.Cos(el) / r
	if x >= 1 {
		return 0
	}
	return math.Pi/2 - el - math.Asin(x)
}

// bisect narrows a change between a (false) and b (true) down to a second
// and returns the first whole second where cond holds.
func bisect(a, b time.Time, cond func(time.Time) bool) time.Time {
	for b.Sub(a) > time.Second {
		m := a.Add(b.Sub(a) / 2)
		if cond(m) {
			b = m
		} else {
			a = m
		}
	}
	return b.Round(time.Second)
}

// describe fills in a pass found between rise and set.
func describe(s Sat, e *elements, o *site, sc *sunCache, rise, peakGuess, set time.Time, minAlt float64,
	altAt func(time.Time) (float64, float64, bool)) (Pass, bool) {
	if !set.After(rise) {
		return Pass{}, false
	}
	// The highest point: golden-section search around the best sample.
	a := peakGuess.Add(-coarseStep)
	b := peakGuess.Add(coarseStep)
	if a.Before(rise) {
		a = rise
	}
	if b.After(set) {
		b = set
	}
	alt := func(t time.Time) float64 {
		v, _, ok := altAt(t)
		if !ok {
			return -90
		}
		return v
	}
	const g = 0.6180339887498949
	for b.Sub(a) > time.Second {
		span := b.Sub(a)
		c := b.Add(-time.Duration(float64(span) * g))
		d := a.Add(time.Duration(float64(span) * g))
		if alt(c) >= alt(d) {
			b = d
		} else {
			a = c
		}
	}
	peak := a.Add(b.Sub(a) / 2).Round(time.Second)

	sunPos := sc.vector(peak)
	lookAt := func(t time.Time) Look {
		l, _ := o.look(e, t, s.StdMag, &sunPos)
		return l
	}
	ls, lp, le := lookAt(rise), lookAt(peak), lookAt(set)
	p := Pass{
		Sat: s.Name, NoradID: s.NoradID, Group: s.Group,
		Start: rise.UTC(), Peak: peak.UTC(), End: set.UTC(),
		StartAz: round1(ls.Az), PeakAz: round1(lp.Az), EndAz: round1(le.Az),
		PeakAlt: round1(math.Max(lp.Alt, minAlt)),
		Mag:     lp.Mag,
		Count:   1,
	}

	// Can it be seen? Only when the sky is dark enough somewhere in the pass.
	if sc.alt(rise) > darkSun && sc.alt(set) > darkSun && sc.alt(peak) > darkSun {
		return p, true
	}
	seen := func(t time.Time) bool {
		if sc.alt(t) > darkSun {
			return false
		}
		r, _, err := e.at(t)
		return err == nil && sunlit(r, sunPos)
	}
	best := math.Inf(1)
	note := func(t time.Time) {
		if m := lookAt(t).Mag; m < best {
			best = m
		}
	}
	var visFrom, visTo time.Time
	prev, prevSeen := rise, seen(rise)
	if prevSeen {
		visFrom = rise
		note(rise)
	}
	for t := rise.Add(visStep); ; t = t.Add(visStep) {
		if t.After(set) {
			t = set
		}
		now := seen(t)
		switch {
		case now && !prevSeen:
			x := bisect(prev, t, seen)
			if visFrom.IsZero() {
				visFrom = x
			}
			visTo = time.Time{}
			note(x)
		case !now && prevSeen:
			visTo = bisect(prev, t, func(x time.Time) bool { return !seen(x) })
		}
		if now {
			note(t)
		}
		prev, prevSeen = t, now
		if !t.Before(set) {
			break
		}
	}
	if visFrom.IsZero() {
		return p, true
	}
	if prevSeen {
		visTo = set
	}
	if seen(peak) {
		note(peak)
	}
	p.Visible = true
	p.VisibleFrom, p.VisibleTo = visFrom.UTC(), visTo.UTC()
	p.Mag = best
	return p, true
}

func round1(x float64) float64 { return math.Round(x*10) / 10 }

// sunCache keeps the Sun's place by the minute (it moves a quarter of a
// degree a minute across the sky and far less among the stars).
type sunCache struct {
	o  *site
	mu sync.Mutex
	m  map[int64]astro.Position
}

func newSunCache(o *site) *sunCache { return &sunCache{o: o, m: map[int64]astro.Position{}} }

func (c *sunCache) body(t time.Time) astro.Position {
	k := t.Unix() / 600 // every ten minutes is plenty for RA and Dec
	c.mu.Lock()
	defer c.mu.Unlock()
	p, ok := c.m[k]
	if !ok {
		p = astro.Body("sun", time.Unix(k*600+300, 0))
		c.m[k] = p
	}
	return p
}

// alt is the Sun's altitude at the place (degrees, its centre, no refraction).
func (c *sunCache) alt(t time.Time) float64 {
	p := c.body(t)
	a, _ := astro.Horizontal(p.RA, p.Dec, c.o.astroObs, t)
	return a
}

// vector is the Sun's place from the Earth's centre (km).
func (c *sunCache) vector(t time.Time) [3]float64 {
	p := c.body(t)
	sr, cr := math.Sincos(p.RA * rad)
	sd, cd := math.Sincos(p.Dec * rad)
	d := p.Dist * auKm
	return [3]float64{d * cd * cr, d * cd * sr, d * sd}
}

// Starlink trains.
const (
	trainGap   = 3 * time.Minute
	trainAz    = 20.0 // degrees between the rising (and setting) points
	trainAlt   = 15.0 // degrees between the highest points
	trainFewer = 3    // fewer than this together are left as single passes
)

// mergeTrains joins Starlink passes that follow each other on the same
// track into one pass each.
func mergeTrains(ps []Pass) []Pass {
	var star, rest []Pass
	for _, p := range ps {
		if p.Group == GroupStarlink || strings.HasPrefix(strings.ToUpper(p.Sat), "STARLINK") {
			star = append(star, p)
		} else {
			rest = append(rest, p)
		}
	}
	if len(star) < trainFewer {
		return ps
	}
	sort.SliceStable(star, func(i, j int) bool { return star[i].Start.Before(star[j].Start) })
	used := make([]bool, len(star))
	for i := range star {
		if used[i] {
			continue
		}
		group := []int{i}
		used[i] = true
		last := i
		for j := i + 1; j < len(star); j++ {
			if star[j].Start.Sub(star[last].Start) > trainGap {
				break
			}
			if used[j] || !sameTrack(star[last], star[j]) {
				continue
			}
			group = append(group, j)
			used[j] = true
			last = j
		}
		if len(group) < trainFewer {
			for _, k := range group {
				rest = append(rest, star[k])
			}
			continue
		}
		rest = append(rest, joinTrain(star, group))
	}
	return rest
}

func sameTrack(a, b Pass) bool {
	return angleDiff(a.StartAz, b.StartAz) <= trainAz &&
		angleDiff(a.EndAz, b.EndAz) <= trainAz &&
		math.Abs(a.PeakAlt-b.PeakAlt) <= trainAlt
}

func angleDiff(a, b float64) float64 {
	d := math.Abs(math.Mod(a-b, 360))
	if d > 180 {
		d = 360 - d
	}
	return d
}

func joinTrain(star []Pass, group []int) Pass {
	first, lastP := star[group[0]], star[group[len(group)-1]]
	t := first
	t.Sat = "Starlink train"
	t.Train = true
	t.Count = len(group)
	t.Start = first.Start
	t.StartAz = first.StartAz
	t.End, t.EndAz = lastP.End, lastP.EndAz
	t.Visible = false
	t.VisibleFrom, t.VisibleTo = time.Time{}, time.Time{}
	bestVis, bestAny := math.Inf(1), math.Inf(1)
	for _, k := range group {
		p := star[k]
		if p.End.After(t.End) {
			t.End, t.EndAz = p.End, p.EndAz
		}
		if p.PeakAlt > t.PeakAlt {
			t.Peak, t.PeakAz, t.PeakAlt = p.Peak, p.PeakAz, p.PeakAlt
		}
		bestAny = math.Min(bestAny, p.Mag)
		if p.Visible {
			t.Visible = true
			bestVis = math.Min(bestVis, p.Mag)
			if t.VisibleFrom.IsZero() || p.VisibleFrom.Before(t.VisibleFrom) {
				t.VisibleFrom = p.VisibleFrom
			}
			if p.VisibleTo.After(t.VisibleTo) {
				t.VisibleTo = p.VisibleTo
			}
		}
	}
	if t.Visible {
		t.Mag = bestVis
	} else {
		t.Mag = bestAny
	}
	return t
}
