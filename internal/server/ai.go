package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-sky/internal/ai"
	"github.com/abjelosevic88/pocket-homelab-sky/internal/astro"
	"github.com/abjelosevic88/pocket-homelab-sky/internal/catalog"
	"github.com/abjelosevic88/pocket-homelab-sky/internal/store"
)

// aiFor is the person's AI: Pocket AI through Core, their own
// OpenAI-compatible key, or nil with why.
func (s *Server) aiFor(ctx context.Context, uid int64, st store.Settings) (*ai.Client, string, string) {
	email := ""
	if u, err := s.store.GetUser(ctx, uid); err == nil {
		email = u.Email
	}
	pocket, status := s.core.SelectAI(ctx, email, st.AIProvider, st.OpenAIKey != "")
	if pocket != nil {
		if !status.Ready {
			return nil, "", "Pocket AI: " + status.Detail
		}
		c := ai.New("", "", "", 150*time.Second)
		c.Pocket = pocket.WithModel(st.AIPocketModel)
		return c, "pocket", ""
	}
	if st.OpenAIKey == "" {
		return nil, "", status.Detail
	}
	return ai.New(st.OpenAIBaseURL, st.OpenAIKey, st.OpenAIModel, 150*time.Second), "openai", ""
}

func (s *Server) handleAIStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	st, _ := s.store.GetSettings(ctx, actorID(ctx))
	cl, used, why := s.aiFor(ctx, actorID(ctx), st)
	writeJSON(w, http.StatusOK, map[string]any{"ready": cl != nil, "provider": used, "detail": why, "plan": st.AIPlan})
}

func (s *Server) handleAIModels(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c, status := s.core.SelectAI(ctx, userFrom(ctx).Email, "pocket", false)
	if c == nil || !status.Ready {
		writeErr(w, http.StatusServiceUnavailable, status.Detail)
		return
	}
	models, err := c.Models(ctx)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, models)
}

func validAIProvider(v string) bool { return core.ValidAIProvider(v) }

func validPocketModel(v string) bool { return core.ValidInferenceModel(v) }

const aiVoice = `You are the night-sky app of one person. You speak plainly and briefly, like a friend who knows the sky and looked at tonight for them: no jargon (or explain it in a few words), no emojis, no markdown headings, no greeting. Only use the facts you are given for positions, times and the forecast; never invent numbers or times. Times are the local time of the place. Directions are compass words.`

// skyFacts is a night at a place as text for the AI.
func (s *Server) skyFacts(v *tonightView, tz *time.Location, st store.Settings) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Place: %v (latitude %.2f, longitude %.2f)", v.Place["name"], v.Place["lat"], v.Place["lon"])
	if bt, _ := v.Place["bortle"].(int); bt > 0 {
		fmt.Fprintf(&b, ", sky darkness Bortle %d of 9 (1 darkest)", bt)
	}
	fmt.Fprintf(&b, "\nNight of %s. Now it is %s.\n", v.Date, time.Now().In(tz).Format("Mon 2 Jan 15:04"))
	n := v.Night
	fmt.Fprintf(&b, "Sunset %s, darkness (%s twilight ends) %s to %s, sunrise %s.\n", hm(n.Sunset, tz), n.DarkKind, hm(&n.DarkFrom, tz), hm(&n.DarkTo, tz), hm(n.Sunrise, tz))
	fmt.Fprintf(&b, "Night rating %d/100 (%s). %s\n", v.Score.Value, v.Score.Label, v.Score.Words)
	if v.Score.CloudAvg != nil {
		fmt.Fprintf(&b, "Average cloud over the dark hours %.0f%%; clear enough for them is at most %d%%.\n", *v.Score.CloudAvg, st.CloudMax)
		b.WriteString("Hourly cloud (total/low/mid/high %), humidity, temperature °C:\n")
		for _, h := range v.Hours {
			if h.Cloud == nil {
				continue
			}
			fmt.Fprintf(&b, "  %s %.0f/%.0f/%.0f/%.0f, %.0f%%, %.0f°C\n", h.Time.In(tz).Format("15:04"), val(h.Cloud), val(h.Low), val(h.Mid), val(h.High), val(h.Humid), val(h.Temp))
		}
	}
	fmt.Fprintf(&b, "Moon: %s\n", v.Moon.Words)
	b.WriteString("Planets:\n")
	for _, p := range v.Planets {
		fmt.Fprintf(&b, "  %s (magnitude %.1f, in %s): %s\n", p.Name, p.Mag, p.ConName, p.Words)
	}
	if len(v.Sights) > 0 {
		b.WriteString("Good sights tonight (catalogue objects well placed):\n")
		for _, x := range v.Sights {
			seen := ""
			if x.Seen {
				seen = " (they have seen it before)"
			}
			fmt.Fprintf(&b, "  %s: %s%s\n", x.Name, x.Words, seen)
		}
	}
	if len(v.Events) > 0 {
		b.WriteString("Sky events coming up:\n")
		for _, e := range v.Events {
			if len(e.Detail) > 0 {
				fmt.Fprintf(&b, "  %s, %s: %s\n", e.Title, e.T.In(tz).Format("Mon 2 Jan 15:04"), e.Detail)
			}
		}
	}
	return b.String()
}

