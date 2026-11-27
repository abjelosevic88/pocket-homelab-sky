package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-sky/internal/astro"
	"github.com/abjelosevic88/pocket-homelab-sky/internal/catalog"
	"github.com/abjelosevic88/pocket-homelab-sky/internal/forecast"
	"github.com/abjelosevic88/pocket-homelab-sky/internal/space"
	"github.com/abjelosevic88/pocket-homelab-sky/internal/store"
)

// What Pocket Search, Pocket Day and Pocket Hub ask Sky (the capabilities
// value in links.go): one route each, in the shape every Pocket app answers
// in. They run as the request's person and read what /search, /day and
// Tonight read; only the capture adds something, one entry in the observing
// log. None of them asks an outside service: the sky is worked out here, and
// the cloud and northern-lights forecasts are used only when a copy is
// already kept.

// dots joins the parts of a line that are there: "Jupiter, Saturn · Home".
func dots(parts ...string) string {
	out := parts[:0:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " · ")
}

// nameList: "Jupiter", "Jupiter and Saturn", "Jupiter, Saturn, M31 and 2
// more": each name once, at most three of them.
func nameList(names []string) string {
	uniq := []string{}
	for _, n := range names {
		if n = strings.TrimSpace(n); n != "" && !oneOf(n, uniq) {
			uniq = append(uniq, n)
		}
	}
	if more := len(uniq) - 3; more > 0 {
		return strings.Join(uniq[:3], ", ") + " and " + strconv.Itoa(more) + " more"
	}
	switch len(uniq) {
	case 0:
		return ""
	case 1:
		return uniq[0]
	}
	return strings.Join(uniq[:len(uniq)-1], ", ") + " and " + uniq[len(uniq)-1]
}

// upperFirst starts a line with a capital letter.
func upperFirst(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	if n == 0 || r == utf8.RuneError || unicode.IsUpper(r) {
		return s
	}
	return string(unicode.ToUpper(r)) + s[n:]
}

// clip cuts a text to at most max bytes at a letter, with "…" when cut.
func clip(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := s[:max-len("…")]
	for !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return strings.TrimSpace(cut) + "…"
}

func thingsToSee(n int) string {
	if n == 0 {
		return ""
	}
	return strconv.Itoa(n) + " " + plural(n, "thing", "things") + " to see"
}

// seenNames are the names of what was seen on a night.
func seenNames(o store.Observation) []string {
	names := make([]string, 0, len(o.Objects))
	for _, x := range o.Objects {
		names = append(names, x.Name)
	}
	return names
}

// mapURL opens something of the sky on the map.
func mapURL(key string) string { return "/map?object=" + url.QueryEscape(key) }

// ---------- search ----------

// objectKinds are the catalogue's kinds in plain words.
var objectKinds = map[string]string{"planet": "planet", "moon": "moon", "sun": "star", "star": "star", "constellation": "constellation", "dso": "deep-sky object"}

// kitLine is a piece of equipment in a few words: "10× with 50 mm lenses".
func kitLine(k store.Kit) string {
	num := func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
	switch k.Kind {
	case "binoculars":
		return num(k.Power) + "× with " + num(k.Aperture) + " mm lenses"
	case "telescope":
		return num(k.Aperture) + " mm aperture, " + num(k.Focal) + " mm focal length"
	case "eyepiece":
		return num(k.Focal) + " mm"
	case "camera":
		return "Sensor " + num(k.SensorW) + " × " + num(k.SensorH) + " mm"
	}
	return ""
}

// searchable reports whether there is a letter or a digit to look for.
func searchable(q string) bool {
	return strings.IndexFunc(q, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) >= 0
}

