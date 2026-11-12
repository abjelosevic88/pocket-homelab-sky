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
	"github.com/abjelosevic88/pocket-homelab-sky/internal/forecast"
	"github.com/abjelosevic88/pocket-homelab-sky/internal/space"
	"github.com/abjelosevic88/pocket-homelab-sky/internal/store"
)

// ---- where: the place a question is about ----

// where is a spot on the Earth to work the sky out for.
type where struct {
	Loc    *store.Location
	Obs    astro.Observer
	TZ     *time.Location
	Name   string
	Bortle int
}

func (w where) view() map[string]any {
	out := map[string]any{"name": w.Name, "lat": w.Obs.Lat, "lon": w.Obs.Lon, "timezone": w.TZ.String(), "bortle": w.Bortle}
	if w.Loc != nil {
		out["id"] = w.Loc.ID
	}
	return out
}

var errNoPlace = errors.New("add a place to watch the sky from first")

func loadTZ(name string) *time.Location {
	if name != "" {
		if l, err := time.LoadLocation(name); err == nil {
			return l
		}
	}
	return time.Local
}

// whereOf is a person's place: ?location=ID, ?lat=&lon= (with ?tz=), else home.
func (s *Server) whereOf(r *http.Request, uid int64) (where, error) {
	ctx := r.Context()
	q := r.URL.Query()
	if q.Get("lat") != "" || q.Get("lon") != "" {
		lat, e1 := strconv.ParseFloat(q.Get("lat"), 64)
		lon, e2 := strconv.ParseFloat(q.Get("lon"), 64)
		if e1 != nil || e2 != nil || lat < -90 || lat > 90 || lon < -180 || lon > 180 {
			return where{}, errors.New("lat is −90 to 90 and lon −180 to 180")
		}
		w := where{Obs: astro.Observer{Lat: lat, Lon: lon}, TZ: loadTZ(q.Get("tz")), Name: strings.TrimSpace(q.Get("name"))}
		if q.Get("tz") == "" {
			if f, _, err := s.forecastAt(ctx, lat, lon); err == nil && f.Timezone != "" {
				w.TZ = loadTZ(f.Timezone)
			}
		}
		if w.Name == "" {
			w.Name = "Where you are"
		}
		return w, nil
	}
	var l *store.Location
	var err error
	if id, _ := strconv.ParseInt(q.Get("location"), 10, 64); id > 0 {
		l, err = s.store.GetLocation(ctx, uid, id)
	} else {
		l, err = s.store.HomeLocation(ctx, uid)
		if errors.Is(err, store.ErrNotFound) {
			return where{}, errNoPlace
		}
	}
	if err != nil {
		return where{}, err
	}
	return whereLoc(l), nil
}

func whereLoc(l *store.Location) where {
	return where{Loc: l, Obs: astro.Observer{Lat: l.Lat, Lon: l.Lon}, TZ: loadTZ(l.Timezone), Name: l.Label(), Bortle: l.Bortle}
}

func (s *Server) failWhere(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errNoPlace):
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "need_place": true})
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, "not found")
	default:
		writeErr(w, http.StatusBadRequest, err.Error())
	}
}

// nightDate is the date of the evening a moment belongs to (before noon: the night before).
func nightDate(t time.Time, tz *time.Location) string {
	lt := t.In(tz)
	if lt.Hour() < 12 {
		lt = lt.AddDate(0, 0, -1)
	}
	return lt.Format("2006-01-02")
}

// ---- directions and words ----

var compass16 = []string{"north", "north-north-east", "north-east", "east-north-east", "east", "east-south-east", "south-east", "south-south-east",
	"south", "south-south-west", "south-west", "west-south-west", "west", "west-north-west", "north-west", "north-north-west"}
var compass8 = []string{"north", "north-east", "east", "south-east", "south", "south-west", "west", "north-west"}

// Direction is a compass word for an azimuth.
func Direction(az float64) string { return compass8[int(math.Round(astro.Rev(az)/45))%8] }

func heightWords(alt float64) string {
	switch {
	case alt >= 70:
		return "almost overhead"
	case alt >= 45:
		return "high"
	case alt >= 25:
		return "halfway up"
	case alt >= 10:
		return "low"
	}
	return "very low"
}

func brightWords(mag float64) string {
	switch {
	case mag < -3:
		return "dazzling"
	case mag < -1:
		return "very bright"
	case mag < 1:
		return "bright"
	case mag < 3:
		return "easy to see"
	case mag < 5:
		return "faint"
	case mag < 6.5:
		return "only from a dark place"
	}
	return "needs binoculars or a telescope"
}

func hm(t *time.Time, tz *time.Location) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.In(tz).Format("15:04")
}

// conOf is the constellation (id) of a position of date.
func conOf(ra, dec float64, t time.Time) string {
	r, d := astro.ToJ2000(ra, dec, t)
	return catalog.ConstellationAt(r, d)
}

// ---- the night ----

type nightHour struct {
	Time    time.Time `json:"time"`
	Cloud   *float64  `json:"cloud"`
	Low     *float64  `json:"low"`
	Mid     *float64  `json:"mid"`
	High    *float64  `json:"high"`
	Humid   *float64  `json:"humidity"`
	Temp    *float64  `json:"temp"`
	Dew     *float64  `json:"dew"`
	Wind    *float64  `json:"wind"`
	SunAlt  float64   `json:"sun_alt"`
	MoonAlt float64   `json:"moon_alt"`
	Dark    bool      `json:"dark"`         // the Sun is 18° down (or as dark as it gets)
	Seeing  *int      `json:"seeing"`       // steady air 1–5 (7Timer!, every three hours)
	Transp  *int      `json:"transparency"` // clear air 1–5
}

