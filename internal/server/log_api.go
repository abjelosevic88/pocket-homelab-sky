package server

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-sky/internal/astro"
	"github.com/abjelosevic88/pocket-homelab-sky/internal/catalog"
	"github.com/abjelosevic88/pocket-homelab-sky/internal/pocket"
	"github.com/abjelosevic88/pocket-homelab-sky/internal/store"
)

// ---- the observing log ----

type objectIn struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	Note string `json:"note"`
}

type observationIn struct {
	LocationID   *int64      `json:"location_id"`
	Place        *string     `json:"place"`
	Lat          *float64    `json:"lat"`
	Lon          *float64    `json:"lon"`
	Night        *string     `json:"night"`
	StartedAt    *string     `json:"started_at"`
	EndedAt      *string     `json:"ended_at"`
	Title        *string     `json:"title"`
	Notes        *string     `json:"notes"`
	Seeing       *int        `json:"seeing"`
	Transparency *int        `json:"transparency"`
	Cloud        *int        `json:"cloud"`
	Bortle       *int        `json:"bortle"`
	Equipment    *string     `json:"equipment"`
	Rating       *int        `json:"rating"`
	Objects      *[]objectIn `json:"objects"`
	SourceRef    *string     `json:"source_ref"`
}

func validLocal(v string) bool {
	if v == "" {
		return true
	}
	_, err := time.Parse("2006-01-02T15:04", v)
	return err == nil
}

// cleanObjects checks the objects: a known key gets its catalogue name, an
// unknown one must be "other:" with a name.
func cleanObjects(in []objectIn) ([]store.ObsObject, string) {
	out := []store.ObsObject{}
	seen := map[string]bool{}
	for _, x := range in {
		k := strings.TrimSpace(x.Key)
		name := strings.TrimSpace(x.Name)
		if k == "" && name != "" {
			k = "other:" + strings.ToLower(name)
		}
		if strings.HasPrefix(k, "other:") || strings.HasPrefix(k, "event:") {
			if name == "" {
				name = strings.TrimSpace(strings.SplitN(k, ":", 2)[1])
			}
		} else if o, ok := objectFor(k); ok {
			name = o.Name
		} else if isMinorKey(k) && name != "" {
			// a comet or asteroid not loaded yet: keep the name it came with
		} else {
			return nil, "unknown object " + k
		}
		if name == "" || len(name) > 120 || len(x.Note) > 2000 {
			return nil, "each object needs a name (at most 120 characters) and a short note"
		}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, store.ObsObject{Key: k, Name: name, Note: strings.TrimSpace(x.Note)})
	}
	if len(out) > 300 {
		return nil, "at most 300 objects a night"
	}
	return out, ""
}

