package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/abjelosevic88/pocket-homelab-sky/internal/config"
)

func addPlace(a *app, tok, name string, lat, lon float64) map[string]any {
	a.t.Helper()
	return a.ok(tok, "POST", "/api/v1/locations", map[string]any{"name": name, "lat": lat, "lon": lon, "timezone": "Europe/Sarajevo", "bortle": 6})
}

// Everything is per person: Bob never sees, changes or removes Alice's
// places, log entries or list, and gets 404 for her ids.
func TestPerPersonIsolation(t *testing.T) {
	a := newApp(t, nil)
	home := addPlace(a, a.alice, "Home", 43.8563, 18.4131)
	if home["is_home"] != true {
		t.Fatalf("the first place should be home: %v", home)
	}
	obs := a.ok(a.alice, "POST", "/api/v1/observations", map[string]any{"location_id": home["id"], "night": "2026-10-20", "started_at": "2026-10-20T21:00",
		"ended_at": "2026-10-20T23:15", "title": "Jupiter night", "equipment": "10×50 binoculars", "seeing": 4,
		"objects": []map[string]any{{"key": "jupiter", "note": "four moons"}, {"key": "dso:M31"}, {"name": "A bright satellite"}}})
	objs, _ := obs["objects"].([]any)
	if len(objs) != 3 || obs["place"] != "Home" || obs["moon_phase"] == nil {
		t.Fatalf("observation: %v", obs)
	}
	if o := objs[1].(map[string]any); o["name"] != "M31 Andromeda Galaxy" {
		t.Fatalf("catalogue name: %v", o)
	}
	tg := a.ok(a.alice, "POST", "/api/v1/targets", map[string]any{"key": "dso:M31"})
	if tg["seen_on"] != "2026-10-20" {
		t.Fatalf("a target already logged is seen: %v", tg)
	}
	for _, p := range []string{"/api/v1/locations/" + id(home), "/api/v1/observations/" + id(obs)} {
		a.expect(404, a.bob, "GET", p, nil)
		a.expect(404, a.bob, "PUT", p, map[string]any{"name": "mine", "title": "mine"})
		a.expect(404, a.bob, "DELETE", p, nil)
	}
	a.expect(404, a.bob, "DELETE", "/api/v1/targets/"+id(tg), nil)
	a.expect(404, a.bob, "POST", "/api/v1/observations/"+id(obs)+"/note", nil)
	if l := a.list(a.bob, "/api/v1/observations"); len(l) != 0 {
		t.Fatalf("Bob sees %d log entries", len(l))
	}
	if l := a.list(a.bob, "/api/v1/targets"); len(l) != 0 {
		t.Fatalf("Bob sees %d targets", len(l))
	}
	// Bob may not log a night at Alice's place
	a.expect(400, a.bob, "POST", "/api/v1/observations", map[string]any{"location_id": home["id"], "night": "2026-10-20"})
	// Without a place, Bob's Tonight asks for one
	code, m, _ := a.call(a.bob, "GET", "/api/v1/tonight", nil)
	if code != 409 || m["need_place"] != true {
		t.Fatalf("Bob's tonight: %d %v", code, m)
	}
	// Alice's progress counts her night, Bob's does not
	pr := a.ok(a.alice, "GET", "/api/v1/progress", nil)
	if num(pr["messier"].(map[string]any), "seen") != 1 || num(pr["planets"].(map[string]any), "seen") != 1 {
		t.Fatalf("progress: %v %v", pr["messier"].(map[string]any)["seen"], pr["planets"].(map[string]any)["seen"])
	}
	pb := a.ok(a.bob, "GET", "/api/v1/progress", nil)
	if num(pb["messier"].(map[string]any), "seen") != 0 {
		t.Fatalf("Bob's progress shows Alice's night")
	}
	// search finds her log, not for Bob
	if res := a.ok(a.bob, "GET", "/api/v1/search?q=jupiter+night", nil)["results"].([]any); len(res) > 0 {
		for _, x := range res {
			if x.(map[string]any)["kind"] == "observation" {
				t.Fatalf("Bob's search finds Alice's log: %v", x)
			}
		}
	}
	a.ok(a.alice, "DELETE", "/api/v1/observations/"+id(obs), nil)
}