func val(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

// aiPlace is the place an AI question is about.
func (s *Server) aiPlace(r *http.Request, uid, id int64) (where, error) {
	if id > 0 {
		l, err := s.store.GetLocation(r.Context(), uid, id)
		if err != nil {
			return where{}, err
		}
		return whereLoc(l), nil
	}
	return s.whereOf(r, uid)
}

// handleAIPlan writes the night in plain words: what to look at, when and
// where, with what (cached for two hours).
func (s *Server) handleAIPlan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in struct {
		Location  int64  `json:"location_id"`
		Date      string `json:"date"`
		Equipment string `json:"equipment"`
		Fresh     bool   `json:"fresh"`
	}
	_ = decode(r, &in)
	wh, err := s.aiPlace(r, uid, in.Location)
	if err != nil {
		s.failWhere(w, err)
		return
	}
	date := in.Date
	if date == "" || !validDate(date) {
		date = nightDate(time.Now(), wh.TZ)
	}
	st, _ := s.store.GetSettings(ctx, uid)
	key := fmt.Sprintf("ai:plan:%d:%s:%s:%s:%d", uid, spot(wh.Obs.Lat, wh.Obs.Lon), date, catalog.Fold(in.Equipment), time.Now().Hour()/2)
	if !in.Fresh {
		if raw, at, ok := s.store.CacheGet(ctx, key); ok {
			var out map[string]any
			if json.Unmarshal([]byte(raw), &out) == nil {
				out["cached"], out["at"] = true, at
				writeJSON(w, http.StatusOK, out)
				return
			}
		}
	}
	cl, used, why := s.aiFor(ctx, uid, st)
	if cl == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": why})
		return
	}
	v, err := s.tonight(ctx, uid, wh, date)
	if err != nil {
		s.fail(w, err, "tonight")
		return
	}
	kit := strings.TrimSpace(in.Equipment)
	if kit == "" {
		if eq, _ := s.store.Equipment(ctx, uid); len(eq) > 0 {
			kit = eq[0]
		} else {
			kit = "their eyes and maybe binoculars"
		}
	}
	prompt := s.skyFacts(v, wh.TZ, st) + fmt.Sprintf(`
They will look with: %s.
Plan their night in 4 to 6 short sentences: whether it is worth going out and when the best hours are (clouds, the Moon), then what to look at in a good order with the time and the direction for each, and one tip for what they have. If the night is cloudy or bright, say so kindly and suggest the next good night only if it is in the facts. Plain text only.`, kit)
	text, err := cl.Complete(ctx, aiVoice, prompt, false)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	out := map[string]any{"ok": true, "provider": used, "text": strings.TrimSpace(text), "place": wh.Name, "date": date}
	if b, err := json.Marshal(out); err == nil {
		_ = s.store.CachePut(ctx, key, string(b))
	}
	out["at"] = store.Now()
	writeJSON(w, http.StatusOK, out)
}

