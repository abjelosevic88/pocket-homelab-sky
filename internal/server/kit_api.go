package server

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-sky/internal/astro"
	"github.com/abjelosevic88/pocket-homelab-sky/internal/catalog"
	"github.com/abjelosevic88/pocket-homelab-sky/internal/store"
)

// ---- equipment ----

func validKit(k *store.Kit) string {
	k.Name = strings.TrimSpace(k.Name)
	switch {
	case !oneOf(k.Kind, []string{"binoculars", "telescope", "eyepiece", "camera"}):
		return "the kind is binoculars, telescope, eyepiece or camera"
	case k.Name == "" || len(k.Name) > 120:
		return "a name of up to 120 characters"
	case k.Aperture < 0 || k.Aperture > 2000 || k.Focal < 0 || k.Focal > 20000 || k.AFOV < 0 || k.AFOV > 130 || k.TFOV < 0 || k.TFOV > 20 || k.Power < 0 || k.Power > 200 || k.SensorW < 0 || k.SensorW > 100 || k.SensorH < 0 || k.SensorH > 100:
		return "a number is out of range"
	case k.Kind == "binoculars" && (k.Power <= 0 || k.Aperture <= 0):
		return "binoculars need the magnification and the lens size (10×50: 10 and 50)"
	case k.Kind == "telescope" && (k.Aperture <= 0 || k.Focal <= 0):
		return "a telescope needs its aperture and focal length in mm"
	case k.Kind == "eyepiece" && k.Focal <= 0:
		return "an eyepiece needs its focal length in mm"
	case k.Kind == "camera" && (k.SensorW <= 0 || k.SensorH <= 0):
		return "a camera needs its sensor size in mm (full frame 36 × 24)"
	case len(k.Notes) > 2000:
		return "the notes are at most 2,000 characters"
	}
	if k.Kind == "binoculars" && k.TFOV == 0 {
		// a typical 50–55° apparent field
		k.TFOV = math.Round(52/k.Power*10) / 10
	}
	if k.Kind == "eyepiece" && k.AFOV == 0 {
		k.AFOV = 52
	}
	return ""
}

func (s *Server) handleListKit(w http.ResponseWriter, r *http.Request) {
	ks, err := s.store.ListKit(r.Context(), actorID(r.Context()))
	if err != nil {
		s.fail(w, err, "equipment")
		return
	}
	writeJSON(w, http.StatusOK, ks)
}

func (s *Server) handleSaveKit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var k store.Kit
	if err := decode(r, &k); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	k.ID = idParam(r, "id")
	if msg := validKit(&k); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.store.SaveKit(ctx, actorID(ctx), &k); err != nil {
		s.fail(w, err, "equipment")
		return
	}
	writeJSON(w, http.StatusOK, k)
}