// pocketSearch answers Pocket Search with what /search finds (the sky's
// objects by name and the observing log) plus the person's plans,
// want-to-see list and equipment, as one list: the object the words name,
// then the person's own things (a title that has the words before the ones
// found by their notes or what they hold), then the other objects.
func (s *Server) pocketSearch(r *http.Request, q string, limit int) ([]core.SearchItem, error) {
	ctx := r.Context()
	uid := actorID(ctx)
	if len([]rune(q)) < 2 || !searchable(q) {
		return nil, nil
	}
	fq := catalog.Fold(q)
	has := func(v ...string) bool { return strings.Contains(catalog.Fold(strings.Join(v, " ")), fq) }

	var objects []core.SearchItem
	for _, o := range searchObjects(q, limit) {
		objects = append(objects, core.SearchItem{ID: "object:" + o.Key, Title: o.Name, Snippet: strings.TrimSpace(o.Type + ". " + o.About),
			Kind: objectKinds[o.Kind], URL: mapURL(o.Key)})
	}

	var named, other []core.SearchItem // the person's own: by title, by the rest
	add := func(it core.SearchItem) {
		if has(it.Title) {
			named = append(named, it)
		} else {
			other = append(other, it)
		}
	}
	obs, err := s.store.ListObservations(ctx, uid, store.ObsFilter{Q: q, Limit: limit})
	if err != nil {
		return nil, err
	}
	for _, o := range obs {
		id := strconv.FormatInt(o.ID, 10)
		add(core.SearchItem{ID: "observation:" + id, Title: firstNonEmpty(o.Title, "Night of "+o.Night), Kind: "observation", Date: o.Night, URL: "/log/" + id,
			Snippet: dots(strings.Join(seenNames(o), ", "), o.Place, clip(o.Notes, 400))})
	}
	plans, err := s.store.FindPlans(ctx, uid, store.PlanFilter{Q: q, Limit: limit})
	if err != nil {
		return nil, err
	}
	for _, p := range plans {
		id := strconv.FormatInt(p.ID, 10)
		things := ""
		if full, err := s.store.GetPlan(ctx, uid, p.ID); err == nil {
			names := make([]string, 0, len(full.Items))
			for _, it := range full.Items {
				names = append(names, it.Name)
			}
			things = strings.Join(names, ", ")
		}
		add(core.SearchItem{ID: "plan:" + id, Title: p.Name, Kind: "plan", Date: p.Night, URL: "/plans/" + id, Snippet: dots(things, p.Setup, clip(p.Notes, 400))})
	}
	targets, err := s.store.ListTargets(ctx, uid)
	if err != nil {
		return nil, err
	}
	for _, t := range targets {
		if !has(t.Name, t.Note) {
			continue
		}
		it := core.SearchItem{ID: "target:" + strconv.FormatInt(t.ID, 10), Title: t.Name, Kind: "want to see", Date: t.CreatedAt, URL: mapURL(t.Key)}
		if strings.HasPrefix(t.Key, "other:") || strings.HasPrefix(t.Key, "event:") {
			it.URL = "/lists" // the person's own words: nothing on the map to open
		}
		if t.SeenOn != "" {
			it.Snippet = dots("First seen on "+t.SeenOn, t.Note)
		} else {
			it.Snippet = dots("Not seen yet", t.Note)
		}
		add(it)
	}
	kit, err := s.store.ListKit(ctx, uid)
	if err != nil {
		return nil, err
	}
	for _, k := range kit {
		if !has(k.Name, k.Kind, k.Notes) {
			continue
		}
		add(core.SearchItem{ID: "equipment:" + strconv.FormatInt(k.ID, 10), Title: k.Name, Kind: "equipment", Date: k.CreatedAt, URL: "/plans?tab=kit",
			Snippet: dots(upperFirst(k.Kind), kitLine(k), clip(k.Notes, 400))})
	}

	out := make([]core.SearchItem, 0, len(objects)+len(named)+len(other))
	if len(objects) > 0 {
		out = append(out, objects[0])
		objects = objects[1:]
	}
	out = append(append(append(out, named...), other...), objects...)
	return out, nil
}

// ---------- day ----------

// eventRank orders the events of a day by how rare they are; the day shows
// the first few.
var eventRank = map[string]int{"solar_eclipse": 0, "lunar_eclipse": 0, "meteor_shower": 1, "opposition": 2, "elongation": 3, "conjunction": 4, "season": 5, "moon_phase": 6}

const dayEvents = 3

