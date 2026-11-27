package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-sky/internal/astro"
	"github.com/abjelosevic88/pocket-homelab-sky/internal/config"
	"github.com/abjelosevic88/pocket-homelab-sky/internal/forecast"
	"github.com/abjelosevic88/pocket-homelab-sky/internal/space"
	"github.com/abjelosevic88/pocket-homelab-sky/internal/store"
)

// fakeCapCore is a Pocket Core that vouches for link tokens
// pcx_<alice|bob>_<search|day|widget|capture|read|write> from Pocket Hub
// and remembers the manifest Sky registers with.
type fakeCapCore struct {
	*httptest.Server
	mu       sync.Mutex
	manifest map[string]any
}

func newCapApp(t *testing.T) (*app, *fakeCapCore) {
	t.Helper()
	fc := &fakeCapCore{}
	scopes := map[string]string{"search": core.ScopeSearch, "day": core.ScopeDay, "widget": core.ScopeWidget, "capture": core.ScopeCapture,
		"read": "sky:read", "write": "sky:write"}
	fc.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/apps/register":
			var m map[string]any
			_ = json.NewDecoder(r.Body).Decode(&m)
			fc.mu.Lock()
			fc.manifest = m
			fc.mu.Unlock()
			io.WriteString(w, `{}`)
		case "/api/v1/auth/introspect":
			var in struct{ Token string }
			_ = json.NewDecoder(r.Body).Decode(&in)
			parts := strings.Split(in.Token, "_")
			if len(parts) != 3 || parts[0] != "pcx" || scopes[parts[2]] == "" {
				io.WriteString(w, `{"active":false}`)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"active": true, "kind": "delegated", "audience": "sky", "scope": scopes[parts[2]],
				"user": map[string]any{"id": 1, "sub": parts[1] + "@example.com", "email": parts[1] + "@example.com", "name": parts[1], "role": "member"},
				"app":  map[string]any{"id": 9, "slug": "hub", "name": "Pocket Hub"}})
		default:
			io.WriteString(w, `{}`)
		}
	}))
	t.Cleanup(fc.Close)
	return newApp(t, func(c *config.Config) { c.CoreURL, c.CoreRegistrationKey = fc.URL, "key" }), fc
}

// countingWx is the demo forecast that counts how often it is asked.
type countingWx struct {
	forecast.Demo
	asked atomic.Int32
}

func (c *countingWx) Clouds(ctx context.Context, lat, lon float64) (*forecast.Forecast, error) {
	c.asked.Add(1)
	return c.Demo.Clouds(ctx, lat, lon)
}

// items are the rows of a capability answer.
func items(t *testing.T, m map[string]any) []map[string]any {
	t.Helper()
	l, ok := m["items"].([]any)
	if !ok {
		t.Fatalf("no items list in %v", m)
	}
	out := []map[string]any{}
	for _, v := range l {
		out = append(out, v.(map[string]any))
	}
	return out
}

// item finds the row with that title.
func item(t *testing.T, m map[string]any, title string) map[string]any {
	t.Helper()
	for _, it := range items(t, m) {
		if it["title"] == title {
			return it
		}
	}
	t.Fatalf("no %q in %v", title, m["items"])
	return nil
}

// sky fills Alice's Sky: a home place, a night in the log, two plans, two
// things to see and a telescope.
func (a *app) sky() map[string]string {
	a.t.Helper()
	home := addPlace(a, a.alice, "Home", 43.8563, 18.4131)
	obs := a.ok(a.alice, "POST", "/api/v1/observations", map[string]any{"location_id": home["id"], "night": "2026-10-20", "started_at": "2026-10-20T21:00",
		"ended_at": "2026-10-20T23:15", "title": "Jupiter night", "equipment": "10×50 binoculars", "notes": "cold and clear",
		"objects": []map[string]any{{"key": "jupiter", "note": "four moons"}, {"key": "dso:M31"}, {"name": "A bright satellite"}}})
	plan := a.ok(a.alice, "POST", "/api/v1/plans", map[string]any{"name": "Winter tour", "night": "2026-10-20", "location_id": home["id"], "setup": "Dobson 200",
		"notes": "start in the east", "items": []map[string]any{{"key": "dso:M42"}, {"key": "saturn"}}})
	old := a.ok(a.alice, "POST", "/api/v1/plans", map[string]any{"name": "Spring galaxies", "night": "2020-03-21", "items": []map[string]any{{"key": "dso:M42"}}})
	m13 := a.ok(a.alice, "POST", "/api/v1/targets", map[string]any{"key": "dso:M13", "note": "from the cottage"})
	comet := a.ok(a.alice, "POST", "/api/v1/targets", map[string]any{"name": "Comet of the year"})
	kit := a.ok(a.alice, "POST", "/api/v1/kit", map[string]any{"kind": "telescope", "name": "Dobson 200", "aperture": 200, "focal": 1200, "notes": "collimate first"})
	return map[string]string{"home": id(home), "obs": id(obs), "plan": id(plan), "old": id(old), "m13": id(m13), "comet": id(comet), "kit": id(kit)}
}

