package server

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/abjelosevic88/pocket-homelab-sky/internal/astro"
	"github.com/abjelosevic88/pocket-homelab-sky/internal/lightmap"
	"github.com/abjelosevic88/pocket-homelab-sky/internal/space"
)

// ---- steady and clear air (7Timer!) ----

type airView struct {
	Seeing       int    `json:"seeing"`       // average over the dark hours, 1–5
	Transparency int    `json:"transparency"` // 1–5
	Words        string `json:"words"`
}

var steadyWords = []string{"", "boiling: the planets look like jelly", "unsteady", "fair", "steady", "very steady: sharp planets and close double stars"}
var clearWords = []string{"", "hazy: faint things are lost", "rather hazy", "fair", "clear", "crystal clear: the faintest galaxies"}

func airOf(hours []nightHour, n astro.Night) *airView {
	var s, t, c float64
	for _, h := range hours {
		if h.Seeing == nil || (!h.Dark && n.DarkKind == "astro") {
			continue
		}
		s += float64(*h.Seeing)
		t += float64(*h.Transp)
		c++
	}
	if c == 0 {
		return nil
	}
	v := &airView{Seeing: int(math.Round(s / c)), Transparency: int(math.Round(t / c))}
	v.Words = fmt.Sprintf("The air: %s, %s.", steadyWords[v.Seeing], clearWords[v.Transparency])
	return v
}

// airAt is the seeing and transparency forecast at a spot (three hours' cache).
func (s *Server) airAt(ctx context.Context, lat, lon float64) *space.Air {
	if s.cfg.DemoData || s.noNet {
		return demoAir()
	}
	a, _, err := cached(ctx, s, "air:"+spot(lat, lon), 3*time.Hour, func() (*space.Air, error) {
		c, cancel := context.WithTimeout(ctx, 12*time.Second)
		defer cancel()
		return space.FetchAir(c, s.http, s.cfg.UserAgent, lat, lon)
	})
	if err != nil {
		return nil
	}
	return a
}

func demoAir() *space.Air {
	start := time.Now().UTC().Truncate(3 * time.Hour)
	a := &space.Air{Fetched: time.Now().UTC()}
	for i := 0; i < 24; i++ {
		a.Hours = append(a.Hours, space.AirHour{Time: start.Add(time.Duration(i*3) * time.Hour), Seeing: 3 + (i/3)%3, Transparency: 4 - (i/4)%2})
	}
	return a
}

// ---- the aurora (NOAA SWPC) ----

type auroraView struct {
	KpMax    float64 `json:"kp_max"`    // the highest Kp forecast for the dark hours
	KpNeeded float64 `json:"kp_needed"` // about what it takes to see it from here
	Chance   int     `json:"chance"`    // OVATION's chance overhead now, %
	Likely   bool    `json:"likely"`
	Words    string  `json:"words"`
	MagLat   float64 `json:"mag_lat"`
}

func (s *Server) auroraNow(ctx context.Context) *space.Aurora {
	if s.cfg.DemoData || s.noNet {
		now := time.Now().UTC().Truncate(3 * time.Hour)
		a := &space.Aurora{Fetched: time.Now().UTC()}
		for i := -8; i < 24; i++ {
			a.Kp = append(a.Kp, space.KpStep{Time: now.Add(time.Duration(i*3) * time.Hour), Kp: 2 + float64((i+8)%4)*0.67, Observed: i < 0})
		}
		return a
	}
	a, _, err := cached(ctx, s, "aurora", 30*time.Minute, func() (*space.Aurora, error) {
		c, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		return space.FetchAurora(c, s.http, s.cfg.UserAgent)
	})
	if err != nil {
		return nil
	}
	return a
}

func (s *Server) auroraTonight(ctx context.Context, w where, n astro.Night) *auroraView {
	return auroraOf(s.auroraNow(ctx), w, n)
}