// notableEvents picks the events worth a line in a journal: the ones that
// can be seen from the place, no quarter Moons, the rarest first, and then
// in the order they happen.
func notableEvents(evs []astro.Event, max int) []astro.Event {
	out := []astro.Event{}
	for _, e := range evs {
		if (e.Visible != nil && !*e.Visible) || (e.Kind == "moon_phase" && (e.Sub == "first" || e.Sub == "last")) {
			continue
		}
		out = append(out, e)
	}
	rank := func(e astro.Event) int {
		if r, ok := eventRank[e.Kind]; ok {
			return r
		}
		return len(eventRank)
	}
	sort.SliceStable(out, func(i, j int) bool { return rank(out[i]) < rank(out[j]) })
	if len(out) > max {
		out = out[:max]
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].T.Before(out[j].T) })
	return out
}

// clockOn is the time of day of a log entry's start or end ("2026-10-20T21:00")
// when it is on that date; a time after midnight belongs to the next day.
func clockOn(local, date string) string {
	if len(local) == 16 && local[:10] == date {
		return local[11:]
	}
	return ""
}

// pocketDay says what the sky was for the person on one date: the nights
// logged for it and what was seen, the plans made for that night, and up to
// three sky events of the day at their home place (none without a place).
// It reads what /day reads, without the forecast.
func (s *Server) pocketDay(r *http.Request, date string, tz *time.Location) (*core.Day, error) {
	ctx := r.Context()
	uid := actorID(ctx)
	obs, err := s.store.ListObservations(ctx, uid, store.ObsFilter{From: date, To: date, Limit: 100})
	if err != nil {
		return nil, err
	}
	plans, err := s.store.FindPlans(ctx, uid, store.PlanFilter{From: date, To: date, Limit: 50})
	if err != nil {
		return nil, err
	}
	var events []astro.Event
	if home, err := s.store.HomeLocation(ctx, uid); err == nil {
		wh := whereLoc(home)
		if tz == nil {
			tz = wh.TZ
		}
		if day, err := time.ParseInLocation("2006-01-02", date, tz); err == nil {
			events = notableEvents(s.eventsNear(ctx, wh, day, day.AddDate(0, 0, 1), nil), dayEvents)
		}
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	if len(obs)+len(plans)+len(events) == 0 {
		return nil, nil
	}

	day := &core.Day{Date: date}
	var seen, planned, inSky []string
	for _, o := range obs {
		id := strconv.FormatInt(o.ID, 10)
		names := seenNames(o)
		seen = append(seen, names...)
		it := core.DayItem{ID: "observation:" + id, Time: clockOn(o.StartedAt, date), Kind: "observed", URL: "/log/" + id, Note: clip(o.Notes, 2000)}
		if it.Time != "" {
			it.End = clockOn(o.EndedAt, date)
		}
		switch {
		case o.Title != "":
			it.Title, it.Sub = o.Title, dots(strings.Join(names, ", "), o.Place, o.Equipment)
		case len(names) > 0:
			it.Title, it.Sub = nameList(names), dots(o.Place, o.Equipment)
		default:
			it.Title, it.Sub = "A night under the sky", dots(o.Place, o.Equipment)
		}
		day.Items = append(day.Items, it)
	}
	for _, p := range plans {
		id := strconv.FormatInt(p.ID, 10)
		planned = append(planned, p.Name)
		day.Items = append(day.Items, core.DayItem{ID: "plan:" + id, Title: p.Name, Sub: dots(thingsToSee(len(p.Items)), p.Setup), Kind: "planned", URL: "/plans/" + id,
			Note: clip(p.Notes, 2000)})
	}
	for _, e := range events {
		inSky = append(inSky, e.Title)
		day.Items = append(day.Items, core.DayItem{ID: "event:" + e.Key, Time: e.T.In(tz).Format("15:04"), Title: e.Title, Sub: e.Detail, Kind: "event", URL: "/events"})
	}

	var parts []string
	switch what := nameList(seen); {
	case what != "":
		parts = append(parts, "observed "+what)
		day.Metrics = append(day.Metrics, core.Metric{Label: "Seen", Value: strconv.Itoa(countNames(seen))})
	case len(obs) == 1 && obs[0].Title != "":
		parts = append(parts, obs[0].Title)
	case len(obs) == 1:
		parts = append(parts, "a night under the sky")
	case len(obs) > 1:
		parts = append(parts, strconv.Itoa(len(obs))+" entries in the observing log")
	}
	if len(planned) > 0 {
		parts = append(parts, plural(len(planned), "plan", "plans")+" for the night: "+nameList(planned))
	}
	if len(inSky) > 0 {
		parts = append(parts, strings.Join(inSky, ", "))
	}
	day.Summary = upperFirst(strings.Join(parts, "; "))
	return day, nil
}

// countNames counts different names.
func countNames(names []string) int {
	uniq := map[string]bool{}
	for _, n := range names {
		if n = strings.TrimSpace(n); n != "" {
			uniq[n] = true
		}
	}
	return len(uniq)
}

// ---------- widget ----------

// How old a kept forecast may be to still say something about tonight.
const (
	widgetForecastAge = 12 * time.Hour
	widgetAuroraAge   = 3 * time.Hour
)

// stored reads a value the app fetched earlier and still has (see cached),
// no older than maxAge. It never fetches.
func stored[T any](ctx context.Context, s *Server, key string, maxAge time.Duration) (*T, bool) {
	raw, at, ok := s.store.CacheGet(ctx, key)
	if !ok {
		return nil, false
	}
	var v T
	if t, err := time.Parse(time.RFC3339Nano, at); err != nil || time.Since(t) > maxAge || json.Unmarshal([]byte(raw), &v) != nil {
		return nil, false
	}
	return &v, true
}

// nightOver reports whether the night is behind us: its dark hours have
// passed (where it does not get dark, the Sun is up again).
func nightOver(n astro.Night, now time.Time) bool {
	if n.DarkHours > 0 {
		return now.After(n.DarkTo)
	}
	return n.Sunrise != nil && now.After(*n.Sunrise)
}

// darkWords says when it is dark: "Dark from 18:40".
func darkWords(n astro.Night, now time.Time, tz *time.Location) string {
	clock := func(t time.Time) string { return t.In(tz).Format("15:04") }
	switch {
	case n.DarkKind == "none" || n.DarkKind == "civil":
		return "The sky stays bright all night"
	case n.DarkHours >= 23.5:
		return "Dark all day"
	case n.DarkKind == "nautical" && now.After(n.DarkFrom):
		return "Never fully dark, darkest until " + clock(n.DarkTo)
	case n.DarkKind == "nautical":
		return "Never fully dark, darkest from " + clock(n.DarkFrom)
	case now.After(n.DarkFrom):
		return "Dark until " + clock(n.DarkTo)
	}
	return "Dark from " + clock(n.DarkFrom)
}

// nightWord names a plan's night next to tonight: "tonight", "tomorrow", "Sat 5 Dec".
func nightWord(night, tonight string) string {
	d, err := store.ParseDate(night)
	t, err2 := store.ParseDate(tonight)
	switch {
	case err != nil || err2 != nil:
		return night
	case night == tonight:
		return "tonight"
	case d.Equal(t.AddDate(0, 0, 1)):
		return "tomorrow"
	case d.Year() == t.Year():
		return d.Format("Mon 2 Jan")
	}
	return d.Format("2 Jan 2006")
}

// pocketWidget is tonight at the person's home place in a few lines: when it
// gets dark and how lit the Moon is, then the best things to look at (a sky
// event, the planets that are up, the best sights for how dark the place is)
// and the next observing plan. The night is the coming one once it is getting
// light. All of it is worked out here; the cloud forecast and the northern
// lights are added only from a copy the app already keeps. Without a place
// there is nothing to show.
func (s *Server) pocketWidget(r *http.Request) (*core.WidgetData, error) {
	return s.widgetAt(r.Context(), actorID(r.Context()), time.Now())
}

// widgetAt is the widget of one person at a moment.
func (s *Server) widgetAt(ctx context.Context, uid int64, now time.Time) (*core.WidgetData, error) {
	home, err := s.store.HomeLocation(ctx, uid)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	wh := whereLoc(home)
	date := nightDate(now, wh.TZ)
	n, err := astro.NightAt(wh.Obs, date, wh.TZ)
	if err != nil {
		return nil, err
	}
	if nightOver(n, now) {
		date = store.AddDays(date, 1)
		if n, err = astro.NightAt(wh.Obs, date, wh.TZ); err != nil {
			return nil, err
		}
	}
	moon := strconv.Itoa(int(math.Round(n.MoonPhase*100))) + "%"
	out := &core.WidgetData{URL: "/", Line: darkWords(n, now, wh.TZ) + ", Moon " + moon}
	if n.DarkKind == "astro" || n.DarkKind == "nautical" {
		out.Metrics = append(out.Metrics, core.Metric{Label: "Dark", Value: n.DarkFrom.In(wh.TZ).Format("15:04") + "–" + n.DarkTo.In(wh.TZ).Format("15:04")})
	}
	out.Metrics = append(out.Metrics, core.Metric{Label: "Moon", Value: moon})
	if f, ok := stored[forecast.Forecast](ctx, s, "fc:"+spot(wh.Obs.Lat, wh.Obs.Lon), widgetForecastAge); ok {
		st, _ := s.store.GetSettings(ctx, uid)
		hours := hoursOf(wh.Obs, n, f, nil)
		if sc := scoreNight(n, hours, st.CloudMax, wh.TZ); sc.CloudAvg != nil {
			out.Line += ". " + strings.TrimSuffix(cloudWords(hours, n, wh.TZ), ".")
			out.Metrics = append(out.Metrics, core.Metric{Label: "Cloud", Value: strconv.Itoa(int(*sc.CloudAvg)) + "%"}, core.Metric{Label: "Night", Value: sc.Label})
		}
	}

	plans, err := s.store.FindPlans(ctx, uid, store.PlanFilter{From: date, Soonest: true, Limit: 1})
	if err != nil {
		return nil, err
	}
	room := 8 - len(plans)
	add := func(it core.WidgetItem) {
		if len(out.Items) < room {
			out.Items = append(out.Items, it)
		}
	}
	if a, ok := stored[space.Aurora](ctx, s, "aurora", widgetAuroraAge); ok {
		if v := auroraOf(a, wh, n); v != nil && v.Likely {
			side := "Northern"
			if wh.Obs.Lat < 0 {
				side = "Southern"
			}
			add(core.WidgetItem{ID: "aurora", Title: side + " lights possible", Detail: fmt.Sprintf("Kp %.0f forecast, about %.0f needed here", v.KpMax, v.KpNeeded), URL: "/"})
		}
	}
	noon, _ := time.ParseInLocation("2006-01-02", date, wh.TZ)
	noon = noon.Add(12 * time.Hour)
	events := 0
	for _, e := range s.eventsNear(ctx, wh, noon, noon.Add(24*time.Hour), nil) {
		if e.Kind == "moon_phase" || e.Kind == "season" || (e.Visible != nil && !*e.Visible) || events == 2 {
			continue
		}
		events++
		words, _, _ := strings.Cut(e.Detail, ". ")
		add(core.WidgetItem{ID: "event:" + e.Key, Title: e.Title, Detail: dots(e.T.In(wh.TZ).Format("15:04"), strings.TrimSuffix(words, ".")), URL: "/events"})
	}
	with := map[string]string{"eyes": "by eye", "binoculars": "in binoculars", "telescope": "in a telescope"}
	planets := 0
	for _, p := range planetsTonight(wh.Obs, n, wh.TZ) {
		if !p.Visible || planets == 4 {
			continue
		}
		planets++
		words, _, _ := strings.Cut(p.Words, ". ")
		if kit := map[string]string{"uranus": "binoculars", "neptune": "telescope"}[p.Key]; kit != "" {
			words += ", " + with[kit] // too faint for the eye
		}
		add(core.WidgetItem{ID: "object:" + p.Key, Title: p.Name, Detail: words, URL: mapURL(p.Key)})
	}
	for i, v := range sightsTonight(wh.Obs, n, wh.Bortle, nil, wh.TZ) {
		if i == 4 {
			break
		}
		add(core.WidgetItem{ID: "object:" + v.Key, Title: v.Name, URL: mapURL(v.Key),
			Detail: fmt.Sprintf("Best around %s, %s in the %s, %s", hm(v.BestAt, wh.TZ), heightWords(v.BestAlt), v.Dir, with[v.Kit])})
	}
	for _, p := range plans {
		id := strconv.FormatInt(p.ID, 10)
		out.Items = append(out.Items, core.WidgetItem{ID: "plan:" + id, Title: p.Name, Detail: dots("Planned for "+nightWord(p.Night, date), thingsToSee(len(p.Items))), URL: "/plans/" + id})
	}
	return out, nil
}

// ---------- capture ----------

// captureWords splits a quick line into a log entry's title and notes: the
// title when one was given (the text is then the notes), else the text's
// first line. A line too long for a title is kept whole in the notes.
func captureWords(in core.CaptureInput) (title, notes string) {
	title, notes = in.Title, in.Text
	if title == "" {
		title, notes, _ = strings.Cut(in.Text, "\n")
	}
	title, notes = strings.Join(strings.Fields(title), " "), strings.TrimSpace(notes)
	if len(title) > 200 {
		title, notes = clip(title, 200), strings.TrimSpace(title+"\n"+notes)
	}
	return title, notes
}

// captured is the answer for a kept line.
func captured(o *store.Observation) core.CaptureResult {
	id := strconv.FormatInt(o.ID, 10)
	return core.CaptureResult{ID: "observation:" + id, Title: firstNonEmpty(o.Title, "Night of "+o.Night), URL: "/log/" + id}
}

// pocketCapture keeps a quick line as a new entry in the observing log, the
// way Log a night does with only its first fields filled in: the night given
// (else tonight, by the home place's clock), at the home place when there is
// one, the line as the title and anything after it as the notes. It goes
// through the same checks and tells the Core bus like POST /observations;
// through an app link the entry is marked as added by that app. The same
// request_id again answers the same entry and adds nothing: the receipt is
// written in the transaction that adds the entry.
func (s *Server) pocketCapture(r *http.Request, in core.CaptureInput) (*core.CaptureResult, error) {
	ctx := r.Context()
	uid := actorID(ctx)
	op, err := core.NewOperation(fmt.Sprintf("sky:user:%d:%s", uid, core.CapturePath), in.RequestID, in)
	if err != nil {
		return nil, err
	}
	var res core.CaptureResult
	replay := func() (*core.CaptureResult, error) {
		found, err := op.Replay(ctx, s.store.DB, &res)
		switch {
		case errors.Is(err, core.ErrOperationConflict):
			return nil, fmt.Errorf("%w: this was already kept with other words", core.ErrCapture)
		case err != nil || !found:
			return nil, err
		}
		return &res, nil
	}
	if kept, err := replay(); kept != nil || err != nil {
		return kept, err
	}

	title, notes := captureWords(in)
	if title == "" {
		return nil, fmt.Errorf("%w: write what you saw first", core.ErrCapture)
	}
	form := observationIn{Title: &title, Notes: &notes, Night: &in.Date}
	tz := time.Local
	if home, err := s.store.HomeLocation(ctx, uid); err == nil {
		form.LocationID, tz = &home.ID, loadTZ(home.Timezone)
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	if in.Date == "" {
		form.Night = ptr(nightDate(time.Now(), tz))
	}
	o := &store.Observation{UserID: uid, Source: linkApp(ctx)}
	if msg := s.applyObservation(ctx, uid, form, o); msg != "" {
		return nil, fmt.Errorf("%w: %s", core.ErrCapture, msg)
	}
	err = s.store.CreateObservationWith(ctx, o, func(tx *sql.Tx) error {
		res = captured(o)
		return op.Save(ctx, tx, res)
	})
	if err != nil {
		// the same line sent twice at once: the other one kept it
		if kept, rerr := replay(); kept != nil || rerr != nil {
			return kept, rerr
		}
		return nil, err
	}
	s.observationLogged(ctx, uid, o)
	return &res, nil
}
