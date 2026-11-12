package server

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"sort"
	"time"

	"github.com/abjelosevic88/pocket-homelab-sky/internal/sat"
)

// ---- satellites: the space station, the brightest satellites, Starlink trains ----
//
// The elements come from CelesTrak, at most every four hours (as CelesTrak
// asks), shared by everyone on the box; passes are worked out per spot and
// kept half an hour.

const satTTL = 4 * time.Hour

func (s *Server) satCatalog(ctx context.Context) *sat.Catalog {
	if s.cfg.DemoData || s.noNet {
		return sat.DemoAt(time.Now().Add(-6 * time.Hour))
	}
	c, _, err := cached(ctx, s, "sat:catalog", satTTL, func() (*sat.Catalog, error) {
		cx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		cat, err := sat.Fetch(cx, s.http, s.cfg.UserAgent)
		if cat == nil {
			return nil, err
		}
		return cat, nil
	})
	if err != nil {
		return nil
	}
	return c
}

type passView struct {
	Name     string     `json:"name"`
	Norad    int        `json:"norad"`
	Group    string     `json:"group"` // station | visual | starlink
	Start    time.Time  `json:"start"`
	Peak     time.Time  `json:"peak"`
	End      time.Time  `json:"end"`
	StartDir string     `json:"start_dir"`
	PeakDir  string     `json:"peak_dir"`
	EndDir   string     `json:"end_dir"`
	PeakAz   float64    `json:"peak_az"`
	PeakAlt  float64    `json:"peak_alt"`
	Mag      float64    `json:"mag"`
	Visible  bool       `json:"visible"`
	From     *time.Time `json:"visible_from,omitempty"`
	To       *time.Time `json:"visible_to,omitempty"`
	Train    bool       `json:"train"`
	Count    int        `json:"count"`
	Words    string     `json:"words"`
}

// passesAt are the passes over a place for the next days (half an hour's cache).
func (s *Server) passesAt(ctx context.Context, w where, days int) []passView {
	cat := s.satCatalog(ctx)
	if cat == nil {
		return []passView{}
	}
	now := time.Now().Truncate(10 * time.Minute)
	key := fmt.Sprintf("sat:passes:%s:%d:%s", spot(w.Obs.Lat, w.Obs.Lon), days, now.Format("200601021504"))
	ps, _, err := cached(ctx, s, key, 30*time.Minute, func() ([]passView, error) {
		raw := sat.Passes(cat.Sats, w.Obs.Lat, w.Obs.Lon, 0, now, now.Add(time.Duration(days)*24*time.Hour), 10)
		out := make([]passView, 0, len(raw))
		for _, p := range raw {
			out = append(out, passOf(p, w))
		}
		return out, nil
	})
	if err != nil {
		return []passView{}
	}
	return ps
}

func passOf(p sat.Pass, w where) passView {
	v := passView{Name: satName(p.Sat), Norad: p.NoradID, Group: p.Group, Start: p.Start, Peak: p.Peak, End: p.End, StartDir: Direction(p.StartAz), PeakDir: Direction(p.PeakAz),
		EndDir: Direction(p.EndAz), PeakAz: math.Round(p.PeakAz), PeakAlt: math.Round(p.PeakAlt), Mag: math.Round(p.Mag*10) / 10, Visible: p.Visible, Train: p.Train, Count: p.Count}
	if p.Visible {
		f, t := p.VisibleFrom, p.VisibleTo
		v.From, v.To = &f, &t
		bright := "faint"
		switch {
		case p.Mag < -2:
			bright = "very bright"
		case p.Mag < 0:
			bright = "bright"
		case p.Mag < 2:
			bright = "easy to see"
		}
		what := "a steady moving star"
		if p.Train {
			what = fmt.Sprintf("a line of %d moving stars, one behind the other", p.Count)
		}
		v.Words = fmt.Sprintf("Look %s at %s: %s (%s), highest %0.f° up in the %s, gone in the %s at %s.", Direction(p.StartAz), f.In(w.TZ).Format("15:04"), what, bright,
			p.PeakAlt, Direction(p.PeakAz), Direction(p.EndAz), t.In(w.TZ).Format("15:04"))
	} else {
		v.Words = "Passes over in daylight or in the Earth's shadow: it can't be seen."
	}
	return v
}