func TestPocketSearch(t *testing.T) {
	a := newApp(t, nil)
	ids := a.sky()
	find := func(tok, q string) []map[string]any {
		t.Helper()
		return items(t, a.ok(tok, "GET", "/api/v1/pocket/search?q="+q, nil))
	}
	// the planet the word names, then the night that has it in its title
	got := find(a.alice, "jupiter")
	if len(got) != 2 || got[0]["id"] != "object:jupiter" || got[0]["kind"] != "planet" || got[0]["title"] != "Jupiter" || got[0]["url"] != "/map?object=jupiter" ||
		!strings.HasPrefix(got[0]["snippet"].(string), "Planet. The largest planet.") {
		t.Fatalf("jupiter: %v", got)
	}
	if n := got[1]; n["id"] != "observation:"+ids["obs"] || n["kind"] != "observation" || n["title"] != "Jupiter night" || n["date"] != "2026-10-20" || n["url"] != "/log/"+ids["obs"] ||
		n["snippet"] != "Jupiter, M31 Andromeda Galaxy, A bright satellite · Home · cold and clear" {
		t.Fatalf("the night: %v", n)
	}
	// by what was seen, by the notes
	if got := find(a.alice, "m31"); len(got) != 2 || got[0]["id"] != "object:dso:M31" || got[0]["kind"] != "deep-sky object" || got[0]["url"] != "/map?object=dso%3AM31" ||
		got[1]["id"] != "observation:"+ids["obs"] {
		t.Fatalf("m31: %v", got)
	}
	if got := find(a.alice, "cold+and"); len(got) != 1 || got[0]["title"] != "Jupiter night" {
		t.Fatalf("by the notes: %v", got)
	}
	// a plan by its name, and by what is on it (the newest night first)
	if got := find(a.alice, "winter"); len(got) != 1 || got[0]["id"] != "plan:"+ids["plan"] || got[0]["kind"] != "plan" || got[0]["date"] != "2026-10-20" ||
		got[0]["url"] != "/plans/"+ids["plan"] || got[0]["snippet"] != "M42 Orion Nebula, Saturn · Dobson 200 · start in the east" {
		t.Fatalf("winter: %v", got)
	}
	if got := find(a.alice, "m42"); len(got) != 3 || got[0]["id"] != "object:dso:M42" || got[1]["id"] != "plan:"+ids["plan"] || got[2]["id"] != "plan:"+ids["old"] {
		t.Fatalf("m42: %v", got)
	}
	// equipment: a title that has the words comes before the plan that only uses it
	got = find(a.alice, "dobson")
	if len(got) != 2 || got[0]["id"] != "equipment:"+ids["kit"] || got[0]["kind"] != "equipment" || got[0]["title"] != "Dobson 200" || got[0]["url"] != "/plans?tab=kit" ||
		got[0]["snippet"] != "Telescope · 200 mm aperture, 1200 mm focal length · collimate first" || got[1]["id"] != "plan:"+ids["plan"] {
		t.Fatalf("dobson: %v", got)
	}
	// the want-to-see list: between the constellation named and the cluster in it
	got = find(a.alice, "hercules")
	if len(got) != 3 || got[0]["kind"] != "constellation" || got[1]["id"] != "target:"+ids["m13"] || got[1]["kind"] != "want to see" || got[1]["title"] != "M13 Great Hercules Cluster" ||
		got[1]["url"] != "/map?object=dso%3AM13" || got[1]["snippet"] != "Not seen yet · from the cottage" || got[2]["id"] != "object:dso:M13" {
		t.Fatalf("hercules: %v", got)
	}
	if got := find(a.alice, "comet"); len(got) != 1 || got[0]["id"] != "target:"+ids["comet"] || got[0]["url"] != "/lists" {
		t.Fatalf("a thing in the person's own words: %v", got)
	}
	a.ok(a.alice, "POST", "/api/v1/observations", map[string]any{"night": "2026-10-22", "objects": []map[string]any{{"key": "dso:M13"}}})
	if got := find(a.alice, "cottage"); len(got) != 1 || got[0]["snippet"] != "First seen on 2026-10-22 · from the cottage" {
		t.Fatalf("seen since: %v", got)
	}
	if got := find(a.alice, "jupiter&limit=1"); len(got) != 1 || got[0]["id"] != "object:jupiter" {
		t.Fatalf("limit 1: %v", got)
	}
	// nothing to look for
	for _, q := range []string{"", "j", "%3F%3F", "*", "%25%25", "__"} {
		if got := find(a.alice, q); len(got) != 0 {
			t.Fatalf("%q finds %v", q, got)
		}
	}
	// the sky is everyone's, the rest is Alice's
	for _, q := range []string{"jupiter", "m31", "m42", "hercules"} {
		for _, it := range find(a.bob, q) {
			if !strings.HasPrefix(it["id"].(string), "object:") {
				t.Fatalf("bob finds alice's %q: %v", q, it)
			}
		}
	}
	for _, q := range []string{"winter", "dobson", "comet", "cold", "cottage"} {
		if got := find(a.bob, q); len(got) != 0 {
			t.Fatalf("bob finds alice's %q: %v", q, got)
		}
	}
	a.expect(401, "", "GET", "/api/v1/pocket/search?q=jupiter", nil)
	// the old route still answers as before
	if old := a.ok(a.alice, "GET", "/api/v1/search?q=jupiter", nil)["results"].([]any); len(old) != 2 || old[1].(map[string]any)["id"] != "log:"+ids["obs"] {
		t.Fatalf("/search changed: %v", old)
	}
}