// auroraOf reads a forecast for one night at a place.
func auroraOf(a *space.Aurora, w where, n astro.Night) *auroraView {
	if a == nil || n.DarkKind == "none" {
		return nil
	}
	v := &auroraView{KpMax: a.MaxKp(n.DarkFrom, n.DarkTo), KpNeeded: space.KpNeeded(w.Obs.Lat, w.Obs.Lon), MagLat: math.Round(space.MagLat(w.Obs.Lat, w.Obs.Lon)*10) / 10}
	if time.Now().After(n.DarkFrom.Add(-6*time.Hour)) && time.Now().Before(n.DarkTo) {
		v.Chance = a.Chance(w.Obs.Lat, w.Obs.Lon)
	}
	side := "northern"
	if w.Obs.Lat < 0 {
		side = "southern"
	}
	switch {
	case v.Chance >= 10 || (v.KpMax >= v.KpNeeded+1 && v.KpNeeded > 0):
		v.Likely = true
		v.Words = fmt.Sprintf("Northern lights likely: a geomagnetic storm (Kp %.0f) is forecast, more than the Kp %.0f it takes here. Look %s, away from town lights; a phone camera sees it before your eyes do.", v.KpMax, v.KpNeeded, map[string]string{"northern": "north", "southern": "south"}[side])
	case v.KpMax >= v.KpNeeded:
		v.Likely = true
		v.Words = fmt.Sprintf("The %s lights are possible low on the %s horizon tonight (Kp %.0f forecast, about %.0f needed here). Best in a long-exposure photo.", side, map[string]string{"northern": "north", "southern": "south"}[side], v.KpMax, v.KpNeeded)
	case v.KpNeeded >= 8.5:
		v.Words = fmt.Sprintf("No %s lights tonight (Kp %.0f). From here they show only in the greatest storms, a few times in ten years.", side, v.KpMax)
	default:
		v.Words = fmt.Sprintf("No %s lights tonight: Kp %.0f is forecast, about %.0f is needed here.", side, v.KpMax, v.KpNeeded)
	}
	return v
}

// ---- how dark the sky is (the light-pollution atlas) ----

type darknessView struct {
	Zone   string `json:"zone"`
	Rank   int    `json:"rank"`
	Bortle int    `json:"bortle"` // about
	Words  string `json:"words"`
}

func (s *Server) lightmapPath() string { return filepath.Join(s.cfg.DataDir, "cache", "lightmap.bin") }

// lightMap is the atlas when it is on disk.
func (s *Server) lightMap() *lightmap.Map {
	s.lmMu.Lock()
	defer s.lmMu.Unlock()
	if s.lm == nil {
		if m, err := lightmap.Open(s.lightmapPath()); err == nil {
			s.lm = m
		}
	}
	return s.lm
}

// fetchLightMap downloads the atlas once, in the background (about 3 MB).
func (s *Server) fetchLightMap(ctx context.Context) {
	if s.cfg.DemoData || s.noNet {
		return
	}
	if _, err := os.Stat(s.lightmapPath()); err == nil {
		return
	}
	for wait := time.Minute; ; wait *= 2 {
		c, cancel := context.WithTimeout(ctx, 3*time.Minute)
		err := lightmap.Build(c, s.http, s.cfg.UserAgent, s.lightmapPath())
		cancel()
		if err == nil {
			s.log.Info("light-pollution atlas downloaded")
			return
		}
		s.log.Warn("light-pollution atlas", "err", err, "retry_in", wait.String())
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		if wait > 6*time.Hour {
			wait = 6 * time.Hour
		}
	}
}

func (s *Server) darknessAt(lat, lon float64) *darknessView {
	m := s.lightMap()
	if m == nil {
		return nil
	}
	r, ok := m.Zone(lat, lon)
	if !ok {
		return nil
	}
	return &darknessView{Zone: lightmap.Zones[r], Rank: r, Bortle: lightmap.Bortle(r), Words: lightmap.Words(r)}
}

// handleDarkness is how dark the sky is at a place from the atlas, and with
// ?near=KM the darkest places within that distance (named when possible).
func (s *Server) handleDarkness(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	wh, err := s.whereOf(r, actorID(ctx))
	if err != nil {
		s.failWhere(w, err)
		return
	}
	m := s.lightMap()
	if m == nil {
		writeJSON(w, http.StatusOK, map[string]any{"available": false, "detail": lightmap.ErrMissing.Error()})
		return
	}
	out := map[string]any{"available": true, "here": s.darknessAt(wh.Obs.Lat, wh.Obs.Lon), "credit": "World Atlas of the Artificial Night Sky Brightness, D. Lorenz (2025 VIIRS data)"}
	if km, _ := strconv.ParseFloat(r.URL.Query().Get("near"), 64); km > 0 {
		km = math.Min(km, 150)
		type spotOut struct {
			lightmap.Spot
			Name   string `json:"name"`
			Dir8   string `json:"dir_words"`
			Words  string `json:"words"`
			Region string `json:"region"`
		}
		spots := []spotOut{}
		for _, sp := range m.Darkest(wh.Obs.Lat, wh.Obs.Lon, km, 5) {
			o := spotOut{Spot: sp, Dir8: Direction(sp.Dir), Words: lightmap.Words(sp.Rank)}
			if p, err := s.placeName(ctx, sp.Lat, sp.Lon, ""); err == nil && p != nil {
				o.Name, o.Region = p.Name, p.Region
			}
			spots = append(spots, o)
		}
		out["spots"] = spots
	}
	writeJSON(w, http.StatusOK, out)
}
