package server

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-sky/internal/forecast"
	"github.com/abjelosevic88/pocket-homelab-sky/internal/store"
)

// exportDoc is the JSON export of one person's sky: places, the observing
// log with what was seen, and the want-to-see list.
type exportDoc struct {
	App          string              `json:"app"`
	Version      string              `json:"version"`
	Exported     string              `json:"exported_at"`
	Locations    []store.Location    `json:"locations"`
	Observations []store.Observation `json:"observations"`
	Targets      []store.Target      `json:"targets"`
}

func (s *Server) export(ctx context.Context, uid int64) exportDoc {
	doc := exportDoc{App: "pocket-sky", Version: Version, Exported: store.Now()}
	doc.Locations, _ = s.store.ListLocations(ctx, uid)
	doc.Observations, _ = s.store.ListObservations(ctx, uid, store.ObsFilter{})
	doc.Targets, _ = s.store.ListTargets(ctx, uid)
	return doc
}

func (s *Server) handleExportJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Disposition", `attachment; filename="pocket-sky-export.json"`)
	writeJSON(w, http.StatusOK, s.export(r.Context(), actorID(r.Context())))
}

// handleExportCSV writes the observing log as CSV: one row per thing seen.
func (s *Server) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	obs, err := s.store.ListObservations(ctx, actorID(ctx), store.ObsFilter{})
	if err != nil {
		s.fail(w, err, "export")
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="pocket-sky-log.csv"`)
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"night", "started", "ended", "place", "title", "equipment", "seeing", "transparency", "cloud_pct", "moon_lit_pct", "bortle", "rating", "object", "object_key", "object_note", "notes"})
	i := func(n int) string {
		if n == 0 {
			return ""
		}
		return strconv.Itoa(n)
	}
	for _, o := range obs {
		cloud, moon := "", ""
		if o.Cloud != nil {
			cloud = strconv.Itoa(*o.Cloud)
		}
		if o.MoonPhase != nil {
			moon = strconv.Itoa(int(*o.MoonPhase*100 + 0.5))
		}
		base := []string{o.Night, o.StartedAt, o.EndedAt, o.Place, o.Title, o.Equipment, i(o.Seeing), i(o.Transparency), cloud, moon, i(o.Bortle), i(o.Rating)}
		if len(o.Objects) == 0 {
			_ = cw.Write(append(base, "", "", "", o.Notes))
			continue
		}
		for k, x := range o.Objects {
			n := ""
			if k == 0 {
				n = o.Notes
			}
			_ = cw.Write(append(append([]string{}, base...), x.Name, x.Key, x.Note, n))
		}
	}
	cw.Flush()
}

// handleImport takes a Pocket Sky export back: places (matched by name and
// spot), log entries (matched by night, start, title and place) and the
// want-to-see list, so importing twice adds nothing.
func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	body, err := s.readBody(r)
	if err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "the file is too large")
		return
	}
	var doc exportDoc
	if err := json.Unmarshal(body, &doc); err != nil || doc.App != "pocket-sky" {
		writeErr(w, http.StatusBadRequest, "that is not a Pocket Sky export (.json)")
		return
	}
	res, err := s.importDoc(ctx, uid, doc)
	if err != nil {
		s.fail(w, err, "import")
		return
	}
	s.afterPlaces(r, uid)
	writeJSON(w, http.StatusOK, res)
}

func obsFingerprint(o store.Observation) string {
	return strings.ToLower(strings.Join([]string{o.Night, o.StartedAt, strings.TrimSpace(o.Title), strings.TrimSpace(o.Place)}, "|"))
}

func (s *Server) importDoc(ctx context.Context, uid int64, doc exportDoc) (map[string]int, error) {
	res := map[string]int{"places": 0, "observations": 0, "targets": 0, "already": 0}
	have, err := s.store.ListLocations(ctx, uid)
	if err != nil {
		return nil, err
	}
	ids := map[int64]int64{} // id in the file → id here
	for _, in := range doc.Locations {
		if in.Name == "" || in.Lat < -90 || in.Lat > 90 || in.Lon < -180 || in.Lon > 180 {
			continue
		}
		found := int64(0)
		for _, l := range have {
			if l.Name == in.Name && forecast.Distance(l.Lat, l.Lon, in.Lat, in.Lon) < 1 {
				found = l.ID
			}
		}
		if found > 0 {
			ids[in.ID] = found
			res["already"]++
			continue
		}
		l := in
		l.ID, l.UserID, l.Source, l.SourceRef = 0, uid, "import", ""
		if l.Bortle < 0 || l.Bortle > 9 {
			l.Bortle = 0
		}
		if err := s.store.CreateLocation(ctx, &l); err != nil {
			return nil, err
		}
		have = append(have, l)
		ids[in.ID] = l.ID
		res["places"]++
	}
	old, err := s.store.ListObservations(ctx, uid, store.ObsFilter{})
	if err != nil {
		return nil, err
	}
	known := map[string]bool{}
	for _, o := range old {
		known[obsFingerprint(o)] = true
	}
	for _, in := range doc.Observations {
		if !validDate(in.Night) || in.Night == "" || known[obsFingerprint(in)] {
			if in.Night != "" {
				res["already"]++
			}
			continue
		}
		o := in
		o.ID, o.UserID, o.Source, o.SourceRef = 0, uid, "import", ""
		o.LocationID = nil
		if in.LocationID != nil {
			if id, ok := ids[*in.LocationID]; ok {
				o.LocationID = &id
			}
		}
		if !validLocal(o.StartedAt) {
			o.StartedAt = ""
		}
		if !validLocal(o.EndedAt) {
			o.EndedAt = ""
		}
		in2 := make([]objectIn, 0, len(in.Objects))
		for _, x := range in.Objects {
			in2 = append(in2, objectIn{Key: x.Key, Name: x.Name, Note: x.Note})
		}
		objs, msg := cleanObjects(in2)
		if msg != "" {
			// unknown keys from a newer version: keep them as plain names
			objs = objs[:0]
			for _, x := range in.Objects {
				objs = append(objs, store.ObsObject{Key: "other:" + strings.ToLower(x.Name), Name: x.Name, Note: x.Note})
			}
		}
		o.Objects = objs
		if err := s.store.CreateObservation(ctx, &o); err != nil {
			return nil, err
		}
		known[obsFingerprint(o)] = true
		res["observations"]++
	}
	for _, t := range doc.Targets {
		t := t
		if t.Key == "" || t.Name == "" {
			continue
		}
		if err := s.store.AddTarget(ctx, uid, &t); err == nil {
			res["targets"]++
		} else {
			res["already"]++
		}
	}
	return res, nil
}
