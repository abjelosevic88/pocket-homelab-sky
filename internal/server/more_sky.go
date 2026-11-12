package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/abjelosevic88/pocket-homelab-sky/internal/astro"
	"github.com/abjelosevic88/pocket-homelab-sky/internal/catalog"
)

// ---- comets and the brightest asteroids ----
//
// Comet orbits come from the Minor Planet Center's file of orbits for
// software (fetched once a day); the four biggest asteroids from JPL's
// Small-Body Database (once a week).

var (
	cometsURL = "https://www.minorplanetcenter.net/iau/Ephemerides/Comets/Soft00Cmt.txt"
	sbdbURL   = "https://ssd-api.jpl.nasa.gov/sbdb.api?full-prec=1&phys-par=1&sstr="
)

func (s *Server) fetchText(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", s.cfg.UserAgent)
	res, err := s.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from %s", res.StatusCode, strings.SplitN(strings.TrimPrefix(url, "https://"), "/", 2)[0])
	}
	return io.ReadAll(io.LimitReader(res.Body, limit))
}

// demoComet is a made-up bright comet near the Sun for the demo.
func demoComets() []astro.Comet {
	tp := time.Now().Add(-10 * 24 * time.Hour)
	return []astro.Comet{{Name: "C/2026 D1 (Demo)", Desig: "C/2026 D1", Q: 0.62, E: 0.998, Incl: 64, Peri: 120, Node: 230, Tp: tp, H: 6.5, K: 4}}
}

func (s *Server) comets(ctx context.Context) []astro.Comet {
	if s.cfg.DemoData || s.noNet {
		return demoComets()
	}
	cs, _, err := cached(ctx, s, "minor:comets", 24*time.Hour, func() ([]astro.Comet, error) {
		c, cancel := context.WithTimeout(ctx, 40*time.Second)
		defer cancel()
		b, err := s.fetchText(c, cometsURL, 8<<20)
		if err != nil {
			return nil, err
		}
		all := astro.ParseMPCComets(string(b))
		if len(all) == 0 {
			return nil, errors.New("no comets in the MPC file")
		}
		// keep only those that can get anywhere near binocular brightness this year
		now := time.Now()
		keep := []astro.Comet{}
		for _, cm := range all {
			if cm.H == astro.NoMag || cm.H > 16 {
				continue
			}
			best := 99.0
			for d := -30; d <= 365; d += 15 {
				if m := cm.At(now.AddDate(0, 0, d)).Mag; m < best {
					best = m
				}
			}
			if best <= 13 {
				keep = append(keep, cm)
			}
		}
		return keep, nil
	})
	if err != nil {
		return []astro.Comet{}
	}
	return cs
}

func (s *Server) asteroids(ctx context.Context) []astro.MinorPlanet {
	if s.cfg.DemoData || s.noNet {
		return []astro.MinorPlanet{{Name: "4 Vesta", A: 2.3615, E: 0.0902, Incl: 7.144, Peri: 151.5, Node: 103.7, M: 30, Epoch: time.Now().AddDate(0, -2, 0), H: 3.25, G: 0.32}}
	}
	ms, _, err := cached(ctx, s, "minor:asteroids", 7*24*time.Hour, func() ([]astro.MinorPlanet, error) {
		out := []astro.MinorPlanet{}
		for _, n := range []string{"1", "2", "3", "4", "6", "7"} {
			c, cancel := context.WithTimeout(ctx, 20*time.Second)
			b, err := s.fetchText(c, sbdbURL+n, 1<<20)
			cancel()
			if err != nil {
				continue
			}
			if m, err := astro.ParseSBDB(b); err == nil {
				out = append(out, m)
			}
		}
		if len(out) == 0 {
			return nil, errors.New("JPL SBDB did not answer")
		}
		return out, nil
	})
	if err != nil {
		return []astro.MinorPlanet{}
	}
	return ms
}

type minorView struct {
	Key     string     `json:"key"`
	Name    string     `json:"name"`
	Kind    string     `json:"kind"` // comet | asteroid
	Mag     float64    `json:"mag"`
	RA      float64    `json:"ra"` // of date
	Dec     float64    `json:"dec"`
	Alt     float64    `json:"alt"`
	Az      float64    `json:"az"`
	Con     string     `json:"con_name"`
	Elong   float64    `json:"elong"`
	DistAU  float64    `json:"dist_au"`
	SunAU   float64    `json:"sun_au"`
	BestAt  *time.Time `json:"best_at"`
	BestAlt float64    `json:"best_alt"`
	Dir     string     `json:"dir"`
	Kit     string     `json:"kit"`
	Words   string     `json:"words"`
}

func kitFor(mag float64) string {
	switch {
	case mag <= 5:
		return "eyes"
	case mag <= 8.5:
		return "binoculars"
	}
	return "telescope"
}