func (s *Server) applyObservation(ctx context.Context, uid int64, in observationIn, o *store.Observation) string {
	if in.LocationID != nil {
		if *in.LocationID <= 0 {
			o.LocationID = nil
		} else {
			l, err := s.store.GetLocation(ctx, uid, *in.LocationID)
			if err != nil {
				return "unknown place"
			}
			o.LocationID = &l.ID
			o.Place, o.Lat, o.Lon = l.Label(), &l.Lat, &l.Lon
			if o.Bortle == 0 {
				o.Bortle = l.Bortle
			}
		}
	}
	trimP(&o.Place, in.Place)
	if in.Lat != nil && in.Lon != nil {
		o.Lat, o.Lon = in.Lat, in.Lon
	}
	trimP(&o.Night, in.Night)
	trimP(&o.StartedAt, in.StartedAt)
	trimP(&o.EndedAt, in.EndedAt)
	trimP(&o.Title, in.Title)
	trimP(&o.Notes, in.Notes)
	trimP(&o.Equipment, in.Equipment)
	set := func(dst *int, v *int) {
		if v != nil {
			*dst = *v
		}
	}
	set(&o.Seeing, in.Seeing)
	set(&o.Transparency, in.Transparency)
	set(&o.Bortle, in.Bortle)
	set(&o.Rating, in.Rating)
	if in.Cloud != nil {
		if *in.Cloud < 0 {
			o.Cloud = nil
		} else {
			c := *in.Cloud
			o.Cloud = &c
		}
	}
	if o.Night == "" && o.StartedAt != "" && validLocal(o.StartedAt) {
		t, _ := time.Parse("2006-01-02T15:04", o.StartedAt)
		if t.Hour() < 12 {
			t = t.AddDate(0, 0, -1)
		}
		o.Night = t.Format("2006-01-02")
	}
	switch {
	case o.Night == "" || !validDate(o.Night):
		return "the night is a date (YYYY-MM-DD)"
	case !validLocal(o.StartedAt) || !validLocal(o.EndedAt):
		return "times are YYYY-MM-DDTHH:MM"
	case o.Seeing < 0 || o.Seeing > 5 || o.Transparency < 0 || o.Transparency > 5 || o.Rating < 0 || o.Rating > 5:
		return "seeing, transparency and the rating are 0 to 5"
	case o.Bortle < 0 || o.Bortle > 9:
		return "the darkness of the sky is 1 to 9 (or not known)"
	case o.Cloud != nil && (*o.Cloud < 0 || *o.Cloud > 100):
		return "cloud is 0 to 100%"
	case len(o.Title) > 200 || len(o.Place) > 200 || len(o.Equipment) > 200:
		return "the title, place and equipment are at most 200 characters"
	case len(o.Notes) > 20000:
		return "the notes are at most 20,000 characters"
	case o.Lat != nil && (*o.Lat < -90 || *o.Lat > 90 || o.Lon == nil || *o.Lon < -180 || *o.Lon > 180):
		return "latitude is −90 to 90 and longitude −180 to 180"
	}
	// The Moon that night, worked out.
	if o.Lat != nil && o.Lon != nil {
		tz := time.Local
		if o.LocationID != nil {
			if l, err := s.store.GetLocation(ctx, uid, *o.LocationID); err == nil {
				tz = loadTZ(l.Timezone)
			}
		}
		if n, err := astro.NightAt(astro.Observer{Lat: *o.Lat, Lon: *o.Lon}, o.Night, tz); err == nil {
			p := math.Round(n.MoonPhase*100) / 100
			o.MoonPhase = &p
		}
	} else {
		d, _ := time.Parse("2006-01-02", o.Night)
		p := math.Round(astro.Body("moon", d.Add(22*time.Hour)).Phase*100) / 100
		o.MoonPhase = &p
	}
	return ""
}

func (s *Server) handleListObservations(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.ObsFilter{From: q.Get("from"), To: q.Get("to"), Q: q.Get("q"), Object: q.Get("object"), Limit: queryInt(r, "limit")}
	if !validDate(f.From) || !validDate(f.To) {
		writeErr(w, http.StatusBadRequest, "from and to are YYYY-MM-DD")
		return
	}
	list, err := s.store.ListObservations(r.Context(), actorID(r.Context()), f)
	if err != nil {
		s.fail(w, err, "log")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleGetObservation(w http.ResponseWriter, r *http.Request) {
	o, err := s.store.GetObservation(r.Context(), actorID(r.Context()), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "log entry")
		return
	}
	writeJSON(w, http.StatusOK, o)
}

func (s *Server) handleCreateObservation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in observationIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	o := &store.Observation{UserID: uid}
	if msg := s.applyObservation(ctx, uid, in, o); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if in.Objects != nil {
		objs, msg := cleanObjects(*in.Objects)
		if msg != "" {
			writeErr(w, http.StatusBadRequest, msg)
			return
		}
		o.Objects = objs
	}
	if app := linkApp(ctx); app != "" {
		o.Source = app
		if in.SourceRef != nil {
			o.SourceRef = strings.TrimSpace(*in.SourceRef)
		}
		if o.SourceRef != "" {
			if old, err := s.store.ObservationBySource(ctx, uid, app, o.SourceRef); err == nil {
				writeJSON(w, http.StatusOK, old)
				return
			}
		}
	}
	if err := s.store.CreateObservation(ctx, o); err != nil {
		s.fail(w, err, "log entry")
		return
	}
	s.observationLogged(ctx, uid, o)
	writeJSON(w, http.StatusCreated, o)
}

// observationLogged tells the Core bus about a new log entry.
func (s *Server) observationLogged(ctx context.Context, uid int64, o *store.Observation) {
	names := make([]string, 0, len(o.Objects))
	for _, x := range o.Objects {
		names = append(names, x.Name)
	}
	s.publish(ctx, uid, "sky.observation.logged", strconv.FormatInt(o.ID, 10), map[string]any{"id": o.ID, "night": o.Night, "title": o.Title, "place": o.Place, "objects": names, "source": o.Source})
}