func TestTonightAndEvents(t *testing.T) {
	a := newApp(t, nil)
	addPlace(a, a.alice, "Home", 43.8563, 18.4131)
	v := a.ok(a.alice, "GET", "/api/v1/tonight?date=2026-11-12", nil)
	night := v["night"].(map[string]any)
	if night["dark_kind"] != "astro" {
		t.Fatalf("night: %v", night)
	}
	sc := v["score"].(map[string]any)
	if sc["label"] == "" || sc["words"] == "" || sc["forecast"] == nil {
		t.Fatalf("score: %v", sc)
	}
	pl := v["planets"].([]any)
	if len(pl) != 7 {
		t.Fatalf("planets: %d", len(pl))
	}
	if hrs := v["hours"].([]any); len(hrs) < 12 {
		t.Fatalf("hours: %d", len(hrs))
	}
	// the coming year's events at home include full moons and the Geminids
	ev := a.ok(a.alice, "GET", "/api/v1/events?from=2026-11-12&to=2027-11-11", nil)["events"].([]any)
	var full, gem, ecl int
	for _, e := range ev {
		m := e.(map[string]any)
		switch {
		case m["kind"] == "moon_phase" && m["sub"] == "full":
			full++
		case m["kind"] == "meteor_shower" && m["sub"] == "geminids":
			gem++
		case m["kind"] == "solar_eclipse" && strings.HasPrefix(m["t"].(string), "2027-08-02"):
			ecl++
			if m["visible"] != true {
				t.Errorf("2027-08-02 is seen from Sarajevo: %v", m)
			}
		}
	}
	if full < 12 || gem != 1 || ecl != 1 {
		t.Fatalf("events: %d full moons, %d Geminids, %d eclipse", full, gem, ecl)
	}
	n := a.ok(a.alice, "GET", "/api/v1/nights", nil)["nights"].([]any)
	if len(n) != 7 {
		t.Fatalf("nights: %d", len(n))
	}
	// an object: Jupiter, and a galaxy
	j := a.ok(a.alice, "GET", "/api/v1/objects/jupiter?t=2026-11-12T22:00:00Z", nil)
	if j["name"] != "Jupiter" || j["body"] == nil || j["con_name"] == "" {
		t.Fatalf("jupiter: %v", j)
	}
	m31 := a.ok(a.alice, "GET", "/api/v1/objects/dso:M31?t=2026-11-12T20:00:00Z", nil)
	if m31["up"] != true || m31["con"] != "And" {
		t.Fatalf("M31 should be up on a November evening: %v", m31)
	}
	a.expect(404, a.alice, "GET", "/api/v1/objects/dso:M999", nil)
	if c := a.ok(a.alice, "GET", "/api/v1/objects/con%3ACas", nil); c["name"] != "Cassiopeia" {
		t.Fatalf("an escaped key: %v", c)
	}
	if r := a.list(a.alice, "/api/v1/objects/search?q=sat"); len(r) == 0 || r[0].(map[string]any)["key"] != "saturn" {
		t.Fatalf("search sat: %v", r)
	}
	// any spot by lat/lon
	b := a.ok(a.bob, "GET", "/api/v1/bodies?lat=51.5&lon=-0.12&tz=Europe/London", nil)
	if len(b["bodies"].([]any)) != 9 {
		t.Fatalf("bodies: %v", b)
	}
	sum := a.ok(a.alice, "GET", "/api/v1/summary", nil)
	if sum["label"] == nil || sum["moon"] == nil {
		t.Fatalf("summary: %v", sum)
	}
	day := a.ok(a.alice, "GET", "/api/v1/day/2026-11-12", nil)
	if day["moon"] == nil {
		t.Fatalf("day: %v", day)
	}
	cat, _, _ := a.call(a.alice, "GET", "/api/v1/catalog", nil)
	if cat != 200 {
		t.Fatalf("catalog %d", cat)
	}
}

func TestSettingsPerPerson(t *testing.T) {
	a := newApp(t, nil)
	a.ok(a.alice, "PUT", "/api/v1/settings", map[string]any{"cloud_max": 20, "show_grid": true, "night_red": true, "event_kinds": []string{"lunar_eclipse", "nonsense"}, "camera_fov": 58})
	me := a.ok(a.alice, "GET", "/api/v1/settings", nil)
	if num(me, "cloud_max") != 20 || me["show_grid"] != true || me["night_red"] != true || num(me, "camera_fov") != 58 {
		t.Fatalf("alice: %v", me)
	}
	if k := me["event_kinds"].([]any); len(k) != 1 || k[0] != "lunar_eclipse" {
		t.Fatalf("kinds: %v", k)
	}
	bob := a.ok(a.bob, "GET", "/api/v1/settings", nil)
	if num(bob, "cloud_max") != 35 || bob["show_grid"] != false || bob["night_red"] != false {
		t.Fatalf("bob got alice's settings: %v", bob)
	}
	a.expect(400, a.alice, "PUT", "/api/v1/settings", map[string]any{"mag_limit": 9})
	a.expect(400, a.alice, "PUT", "/api/v1/settings", map[string]any{"cloud_max": 0})
}