func (s *Server) handleDeleteKit(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteKit(r.Context(), actorID(r.Context()), idParam(r, "id")); err != nil {
		s.fail(w, err, "equipment")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- observing plans ----

type planItemView struct {
	store.PlanItem
	Type    string     `json:"type"`
	Mag     *float64   `json:"mag"`
	Con     string     `json:"con_name"`
	BestAt  *time.Time `json:"best_at"`
	BestAlt float64    `json:"best_alt"`
	Dir     string     `json:"dir"`
	UpFrom  *time.Time `json:"up_from"`
	UpTo    *time.Time `json:"up_to"`
	Words   string     `json:"words"`
}

type planView struct {
	*store.Plan
	Items []planItemView `json:"items"`
	Place map[string]any `json:"place"`
	Night *astro.Night   `json:"night"`
}

// timeItems works out when each item is best on the plan's night.
func (s *Server) timeItems(ctx context.Context, uid int64, p *store.Plan) planView {
	v := planView{Plan: p, Items: []planItemView{}}
	var wh *where
	if p.LocationID != nil {
		if l, err := s.store.GetLocation(ctx, uid, *p.LocationID); err == nil {
			w := whereLoc(l)
			wh = &w
		}
	}
	if wh == nil {
		if l, err := s.store.HomeLocation(ctx, uid); err == nil {
			w := whereLoc(l)
			wh = &w
		}
	}
	var n astro.Night
	var from, to time.Time
	if wh != nil {
		v.Place = wh.view()
		if nn, err := astro.NightAt(wh.Obs, p.Night, wh.TZ); err == nil {
			n = nn
			v.Night = &n
			from, to = nightWindow(n)
		}
	}
	for _, it := range p.Items {
		iv := planItemView{PlanItem: it}
		obj, ok := objectFor(it.Key)
		if ok {
			iv.Type, iv.Mag, iv.Con = obj.Type, obj.Mag, catalog.ConName(obj.Con)
		}
		if ok && wh != nil && to.After(from) {
			var f astro.AltFunc
			if _, body := astro.Names[it.Key]; body {
				f = astro.BodyAlt(it.Key, wh.Obs)
			} else {
				ra, dec := astro.OfDate(obj.RA, obj.Dec, from)
				f = astro.FixedAlt(ra, dec, wh.Obs)
			}
			best, alt := astro.Culmination(f, from, to)
			_, az := objAltAz(it.Key, obj, wh.Obs, best)
			iv.BestAt, iv.BestAlt, iv.Dir = &best, math.Round(alt), Direction(az)
			var upF, upT time.Time
			for t := from; !t.After(to); t = t.Add(10 * time.Minute) {
				if f(t) > 20 {
					if upF.IsZero() {
						upF = t
					}
					upT = t
				}
			}
			if !upF.IsZero() {
				iv.UpFrom, iv.UpTo = &upF, &upT
				iv.Words = fmt.Sprintf("Best at %s, %s in the %s; well up from %s to %s.", best.In(wh.TZ).Format("15:04"), heightWords(alt), iv.Dir, upF.In(wh.TZ).Format("15:04"), upT.In(wh.TZ).Format("15:04"))
			} else if alt > 0 {
				iv.Words = fmt.Sprintf("Only low that night: at most %.0f° up in the %s around %s.", alt, iv.Dir, best.In(wh.TZ).Format("15:04"))
			} else {
				iv.Words = "Not up during the dark hours that night."
			}
		}
		v.Items = append(v.Items, iv)
	}
	return v
}

type planIn struct {
	Name       string     `json:"name"`
	Night      string     `json:"night"`
	LocationID *int64     `json:"location_id"`
	Setup      string     `json:"setup"`
	Notes      string     `json:"notes"`
	Items      []objectIn `json:"items"`
	Done       []string   `json:"done"` // keys ticked off
}

func (s *Server) handleListPlans(w http.ResponseWriter, r *http.Request) {
	ps, err := s.store.ListPlans(r.Context(), actorID(r.Context()))
	if err != nil {
		s.fail(w, err, "plans")
		return
	}
	writeJSON(w, http.StatusOK, ps)
}

func (s *Server) handleGetPlan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	p, err := s.store.GetPlan(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "plan")
		return
	}
	writeJSON(w, http.StatusOK, s.timeItems(ctx, uid, p))
}