type score struct {
	Value    int      `json:"value"` // 0–100
	Label    string   `json:"label"` // Great | Good | Fair | Poor | Cloudy
	Words    string   `json:"words"`
	CloudAvg *float64 `json:"cloud_avg"` // over the dark hours; nil without a forecast
	Clear    bool     `json:"clear"`     // clear enough by the person's own measure
	Forecast bool     `json:"forecast"`  // the forecast covers this night
	Fog      bool     `json:"fog"`
}

type moonView struct {
	Phase     float64    `json:"phase"` // lit fraction
	PhaseName string     `json:"phase_name"`
	Age       float64    `json:"age"` // days since new
	Waxing    bool       `json:"waxing"`
	Rise      *time.Time `json:"rise"`
	Set       *time.Time `json:"set"`
	UpDark    float64    `json:"up_dark"`
	Con       string     `json:"con"`
	DistKm    float64    `json:"dist_km"`
	Words     string     `json:"words"`
	NextNew   *time.Time `json:"next_new"`
	NextFull  *time.Time `json:"next_full"`
}

type bodyView struct {
	Key     string     `json:"key"`
	Name    string     `json:"name"`
	Mag     float64    `json:"mag"`
	Phase   float64    `json:"phase"`
	Con     string     `json:"con"`
	ConName string     `json:"con_name"`
	Rise    *time.Time `json:"rise"`
	Set     *time.Time `json:"set"`
	BestAt  *time.Time `json:"best_at"`
	BestAlt float64    `json:"best_alt"`
	BestAz  float64    `json:"best_az"`
	Dir     string     `json:"dir"`
	Visible bool       `json:"visible"`
	When    string     `json:"when"` // evening | all night | late | morning | ""
	Words   string     `json:"words"`
	Elong   float64    `json:"elong"`
	SizeSec float64    `json:"size"`
	DistAU  float64    `json:"dist_au"`
}

type sightView struct {
	Key     string     `json:"key"`
	Name    string     `json:"name"`
	Type    string     `json:"type"`
	Mag     *float64   `json:"mag"`
	Con     string     `json:"con"`
	ConName string     `json:"con_name"`
	BestAt  *time.Time `json:"best_at"`
	BestAlt float64    `json:"best_alt"`
	Dir     string     `json:"dir"`
	Words   string     `json:"words"`
	Seen    bool       `json:"seen"`
	Kit     string     `json:"kit"` // eyes | binoculars | telescope
}

type tonightView struct {
	Place      map[string]any `json:"place"`
	Date       string         `json:"date"`
	Night      astro.Night    `json:"night"`
	Score      score          `json:"score"`
	Moon       moonView       `json:"moon"`
	Planets    []bodyView     `json:"planets"`
	Sights     []sightView    `json:"sights"`
	Hours      []nightHour    `json:"hours"`
	Events     []astro.Event  `json:"events"`
	Stale      bool           `json:"stale"`
	ForecastAt string         `json:"forecast_at"`
	Air        *airView       `json:"air"`
	Aurora     *auroraView    `json:"aurora"`
	Darkness   *darknessView  `json:"darkness"`
}

// nightWindow is the part of the night planets and sights are judged in:
// from the end of civil twilight (or sunset) to its start in the morning.
func nightWindow(n astro.Night) (time.Time, time.Time) {
	from, to := n.DarkFrom, n.DarkTo
	if n.CivilDusk != nil {
		from = *n.CivilDusk
	} else if n.Sunset != nil {
		from = *n.Sunset
	}
	if n.CivilDawn != nil && n.CivilDawn.After(from) {
		to = *n.CivilDawn
	} else if n.Sunrise != nil && n.Sunrise.After(from) {
		to = *n.Sunrise
	}
	if !to.After(from) {
		from, to = n.DarkFrom, n.DarkTo
	}
	return from, to
}

// planetsTonight says when and where each planet can be seen tonight.
func planetsTonight(o astro.Observer, n astro.Night, tz *time.Location) []bodyView {
	from, to := nightWindow(n)
	out := []bodyView{}
	if !to.After(from) {
		return out
	}
	for _, p := range astro.Planets {
		f := astro.BodyAlt(p, o)
		mid := from.Add(to.Sub(from) / 2)
		pos := astro.At(p, o, mid)
		v := bodyView{Key: p, Name: astro.Names[p], Mag: math.Round(pos.Mag*10) / 10, Phase: math.Round(pos.Phase*100) / 100, Elong: math.Round(pos.Elong),
			SizeSec: math.Round(pos.Size*10) / 10, DistAU: math.Round(pos.Dist*1000) / 1000}
		v.Con = conOf(pos.RA, pos.Dec, mid)
		v.ConName = catalog.ConName(v.Con)
		// rise/set nearest the night
		for _, c := range astro.Crossings(f, astro.HorizonPoint, from.Add(-6*time.Hour), to.Add(6*time.Hour)) {
			c := c
			if c.Rising && (v.Rise == nil || math.Abs(c.T.Sub(mid).Hours()) < math.Abs(v.Rise.Sub(mid).Hours())) {
				v.Rise = &c.T
			}
			if !c.Rising && (v.Set == nil || math.Abs(c.T.Sub(mid).Hours()) < math.Abs(v.Set.Sub(mid).Hours())) {
				v.Set = &c.T
			}
		}
		best, alt := astro.Culmination(f, from, to)
		bp := astro.At(p, o, best)
		v.BestAt, v.BestAlt, v.BestAz, v.Dir = &best, math.Round(alt*10)/10, math.Round(bp.Az), Direction(bp.Az)
		limit := 8.0
		if p == "uranus" || p == "neptune" {
			limit = 20
		}
		v.Visible = alt >= limit && pos.Elong > 12
		if v.Visible {
			upFrom, upTo := time.Time{}, time.Time{}
			for t := from; !t.After(to); t = t.Add(10 * time.Minute) {
				if f(t) >= limit {
					if upFrom.IsZero() {
						upFrom = t
					}
					upTo = t
				}
			}
			span := to.Sub(from)
			startShare := upFrom.Sub(from).Hours() / span.Hours()
			endShare := to.Sub(upTo).Hours() / span.Hours()
			switch {
			case startShare < 0.1 && endShare < 0.1:
				v.When = "all night"
			case startShare < 0.1:
				v.When = "evening"
			case endShare < 0.1 && startShare > 0.5:
				v.When = "morning"
			case endShare < 0.1:
				v.When = "late"
			default:
				v.When = "late"
			}
			kit := ""
			switch p {
			case "uranus":
				kit = " You need binoculars and a chart to pick it out."
			case "neptune":
				kit = " Too faint for the eye: use a telescope and a chart."
			}
			whenWords := map[string]string{"all night": "Up all night", "evening": "In the evening", "late": "Later in the night", "morning": "Before dawn"}[v.When]
			v.Words = fmt.Sprintf("%s, %s in the %s around %s. %s (magnitude %.1f).%s", whenWords, heightWords(alt), v.Dir, best.In(tz).Format("15:04"),
				capFirst(brightWords(pos.Mag)), v.Mag, kit)
		} else if pos.Elong <= 12 {
			v.Words = "Too close to the Sun to see now."
		} else {
			v.Words = "Below the horizon or too low during the night."
		}
		out = append(out, v)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Visible != out[j].Visible {
			return out[i].Visible
		}
		return out[i].Mag < out[j].Mag
	})
	return out
}

func capFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// moonTonight describes the Moon for the night.
func moonTonight(o astro.Observer, n astro.Night, tz *time.Location) moonView {
	mid := n.DarkFrom.Add(n.DarkTo.Sub(n.DarkFrom) / 2)
	m := astro.Body("moon", mid)
	mv := moonView{Phase: math.Round(n.MoonPhase*1000) / 1000, PhaseName: astro.MoonPhaseName(n.MoonAge), Age: math.Round(n.MoonAge/360*29.53*10) / 10,
		Waxing: n.MoonAge < 180, Rise: n.MoonRise, Set: n.MoonSet, UpDark: math.Round(n.MoonUpDark*100) / 100, DistKm: math.Round(m.Dist * 149597870.7)}
	mv.Con = conOf(m.RA, m.Dec, mid)
	pct := int(math.Round(n.MoonPhase * 100))
	switch {
	case n.MoonUpDark < 0.05 || pct < 3:
		mv.Words = fmt.Sprintf("%s (%d%% lit), out of the way all night: a dark sky.", mv.PhaseName, pct)
	case n.MoonUpDark > 0.95:
		mv.Words = fmt.Sprintf("%s (%d%% lit), up all night", mv.PhaseName, pct)
		if pct > 60 {
			mv.Words += ": its light hides the faint stars."
		} else {
			mv.Words += "."
		}
	case n.MoonSet != nil && (n.MoonRise == nil || n.MoonSet.Before(*n.MoonRise)):
		mv.Words = fmt.Sprintf("%s (%d%% lit), setting at %s; dark after that.", mv.PhaseName, pct, hm(n.MoonSet, tz))
	case n.MoonRise != nil:
		mv.Words = fmt.Sprintf("%s (%d%% lit), rising at %s; dark until then.", mv.PhaseName, pct, hm(n.MoonRise, tz))
	default:
		mv.Words = fmt.Sprintf("%s (%d%% lit).", mv.PhaseName, pct)
	}
	ph := astro.MoonPhases(mid, mid.Add(31*24*time.Hour))
	for i := range ph {
		e := ph[i]
		if e.Sub == "new" && mv.NextNew == nil {
			mv.NextNew = &e.T
		}
		if e.Sub == "full" && mv.NextFull == nil {
			mv.NextFull = &e.T
		}
	}
	return mv
}

// hoursOf is the night hour by hour: sunset to sunrise, with the forecast.
func hoursOf(o astro.Observer, n astro.Night, f *forecast.Forecast, air *space.Air) []nightHour {
	from, to := n.DarkFrom.Add(-2*time.Hour), n.DarkTo.Add(2*time.Hour)
	if n.Sunset != nil {
		from = *n.Sunset
	}
	if n.Sunrise != nil && n.Sunrise.After(from) {
		to = *n.Sunrise
	}
	start := from.Truncate(time.Hour)
	out := []nightHour{}
	for t := start; !t.After(to); t = t.Add(time.Hour) {
		mid := t.Add(30 * time.Minute)
		h := nightHour{Time: t, SunAlt: math.Round(astro.At("sun", o, mid).Alt*10) / 10, MoonAlt: math.Round(astro.At("moon", o, mid).Alt*10) / 10}
		h.Dark = !mid.Before(n.DarkFrom) && mid.Before(n.DarkTo)
		if ah := air.At(mid); ah != nil {
			sv, tv := ah.Seeing, ah.Transparency
			h.Seeing, h.Transp = &sv, &tv
		}
		if fh := f.At(t); fh != nil {
			h.Cloud, h.Low, h.Mid, h.High, h.Humid, h.Temp, h.Dew, h.Wind = fh.Cloud, fh.Low, fh.Mid, fh.High, fh.Humidity, fh.Temp, fh.Dew, fh.Wind
		}
		out = append(out, h)
	}
	return out
}

