package server

import (
	"context"

	"fmt"
	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-sky/internal/astro"
	"github.com/abjelosevic88/pocket-homelab-sky/internal/notify"
	"github.com/abjelosevic88/pocket-homelab-sky/internal/store"
)

// ---- the evening message ----
//
// Every minute the janitor looks whose hour it is (17:00 by default). Each
// person gets at most one message an evening: tonight is clear and dark at
// one of their places (by their own measure of "clear"), and/or a sky event
// they chose happens in the next day. Nothing to say, no message. The same
// news goes on the Core bus: sky.clear_night and sky.event.

func (s *Server) evenings(ctx context.Context) {
	ids, err := s.store.SettingsUsers(ctx)
	if err != nil {
		return
	}
	now := time.Now()
	for _, uid := range ids {
		st, err := s.store.GetSettings(ctx, uid)
		if err != nil {
			continue
		}
		home, err := s.store.HomeLocation(ctx, uid)
		if err != nil {
			continue
		}
		lt := now.In(loadTZ(home.Timezone))
		if lt.Hour() != st.NotifyHour || lt.Minute() > 10 {
			continue
		}
		if ok, _ := s.store.MarkSent(ctx, uid, "evening:"+lt.Format("2006-01-02")); !ok {
			continue
		}
		s.evening(ctx, uid, st, false)
	}
}

// evening builds and sends one person's evening message (force: for "Send
// it now", sent even when there is little to say). It returns the text.
func (s *Server) evening(ctx context.Context, uid int64, st store.Settings, force bool) (string, bool) {
	locs, err := s.store.ListLocations(ctx, uid)
	if err != nil || len(locs) == 0 {
		return "", false
	}
	var lines []string
	title := ""
	prio := "info"
	link := strings.TrimRight(s.cfg.BaseURL, "/") + "/"
	// Clear nights at the places the person wants to hear about.
	for i := range locs {
		l := &locs[i]
		if !l.Alerts && !l.IsHome {
			continue
		}
		wh := whereLoc(l)
		date := nightDate(time.Now(), wh.TZ)
		v, err := s.tonight(ctx, uid, wh, date)
		if err != nil {
			continue
		}
		clear := v.Score.Clear && v.Score.Value >= 50 && v.Night.DarkKind != "none"
		if clear {
			key := fmt.Sprintf("clear:%s:%d", date, l.ID)
			fresh, _ := s.store.MarkSent(ctx, uid, key+":bus")
			if fresh {
				s.publish(ctx, uid, "sky.clear_night", key, map[string]any{"place": l.Name, "date": date, "score": v.Score.Value, "label": v.Score.Label,
					"cloud": v.Score.CloudAvg, "dark_from": v.Night.DarkFrom, "dark_to": v.Night.DarkTo, "moon": v.Moon.PhaseName})
			}
		}
		if !(clear && st.NotifyClear) && !(force && l.IsHome) {
			continue
		}
		vis := []string{}
		for _, p := range v.Planets {
			if p.Visible && len(vis) < 4 {
				vis = append(vis, p.Name+" ("+p.When+")")
			}
		}
		line := fmt.Sprintf("%s: %s night (%d). %s", l.Name, v.Score.Label, v.Score.Value, v.Score.Words)
		if v.Night.DarkKind != "none" {
			line += fmt.Sprintf(" Dark from %s to %s.", hm(&v.Night.DarkFrom, wh.TZ), hm(&v.Night.DarkTo, wh.TZ))
		}
		line += " Moon: " + v.Moon.Words
		if len(vis) > 0 {
			line += " Planets: " + strings.Join(vis, ", ") + "."
		}
		lines = append(lines, line)
		if title == "" {
			if clear {
				title = "Clear sky tonight at " + l.Name
			} else {
				title = "Tonight's sky at " + l.Name
			}
		}
	}
	// Sky events in the next day, of the kinds the person chose.
	if home, err := s.store.HomeLocation(ctx, uid); err == nil && (st.NotifyEvents || force) {
		wh := whereLoc(home)
		now := time.Now()
		for _, e := range s.eventsNear(ctx, wh, now.Add(-time.Hour), now.Add(30*time.Hour), st.EventKinds) {
			if e.T.Before(now.Add(-time.Hour)) || e.T.After(now.Add(26*time.Hour)) || (e.Visible != nil && !*e.Visible) {
				continue
			}
			fresh, _ := s.store.MarkSent(ctx, uid, "event:"+e.Key)
			if !fresh && !force {
				continue
			}
			if fresh {
				s.publish(ctx, uid, "sky.event", e.Key, map[string]any{"kind": e.Kind, "title": e.Title, "at": e.T, "detail": e.Detail, "place": home.Name})
			}
			when := e.T.In(wh.TZ).Format("Mon 15:04")
			if e.Start != nil && e.End != nil {
				when = e.Start.In(wh.TZ).Format("Mon 15:04") + "–" + e.End.In(wh.TZ).Format("15:04")
			}
			lines = append(lines, fmt.Sprintf("✦ %s, %s. %s", e.Title, when, e.Detail))
			if strings.Contains(e.Kind, "eclipse") {
				prio = "high"
			}
			if title == "" {
				title = e.Title
			}
		}
	}
	// The northern (or southern) lights tonight at home, when a storm is forecast.
	if home, err := s.store.HomeLocation(ctx, uid); err == nil && (oneOf("aurora", st.EventKinds) && st.NotifyEvents || force) {
		wh := whereLoc(home)
		if n, err := astro.NightAt(wh.Obs, nightDate(time.Now(), wh.TZ), wh.TZ); err == nil {
			if a := s.auroraTonight(ctx, wh, n); a != nil && a.Likely {
				key := "aurora:" + n.Date
				fresh, _ := s.store.MarkSent(ctx, uid, key)
				if fresh || force {
					lines = append(lines, "✦ "+a.Words)
					if title == "" {
						title = "Northern lights possible tonight"
					}
					prio = "high"
				}
				if fresh {
					s.publish(ctx, uid, "sky.event", key, map[string]any{"kind": "aurora", "title": "Aurora possible", "kp": a.KpMax, "kp_needed": a.KpNeeded, "place": home.Name})
				}
			}
		}
	}
	if len(lines) == 0 {
		return "", false
	}
	body := strings.Join(lines, "\n")
	_ = link
	return body, s.sendMessage(ctx, uid, st, title, body, prio)
}