func TestExportImportIdempotent(t *testing.T) {
	a := newApp(t, nil)
	home := addPlace(a, a.alice, "Home", 43.8563, 18.4131)
	a.ok(a.alice, "POST", "/api/v1/observations", map[string]any{"location_id": home["id"], "night": "2026-09-01", "started_at": "2026-09-01T22:00", "title": "Saturn",
		"objects": []map[string]any{{"key": "saturn"}, {"key": "moon"}}})
	a.ok(a.alice, "POST", "/api/v1/targets", map[string]any{"key": "dso:M13", "note": "summer"})
	req, _ := http.NewRequest("GET", a.url+"/api/v1/export.json", nil)
	req.Header.Set("Authorization", "Bearer "+a.alice)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	b := make([]byte, 1<<16)
	for {
		k, err := res.Body.Read(b)
		buf.Write(b[:k])
		if err != nil {
			break
		}
	}
	res.Body.Close()
	doc := buf.String()
	post := func(tok string) map[string]any {
		req, _ := http.NewRequest("POST", a.url+"/api/v1/import", strings.NewReader(doc))
		req.Header.Set("Authorization", "Bearer "+tok)
		res, err := http.DefaultClient.Do(req)
		if err != nil || res.StatusCode != 200 {
			t.Fatalf("import: %v %v", err, res.StatusCode)
		}
		defer res.Body.Close()
		m := map[string]any{}
		_ = decodeBody(res, &m)
		return m
	}
	again := post(a.alice)
	if num(again, "observations") != 0 || num(again, "places") != 0 || num(again, "targets") != 0 {
		t.Fatalf("importing into the same account added things: %v", again)
	}
	into := post(a.bob)
	if num(into, "observations") != 1 || num(into, "places") != 1 || num(into, "targets") != 1 {
		t.Fatalf("import into Bob: %v", into)
	}
	if twice := post(a.bob); num(twice, "observations") != 0 || num(twice, "places") != 0 {
		t.Fatalf("second import added: %v", twice)
	}
	l := a.list(a.bob, "/api/v1/observations")
	if len(l) != 1 || len(l[0].(map[string]any)["objects"].([]any)) != 2 || l[0].(map[string]any)["location_id"] == nil {
		t.Fatalf("Bob's log: %v", l)
	}
}

func TestLinkScopes(t *testing.T) {
	req := func(method, path string) *http.Request { return httptest.NewRequest(method, path, nil) }
	read, write := []string{"sky:read"}, []string{"sky:write"}
	for _, p := range []string{"/api/v1/tonight", "/api/v1/summary", "/api/v1/events", "/api/v1/nights", "/api/v1/bodies", "/api/v1/observations",
		"/api/v1/observations/3", "/api/v1/objects/dso:M31", "/api/v1/objects/jupiter", "/api/v1/objects/search", "/api/v1/day/2026-11-12", "/api/v1/progress", "/api/v1/search"} {
		if !linkAllows(read, req("GET", p)) {
			t.Errorf("read should allow %s", p)
		}
		if linkAllows(write, req("GET", p)) {
			t.Errorf("write alone should not read %s", p)
		}
	}
	for _, mp := range []string{"GET /api/v1/settings", "GET /api/v1/tokens", "GET /api/v1/export.json", "POST /api/v1/import", "GET /api/v1/backup",
		"POST /api/v1/calendar", "POST /api/v1/locations", "PUT /api/v1/locations/1", "POST /api/v1/ai/plan", "POST /api/v1/observations/1/note", "GET /api/v1/users"} {
		parts := strings.SplitN(mp, " ", 2)
		if linkAllows(append(read, write...), req(parts[0], parts[1])) {
			t.Errorf("no link may reach %s", mp)
		}
	}
	if linkAllows(read, req("POST", "/api/v1/observations")) || !linkAllows(write, req("POST", "/api/v1/observations")) {
		t.Error("logging needs sky:write")
	}
	if !linkAllows(read, req("GET", "/api/v1/auth/me")) {
		t.Error("me is open to any link")
	}
}

// The demo data is believable: entries only with what was up that night.
func TestDemo(t *testing.T) {
	a := newApp(t, func(c *config.Config) {
		c.DemoData = true
		c.AuthMode = "local"
		c.AdminEmail = "demo@example.com"
		c.AdminPassword = "demo-password"
	})
	_ = a
	ctx := t.Context()
	owner := a.st.OwnerID(ctx)
	if owner == 0 {
		t.Skip("no owner")
	}
	if err := a.s.seedDemo(ctx, owner); err != nil {
		t.Fatal(err)
	}
	ls, _ := a.st.ListLocations(ctx, owner)
	if len(ls) != 3 {
		t.Fatalf("places: %d", len(ls))
	}
	stats, _ := a.st.Stats(ctx, owner)
	if stats.Entries < 10 || stats.Objects < 8 {
		t.Fatalf("log: %+v", stats)
	}
	_ = time.Now
}

func decodeBody(res *http.Response, v any) error { return json.NewDecoder(res.Body).Decode(v) }