func (s *Server) handleSavePlan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in planIn
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	p := &store.Plan{ID: idParam(r, "id")}
	if p.ID > 0 {
		if p, err = s.store.GetPlan(ctx, uid, p.ID); err != nil {
			s.fail(w, err, "plan")
			return
		}
	} else {
		p.Name = "Observing plan"
		if wh, err := s.whereOf(r, uid); err == nil {
			p.Night = nightDate(time.Now(), wh.TZ)
		} else {
			p.Night = time.Now().Format("2006-01-02")
		}
	}
	if keys["name"] {
		p.Name = strings.TrimSpace(in.Name)
	}
	if keys["night"] {
		p.Night = in.Night
	}
	if keys["location_id"] {
		p.LocationID = nil
		if in.LocationID != nil && *in.LocationID > 0 {
			if _, err := s.store.GetLocation(ctx, uid, *in.LocationID); err != nil {
				writeErr(w, http.StatusBadRequest, "unknown place")
				return
			}
			p.LocationID = in.LocationID
		}
	}
	if keys["setup"] {
		p.Setup = strings.TrimSpace(in.Setup)
	}
	if keys["notes"] {
		p.Notes = strings.TrimSpace(in.Notes)
	}
	if p.Name == "" || len(p.Name) > 200 || !validDate(p.Night) || p.Night == "" || len(p.Notes) > 20000 || len(p.Setup) > 200 {
		writeErr(w, http.StatusBadRequest, "a name, a night (YYYY-MM-DD) and short notes")
		return
	}
	var items []store.PlanItem
	if keys["items"] {
		objs, msg := cleanObjects(in.Items)
		if msg != "" {
			writeErr(w, http.StatusBadRequest, msg)
			return
		}
		done := map[string]bool{}
		for _, k := range in.Done {
			done[k] = true
		}
		items = make([]store.PlanItem, 0, len(objs))
		for _, o := range objs {
			items = append(items, store.PlanItem{Key: o.Key, Name: o.Name, Note: o.Note, Done: done[o.Key]})
		}
	}
	if err := s.store.SavePlan(ctx, uid, p, items); err != nil {
		s.fail(w, err, "plan")
		return
	}
	writeJSON(w, http.StatusOK, s.timeItems(ctx, uid, p))
}

// handleOrderPlan puts the plan's items in the order to see them: by the time
// each is best that night (things not up go to the end).
func (s *Server) handleOrderPlan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	p, err := s.store.GetPlan(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "plan")
		return
	}
	v := s.timeItems(ctx, uid, p)
	sort.SliceStable(v.Items, func(i, j int) bool {
		a, b := v.Items[i], v.Items[j]
		if (a.UpFrom == nil) != (b.UpFrom == nil) {
			return a.UpFrom != nil
		}
		if a.UpFrom == nil {
			return false
		}
		// what sets first comes first; then what is best earlier
		if !a.UpTo.Equal(*b.UpTo) && math.Abs(a.UpTo.Sub(*b.UpTo).Hours()) > 1 {
			return a.UpTo.Before(*b.UpTo)
		}
		return a.BestAt.Before(*b.BestAt)
	})
	items := make([]store.PlanItem, len(v.Items))
	for i, it := range v.Items {
		items[i] = it.PlanItem
	}
	if err := s.store.SavePlan(ctx, uid, p, items); err != nil {
		s.fail(w, err, "plan")
		return
	}
	writeJSON(w, http.StatusOK, s.timeItems(ctx, uid, p))
}