func TestPocketDay(t *testing.T) {
	a := newApp(t, nil)
	wx := &countingWx{}
	a.s.wx = wx
	ids := a.sky()
	asked := wx.asked.Load()

	day := a.ok(a.alice, "GET", "/api/v1/pocket/day/2026-10-20", nil)
	if day["date"] != "2026-10-20" || day["summary"] != "Observed Jupiter, M31 Andromeda Galaxy and A bright satellite; plan for the night: Winter tour" || len(items(t, day)) != 2 {
		t.Fatalf("day: %v", day)
	}
	if ms := day["metrics"].([]any); len(ms) != 1 || ms[0].(map[string]any)["label"] != "Seen" || ms[0].(map[string]any)["value"] != "3" {
		t.Fatalf("metrics: %v", ms)
	}
	if it := item(t, day, "Jupiter night"); it["id"] != "observation:"+ids["obs"] || it["kind"] != "observed" || it["time"] != "21:00" || it["end"] != "23:15" ||
		it["sub"] != "Jupiter, M31 Andromeda Galaxy, A bright satellite · Home · 10×50 binoculars" || it["note"] != "cold and clear" || it["url"] != "/log/"+ids["obs"] {
		t.Fatalf("observed: %v", it)
	}
	if it := item(t, day, "Winter tour"); it["id"] != "plan:"+ids["plan"] || it["kind"] != "planned" || it["sub"] != "2 things to see · Dobson 200" || it["note"] != "start in the east" ||
		it["time"] != nil || it["url"] != "/plans/"+ids["plan"] {
		t.Fatalf("planned: %v", it)
	}
	// a night without a title is named by what was seen; one that ran past midnight has no clock on this date
	a.ok(a.alice, "POST", "/api/v1/observations", map[string]any{"night": "2026-10-23", "started_at": "2026-10-24T01:30", "objects": []map[string]any{{"key": "saturn"}, {"key": "dso:M42"}}})
	late := a.ok(a.alice, "GET", "/api/v1/pocket/day/2026-10-23", nil)
	if it := item(t, late, "Saturn and M42 Orion Nebula"); late["summary"] != "Observed Saturn and M42 Orion Nebula" || it["time"] != nil || it["sub"] != nil {
		t.Fatalf("a night named by what was seen: %v", late)
	}
	// a sky event of the day at home, at the place's clock (the Orionids peak at 18:08 UTC)
	ev := a.ok(a.alice, "GET", "/api/v1/pocket/day/2026-10-21", nil)
	if it := item(t, ev, "Orionids meteor shower"); ev["summary"] != "Orionids meteor shower" || len(items(t, ev)) != 1 || it["kind"] != "event" || it["time"] != "20:08" ||
		it["id"] != "event:meteor_shower:orionids:2026-10-21" || it["url"] != "/events" || !strings.HasPrefix(it["sub"].(string), "Up to about 20 meteors an hour") {
		t.Fatalf("event: %v", ev)
	}
	// the day is the caller's when they say where they are
	if d := a.ok(a.alice, "GET", "/api/v1/pocket/day/2026-10-21?tz=Pacific/Auckland", nil); d["summary"] != nil || len(items(t, d)) != 0 {
		t.Fatalf("21 October in Auckland: %v", d)
	}
	if d := a.ok(a.alice, "GET", "/api/v1/pocket/day/2026-10-22?tz=Pacific/Auckland", nil); item(t, d, "Orionids meteor shower")["time"] != "07:08" {
		t.Fatalf("22 October in Auckland: %v", d)
	}
	// a full Moon is worth a line, a quarter Moon is not
	if d := a.ok(a.alice, "GET", "/api/v1/pocket/day/2026-10-26", nil); d["summary"] != "Full Moon" {
		t.Fatalf("full Moon: %v", d)
	}
	if d := a.ok(a.alice, "GET", "/api/v1/pocket/day/2026-10-18", nil); d["summary"] != nil || len(items(t, d)) != 0 {
		t.Fatalf("first quarter: %v", d)
	}
	a.expect(400, a.alice, "GET", "/api/v1/pocket/day/yesterday", nil)
	// none of it asked for a forecast
	if wx.asked.Load() != asked {
		t.Fatalf("the day asked for the forecast %d times", wx.asked.Load()-asked)
	}

	// somebody else's day; without a place there are no events, only what was logged
	if d := a.ok(a.bob, "GET", "/api/v1/pocket/day/2026-10-20", nil); d["summary"] != nil || len(items(t, d)) != 0 {
		t.Fatalf("bob sees alice's day: %v", d)
	}
	if d := a.ok(a.bob, "GET", "/api/v1/pocket/day/2026-10-21", nil); d["summary"] != nil || len(items(t, d)) != 0 {
		t.Fatalf("events without a place: %v", d)
	}
	a.ok(a.bob, "POST", "/api/v1/observations", map[string]any{"night": "2026-10-21", "title": "Shooting stars from the roof"})
	a.ok(a.bob, "POST", "/api/v1/observations", map[string]any{"night": "2026-10-21", "notes": "second look"})
	if d := a.ok(a.bob, "GET", "/api/v1/pocket/day/2026-10-21", nil); d["summary"] != "2 entries in the observing log" || len(items(t, d)) != 2 ||
		item(t, d, "A night under the sky")["note"] != "second look" {
		t.Fatalf("bob's day: %v", d)
	}
	// the old route still answers as before, the forecast included
	if old := a.ok(a.alice, "GET", "/api/v1/day/2026-10-20", nil); len(old["observations"].([]any)) != 1 || old["moon"] == nil || old["label"] == nil || wx.asked.Load() == asked {
		t.Fatalf("/day changed: %v", old)
	}
}

// tonightOf is the night the widget is about, worked out the same way.
func tonightOf(t *testing.T, lat, lon float64, tz *time.Location) (string, astro.Night) {
	t.Helper()
	date := nightDate(time.Now(), tz)
	n, err := astro.NightAt(astro.Observer{Lat: lat, Lon: lon}, date, tz)
	if err != nil {
		t.Fatal(err)
	}
	if nightOver(n, time.Now()) {
		date = store.AddDays(date, 1)
		n, _ = astro.NightAt(astro.Observer{Lat: lat, Lon: lon}, date, tz)
	}
	return date, n
}