// handleAIAsk answers a question about the sky tonight at a place.
func (s *Server) handleAIAsk(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	s.loadMinor(ctx)
	uid := actorID(ctx)
	var in struct {
		Question string `json:"question"`
		Location int64  `json:"location_id"`
		Object   string `json:"object"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	in.Question = strings.TrimSpace(in.Question)
	if len(in.Question) < 3 || len(in.Question) > 1000 {
		writeErr(w, http.StatusBadRequest, "ask a question of up to 1000 characters")
		return
	}
	wh, err := s.aiPlace(r, uid, in.Location)
	if err != nil {
		s.failWhere(w, err)
		return
	}
	st, _ := s.store.GetSettings(ctx, uid)
	cl, used, why := s.aiFor(ctx, uid, st)
	if cl == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": why})
		return
	}
	v, err := s.tonight(ctx, uid, wh, nightDate(time.Now(), wh.TZ))
	if err != nil {
		s.fail(w, err, "tonight")
		return
	}
	extra := ""
	if o, ok := objectFor(in.Object); ok {
		extra = "\nThey are asking while looking at: " + o.Name + " (" + o.Type + "). " + o.About + "\n"
		if found := s.objectFacts(o, wh); found != "" {
			extra += found
		}
	}
	prompt := s.skyFacts(v, wh.TZ, st) + extra + `
Question: """` + in.Question + `"""
Answer in at most 5 short sentences from the facts above and what any good book about the sky says. For positions and times only use the facts above; if they are not there, say so. Plain text only.`
	text, err := cl.Complete(ctx, aiVoice, prompt, false)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "provider": used, "answer": strings.TrimSpace(text), "place": wh.Name})
}

// objectFacts is where an object is tonight, as text.
func (s *Server) objectFacts(o catalog.Object, wh where) string {
	now := time.Now()
	n, err := astro.NightAt(wh.Obs, nightDate(now, wh.TZ), wh.TZ)
	if err != nil {
		return ""
	}
	from, to := nightWindow(n)
	var f astro.AltFunc
	if _, ok := astro.Names[o.Key]; ok {
		f = astro.BodyAlt(o.Key, wh.Obs)
	} else if o.Kind != "" {
		ra, dec := astro.OfDate(o.RA, o.Dec, now)
		f = astro.FixedAlt(ra, dec, wh.Obs)
	} else {
		return ""
	}
	best, alt := astro.Culmination(f, from, to)
	_, az := objAltAz(o.Key, o, wh.Obs, best)
	alt0, az0 := objAltAz(o.Key, o, wh.Obs, now)
	return fmt.Sprintf("Now it is %.0f° up in the %s. Tonight it is highest at %s, %.0f° up in the %s.\n", alt0, Direction(az0), best.In(wh.TZ).Format("15:04"), alt, Direction(az))
}

// handleAIExplain tells about one object in a few friendly sentences (cached a month).
func (s *Server) handleAIExplain(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	s.loadMinor(ctx)
	uid := actorID(ctx)
	var in struct {
		Key  string `json:"key"`
		Kids bool   `json:"kids"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	o, ok := objectFor(in.Key)
	if !ok {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	st, _ := s.store.GetSettings(ctx, uid)
	key := fmt.Sprintf("ai:explain:%s:%v:%s", o.Key, in.Kids, st.AIProvider+st.AIPocketModel)
	if raw, at, ok := s.store.CacheGet(ctx, key); ok {
		if t, err := time.Parse(time.RFC3339Nano, at); err == nil && time.Since(t) < 30*24*time.Hour {
			var out map[string]any
			if json.Unmarshal([]byte(raw), &out) == nil {
				out["cached"] = true
				writeJSON(w, http.StatusOK, out)
				return
			}
		}
	}
	cl, used, why := s.aiFor(ctx, uid, st)
	if cl == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": why})
		return
	}
	who := "an adult who is curious but not an astronomer"
	if in.Kids {
		who = "a child of about eight, with one fun comparison"
	}
	facts := fmt.Sprintf("%s: %s. %s", o.Name, o.Type, o.About)
	if o.Alt != "" {
		facts += " Also called " + o.Alt + "."
	}
	if o.Con != "" {
		facts += " In the constellation " + catalog.ConName(o.Con) + "."
	}
	if o.Mag != nil {
		facts += fmt.Sprintf(" Magnitude %.1f.", *o.Mag)
	}
	prompt := "Object: " + facts + "\nTell " + who + " about it in 4 short sentences: what it is, how far and how big (only well-known figures), something remarkable, and how to find or see it. Plain text only."
	text, err := cl.Complete(ctx, aiVoice, prompt, false)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	out := map[string]any{"ok": true, "provider": used, "text": strings.TrimSpace(text), "key": o.Key}
	if b, err := json.Marshal(out); err == nil {
		_ = s.store.CachePut(ctx, key, string(b))
	}
	writeJSON(w, http.StatusOK, out)
}