// scoreNight rates a night for the sky: clouds, the Moon and how dark it gets.
func scoreNight(n astro.Night, hours []nightHour, cloudMax int, tz *time.Location) score {
	sc := score{}
	var sum, cnt float64
	fog := 0
	for _, h := range hours {
		if !h.Dark && n.DarkKind == "astro" {
			continue
		}
		if h.Cloud != nil {
			sum += *h.Cloud
			cnt++
		}
		if h.Humid != nil && h.Temp != nil && h.Dew != nil && *h.Humid >= 95 && *h.Temp-*h.Dew < 1.5 {
			fog++
		}
	}
	darkF := map[string]float64{"astro": 1, "nautical": 0.8, "civil": 0.5, "none": 0.15}[n.DarkKind]
	if n.DarkHours < 3 && n.DarkKind == "astro" {
		darkF = 0.7 + 0.1*n.DarkHours
	}
	moonPen := n.MoonPhase * n.MoonUpDark
	base := 100 * darkF * (1 - 0.55*moonPen)
	if cnt > 0 {
		avg := math.Round(sum / cnt)
		sc.CloudAvg, sc.Forecast = &avg, true
		base *= math.Pow(1-avg/100, 1.3)
		sc.Clear = avg <= float64(cloudMax)
		if float64(fog)/cnt > 0.3 {
			sc.Fog = true
			base *= 0.8
		}
	}
	sc.Value = int(math.Round(math.Max(0, math.Min(100, base))))
	switch {
	case sc.CloudAvg != nil && *sc.CloudAvg >= 85:
		sc.Label = "Cloudy"
	case sc.Value >= 75:
		sc.Label = "Great"
	case sc.Value >= 55:
		sc.Label = "Good"
	case sc.Value >= 35:
		sc.Label = "Fair"
	default:
		sc.Label = "Poor"
	}
	var parts []string
	if sc.CloudAvg == nil {
		parts = append(parts, "No cloud forecast for this night yet.")
	} else {
		parts = append(parts, cloudWords(hours, n, tz))
	}
	if sc.Fog {
		parts = append(parts, "Damp air: mist or dew is likely, so keep lenses covered.")
	}
	switch n.DarkKind {
	case "nautical":
		parts = append(parts, "It never gets fully dark this time of year.")
	case "civil", "none":
		parts = append(parts, "The sky stays bright all night this time of year.")
	}
	sc.Words = strings.Join(parts, " ")
	return sc
}

// cloudWords sums up the cloud over the night in a sentence.
func cloudWords(hours []nightHour, n astro.Night, tz *time.Location) string {
	type run struct {
		clear    bool
		from, to time.Time
	}
	var runs []run
	for _, h := range hours {
		if h.Cloud == nil || h.Time.Before(n.DarkFrom.Add(-time.Hour)) || h.Time.After(n.DarkTo) {
			continue
		}
		c := *h.Cloud < 40
		if len(runs) > 0 && runs[len(runs)-1].clear == c {
			runs[len(runs)-1].to = h.Time.Add(time.Hour)
		} else {
			runs = append(runs, run{c, h.Time, h.Time.Add(time.Hour)})
		}
	}
	switch {
	case len(runs) == 0:
		return "No cloud forecast for the dark hours."
	case len(runs) == 1 && runs[0].clear:
		return "Clear skies forecast all night."
	case len(runs) == 1:
		return "Cloudy all night."
	}
	// the longest clear run
	best := run{}
	for _, r := range runs {
		if r.clear && r.to.Sub(r.from) > best.to.Sub(best.from) {
			best = r
		}
	}
	if best.from.IsZero() {
		return "Cloudy all night."
	}
	return fmt.Sprintf("Clearest from %s to %s; cloudy at other times.", best.from.In(tz).Format("15:04"), best.to.In(tz).Format("15:04"))
}

// sightsTonight are catalogue favourites well placed tonight for the sky's darkness.
func sightsTonight(o astro.Observer, n astro.Night, bortle int, seen map[string]store.Seen, tz *time.Location) []sightView {
	if n.DarkKind == "none" {
		return []sightView{}
	}
	mid := n.DarkFrom.Add(n.DarkTo.Sub(n.DarkFrom) / 2)
	limit := 7.0
	switch {
	case bortle >= 7:
		limit = 5.5
	case bortle >= 5:
		limit = 6.8
	case bortle > 0 && bortle <= 3:
		limit = 8.5
	}
	if n.MoonPhase*n.MoonUpDark > 0.5 {
		limit -= 1
	}
	prec := astro.Precession(mid)
	type cand struct {
		v     sightView
		score float64
	}
	var cs []cand
	consider := func(x catalog.DSO) {
		if x.Mag == nil || *x.Mag > limit {
			return
		}
		ra, dec := astro.RADec(prec.Apply(astro.Vec(x.RA, x.Dec)))
		f := astro.FixedAlt(ra, dec, o)
		best, alt := astro.Culmination(f, n.DarkFrom, n.DarkTo)
		if alt < 25 {
			return
		}
		_, az := astro.Horizontal(ra, dec, o, best)
		kit := "telescope"
		switch {
		case *x.Mag <= 4.5:
			kit = "eyes"
		case *x.Mag <= 7:
			kit = "binoculars"
		}
		o := catalog.DSOObject(x)
		_, wasSeen := seen[o.Key]
		v := sightView{Key: o.Key, Name: o.Name, Type: o.Type, Mag: x.Mag, Con: x.Con, ConName: catalog.ConName(x.Con), BestAt: &best, BestAlt: math.Round(alt),
			Dir: Direction(az), Seen: wasSeen, Kit: kit}
		v.Words = fmt.Sprintf("%s in %s; best around %s, %s in the %s. %s.", o.Type, v.ConName, best.In(tz).Format("15:04"), heightWords(alt), v.Dir,
			map[string]string{"eyes": "Visible to the eye from a dark place", "binoculars": "A good sight in binoculars", "telescope": "Best in a telescope"}[kit])
		cs = append(cs, cand{v, alt/90*2 - *x.Mag/3})
	}
	for _, m := range catalog.Messier() {
		consider(m)
	}
	for _, h := range catalog.Highlights() {
		consider(h)
	}
	sort.SliceStable(cs, func(i, j int) bool { return cs[i].score > cs[j].score })
	out := []sightView{}
	for i := 0; i < len(cs) && len(out) < 10; i++ {
		out = append(out, cs[i].v)
	}
	return out
}