// handleMinor is the comets and asteroids worth looking for now, brightest first.
func (s *Server) handleMinor(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	s.loadMinor(ctx)
	wh, err := s.whereOf(r, actorID(ctx))
	if err != nil {
		s.failWhere(w, err)
		return
	}
	t, err := timeParam(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	n, _ := astro.NightAt(wh.Obs, nightDate(t, wh.TZ), wh.TZ)
	from, to := nightWindow(n)
	out := []minorView{}
	add := func(key, name, kind string, at func(time.Time) astro.Position) {
		p := at(t)
		if p.Mag > 12.5 || p.Elong < 15 {
			return
		}
		tp := astro.Topocentric(p, wh.Obs, t)
		alt, az := astro.Horizontal(tp.RA, tp.Dec, wh.Obs, t)
		v := minorView{Key: key, Name: name, Kind: kind, Mag: round(p.Mag, 1), RA: round(p.RA, 3), Dec: round(p.Dec, 3), Alt: round(alt, 1), Az: round(az, 1),
			Elong: math.Round(p.Elong), DistAU: round(p.Dist, 2), SunAU: round(p.Helio, 2), Kit: kitFor(p.Mag)}
		v.Con = catalog.ConName(conOf(p.RA, p.Dec, t))
		if to.After(from) {
			f := astro.FixedAlt(p.RA, p.Dec, wh.Obs)
			best, ba := astro.Culmination(f, from, to)
			_, baz := astro.Horizontal(p.RA, p.Dec, wh.Obs, best)
			v.BestAt, v.BestAlt, v.Dir = &best, math.Round(ba), Direction(baz)
			if ba > 10 {
				v.Words = fmt.Sprintf("In %s, best around %s tonight, %s in the %s. Magnitude %.1f: %s.", v.Con, best.In(wh.TZ).Format("15:04"), heightWords(ba), v.Dir, p.Mag,
					map[string]string{"eyes": "visible to the eye from a dark place", "binoculars": "a fuzzy star in binoculars", "telescope": "needs a telescope and a chart"}[v.Kit])
			} else {
				v.Words = fmt.Sprintf("In %s, but too low during the dark hours tonight.", v.Con)
			}
		}
		out = append(out, v)
	}
	for _, c := range s.comets(ctx) {
		c := c
		add("comet:"+c.Desig, c.Name, "comet", c.At)
	}
	for _, m := range s.asteroids(ctx) {
		m := m
		add("asteroid:"+strings.Fields(m.Name)[0], m.Name, "asteroid", m.At)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Mag < out[j].Mag })
	if len(out) > 20 {
		out = out[:20]
	}
	writeJSON(w, http.StatusOK, map[string]any{"place": wh.view(), "objects": out})
}

// ---- Jupiter's moons, the Great Red Spot, Saturn's rings ----

func (s *Server) handlePlanetDetail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	t, err := timeParam(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	st, _ := s.store.GetSettings(ctx, uid)
	B, P := astro.SaturnRings(t)
	tilt := "wide open: the rings are a joy in any telescope"
	switch b := math.Abs(B); {
	case b < 3:
		tilt = "almost edge-on: the rings are a thin line, or gone"
	case b < 10:
		tilt = "tilted a little: the rings are a narrow ellipse"
	case b < 20:
		tilt = "well tilted: the rings and the gap between them and the planet show well"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"t":       t.UTC(),
		"jupiter": map[string]any{"moons": astro.GalileanMoons(t), "red_spot_transits": astro.RedSpotTransits(t.Add(-2*time.Hour), t.Add(26*time.Hour), st.RedSpotLon), "red_spot_lon": st.RedSpotLon},
		"saturn":  map[string]any{"ring_tilt": round(B, 1), "ring_pa": round(P, 1), "words": tilt},
	})
}

// ---- the Moon's face ----