func TestPocketWidget(t *testing.T) {
	a := newApp(t, nil)
	wx := &countingWx{}
	a.s.wx = wx
	ctx := context.Background()
	// no place to watch the sky from: nothing to show, even with a plan
	a.ok(a.bob, "POST", "/api/v1/plans", map[string]any{"name": "Some night", "night": "2099-01-01"})
	if w := a.ok(a.bob, "GET", "/api/v1/pocket/widget", nil); w["line"] != nil || w["metrics"] != nil || len(items(t, w)) != 0 {
		t.Fatalf("without a place: %v", w)
	}
	a.sky()
	asked := wx.asked.Load()
	w := a.ok(a.alice, "GET", "/api/v1/pocket/widget", nil)
	if !regexp.MustCompile(`^Dark (from|until) \d\d:\d\d, Moon \d{1,3}%$`).MatchString(w["line"].(string)) || w["url"] != "/" {
		t.Fatalf("widget: %v", w)
	}
	ms := w["metrics"].([]any)
	if len(ms) != 2 || ms[0].(map[string]any)["label"] != "Dark" || !regexp.MustCompile(`^\d\d:\d\d–\d\d:\d\d$`).MatchString(ms[0].(map[string]any)["value"].(string)) ||
		ms[1].(map[string]any)["label"] != "Moon" {
		t.Fatalf("metrics: %v", ms)
	}
	rows := items(t, w)
	if len(rows) == 0 || len(rows) > 8 {
		t.Fatalf("rows: %v", rows)
	}
	for _, r := range rows {
		// the only plans are for nights gone by: what is left is the sky
		if !strings.HasPrefix(r["id"].(string), "object:") && !strings.HasPrefix(r["id"].(string), "event:") || !strings.HasPrefix(r["url"].(string), "/") || r["detail"] == "" {
			t.Fatalf("row: %v", r)
		}
	}
	// the next plan closes the list, the soonest one
	far := a.ok(a.alice, "POST", "/api/v1/plans", map[string]any{"name": "New Year under the stars", "night": "2099-01-01", "items": []map[string]any{{"key": "dso:M42"}}})
	rows = items(t, a.ok(a.alice, "GET", "/api/v1/pocket/widget", nil))
	if last := rows[len(rows)-1]; len(rows) > 8 || last["id"] != "plan:"+id(far) || last["title"] != "New Year under the stars" || last["detail"] != "Planned for 1 Jan 2099 · 1 thing to see" ||
		last["url"] != "/plans/"+id(far) {
		t.Fatalf("the next plan: %v", rows)
	}
	loc, _ := time.LoadLocation("Europe/Sarajevo")
	date, night := tonightOf(t, 43.8563, 18.4131, loc)
	a.ok(a.alice, "POST", "/api/v1/plans", map[string]any{"name": "Out on the balcony", "night": date})
	rows = items(t, a.ok(a.alice, "GET", "/api/v1/pocket/widget", nil))
	if last := rows[len(rows)-1]; last["title"] != "Out on the balcony" || last["detail"] != "Planned for tonight" {
		t.Fatalf("tonight's plan: %v", rows)
	}

	// so far nothing was asked of the forecast, and none is kept
	var kept int
	_ = a.st.DB.QueryRow(`SELECT COUNT(1) FROM cache WHERE key LIKE 'fc:%' OR key = 'aurora'`).Scan(&kept)
	if wx.asked.Load() != asked || kept != 0 {
		t.Fatalf("the widget asked for the forecast (%d times, %d kept)", wx.asked.Load()-asked, kept)
	}
	// once the app has one (the person opened Tonight), the widget says how the night looks
	a.ok(a.alice, "GET", "/api/v1/tonight?date="+date, nil)
	asked = wx.asked.Load()
	w = a.ok(a.alice, "GET", "/api/v1/pocket/widget", nil)
	ms = w["metrics"].([]any)
	if !regexp.MustCompile(`^Dark (from|until) \d\d:\d\d, Moon \d{1,3}%\. [A-Z][^.]+$`).MatchString(w["line"].(string)) || len(ms) != 4 ||
		ms[2].(map[string]any)["label"] != "Cloud" || ms[3].(map[string]any)["label"] != "Night" || wx.asked.Load() != asked {
		t.Fatalf("with a forecast kept: %v", w)
	}
	// and a strong northern-lights forecast it already has leads the list
	storm := space.Aurora{Fetched: time.Now().UTC()}
	for at := night.DarkFrom.Add(-6 * time.Hour); at.Before(night.DarkTo); at = at.Add(3 * time.Hour) {
		storm.Kp = append(storm.Kp, space.KpStep{Time: at.UTC(), Kp: 9})
	}
	b, _ := json.Marshal(storm)
	if err := a.st.CachePut(ctx, "aurora", string(b)); err != nil {
		t.Fatal(err)
	}
	rows = items(t, a.ok(a.alice, "GET", "/api/v1/pocket/widget", nil))
	if rows[0]["id"] != "aurora" || rows[0]["title"] != "Northern lights possible" || !strings.HasPrefix(rows[0]["detail"].(string), "Kp 9 forecast, about ") ||
		rows[len(rows)-1]["title"] != "Out on the balcony" || len(rows) > 8 {
		t.Fatalf("northern lights: %v", rows)
	}
	// a copy too old to trust says nothing
	old := time.Now().Add(-4 * time.Hour).UTC().Format(time.RFC3339Nano)
	if _, err := a.st.DB.Exec(`UPDATE cache SET fetched_at = ? WHERE key = 'aurora'`, old); err != nil {
		t.Fatal(err)
	}
	if rows = items(t, a.ok(a.alice, "GET", "/api/v1/pocket/widget", nil)); rows[0]["id"] == "aurora" {
		t.Fatalf("an old northern-lights forecast: %v", rows)
	}
	if _, err := a.st.DB.Exec(`UPDATE cache SET fetched_at = ? WHERE key LIKE 'fc:%'`, time.Now().Add(-13*time.Hour).UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if w = a.ok(a.alice, "GET", "/api/v1/pocket/widget", nil); len(w["metrics"].([]any)) != 2 || strings.Contains(w["line"].(string), ". ") {
		t.Fatalf("an old cloud forecast: %v", w)
	}
	// Bob's widget is his own
	addPlace(a, a.bob, "Bob's roof", 43.8563, 18.4131)
	rows = items(t, a.ok(a.bob, "GET", "/api/v1/pocket/widget", nil))
	if last := rows[len(rows)-1]; last["title"] != "Some night" || last["detail"] != "Planned for 1 Jan 2099" {
		t.Fatalf("bob's widget: %v", rows)
	}
}

// The widget at fixed moments of one night: before dark, in the dark hours,
// and the morning after, when it is about the night to come.
func TestPocketWidgetThroughTheNight(t *testing.T) {
	a := newApp(t, nil)
	ids := a.sky()
	ctx := context.Background()
	loc, _ := time.LoadLocation("Europe/Sarajevo")
	home := astro.Observer{Lat: 43.8563, Lon: 18.4131}
	at := func(local string) *core.WidgetData {
		t.Helper()
		now, err := time.ParseInLocation("2006-01-02T15:04", local, loc)
		if err != nil {
			t.Fatal(err)
		}
		w, err := a.s.widgetAt(ctx, a.ids["Alice"], now)
		if err != nil || w == nil {
			t.Fatalf("widget at %s: %v %v", local, w, err)
		}
		return w
	}
	clock := func(m time.Time) string { return m.In(loc).Format("15:04") }
	moon := func(n astro.Night) string { return itoa(int64(n.MoonPhase*100+0.5)) + "%" }
	n, _ := astro.NightAt(home, "2026-10-20", loc)
	next, _ := astro.NightAt(home, "2026-10-21", loc)

	w := at("2026-10-20T15:00")
	if w.Line != "Dark from "+clock(n.DarkFrom)+", Moon "+moon(n) || w.Metrics[0].Value != clock(n.DarkFrom)+"–"+clock(n.DarkTo) || w.Metrics[1].Value != moon(n) {
		t.Fatalf("in the afternoon: %q %v", w.Line, w.Metrics)
	}
	// the planets that are up, brightest first, then the best sights, then the plan for this night
	if len(w.Items) != 8 || w.Items[0].ID != "object:jupiter" || w.Items[0].Title != "Jupiter" || !strings.HasPrefix(w.Items[0].Detail, "Before dawn, high in the south-east around ") ||
		w.Items[0].URL != "/map?object=jupiter" || w.Items[1].Title != "Saturn" || w.Items[3].Title != "Uranus" || !strings.HasSuffix(w.Items[3].Detail, ", in binoculars") ||
		w.Items[4].ID != "object:dso:hyades" || !regexp.MustCompile(`^Best around \d\d:\d\d, high in the south, by eye$`).MatchString(w.Items[4].Detail) || w.Items[4].URL != "/map?object=dso%3Ahyades" {
		t.Fatalf("rows: %+v", w.Items)
	}
	if last := w.Items[7]; last.ID != "plan:"+ids["plan"] || last.Title != "Winter tour" || last.Detail != "Planned for tonight · 2 things to see" || last.URL != "/plans/"+ids["plan"] {
		t.Fatalf("tonight's plan: %+v", last)
	}
	if w = at("2026-10-20T23:30"); w.Line != "Dark until "+clock(n.DarkTo)+", Moon "+moon(n) {
		t.Fatalf("at night: %q", w.Line)
	}
	// after midnight it is still the same night
	if w = at("2026-10-21T03:00"); w.Line != "Dark until "+clock(n.DarkTo)+", Moon "+moon(n) || w.Items[len(w.Items)-1].Detail != "Planned for tonight · 2 things to see" {
		t.Fatalf("after midnight: %q %+v", w.Line, w.Items)
	}
	// in the morning it is about the night to come: the Orionids peak that evening, and the plan is behind us
	w = at("2026-10-21T08:00")
	if w.Line != "Dark from "+clock(next.DarkFrom)+", Moon "+moon(next) {
		t.Fatalf("the morning after: %q", w.Line)
	}
	if ev := w.Items[0]; ev.ID != "event:meteor_shower:orionids:2026-10-21" || ev.Title != "Orionids meteor shower" || ev.URL != "/events" ||
		ev.Detail != "20:08 · Up to about 20 meteors an hour at best in a dark sky (fewer from town)" {
		t.Fatalf("tonight's event: %+v", ev)
	}
	for _, it := range w.Items {
		if strings.HasPrefix(it.ID, "plan:") {
			t.Fatalf("a plan for a night gone by: %+v", it)
		}
	}
	// a plan still to come is named by its night
	if w = at("2026-10-15T15:00"); w.Items[len(w.Items)-1].Detail != "Planned for Tue 20 Oct · 2 things to see" {
		t.Fatalf("a plan in five days: %+v", w.Items)
	}
	if w, err := a.s.widgetAt(ctx, a.ids["Bob"], time.Now()); w != nil || err != nil {
		t.Fatalf("bob without a place: %v %v", w, err)
	}
}

// The words of the widget for fixed moments.
func TestWidgetWords(t *testing.T) {
	sarajevo, _ := time.LoadLocation("Europe/Sarajevo")
	berlin, _ := time.LoadLocation("Europe/Berlin")
	oslo, _ := time.LoadLocation("Europe/Oslo")
	n, err := astro.NightAt(astro.Observer{Lat: 43.8563, Lon: 18.4131}, "2026-10-20", sarajevo)
	if err != nil || n.DarkKind != "astro" {
		t.Fatalf("night: %v %v", n, err)
	}
	from, to := n.DarkFrom.In(sarajevo).Format("15:04"), n.DarkTo.In(sarajevo).Format("15:04")
	for _, c := range []struct {
		now  time.Time
		over bool
		want string
	}{
		{n.DarkFrom.Add(-5 * time.Hour), false, "Dark from " + from},
		{n.DarkFrom.Add(time.Hour), false, "Dark until " + to},
		{n.DarkTo.Add(time.Minute), true, ""},
	} {
		if got := nightOver(n, c.now); got != c.over {
			t.Errorf("nightOver at %s = %v", c.now, got)
		}
		if got := darkWords(n, c.now, sarajevo); c.want != "" && got != c.want {
			t.Errorf("darkWords at %s = %q, want %q", c.now, got, c.want)
		}
	}
	// midsummer further north: never fully dark, then not dark at all
	mid, _ := astro.NightAt(astro.Observer{Lat: 52.52, Lon: 13.405}, "2026-06-21", berlin)
	if got := darkWords(mid, mid.DarkFrom.Add(-time.Hour), berlin); mid.DarkKind != "nautical" || got != "Never fully dark, darkest from "+mid.DarkFrom.In(berlin).Format("15:04") {
		t.Errorf("Berlin in June: %s %q", mid.DarkKind, got)
	}
	sun, _ := astro.NightAt(astro.Observer{Lat: 69.65, Lon: 18.96}, "2026-06-21", oslo)
	if got := darkWords(sun, sun.DarkFrom, oslo); sun.DarkKind != "none" || got != "The sky stays bright all night" || nightOver(sun, sun.DarkFrom.Add(time.Hour)) {
		t.Errorf("Tromsø in June: %s %q", sun.DarkKind, got)
	}
	for night, want := range map[string]string{"2026-10-20": "tonight", "2026-10-21": "tomorrow", "2026-10-24": "Sat 24 Oct", "2027-01-02": "2 Jan 2027"} {
		if got := nightWord(night, "2026-10-20"); got != want {
			t.Errorf("nightWord(%s) = %q, want %q", night, got, want)
		}
	}
	if got := nameList([]string{"Jupiter", "Saturn", "Jupiter", "M31", "M42", "M45"}); got != "Jupiter, Saturn, M31 and 2 more" {
		t.Errorf("nameList = %q", got)
	}
	// the rarest events of a day, in the order they happen; hidden ones and quarter Moons stay out
	no, at := false, time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	evs := notableEvents([]astro.Event{
		{Kind: "moon_phase", Sub: "first", Title: "First quarter", T: at},
		{Kind: "moon_phase", Sub: "full", Title: "Full Moon", T: at.Add(time.Hour)},
		{Kind: "conjunction", Title: "The Moon and Mars", T: at.Add(2 * time.Hour)},
		{Kind: "lunar_eclipse", Title: "Hidden eclipse", T: at.Add(3 * time.Hour), Visible: &no},
		{Kind: "season", Title: "Equinox", T: at.Add(4 * time.Hour)},
		{Kind: "meteor_shower", Title: "Orionids", T: at.Add(5 * time.Hour)},
	}, 3)
	if len(evs) != 3 || evs[0].Title != "The Moon and Mars" || evs[1].Title != "Equinox" || evs[2].Title != "Orionids" {
		t.Errorf("notable events: %v", evs)
	}
}

// capture posts a quick line and returns the answer.
func capture(a *app, tok, requestID, text string, more map[string]any) (int, map[string]any) {
	body := map[string]any{"request_id": requestID, "kind": "observation", "text": text}
	for k, v := range more {
		body[k] = v
	}
	code, m, _ := a.call(tok, "POST", "/api/v1/pocket/capture", body)
	return code, m
}

// logOf is a person's whole observing log, newest night first.
func (a *app) logOf(tok string) []map[string]any {
	a.t.Helper()
	out := []map[string]any{}
	for _, o := range a.list(tok, "/api/v1/observations") {
		out = append(out, o.(map[string]any))
	}
	return out
}

func TestPocketCapture(t *testing.T) {
	a := newApp(t, nil)
	keep := func(tok, requestID, text string, more map[string]any) map[string]any {
		t.Helper()
		code, m := capture(a, tok, requestID, text, more)
		if code != 201 || m["url"] != "/log/"+strings.TrimPrefix(m["id"].(string), "observation:") {
			t.Fatalf("capture %q → %d %v", text, code, m)
		}
		return a.ok(tok, "GET", "/api/v1/observations/"+strings.TrimPrefix(m["id"].(string), "observation:"), nil)
	}
	// without a place: the line is the night's title, the night is tonight
	o := keep(a.alice, "req-0000000000000001", "Saw Jupiter's moons, clear sky", nil)
	if o["title"] != "Saw Jupiter's moons, clear sky" || o["notes"] != "" || o["night"] != nightDate(time.Now(), time.Local) || o["place"] != "" || o["location_id"] != nil ||
		o["source"] != "" || o["moon_phase"] == nil || len(o["objects"].([]any)) != 0 {
		t.Fatalf("the entry: %v", o)
	}
	// the same request again: the same answer, nothing added
	code, first := capture(a, a.alice, "req-0000000000000001", "Saw Jupiter's moons, clear sky", nil)
	if code != 201 || first["id"] != "observation:"+id(o) || first["title"] != "Saw Jupiter's moons, clear sky" || len(a.logOf(a.alice)) != 1 {
		t.Fatalf("a retry: %d %v, log %v", code, first, a.logOf(a.alice))
	}
	// the same id with other words is refused, and nothing is added
	if code, m := capture(a, a.alice, "req-0000000000000001", "Saw Saturn's rings", nil); code != 400 || m["error"] != "this was already kept with other words" || len(a.logOf(a.alice)) != 1 {
		t.Fatalf("the same id, other words: %d %v", code, m)
	}

	// with a home place the night is logged there, on the night said
	home := addPlace(a, a.alice, "Home", 43.8563, 18.4131)
	o = keep(a.alice, "req-0000000000000002", "The Pleiades over the roofs\nseven stars by eye,\ndozens in binoculars", map[string]any{"date": "2026-01-05"})
	if o["title"] != "The Pleiades over the roofs" || o["notes"] != "seven stars by eye,\ndozens in binoculars" || o["night"] != "2026-01-05" || o["place"] != "Home" ||
		num(o, "location_id") != num(home, "id") || num(o, "bortle") != 6 {
		t.Fatalf("at home: %v", o)
	}
	loc, _ := time.LoadLocation("Europe/Sarajevo")
	if o = keep(a.alice, "req-0000000000000003", "Venus low in the west", nil); o["night"] != nightDate(time.Now(), loc) {
		t.Fatalf("no date means tonight at home: %v", o)
	}
	// a title of its own keeps the text as the notes
	if o = keep(a.alice, "req-0000000000000004", "thin crescent beside it", map[string]any{"title": "Moon   and Venus"}); o["title"] != "Moon and Venus" || o["notes"] != "thin crescent beside it" {
		t.Fatalf("title and text: %v", o)
	}
	// a line too long for a title is kept whole in the notes
	long := strings.Repeat("ž", 150)
	if o = keep(a.alice, "req-0000000000000005", long, nil); len(o["title"].(string)) > 200 || !strings.HasSuffix(o["title"].(string), "…") || o["notes"] != long {
		t.Fatalf("a long line: %v", o)
	}
	// the same line sent several times at once is kept once
	var wg sync.WaitGroup
	answers := make([]string, 6)
	for i := range answers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if code, m := capture(a, a.alice, "req-0000000000000010", "Orion rising", nil); code == 201 {
				answers[i], _ = m["id"].(string)
			}
		}()
	}
	wg.Wait()
	rising := 0
	for _, e := range a.logOf(a.alice) {
		if e["title"] == "Orion rising" {
			rising++
		}
	}
	for _, got := range answers {
		if got == "" || got != answers[0] || rising != 1 {
			t.Fatalf("sent at once: %v, kept %d times", answers, rising)
		}
	}
	// what cannot be kept says why
	before := len(a.logOf(a.alice))
	if code, m := capture(a, a.alice, "req-0000000000000006", "x\n"+strings.Repeat("n", 20001), nil); code != 400 || m["error"] != "the notes are at most 20,000 characters" {
		t.Fatalf("too long: %d %v", code, m)
	}
	a.expect(400, a.alice, "POST", "/api/v1/pocket/capture", map[string]any{"request_id": "short", "text": "x"})
	a.expect(400, a.alice, "POST", "/api/v1/pocket/capture", map[string]any{"request_id": "req-0000000000000007", "kind": "plan", "text": "x"})
	a.expect(400, a.alice, "POST", "/api/v1/pocket/capture", map[string]any{"request_id": "req-0000000000000007", "text": "x", "date": "tonight"})
	a.expect(400, a.alice, "POST", "/api/v1/pocket/capture", map[string]any{"request_id": "req-0000000000000007", "text": "  "})
	a.expect(401, "", "POST", "/api/v1/pocket/capture", map[string]any{"request_id": "req-0000000000000007", "text": "x"})
	if len(a.logOf(a.alice)) != before {
		t.Fatal("a refused line was kept")
	}
	// it shows on its day, and Search finds it
	if d := a.ok(a.alice, "GET", "/api/v1/pocket/day/2026-01-05", nil); item(t, d, "The Pleiades over the roofs")["kind"] != "observed" {
		t.Fatalf("the day: %v", d)
	}
	if got := items(t, a.ok(a.alice, "GET", "/api/v1/pocket/search?q=roofs", nil)); len(got) != 1 || got[0]["title"] != "The Pleiades over the roofs" {
		t.Fatalf("search: %v", got)
	}
	// per person: a request id is one's own, and so is the log
	if o = keep(a.bob, "req-0000000000000001", "Saw Saturn's rings", nil); o["title"] != "Saw Saturn's rings" || o["place"] != "" {
		t.Fatalf("bob's line: %v", o)
	}
	if len(a.logOf(a.bob)) != 1 || len(a.logOf(a.alice)) != before {
		t.Fatalf("bob's log %d, alice's %d (was %d)", len(a.logOf(a.bob)), len(a.logOf(a.alice)), before)
	}
}