// tonight works out a whole night at a place.
func (s *Server) tonight(ctx context.Context, uid int64, w where, date string) (*tonightView, error) {
	n, err := astro.NightAt(w.Obs, date, w.TZ)
	if err != nil {
		return nil, err
	}
	st, _ := s.store.GetSettings(ctx, uid)
	f, stale, ferr := s.forecastAt(ctx, w.Obs.Lat, w.Obs.Lon)
	if ferr != nil {
		f = nil
	}
	air := s.airAt(ctx, w.Obs.Lat, w.Obs.Lon)
	hours := hoursOf(w.Obs, n, f, air)
	v := &tonightView{Place: w.view(), Date: date, Night: n, Hours: hours, Stale: stale}
	v.Air = airOf(hours, n)
	v.Aurora = s.auroraTonight(ctx, w, n)
	v.Darkness = s.darknessAt(w.Obs.Lat, w.Obs.Lon)
	if f != nil {
		v.ForecastAt = f.Fetched
	}
	v.Score = scoreNight(n, hours, st.CloudMax, w.TZ)
	if v.Air != nil && v.Air.Transparency > 0 && v.Score.Forecast {
		// hazy air dims the faint things: up to a fifth off
		v.Score.Value = int(math.Round(float64(v.Score.Value) * (0.8 + 0.05*float64(v.Air.Transparency-1))))
		v.Score.Words += " " + v.Air.Words
	}
	v.Moon = moonTonight(w.Obs, n, w.TZ)
	v.Planets = planetsTonight(w.Obs, n, w.TZ)
	seen, _ := s.store.SeenObjects(ctx, uid)
	v.Sights = sightsTonight(w.Obs, n, w.Bortle, seen, w.TZ)
	day, _ := time.ParseInLocation("2006-01-02", date, w.TZ)
	v.Events = s.eventsNear(ctx, w, day.Add(-12*time.Hour), day.Add(8*24*time.Hour), nil)
	return v, nil
}