// sendMessage sends one message: to the person's message URL (Apprise, ntfy
// or a webhook) and, through Pocket Core, to their phones with Pocket Hub.
// True when it went anywhere.
func (s *Server) sendMessage(ctx context.Context, uid int64, st store.Settings, title, body, prio string) bool {
	sent := false
	link := strings.TrimRight(s.cfg.BaseURL, "/") + "/"
	if st.NotifyURL != "" {
		n := notify.New(st.NotifyURL, st.NotifyFormat)
		if err := n.Send(ctx, notify.Message{Title: title, Body: body, Priority: prio, Tags: []string{"star2"}, Link: link}); err != nil {
			s.log.Warn("notify", "user", uid, "err", err)
		} else {
			sent = true
		}
	}
	if st.PushPhone && s.core.Configured() {
		if u, err := s.store.GetUser(ctx, uid); err == nil {
			res, err := s.core.Notify(ctx, u.Email, core.PushMessage{Title: title, Body: body, URL: link, Source: "sky", Thread: "sky", Sound: prio == "high"})
			if err == nil && res != nil && res.Sent > 0 {
				sent = true
			} else if err != nil {
				s.log.Debug("push", "user", uid, "err", err)
			}
		}
	}
	return sent
}

// janitor runs the background work: every minute the evening messages (at
// each person's own hour), every six hours forgetting old caches and sent marks.
func (s *Server) janitor(ctx context.Context) {
	minute := time.NewTicker(time.Minute)
	tidy := time.NewTicker(6 * time.Hour)
	defer minute.Stop()
	defer tidy.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-minute.C:
			s.evenings(ctx)
			s.satAlerts(ctx)
		case <-tidy.C:
			old := time.Now().AddDate(0, 0, -60).UTC().Format(time.RFC3339Nano)
			_ = s.store.PurgeSent(ctx, old)
			_ = s.store.CachePurge(ctx, "events:", time.Now().Add(-48*time.Hour).UTC().Format(time.RFC3339Nano))
			_ = s.store.CachePurge(ctx, "fc:", time.Now().Add(-7*24*time.Hour).UTC().Format(time.RFC3339Nano))
			_ = s.store.PurgeSessions(ctx)
		}
	}
}

func (s *Server) handleNotifyTest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	st, _ := s.store.GetSettings(ctx, actorID(ctx))
	n := notify.New(st.NotifyURL, st.NotifyFormat)
	if err := n.Send(ctx, notify.Message{Title: "Pocket Sky", Body: notifyTestBody, Priority: "info", Tags: []string{"star2"}}); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleCheckNow sends the evening message now and says what it said.
func (s *Server) handleCheckNow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	st, _ := s.store.GetSettings(ctx, uid)
	body, sent := s.evening(ctx, uid, st, true)
	writeJSON(w, http.StatusOK, map[string]any{"sent": sent, "body": body})
}

const notifyTestBody = "Notifications work. Clear, dark nights at your places and the sky events you chose will arrive here in the evening."