// A kept line tells the Core bus once, a retry not again.
func TestPocketCaptureEvent(t *testing.T) {
	cfg := testConfig(t.TempDir())
	cfg.CoreURL = "http://core.invalid"
	st := openTestStore(t, cfg)
	s := New(cfg, st, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	s.noNet = true
	ctx := context.Background()
	if err := s.core.Init(ctx); err != nil {
		t.Fatal(err)
	}
	u := &store.User{Email: "ana@example.com", Name: "Ana", Role: "admin"}
	if err := st.CreateUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	tok, _, _ := st.CreateAPIToken(ctx, u.ID, "t")
	ts := httptest.NewServer(s.Router())
	t.Cleanup(ts.Close)
	a := &app{t: t, url: ts.URL, s: s, st: st}
	for range 2 {
		if code, m := capture(a, tok, "req-0000000000000001", "Saw the Milky Way", map[string]any{"date": "2026-08-12"}); code != 201 {
			t.Fatalf("capture → %d %v", code, m)
		}
	}
	rows, err := st.DB.QueryContext(ctx, `SELECT body FROM pocket_outbox ORDER BY created_at, rowid`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	logged := 0
	for rows.Next() {
		var b []byte
		if err := rows.Scan(&b); err != nil {
			t.Fatal(err)
		}
		var ev struct {
			Topic   string
			Payload map[string]any
		}
		if err := json.Unmarshal(b, &ev); err != nil {
			t.Fatal(err)
		}
		if ev.Topic != "sky.observation.logged" {
			continue
		}
		logged++
		if ev.Payload["title"] != "Saw the Milky Way" || ev.Payload["night"] != "2026-08-12" || ev.Payload["source"] != "" || ev.Payload["user_email"] != "ana@example.com" {
			t.Errorf("payload = %v", ev.Payload)
		}
	}
	if logged != 1 {
		t.Fatalf("sky.observation.logged events = %d, want 1", logged)
	}
}

// A link token with one pocket:* scope reaches its one route and nothing else.
func TestPocketLinkTokens(t *testing.T) {
	a, _ := newCapApp(t)
	ids := a.sky()
	line := func(requestID string) map[string]any {
		return map[string]any{"request_id": requestID, "kind": "observation", "text": "Saw the Milky Way"}
	}
	for _, c := range []struct {
		tok, method, path string
		body              any
		want              int
	}{
		{"pcx_alice_search", "GET", "/api/v1/pocket/search?q=jupiter", nil, 200},
		{"pcx_alice_day", "GET", "/api/v1/pocket/day/2026-10-20", nil, 200},
		{"pcx_alice_widget", "GET", "/api/v1/pocket/widget", nil, 200},
		{"pcx_alice_capture", "POST", "/api/v1/pocket/capture", line("req-0000000000000001"), 201},
		{"pcx_alice_widget", "GET", "/api/v1/auth/me", nil, 200},
		// another capability's route
		{"pcx_alice_search", "GET", "/api/v1/pocket/day/2026-10-20", nil, 403},
		{"pcx_alice_search", "GET", "/api/v1/pocket/widget", nil, 403},
		{"pcx_alice_search", "POST", "/api/v1/pocket/capture", line("req-0000000000000002"), 403},
		{"pcx_alice_day", "GET", "/api/v1/pocket/search?q=jupiter", nil, 403},
		{"pcx_alice_day", "GET", "/api/v1/pocket/widget", nil, 403},
		{"pcx_alice_widget", "GET", "/api/v1/pocket/search?q=jupiter", nil, 403},
		{"pcx_alice_widget", "GET", "/api/v1/pocket/day/2026-10-20", nil, 403},
		{"pcx_alice_widget", "POST", "/api/v1/pocket/capture", line("req-0000000000000003"), 403},
		{"pcx_alice_capture", "GET", "/api/v1/pocket/widget", nil, 403},
		{"pcx_alice_capture", "GET", "/api/v1/pocket/search?q=jupiter", nil, 403},
		// the rest of the app
		{"pcx_alice_widget", "GET", "/api/v1/overview", nil, 403},
		{"pcx_alice_widget", "GET", "/api/v1/summary", nil, 403},
		{"pcx_alice_widget", "GET", "/api/v1/tonight", nil, 403},
		{"pcx_alice_widget", "GET", "/api/v1/plans", nil, 403},
		{"pcx_alice_search", "GET", "/api/v1/search?q=jupiter", nil, 403},
		{"pcx_alice_search", "GET", "/api/v1/objects/search?q=jupiter", nil, 403},
		{"pcx_alice_search", "GET", "/api/v1/kit", nil, 403},
		{"pcx_alice_day", "GET", "/api/v1/day/2026-10-20", nil, 403},
		{"pcx_alice_day", "GET", "/api/v1/observations", nil, 403},
		{"pcx_alice_day", "GET", "/api/v1/events", nil, 403},
		{"pcx_alice_capture", "POST", "/api/v1/observations", map[string]any{"night": "2026-10-20"}, 403},
		{"pcx_alice_capture", "PUT", "/api/v1/observations/" + ids["obs"], map[string]any{"title": "x"}, 403},
		{"pcx_alice_capture", "DELETE", "/api/v1/observations/" + ids["obs"], nil, 403},
		{"pcx_alice_capture", "POST", "/api/v1/targets", map[string]any{"key": "saturn"}, 403},
		{"pcx_alice_capture", "GET", "/api/v1/settings", nil, 403},
		// the app's own scopes do not open the new routes
		{"pcx_alice_read", "GET", "/api/v1/pocket/search?q=jupiter", nil, 403},
		{"pcx_alice_read", "GET", "/api/v1/pocket/day/2026-10-20", nil, 403},
		{"pcx_alice_read", "GET", "/api/v1/pocket/widget", nil, 403},
		{"pcx_alice_write", "POST", "/api/v1/pocket/capture", line("req-0000000000000004"), 403},
		{"pcx_alice_read", "GET", "/api/v1/search?q=jupiter", nil, 200},
		{"pcx_alice_read", "GET", "/api/v1/day/2026-10-20", nil, 200},
		{"pcx_nobody", "GET", "/api/v1/pocket/widget", nil, 401},
	} {
		if code, m, _ := a.call(c.tok, c.method, c.path, c.body); code != c.want {
			t.Errorf("%s %s %s → %d, want %d (%v)", c.tok, c.method, c.path, code, c.want, m)
		}
	}
	// the token acts for its person only
	if got := items(t, a.ok("pcx_alice_search", "GET", "/api/v1/pocket/search?q=winter", nil)); len(got) != 1 || got[0]["title"] != "Winter tour" {
		t.Fatalf("alice through the link: %v", got)
	}
	if got := items(t, a.ok("pcx_bob_search", "GET", "/api/v1/pocket/search?q=winter", nil)); len(got) != 0 {
		t.Fatalf("bob through the link finds alice's plan: %v", got)
	}
	if w := a.ok("pcx_bob_widget", "GET", "/api/v1/pocket/widget", nil); len(items(t, w)) != 0 {
		t.Fatalf("bob's widget through the link: %v", w)
	}
	if d := a.ok("pcx_bob_day", "GET", "/api/v1/pocket/day/2026-10-20", nil); len(items(t, d)) != 0 {
		t.Fatalf("bob's day through the link: %v", d)
	}
	if d := a.ok("pcx_alice_day", "GET", "/api/v1/pocket/day/2026-10-20", nil); len(items(t, d)) != 2 {
		t.Fatalf("alice's day through the link: %v", d)
	}
	// only the one allowed capture landed, marked as added by the app that sent it; a retry adds nothing
	if code, m := capture(a, "pcx_alice_capture", "req-0000000000000001", "Saw the Milky Way", nil); code != 201 {
		t.Fatalf("retry through the link: %d %v", code, m)
	}
	var kept []map[string]any
	for _, o := range a.logOf(a.alice) {
		if o["title"] == "Saw the Milky Way" {
			kept = append(kept, o)
		}
	}
	if len(a.logOf(a.alice)) != 2 || len(kept) != 1 || kept[0]["source"] != "hub" || kept[0]["place"] != "Home" {
		t.Fatalf("alice's log: %v", a.logOf(a.alice))
	}
	if code, m := capture(a, "pcx_bob_capture", "req-0000000000000001", "Saw the Milky Way", nil); code != 201 || len(a.logOf(a.bob)) != 1 || len(a.logOf(a.alice)) != 2 {
		t.Fatalf("bob's own line: %d %v", code, m)
	}
}

func TestPocketLinkScopes(t *testing.T) {
	req := func(m, p string) *http.Request { return httptest.NewRequest(m, p, nil) }
	for _, c := range []struct {
		scope string
		r     *http.Request
		want  bool
	}{
		{core.ScopeSearch, req("GET", "/api/v1/pocket/search?q=x"), true},
		{core.ScopeDay, req("GET", "/api/v1/pocket/day/2026-01-02"), true},
		{core.ScopeWidget, req("GET", "/api/v1/pocket/widget"), true},
		{core.ScopeCapture, req("POST", "/api/v1/pocket/capture"), true},
		{core.ScopeWidget, req("GET", "/api/v1/auth/me"), true},
		{core.ScopeSearch, req("POST", "/api/v1/pocket/search"), false},
		{core.ScopeDay, req("GET", "/api/v1/pocket/day/today"), false},
		{core.ScopeDay, req("GET", "/api/v1/day/2026-01-02"), false},
		{core.ScopeWidget, req("GET", "/api/v1/tonight"), false},
		{core.ScopeCapture, req("GET", "/api/v1/pocket/capture"), false},
		{core.ScopeCapture, req("POST", "/api/v1/observations"), false},
		{"sky:read", req("GET", "/api/v1/pocket/widget"), false},
		{"sky:read", req("GET", "/api/v1/pocket/search?q=x"), false},
		{"sky:write", req("POST", "/api/v1/pocket/capture"), false},
	} {
		if got := linkAllows([]string{c.scope}, c.r); got != c.want {
			t.Errorf("%s %s %s = %v, want %v", c.scope, c.r.Method, c.r.URL.Path, got, c.want)
		}
	}
}

// Sky tells Core what it can do, so Search, Day and Hub find it on their own.
func TestManifestCapabilities(t *testing.T) {
	a, fc := newCapApp(t)
	if err := a.s.registerWithCore(context.Background()); err != nil {
		t.Fatal(err)
	}
	fc.mu.Lock()
	m := fc.manifest
	fc.mu.Unlock()
	if m["slug"] != "sky" || m["category"] != "Places" {
		t.Fatalf("manifest: %v %v", m["slug"], m["category"])
	}
	search, _ := m["search"].(map[string]any)
	day, _ := m["day"].(map[string]any)
	widget, _ := m["widget"].(map[string]any)
	cap, _ := m["capture"].(map[string]any)
	if search == nil || day == nil || day["label"] != "Sky" || day["one"] != "observation" || day["many"] != "observations" || widget == nil || widget["title"] != "Tonight's sky" || cap == nil {
		t.Fatalf("capabilities: %v %v %v %v", search, day, widget, cap)
	}
	bangs := []string{}
	for _, b := range search["bangs"].([]any) {
		bangs = append(bangs, b.(string))
	}
	if strings.Join(bangs, " ") != "sky stars astro" {
		t.Fatalf("bangs: %v", bangs)
	}
	if k := cap["kinds"].([]any)[0].(map[string]any); len(cap["kinds"].([]any)) != 1 || k["kind"] != "observation" || len(k["fields"].([]any)) != 1 || k["fields"].([]any)[0] != "date" {
		t.Fatalf("capture kinds: %v", cap)
	}
	offered := map[string]bool{}
	for _, o := range m["offers"].([]any) {
		offered[o.(map[string]any)["scope"].(string)] = true
	}
	for _, sc := range []string{"sky:read", "sky:write", core.ScopeMoveExport, core.ScopeMoveImport, core.ScopeSearch, core.ScopeDay, core.ScopeWidget, core.ScopeCapture} {
		if !offered[sc] {
			t.Errorf("%s is not offered", sc)
		}
	}
}