func (s *Server) handleDeletePlan(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeletePlan(r.Context(), actorID(r.Context()), idParam(r, "id")); err != nil {
		s.fail(w, err, "plan")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleExportPlan writes a plan for other programs: SkySafari (.skylist),
// Stellarium's observing list (.json) or a spreadsheet (.csv).
func (s *Server) handleExportPlan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	p, err := s.store.GetPlan(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "plan")
		return
	}
	v := s.timeItems(ctx, uid, p)
	name := strings.Map(func(r rune) rune {
		if r == ' ' || r == '/' || r == '\\' {
			return '-'
		}
		return r
	}, p.Name)
	switch r.URL.Query().Get("format") {
	case "skysafari":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`.skylist"`)
		var b strings.Builder
		b.WriteString("SkySafariObservingListVersion=3.0\r\n")
		for _, it := range v.Items {
			b.WriteString("SkyObject=BeginObject\r\n")
			fmt.Fprintf(&b, "\tObjectID=%s\r\n", skySafariID(it.Key))
			fmt.Fprintf(&b, "\tCommonName=%s\r\n", it.Name)
			if cn := skySafariCatalog(it.Key); cn != "" {
				fmt.Fprintf(&b, "\tCatalogNumber=%s\r\n", cn)
			}
			if it.Note != "" {
				fmt.Fprintf(&b, "\tComment=%s\r\n", strings.ReplaceAll(it.Note, "\n", " "))
			}
			b.WriteString("EndObject=SkyObject\r\n")
		}
		_, _ = w.Write([]byte(b.String()))
	case "stellarium":
		objs := []map[string]any{}
		for _, it := range v.Items {
			o := map[string]any{"designation": stellariumName(it.Key, it.Name), "nameI18n": it.Name, "type": it.Type, "constellation": it.Con, "fov": 2, "jd": 0, "location": "", "magnitude": "", "isVisibleMarker": true}
			if obj, ok := objectFor(it.Key); ok && obj.Kind != "planet" && obj.Kind != "moon" {
				o["ra"], o["dec"] = raHMS(obj.RA), decDMS(obj.Dec)
			}
			if it.Mag != nil {
				o["magnitude"] = strconv.FormatFloat(*it.Mag, 'f', 1, 64)
			}
			objs = append(objs, o)
		}
		id := fmt.Sprintf("{%08x-0000-4000-8000-%012x}", p.ID, time.Now().Unix())
		doc := map[string]any{"defaultListOlud": id, "observingLists": map[string]any{id: map[string]any{"creation date": time.Now().Format("2006-01-02 15:04:05"), "description": p.Notes, "name": p.Name, "objects": objs, "sorting": ""}},
			"shortName": "Observing list for Stellarium", "version": "2.1"}
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`.json"`)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(doc)
	default:
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`.csv"`)
		cw := csv.NewWriter(w)
		_ = cw.Write([]string{"order", "object", "type", "constellation", "magnitude", "best time", "height", "direction", "note"})
		tz := time.Local
		if v.Place != nil {
			tz = loadTZ(fmt.Sprint(v.Place["timezone"]))
		}
		for i, it := range v.Items {
			mag, best := "", ""
			if it.Mag != nil {
				mag = strconv.FormatFloat(*it.Mag, 'f', 1, 64)
			}
			if it.BestAt != nil {
				best = it.BestAt.In(tz).Format("15:04")
			}
			_ = cw.Write([]string{strconv.Itoa(i + 1), it.Name, it.Type, it.Con, mag, best, strconv.Itoa(int(it.BestAlt)) + "°", it.Dir, it.Note})
		}
		cw.Flush()
	}
}

func skySafariID(key string) string {
	switch {
	case key == "sun" || key == "moon" || oneOf(key, astro.Planets):
		return "1," + map[string]string{"sun": "0", "mercury": "1", "venus": "2", "moon": "301", "mars": "4", "jupiter": "5", "saturn": "6", "uranus": "7", "neptune": "8"}[key] + ",-1"
	case strings.HasPrefix(key, "star:"):
		return "2,-1,-1"
	}
	return "4,-1,-1"
}

func skySafariCatalog(key string) string {
	switch {
	case strings.HasPrefix(key, "dso:M"):
		return "M " + strings.TrimPrefix(key, "dso:M")
	case strings.HasPrefix(key, "star:"):
		return "HIP " + strings.TrimPrefix(key, "star:")
	case strings.HasPrefix(key, "dso:"):
		if o, ok := objectFor(key); ok && o.Alt != "" {
			return strings.Split(o.Alt, " / ")[0]
		}
	}
	return ""
}

func stellariumName(key, name string) string {
	switch {
	case strings.HasPrefix(key, "dso:M"):
		return "M" + strings.TrimPrefix(key, "dso:M")
	case strings.HasPrefix(key, "star:"):
		return "HIP " + strings.TrimPrefix(key, "star:")
	}
	if _, ok := astro.Names[key]; ok {
		return astro.Names[key]
	}
	return name
}

func raHMS(deg float64) string {
	h := deg / 15
	hh := math.Floor(h)
	m := (h - hh) * 60
	mm := math.Floor(m)
	return fmt.Sprintf("%02.0fh%02.0fm%04.1fs", hh, mm, (m-mm)*60)
}

func decDMS(deg float64) string {
	sign := "+"
	if deg < 0 {
		sign, deg = "-", -deg
	}
	d := math.Floor(deg)
	m := (deg - d) * 60
	mm := math.Floor(m)
	return fmt.Sprintf("%s%02.0f°%02.0f'%04.1f\"", sign, d, mm, (m-mm)*60)
}