func (s *Server) handleMoonMap(w http.ResponseWriter, r *http.Request) {
	t, err := timeParam(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	lon, morning := astro.Terminator(t)
	lla, llo := astro.Libration(t)
	m := astro.Body("moon", t)
	sn := astro.Body("sun", t)
	writeJSON(w, http.StatusOK, map[string]any{
		"t": t.UTC(), "phase": round(m.Phase, 3), "age": round(astro.Rev(m.Lon-sn.Lon), 1), "colongitude": round(astro.SunColongitude(t), 1),
		"terminator": round(lon, 1), "morning": morning, "libration": map[string]float64{"lat": round(lla, 2), "lon": round(llo, 2)},
		"best": astro.BestOnMoon(t, 8), "features": astro.MoonFeatures,
	})
}

// ---- double stars and variable stars ----

func doubleKey(d astro.DoubleStar) string {
	return "dbl:" + strings.ToLower(strings.NewReplacer(" ", "-", "&", "and", "(", "", ")", "", "'", "").Replace(d.Name))
}

func doubleObject(key string) (catalog.Object, bool) {
	for _, d := range astro.DoubleStars {
		if doubleKey(d) == key {
			m := d.Mag1
			about := fmt.Sprintf("A double star: %s, %.1f″ apart (magnitudes %.1f and %.1f). %s", d.Colors, d.SepArcsec, d.Mag1, d.Mag2, d.About)
			return catalog.Object{Key: key, Name: d.Name, Kind: "star", Type: "Double star", RA: d.RA, Dec: d.Dec, Mag: &m, Con: d.Con, About: about}, true
		}
	}
	return catalog.Object{}, false
}

func (s *Server) handleDoubles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	wh, err := s.whereOf(r, uid)
	type row struct {
		astro.DoubleStar
		Key     string     `json:"key"`
		ConName string     `json:"con_name"`
		BestAt  *time.Time `json:"best_at"`
		BestAlt float64    `json:"best_alt"`
		Dir     string     `json:"dir"`
		Seen    string     `json:"seen"`
	}
	seen, _ := s.store.SeenObjects(ctx, uid)
	out := []row{}
	var from, to time.Time
	if err == nil {
		if n, err := astro.NightAt(wh.Obs, nightDate(time.Now(), wh.TZ), wh.TZ); err == nil {
			from, to = nightWindow(n)
		}
	}
	for _, d := range astro.DoubleStars {
		rw := row{DoubleStar: d, Key: doubleKey(d), ConName: catalog.ConName(d.Con)}
		if v, ok := seen[rw.Key]; ok {
			rw.Seen = v.First
		}
		if to.After(from) {
			ra, dec := astro.OfDate(d.RA, d.Dec, from)
			best, a := astro.Culmination(astro.FixedAlt(ra, dec, wh.Obs), from, to)
			_, az := astro.Horizontal(ra, dec, wh.Obs, best)
			rw.BestAt, rw.BestAlt, rw.Dir = &best, math.Round(a), Direction(az)
		}
		out = append(out, rw)
	}
	var vars []map[string]any
	if err == nil {
		now := time.Now()
		for _, v := range []struct {
			name, key, what string
			ra, dec         float64
			times           []time.Time
		}{
			{"Algol", "star:14576", "at its faintest: it drops from magnitude 2.1 to 3.4 for a few hours as its dim partner passes in front", 47.04, 40.96, astro.AlgolMinima(now, now.AddDate(0, 0, 14))},
			{"Delta Cephei", "star:110991", "at its brightest (magnitude 3.5, down to 4.4 between): the star that measures the universe", 337.29, 58.42, astro.DeltaCepheiMaxima(now, now.AddDate(0, 0, 14))},
		} {
			ra, dec := astro.OfDate(v.ra, v.dec, now)
			f := astro.FixedAlt(ra, dec, wh.Obs)
			good := []time.Time{}
			for _, t := range v.times {
				if f(t) > 15 && astro.At("sun", wh.Obs, t).Alt < -12 {
					good = append(good, t)
				}
			}
			vars = append(vars, map[string]any{"name": v.name, "key": v.key, "what": v.what, "times": good})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"doubles": out, "variables": vars})
}

// ---- comets and asteroids as objects ----
//
// The info card, the log, plans and Pocket AI look objects up by key;
// comets ("comet:<designation>") and asteroids ("asteroid:<number>") move,
// so they are kept here with their orbits as they are loaded.

type minorEntry struct {
	name, kind string
	at         func(time.Time) astro.Position
}

var minorReg sync.Map // key → minorEntry

func isMinorKey(key string) bool {
	return strings.HasPrefix(key, "comet:") || strings.HasPrefix(key, "asteroid:")
}

// loadMinor makes sure the comets and asteroids are known (from the cache).
func (s *Server) loadMinor(ctx context.Context) {
	for _, c := range s.comets(ctx) {
		c := c
		minorReg.Store("comet:"+c.Desig, minorEntry{c.Name, "comet", c.At})
	}
	for _, m := range s.asteroids(ctx) {
		m := m
		if f := strings.Fields(m.Name); len(f) > 0 {
			minorReg.Store("asteroid:"+f[0], minorEntry{m.Name, "asteroid", m.At})
		}
	}
}

func minorOf(key string) (minorEntry, bool) {
	v, ok := minorReg.Load(key)
	if !ok {
		return minorEntry{}, false
	}
	return v.(minorEntry), true
}

// minorObject is a comet or asteroid as an object, where it is now.
func minorObject(key string) (catalog.Object, bool) {
	e, ok := minorOf(key)
	if !ok {
		return catalog.Object{}, false
	}
	now := time.Now()
	p := e.at(now)
	o := catalog.Object{Key: key, Name: e.name, Kind: e.kind, RA: p.RA, Dec: p.Dec, Con: conOf(p.RA, p.Dec, now)}
	if p.Mag < astro.NoMag {
		m := round(p.Mag, 1)
		o.Mag = &m
	}
	kit := map[string]string{"eyes": "bright enough to see with the eyes from a dark place", "binoculars": "a faint fuzzy star in binoculars", "telescope": "needs a telescope and a chart"}[kitFor(p.Mag)]
	if e.kind == "comet" {
		o.Type = "Comet"
		o.About = fmt.Sprintf("A comet: a ball of ice and dust on a long orbit, %.2f AU from the Sun and %.2f AU from us now. Right now it is %s; comets often turn out brighter or fainter than predicted.", p.Helio, p.Dist, kit)
	} else {
		o.Type = "Asteroid"
		o.About = fmt.Sprintf("One of the biggest asteroids, a small rocky world between Mars and Jupiter, %.2f AU from us now. It looks like a star and is %s; it moves a little against the stars from night to night.", p.Dist, kit)
	}
	return o, true
}