func (s *Server) handlePasses(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	wh, err := s.whereOf(r, actorID(ctx))
	if err != nil {
		s.failWhere(w, err)
		return
	}
	days := 3
	if n := queryInt(r, "days"); n > 0 && n <= 7 {
		days = n
	}
	ps := s.passesAt(ctx, wh, days)
	if queryBool(r, "visible") {
		keep := []passView{}
		for _, p := range ps {
			if p.Visible {
				keep = append(keep, p)
			}
		}
		ps = keep
	}
	if g := r.URL.Query().Get("group"); g != "" {
		keep := []passView{}
		for _, p := range ps {
			if p.Group == g {
				keep = append(keep, p)
			}
		}
		ps = keep
	}
	cat := s.satCatalog(ctx)
	var fetched time.Time
	if cat != nil {
		fetched = cat.Fetched
	}
	writeJSON(w, http.StatusOK, map[string]any{"place": wh.view(), "passes": ps, "elements_at": fetched})
}

type satNow struct {
	Name   string  `json:"name"`
	Norad  int     `json:"norad"`
	Group  string  `json:"group"`
	Alt    float64 `json:"alt"`
	Az     float64 `json:"az"`
	Alt2   float64 `json:"alt2"` // a minute later, to move it smoothly
	Az2    float64 `json:"az2"`
	Mag    float64 `json:"mag"`
	Sunlit bool    `json:"sunlit"`
	Km     float64 `json:"km"`
}

// handleSatellitesNow is every tracked satellite above the horizon at a
// moment, with where it will be a minute later (the map moves it in between).
func (s *Server) handleSatellitesNow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
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
	out := []satNow{}
	cat := s.satCatalog(ctx)
	if cat != nil {
		for _, st := range cat.Sats {
			a, err := st.At(t, wh.Obs.Lat, wh.Obs.Lon, 0)
			if err != nil || a.Alt < -2 {
				continue
			}
			b, err := st.At(t.Add(time.Minute), wh.Obs.Lat, wh.Obs.Lon, 0)
			if err != nil {
				continue
			}
			out = append(out, satNow{Name: st.Name, Norad: st.NoradID, Group: st.Group, Alt: round(a.Alt, 3), Az: round(a.Az, 3), Alt2: round(b.Alt, 3), Az2: round(b.Az, 3),
				Mag: round(a.Mag, 1), Sunlit: a.Sunlit, Km: math.Round(a.RangeKm)})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Mag < out[j].Mag })
	writeJSON(w, http.StatusOK, map[string]any{"t": t.UTC(), "satellites": out})
}

// satAlerts runs every minute: ten minutes before a visible pass of a space
// station (or a Starlink train) at home, one message.
func (s *Server) satAlerts(ctx context.Context) {
	ids, err := s.store.SettingsUsers(ctx)
	if err != nil {
		return
	}
	now := time.Now()
	for _, uid := range ids {
		st, err := s.store.GetSettings(ctx, uid)
		if err != nil || !st.NotifyEvents || !oneOf("satellite", st.EventKinds) {
			continue
		}
		home, err := s.store.HomeLocation(ctx, uid)
		if err != nil {
			continue
		}
		wh := whereLoc(home)
		for _, p := range s.passesAt(ctx, wh, 1) {
			if !p.Visible || p.From == nil || (p.Group != "station" && !p.Train) || p.Mag > 1.5 {
				continue
			}
			lead := p.From.Sub(now)
			if lead < 8*time.Minute || lead > 11*time.Minute {
				continue
			}
			key := fmt.Sprintf("sat:%d:%s", p.Norad, p.From.UTC().Format("200601021504"))
			if ok, _ := s.store.MarkSent(ctx, uid, key); !ok {
				continue
			}
			title := p.Name + " in 10 minutes"
			if p.Name == "ISS (ZARYA)" {
				title = "The space station in 10 minutes"
			}
			s.publish(ctx, uid, "sky.event", key, map[string]any{"kind": "satellite", "title": title, "at": p.From, "detail": p.Words, "place": home.Name})
			s.sendMessage(ctx, uid, st, title, p.Words, "info")
		}
	}
}

// satNames maps the catalogue's names to plain ones.
func satName(n string) string {
	switch n {
	case "ISS (ZARYA)":
		return "International Space Station"
	case "CSS (TIANHE)":
		return "Tiangong space station"
	}
	return n
}