func (s *Server) handleUpdateObservation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	o, err := s.store.GetObservation(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "log entry")
		return
	}
	if app := linkApp(ctx); app != "" && o.Source != app {
		writeErr(w, http.StatusForbidden, "this log entry was not added by "+app)
		return
	}
	var in observationIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if msg := s.applyObservation(ctx, uid, in, o); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	var objs []store.ObsObject
	if in.Objects != nil {
		var msg string
		if objs, msg = cleanObjects(*in.Objects); msg != "" {
			writeErr(w, http.StatusBadRequest, msg)
			return
		}
	}
	if err := s.store.UpdateObservation(ctx, o, objs); err != nil {
		s.fail(w, err, "log entry")
		return
	}
	writeJSON(w, http.StatusOK, o)
}

func (s *Server) handleDeleteObservation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	o, err := s.store.GetObservation(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "log entry")
		return
	}
	if app := linkApp(ctx); app != "" && o.Source != app {
		writeErr(w, http.StatusForbidden, "this log entry was not added by "+app)
		return
	}
	if err := s.store.DeleteObservation(ctx, uid, o.ID); err != nil {
		s.fail(w, err, "log entry")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleEquipment is what the person has used before, for the form.
func (s *Server) handleEquipment(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.Equipment(r.Context(), actorID(r.Context()))
	if err != nil {
		s.fail(w, err, "equipment")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// ---- the want-to-see list ----

func (s *Server) handleListTargets(w http.ResponseWriter, r *http.Request) {
	ts, err := s.store.ListTargets(r.Context(), actorID(r.Context()))
	if err != nil {
		s.fail(w, err, "list")
		return
	}
	writeJSON(w, http.StatusOK, ts)
}

func (s *Server) handleCreateTarget(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		Key  string `json:"key"`
		Name string `json:"name"`
		Note string `json:"note"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	objs, msg := cleanObjects([]objectIn{{Key: in.Key, Name: in.Name, Note: in.Note}})
	if msg != "" || len(objs) != 1 {
		writeErr(w, http.StatusBadRequest, firstNonEmpty(msg, "say what you want to see"))
		return
	}
	t := &store.Target{Key: objs[0].Key, Name: objs[0].Name, Note: objs[0].Note}
	if err := s.store.AddTarget(ctx, actorID(ctx), t); err != nil {
		if errors.Is(err, store.ErrConflict) {
			writeErr(w, http.StatusConflict, t.Name+" is already on your list")
			return
		}
		s.fail(w, err, "list")
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) handleUpdateTarget(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		Note string `json:"note"`
	}
	if err := decode(r, &in); err != nil || len(in.Note) > 2000 {
		writeErr(w, http.StatusBadRequest, "a short note")
		return
	}
	uid := actorID(ctx)
	if err := s.store.UpdateTarget(ctx, uid, idParam(r, "id"), strings.TrimSpace(in.Note)); err != nil {
		s.fail(w, err, "list")
		return
	}
	t, err := s.store.GetTarget(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "list")
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleDeleteTarget(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteTarget(r.Context(), actorID(r.Context()), idParam(r, "id")); err != nil {
		s.fail(w, err, "list")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- progress: the Messier list, the planets, the constellations ----

type progressItem struct {
	Key     string   `json:"key"`
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Con     string   `json:"con"`
	Mag     *float64 `json:"mag"`
	Seen    string   `json:"seen"` // first night, "" = not yet
	Times   int      `json:"times"`
	Sub     string   `json:"sub,omitempty"`
	Visible bool     `json:"visible_tonight"`
}

func (s *Server) handleProgress(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	seen, err := s.store.SeenObjects(ctx, uid)
	if err != nil {
		s.fail(w, err, "progress")
		return
	}
	stats, _ := s.store.Stats(ctx, uid)
	// what is up tonight at home, to suggest the next ones
	up := map[string]bool{}
	if wh, err := s.whereOf(r, uid); err == nil {
		if n, err := astro.NightAt(wh.Obs, nightDate(time.Now(), wh.TZ), wh.TZ); err == nil && n.DarkKind != "none" {
			prec := astro.Precession(n.DarkFrom)
			for _, m := range append(append([]catalog.DSO{}, catalog.Messier()...), catalog.Highlights()...) {
				ra, dec := astro.RADec(prec.Apply(astro.Vec(m.RA, m.Dec)))
				if _, a := astro.Culmination(astro.FixedAlt(ra, dec, wh.Obs), n.DarkFrom, n.DarkTo); a > 25 {
					up["dso:"+m.ID] = true
				}
			}
			for _, p := range planetsTonight(wh.Obs, n, wh.TZ) {
				up[p.Key] = p.Visible
			}
		}
	}
	item := func(o catalog.Object) progressItem {
		v := seen[o.Key]
		return progressItem{Key: o.Key, Name: o.Name, Type: o.Type, Con: o.Con, Mag: o.Mag, Seen: v.First, Times: v.Times, Visible: up[o.Key]}
	}
	count := func(list []progressItem) int {
		n := 0
		for _, x := range list {
			if x.Seen != "" {
				n++
			}
		}
		return n
	}
	messier := []progressItem{}
	for _, m := range catalog.Messier() {
		messier = append(messier, item(catalog.DSOObject(m)))
	}
	high := []progressItem{}
	for _, h := range catalog.Highlights() {
		high = append(high, item(catalog.DSOObject(h)))
	}
	planets := []progressItem{}
	for _, b := range append([]string{"moon"}, astro.Planets...) {
		o, _ := objectFor(b)
		planets = append(planets, item(o))
	}
	cons := []progressItem{}
	for _, c := range catalog.Constellations() {
		cons = append(cons, item(catalog.ConObject(c)))
	}
	sort.SliceStable(cons, func(i, j int) bool { return cons[i].Name < cons[j].Name })
	writeJSON(w, http.StatusOK, map[string]any{
		"stats":          stats,
		"messier":        map[string]any{"seen": count(messier), "total": len(messier), "items": messier},
		"highlights":     map[string]any{"seen": count(high), "total": len(high), "items": high},
		"planets":        map[string]any{"seen": count(planets), "total": len(planets), "items": planets},
		"constellations": map[string]any{"seen": count(cons), "total": len(cons), "items": cons},
	})
}

// ---- to the other Pocket apps ----

// handleAddToCalendar puts an event or an observing night on the person's
// calendar in Pocket Calendar.
func (s *Server) handleAddToCalendar(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Start       string `json:"start"` // local YYYY-MM-DDTHH:MM, or a date
		End         string `json:"end"`
		Location    string `json:"location"`
		URL         string `json:"url"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" || len(in.Title) > 200 || (!validLocal(in.Start) && !validDate(in.Start)) || in.Start == "" || !validLocal(in.End) {
		writeErr(w, http.StatusBadRequest, "a title and a start (YYYY-MM-DDTHH:MM)")
		return
	}
	sib, err := s.clients(ctx, uid).conn(ctx, appCalendar, scopeCalendarWrite)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	id, err := sib.client.CreateEvent(ctx, pocketEvent(in.Title, in.Description, in.Location, in.URL, in.Start, in.End))
	if err != nil {
		writeErr(w, http.StatusBadGateway, "Pocket Calendar: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "event_id": id, "open_url": sib.public})
}

// handleObservationNote saves a log entry as a note in Pocket Notes.
func (s *Server) handleObservationNote(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	o, err := s.store.GetObservation(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "log entry")
		return
	}
	sib, err := s.clients(ctx, uid).conn(ctx, appNotes, scopeNotesWrite)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	title := o.Title
	if title == "" {
		title = "Under the sky, " + o.Night
	}
	n, err := sib.client.CreateNote(ctx, title, "Sky", observationMarkdown(o))
	if err != nil {
		writeErr(w, http.StatusBadGateway, "Pocket Notes: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "note_id": n.ID, "open_url": sib.public})
}

func stars(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat("★", n) + strings.Repeat("☆", 5-n)
}

// observationMarkdown is a log entry as Markdown.
func observationMarkdown(o *store.Observation) string {
	var b strings.Builder
	fmt.Fprintf(&b, "**Night of %s**", o.Night)
	if o.Place != "" {
		fmt.Fprintf(&b, " at %s", o.Place)
	}
	b.WriteString("\n\n")
	if o.StartedAt != "" {
		fmt.Fprintf(&b, "- Time: %s", strings.Replace(o.StartedAt, "T", " ", 1))
		if o.EndedAt != "" {
			fmt.Fprintf(&b, " – %s", o.EndedAt[strings.Index(o.EndedAt, "T")+1:])
		}
		b.WriteString("\n")
	}
	if o.Equipment != "" {
		fmt.Fprintf(&b, "- Equipment: %s\n", o.Equipment)
	}
	if o.Seeing > 0 || o.Transparency > 0 {
		fmt.Fprintf(&b, "- Seeing %d/5, transparency %d/5\n", o.Seeing, o.Transparency)
	}
	if o.Cloud != nil {
		fmt.Fprintf(&b, "- Cloud: %d%%\n", *o.Cloud)
	}
	if o.MoonPhase != nil {
		fmt.Fprintf(&b, "- Moon: %.0f%% lit\n", *o.MoonPhase*100)
	}
	if o.Rating > 0 {
		fmt.Fprintf(&b, "- Night: %s\n", stars(o.Rating))
	}
	if len(o.Objects) > 0 {
		b.WriteString("\n**Seen**\n\n")
		for _, x := range o.Objects {
			if x.Note != "" {
				fmt.Fprintf(&b, "- %s: %s\n", x.Name, x.Note)
			} else {
				fmt.Fprintf(&b, "- %s\n", x.Name)
			}
		}
	}
	if o.Notes != "" {
		b.WriteString("\n" + o.Notes + "\n")
	}
	b.WriteString("\n*From Pocket Sky*\n")
	return b.String()
}

// handleImportPlaces adds the places of Pocket Weather or Pocket Places as
// places to watch the sky from (once each; run again to pick up new ones).
func (s *Server) handleImportPlaces(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in struct {
		From string  `json:"from"` // weather | places
		IDs  []int64 `json:"ids"`  // only these (empty = list them, add nothing)
		All  bool    `json:"all"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	a, ok := appBySlug(in.From)
	if !ok || (a.slug != "weather" && a.slug != "places") {
		writeErr(w, http.StatusBadRequest, "from is weather or places")
		return
	}
	sib, err := s.clients(ctx, uid).conn(ctx, a, a.scopes...)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	var spots []pocketSpot
	if a.slug == "weather" {
		list, err := sib.client.WeatherPlaces(ctx)
		if err != nil {
			writeErr(w, http.StatusBadGateway, a.name+": "+err.Error())
			return
		}
		for _, p := range list {
			spots = append(spots, pocketSpot{p.ID, p.Name, firstNonEmpty(p.Region, p.City), p.Country, p.Lat, p.Longitude(), p.Elevation, p.Timezone})
		}
	} else {
		list, err := sib.client.SavedPlaces(ctx)
		if err != nil {
			writeErr(w, http.StatusBadGateway, a.name+": "+err.Error())
			return
		}
		for _, p := range list {
			spots = append(spots, pocketSpot{p.ID, p.Name, p.City, p.Country, p.Lat, p.Longitude(), p.Elevation, p.Timezone})
		}
	}
	want := map[int64]bool{}
	for _, id := range in.IDs {
		want[id] = true
	}
	type row struct {
		pocketSpot
		Added bool `json:"added"`
		Have  bool `json:"have"`
	}
	out := []row{}
	added := 0
	for _, sp := range spots {
		if sp.Lat == nil || sp.Lon == nil || (*sp.Lat == 0 && *sp.Lon == 0) {
			continue
		}
		ref := strconv.FormatInt(sp.ID, 10)
		_, err := s.store.LocationBySource(ctx, uid, a.slug, ref)
		rw := row{pocketSpot: sp, Have: err == nil}
		if !rw.Have && (in.All || want[sp.ID]) {
			l := &store.Location{UserID: uid, Name: sp.Name, Region: sp.Region, Country: sp.Country, Lat: *sp.Lat, Lon: *sp.Lon, Elevation: sp.Elevation,
				Timezone: sp.Timezone, Alerts: false, Source: a.slug, SourceRef: ref}
			if l.Timezone == "" {
				if f, _, err := s.forecastAt(ctx, l.Lat, l.Lon); err == nil {
					l.Timezone = f.Timezone
				}
			}
			if err := s.store.CreateLocation(ctx, l); err == nil {
				rw.Added, rw.Have = true, true
				added++
			}
		}
		out = append(out, rw)
	}
	if added > 0 {
		s.afterPlaces(r, uid)
	}
	writeJSON(w, http.StatusOK, map[string]any{"places": out, "added": added})
}

type pocketSpot struct {
	ID        int64    `json:"id"`
	Name      string   `json:"name"`
	Region    string   `json:"region"`
	Country   string   `json:"country"`
	Lat       *float64 `json:"lat"`
	Lon       *float64 `json:"lon"`
	Elevation *float64 `json:"elevation"`
	Timezone  string   `json:"timezone"`
}

func pocketEvent(title, desc, loc, url, start, end string) pocket.Event {
	return pocket.Event{Title: title, Description: desc, Location: loc, URL: url, Start: start, End: end, Minutes: 60}
}