// handleTonight is the night ahead at a place (?date= for another night).
func (s *Server) handleTonight(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	wh, err := s.whereOf(r, uid)
	if err != nil {
		s.failWhere(w, err)
		return
	}
	date := r.URL.Query().Get("date")
	if date == "" {
		date = nightDate(time.Now(), wh.TZ)
	} else if !validDate(date) {
		writeErr(w, http.StatusBadRequest, "the date is YYYY-MM-DD")
		return
	}
	v, err := s.tonight(ctx, uid, wh, date)
	if err != nil {
		s.fail(w, err, "tonight")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

type nightSummary struct {
	Date      string    `json:"date"`
	Score     score     `json:"score"`
	DarkFrom  time.Time `json:"dark_from"`
	DarkTo    time.Time `json:"dark_to"`
	DarkKind  string    `json:"dark_kind"`
	DarkHours float64   `json:"dark_hours"`
	MoonPhase float64   `json:"moon_phase"`
	MoonName  string    `json:"moon_name"`
	MoonUp    float64   `json:"moon_up_dark"`
}

// nights rates the coming nights at a place.
func (s *Server) nights(ctx context.Context, uid int64, w where, days int) []nightSummary {
	st, _ := s.store.GetSettings(ctx, uid)
	f, _, err := s.forecastAt(ctx, w.Obs.Lat, w.Obs.Lon)
	if err != nil {
		f = nil
	}
	air := s.airAt(ctx, w.Obs.Lat, w.Obs.Lon)
	start := nightDate(time.Now(), w.TZ)
	out := []nightSummary{}
	for i := 0; i < days; i++ {
		d := store.AddDays(start, i)
		n, err := astro.NightAt(w.Obs, d, w.TZ)
		if err != nil {
			continue
		}
		hours := hoursOf(w.Obs, n, f, air)
		out = append(out, nightSummary{Date: d, Score: scoreNight(n, hours, st.CloudMax, w.TZ), DarkFrom: n.DarkFrom, DarkTo: n.DarkTo, DarkKind: n.DarkKind,
			DarkHours: math.Round(n.DarkHours*10) / 10, MoonPhase: math.Round(n.MoonPhase*100) / 100, MoonName: astro.MoonPhaseName(n.MoonAge), MoonUp: math.Round(n.MoonUpDark*100) / 100})
	}
	return out
}

func (s *Server) handleNights(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	wh, err := s.whereOf(r, uid)
	if err != nil {
		s.failWhere(w, err)
		return
	}
	days := 7
	if n := queryInt(r, "days"); n > 0 && n <= 14 {
		days = n
	}
	writeJSON(w, http.StatusOK, map[string]any{"place": wh.view(), "nights": s.nights(ctx, uid, wh, days)})
}

// ---- the Sun, the Moon and the planets now ----

type bodyNow struct {
	Key     string     `json:"key"`
	Name    string     `json:"name"`
	RA      float64    `json:"ra"`
	Dec     float64    `json:"dec"`
	Alt     float64    `json:"alt"`
	Az      float64    `json:"az"`
	Dir     string     `json:"dir"`
	Mag     float64    `json:"mag"`
	Phase   float64    `json:"phase"`
	Elong   float64    `json:"elong"`
	Size    float64    `json:"size"`
	DistAU  float64    `json:"dist_au"`
	DistKm  float64    `json:"dist_km"`
	Con     string     `json:"con"`
	ConName string     `json:"con_name"`
	Up      bool       `json:"up"`
	Rise    *time.Time `json:"rise"`
	Set     *time.Time `json:"set"`
}

func bodiesAt(o astro.Observer, t time.Time) []bodyNow {
	out := make([]bodyNow, 0, len(astro.Bodies))
	for _, b := range astro.Bodies {
		p := astro.At(b, o, t)
		v := bodyNow{Key: b, Name: astro.Names[b], RA: round(p.RA, 3), Dec: round(p.Dec, 3), Alt: round(p.Alt, 2), Az: round(p.Az, 2), Dir: Direction(p.Az),
			Mag: round(p.Mag, 1), Phase: round(p.Phase, 3), Elong: round(p.Elong, 1), Size: round(p.Size, 1), DistAU: round(p.Dist, 4), DistKm: math.Round(p.Dist * 149597870.7)}
		v.Con = conOf(p.RA, p.Dec, t)
		v.ConName = catalog.ConName(v.Con)
		v.Up = p.Alt > astro.HorizonOf(b)
		f := astro.BodyAlt(b, o)
		if r := astro.NextCrossing(f, astro.HorizonOf(b), t, 30, true); !r.IsZero() {
			v.Rise = &r
		}
		if r := astro.NextCrossing(f, astro.HorizonOf(b), t, 30, false); !r.IsZero() {
			v.Set = &r
		}
		out = append(out, v)
	}
	return out
}

func round(v float64, d int) float64 {
	p := math.Pow(10, float64(d))
	return math.Round(v*p) / p
}

func timeParam(r *http.Request) (time.Time, error) {
	v := r.URL.Query().Get("t")
	if v == "" {
		return time.Now(), nil
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, nil
	}
	if ms, err := strconv.ParseInt(v, 10, 64); err == nil {
		return time.UnixMilli(ms), nil
	}
	return time.Time{}, errors.New("t is an RFC 3339 time or milliseconds")
}

func (s *Server) handleBodies(w http.ResponseWriter, r *http.Request) {
	uid := actorID(r.Context())
	wh, err := s.whereOf(r, uid)
	if err != nil {
		s.failWhere(w, err)
		return
	}
	t, err := timeParam(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"place": wh.view(), "t": t.UTC(), "bodies": bodiesAt(wh.Obs, t)})
}

// ---- events ----

// eventsNear are the sky events between two moments for a place (cached
// for a day by spot), optionally only some kinds.
func (s *Server) eventsNear(ctx context.Context, w where, from, to time.Time, kinds []string) []astro.Event {
	from, to = from.UTC().Truncate(time.Hour), to.UTC().Truncate(time.Hour)
	key := fmt.Sprintf("events:%s:%s:%s", spot(w.Obs.Lat, w.Obs.Lon), from.Format("2006010215"), to.Format("2006010215"))
	evs, _, err := cached(ctx, s, key, 24*time.Hour, func() ([]astro.Event, error) {
		o := w.Obs
		return astro.AllEvents(from, to, &o), nil
	})
	if err != nil {
		return []astro.Event{}
	}
	out := []astro.Event{}
	for _, e := range evs {
		if len(kinds) > 0 && !oneOf(e.Kind, kinds) {
			continue
		}
		// Penumbral eclipses too slight to notice stay out of the way.
		if e.Kind == "lunar_eclipse" && e.Sub == "penumbral" && e.Value < 0.3 {
			continue
		}
		out = append(out, e)
	}
	return out
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	wh, err := s.whereOf(r, uid)
	if err != nil {
		s.failWhere(w, err)
		return
	}
	q := r.URL.Query()
	from := time.Now().Add(-24 * time.Hour)
	to := from.AddDate(1, 0, 1)
	if v := q.Get("from"); v != "" {
		d, err := time.ParseInLocation("2006-01-02", v, wh.TZ)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "from is YYYY-MM-DD")
			return
		}
		from = d
		to = d.AddDate(1, 0, 0)
	}
	if v := q.Get("to"); v != "" {
		d, err := time.ParseInLocation("2006-01-02", v, wh.TZ)
		if err != nil || !d.After(from) {
			writeErr(w, http.StatusBadRequest, "to is YYYY-MM-DD after from")
			return
		}
		to = d.AddDate(0, 0, 1)
	}
	if to.Sub(from) > 3*366*24*time.Hour {
		writeErr(w, http.StatusBadRequest, "at most three years at a time")
		return
	}
	var kinds []string
	if k := strings.TrimSpace(q.Get("kinds")); k != "" {
		kinds = strings.Split(k, ",")
	}
	evs := s.eventsNear(ctx, wh, from, to, kinds)
	if queryBool(r, "visible") {
		keep := evs[:0]
		for _, e := range evs {
			if e.Visible == nil || *e.Visible {
				keep = append(keep, e)
			}
		}
		evs = keep
	}
	writeJSON(w, http.StatusOK, map[string]any{"place": wh.view(), "events": evs})
}

// ---- objects ----

// objectFor is any object by key, planets and the Moon included.
func objectFor(key string) (catalog.Object, bool) {
	if _, ok := astro.Names[key]; ok {
		kind := "planet"
		if key == "sun" || key == "moon" {
			kind = key
		}
		about := map[string]string{
			"sun":     "Our star. Never look at it directly or through binoculars or a telescope without a proper solar filter.",
			"moon":    "Our Moon, 384,000 km away. The line between light and dark (the terminator) shows craters and mountains best.",
			"mercury": "The smallest planet and the closest to the Sun: only seen low in the twilight, for a few weeks at a time.",
			"venus":   "The brightest planet, wrapped in cloud. It shows phases like the Moon in a small telescope.",
			"mars":    "The red planet. Every two years it comes close and shines bright orange; a telescope shows its ice caps then.",
			"jupiter": "The largest planet. Binoculars show up to four of its moons as tiny stars in a line beside it.",
			"saturn":  "The ringed planet: any small telescope shows the rings.",
			"uranus":  "An ice giant, just at the limit of the eye from a dark place; a green-blue dot in binoculars.",
			"neptune": "The farthest planet: a faint blue-grey star in a telescope.",
		}[key]
		typ := map[string]string{"sun": "Star", "moon": "Moon"}[key]
		if typ == "" {
			typ = "Planet"
		}
		return catalog.Object{Key: key, Name: astro.Names[key], Kind: kind, Type: typ, About: about}, true
	}
	if strings.HasPrefix(key, "dbl:") {
		return doubleObject(key)
	}
	return catalog.Lookup(key)
}

type objectDetail struct {
	catalog.Object
	ConName  string         `json:"con_name"`
	RADate   float64        `json:"ra_date"`
	DecDate  float64        `json:"dec_date"`
	Alt      float64        `json:"alt"`
	Az       float64        `json:"az"`
	Dir      string         `json:"dir"`
	Up       bool           `json:"up"`
	Rise     *time.Time     `json:"rise"`
	Transit  *time.Time     `json:"transit"`
	Set      *time.Time     `json:"set"`
	Always   string         `json:"always,omitempty"`
	BestAt   *time.Time     `json:"best_at"`
	BestAlt  float64        `json:"best_alt"`
	Tonight  string         `json:"tonight"` // plain words
	Body     *bodyNow       `json:"body,omitempty"`
	Seen     *store.Seen    `json:"seen"`
	TargetID int64          `json:"target_id"`
	Place    map[string]any `json:"place"`
}

func (s *Server) handleObject(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	key := chiParam(r, "key")
	obj, ok := objectFor(key)
	if !ok {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	wh, err := s.whereOf(r, uid)
	if err != nil {
		s.failWhere(w, err)
		return
	}
	t, err := timeParam(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	d := objectDetail{Object: obj, Place: wh.view()}
	n, _ := astro.NightAt(wh.Obs, nightDate(t, wh.TZ), wh.TZ)
	from, to := nightWindow(n)
	var f astro.AltFunc
	var level float64
	if obj.Kind == "planet" || obj.Kind == "sun" || obj.Kind == "moon" {
		p := astro.At(key, wh.Obs, t)
		d.RADate, d.DecDate, d.Alt, d.Az = round(p.RA, 3), round(p.Dec, 3), round(p.Alt, 2), round(p.Az, 2)
		bn := bodiesAt(wh.Obs, t)
		for i := range bn {
			if bn[i].Key == key {
				d.Body = &bn[i]
				d.Con = bn[i].Con
			}
		}
		mag := round(p.Mag, 1)
		d.Mag = &mag
		f, level = astro.BodyAlt(key, wh.Obs), astro.HorizonOf(key)
	} else {
		ra, dec := astro.OfDate(obj.RA, obj.Dec, t)
		d.RADate, d.DecDate = round(ra, 3), round(dec, 3)
		alt, az := astro.Horizontal(ra, dec, wh.Obs, t)
		d.Alt, d.Az = round(alt, 2), round(az, 2)
		f, level = astro.FixedAlt(ra, dec, wh.Obs), astro.HorizonPoint
	}
	d.ConName = catalog.ConName(d.Con)
	d.Dir = Direction(d.Az)
	d.Up = d.Alt > level
	day := t.In(wh.TZ)
	dayStart := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, wh.TZ)
	rs := riseSetFrom(f, level, dayStart, dayStart.Add(24*time.Hour))
	d.Rise, d.Transit, d.Set, d.Always = rs.Rise, rs.Transit, rs.Set, rs.Always
	if obj.Kind != "sun" && to.After(from) {
		best, alt := astro.Culmination(f, from, to)
		d.BestAt, d.BestAlt = &best, round(alt, 1)
		_, baz := objAltAz(key, obj, wh.Obs, best)
		switch {
		case obj.Kind == "constellation" && alt > 20:
			d.Tonight = fmt.Sprintf("Best around %s tonight, %s in the %s.", best.In(wh.TZ).Format("15:04"), heightWords(alt), Direction(baz))
		case alt > 10:
			d.Tonight = fmt.Sprintf("Best around %s tonight, %s in the %s.", best.In(wh.TZ).Format("15:04"), heightWords(alt), Direction(baz))
		case alt > 0:
			d.Tonight = "Only very low tonight; you need a clear, flat horizon."
		default:
			d.Tonight = "Not up during the dark hours tonight."
		}
		if obj.Kind == "planet" && d.Body != nil && d.Body.Elong < 12 {
			d.Tonight = "Too close to the Sun to see now."
		}
	}
	if seen, err := s.store.SeenObjects(ctx, uid); err == nil {
		if v, ok := seen[key]; ok {
			d.Seen = &v
		}
	}
	if ts, err := s.store.ListTargets(ctx, uid); err == nil {
		for _, tg := range ts {
			if tg.Key == key {
				d.TargetID = tg.ID
			}
		}
	}
	writeJSON(w, http.StatusOK, d)
}

func objAltAz(key string, obj catalog.Object, o astro.Observer, t time.Time) (float64, float64) {
	if _, ok := astro.Names[key]; ok {
		p := astro.At(key, o, t)
		return p.Alt, p.Az
	}
	ra, dec := astro.OfDate(obj.RA, obj.Dec, t)
	return astro.Horizontal(ra, dec, o, t)
}

func riseSetFrom(f astro.AltFunc, level float64, from, to time.Time) astro.RiseSet {
	var rs astro.RiseSet
	cs := astro.Crossings(f, level, from, to)
	for _, c := range cs {
		c := c
		if c.Rising && rs.Rise == nil {
			rs.Rise = &c.T
		}
		if !c.Rising && rs.Set == nil {
			rs.Set = &c.T
		}
	}
	tt, a := astro.Culmination(f, from, to)
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

// searchObjects finds planets and catalogue objects by name.
func searchObjects(q string, limit int) []catalog.Object {
	fq := catalog.Fold(q)
	out := []catalog.Object{}
	if fq == "" {
		return out
	}
	for _, b := range astro.Bodies {
		if strings.HasPrefix(catalog.Fold(astro.Names[b]), fq) {
			o, _ := objectFor(b)
			out = append(out, o)
		}
	}
	out = append(out, catalog.Search(q, limit)...)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (s *Server) handleObjectSearch(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit")
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	writeJSON(w, http.StatusOK, searchObjects(r.URL.Query().Get("q"), limit))
}

// handleCatalog is the fixed sky the map is drawn from (stars, lines,
// borders, deep-sky objects, the Milky Way): the same for everyone, cached.
func (s *Server) handleCatalog(w http.ResponseWriter, r *http.Request) {
	b, etag, err := catalog.ClientJSON()
	if err != nil {
		s.fail(w, err, "catalog")
		return
	}
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(b)
}

// ---- for other apps: summary, a day, search ----

// summaryOf is the short view for Homepage, Pocket Day and Pocket AI.
func (s *Server) summaryOf(ctx context.Context, uid int64, wh where) map[string]any {
	date := nightDate(time.Now(), wh.TZ)
	out := map[string]any{"place": wh.view(), "date": date}
	v, err := s.tonight(ctx, uid, wh, date)
	if err != nil {
		return out
	}
	vis := []string{}
	for _, p := range v.Planets {
		if p.Visible {
			vis = append(vis, p.Name)
		}
	}
	out["score"], out["label"], out["words"] = v.Score.Value, v.Score.Label, v.Score.Words
	out["cloud"] = v.Score.CloudAvg
	out["dark_from"], out["dark_to"], out["dark_kind"] = v.Night.DarkFrom, v.Night.DarkTo, v.Night.DarkKind
	out["sunset"], out["sunrise"] = v.Night.Sunset, v.Night.Sunrise
	out["moon"] = map[string]any{"phase": v.Moon.Phase, "name": v.Moon.PhaseName, "rise": v.Moon.Rise, "set": v.Moon.Set, "words": v.Moon.Words}
	out["planets"] = vis
	if len(v.Events) > 0 {
		out["next_event"] = v.Events[0]
	}
	return out
}

func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	wh, err := s.whereOf(r, uid)
	if err != nil {
		if errors.Is(err, errNoPlace) {
			writeJSON(w, http.StatusOK, map[string]any{"place": nil, "words": "No place to watch the sky from yet."})
			return
		}
		s.failWhere(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.summaryOf(ctx, uid, wh))
}

// handleDay is one date for Pocket Day: that night's sky, the events and
// what was logged.
func (s *Server) handleDay(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	date := chiParam(r, "date")
	if !validDate(date) || date == "" {
		writeErr(w, http.StatusBadRequest, "the date is YYYY-MM-DD")
		return
	}
	out := map[string]any{"date": date}
	obs, _ := s.store.ListObservations(ctx, uid, store.ObsFilter{From: date, To: date})
	out["observations"] = obs
	wh, err := s.whereOf(r, uid)
	if err == nil {
		if v, err := s.tonight(ctx, uid, wh, date); err == nil {
			vis := []string{}
			for _, p := range v.Planets {
				if p.Visible {
					vis = append(vis, p.Name)
				}
			}
			day, _ := time.ParseInLocation("2006-01-02", date, wh.TZ)
			evs := []astro.Event{}
			for _, e := range v.Events {
				if !e.T.Before(day) && e.T.Before(day.AddDate(0, 0, 1)) {
					evs = append(evs, e)
				}
			}
			out["place"], out["score"], out["label"], out["words"] = wh.view(), v.Score.Value, v.Score.Label, v.Score.Words
			out["dark_from"], out["dark_to"], out["moon"], out["planets"], out["events"] = v.Night.DarkFrom, v.Night.DarkTo, v.Moon, vis, evs
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// handleSearch is the search for Pocket Search and the command palette:
// objects of the sky and the log.
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	res := []map[string]any{}
	if len([]rune(q)) >= 2 {
		for _, o := range searchObjects(q, 8) {
			res = append(res, map[string]any{"id": o.Key, "kind": o.Kind, "title": o.Name, "snippet": strings.TrimSpace(o.Type + ". " + o.About), "url": "/map?object=" + o.Key})
		}
		obs, _ := s.store.ListObservations(ctx, uid, store.ObsFilter{Q: q, Limit: 10})
		for _, o := range obs {
			names := make([]string, 0, len(o.Objects))
			for _, x := range o.Objects {
				names = append(names, x.Name)
			}
			title := o.Title
			if title == "" {
				title = "Night of " + o.Night
			}
			res = append(res, map[string]any{"id": "log:" + strconv.FormatInt(o.ID, 10), "kind": "observation", "title": title, "date": o.Night,
				"snippet": strings.TrimSpace(strings.Join(names, ", ") + " " + o.Place), "url": "/log/" + strconv.FormatInt(o.ID, 10)})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": res})
}

// handleOverview is the person's places with how tonight looks at each, and
// the next sky event at home: what the sidebar and Homepage show.
func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	ls, err := s.store.ListLocations(ctx, uid)
	if err != nil {
		s.fail(w, err, "places")
		return
	}
	type row struct {
		store.Location
		Tonight *nightSummary `json:"tonight"`
	}
	out := make([]row, 0, len(ls))
	var next *astro.Event
	for i := range ls {
		l := &ls[i]
		wh := whereLoc(l)
		rw := row{Location: *l}
		if n := s.nights(ctx, uid, wh, 1); len(n) == 1 {
			rw.Tonight = &n[0]
		}
		if l.IsHome {
			now := time.Now()
			for _, e := range s.eventsNear(ctx, wh, now.Truncate(24*time.Hour), now.Truncate(24*time.Hour).Add(40*24*time.Hour), nil) {
				e := e
				if e.T.After(now.Add(-3*time.Hour)) && e.Kind != "moon_phase" && e.Kind != "season" && (e.Visible == nil || *e.Visible) {
					next = &e
					break
				}
			}
		}
		out = append(out, rw)
	}
	writeJSON(w, http.StatusOK, map[string]any{"places": out, "next_event": next})
}
