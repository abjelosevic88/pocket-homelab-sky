package server

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/abjelosevic88/pocket-homelab-sky/internal/astro"
	"github.com/abjelosevic88/pocket-homelab-sky/internal/store"
)

// seedDemo gives the owner three places — home in Sarajevo, the dark
// plateau of Bjelašnica and the cottage by Boračko lake — eight months of
// observing log with only what was really up on each of those nights, and
// a want-to-see list (DEMO_DATA, for `make run`, the CI smoke test and the
// screenshots). The cloud forecast is the demo source's (forecast.Demo).
func (s *Server) seedDemo(ctx context.Context, uid int64) error {
	// with AUTH_MODE=none this runs inside the first request; finish even
	// when that client goes away
	ctx = context.WithoutCancel(ctx)
	if ls, _ := s.store.ListLocations(ctx, uid); len(ls) > 0 {
		return nil
	}
	rng := rand.New(rand.NewSource(42))
	elev := func(v float64) *float64 { return &v }
	places := []store.Location{
		{Name: "Home", Region: "Sarajevo", Country: "Bosnia and Herzegovina", CountryCode: "BA", Lat: 43.8563, Lon: 18.4131, Elevation: elev(530), Timezone: "Europe/Sarajevo", IsHome: true, Alerts: true, Bortle: 7, Note: "Balcony facing south; the hill hides the north-east."},
		{Name: "Bjelašnica", Region: "Federation of Bosnia and Herzegovina", Country: "Bosnia and Herzegovina", CountryCode: "BA", Lat: 43.7158, Lon: 18.2569, Elevation: elev(1290), Timezone: "Europe/Sarajevo", Alerts: true, Bortle: 3, Note: "Car park above the village: open sky all round, the Milky Way shows."},
		{Name: "Boračko jezero", Region: "Konjic", Country: "Bosnia and Herzegovina", CountryCode: "BA", Lat: 43.5536, Lon: 18.0322, Elevation: elev(405), Timezone: "Europe/Sarajevo", Alerts: false, Bortle: 4, Note: "The cottage by the lake; mountains low in the south."},
	}
	for i := range places {
		places[i].UserID = uid
		if err := s.store.CreateLocation(ctx, &places[i]); err != nil {
			return err
		}
	}
	equipment := []string{"10×50 binoculars", "8-inch Dobsonian", "Eyes", "10×50 binoculars", "8-inch Dobsonian"}
	notes := []string{
		"Steady air after the rain; the views were sharp.",
		"Dew on everything after an hour. Next time bring the hair dryer for the eyepieces.",
		"Neighbour's light was on, so I moved to the other end of the balcony.",
		"Took the kids. They loved the Moon's craters most.",
		"Thin high cloud came in after midnight and spoiled the faint stuff.",
		"",
		"Cold but crystal clear. Saw the Milky Way from horizon to horizon.",
		"",
	}
	itemNotes := map[string]string{
		"jupiter": "Four moons in a line, two on each side.", "saturn": "The rings tilted nicely; Titan beside it.", "moon": "Craters along the terminator stood out.",
		"dso:M31": "A long oval glow, the core clear.", "dso:M45": "Lovely in the binoculars.", "dso:M42": "The wings of the nebula and the four Trapezium stars.",
		"dso:M13": "Fuzzy ball; the edges broke into stars at 150×.", "dso:double-cluster": "Two clusters side by side, a favourite.", "dso:M57": "Tiny smoke ring, needed averted vision.",
	}
	now := time.Now()
	tz := loadTZ("Europe/Sarajevo")
	start := now.AddDate(0, -8, 0)
	var made []store.Observation
	for d := start; d.Before(now.AddDate(0, 0, -2)); d = d.AddDate(0, 0, 3+rng.Intn(9)) {
		pi := 0
		switch r := rng.Intn(10); {
		case r >= 8:
			pi = 1
		case r == 7:
			pi = 2
		}
		pl := places[pi]
		date := d.In(tz).Format("2006-01-02")
		wh := whereLoc(&pl)
		n, err := astro.NightAt(wh.Obs, date, tz)
		if err != nil || n.DarkKind == "none" {
			continue
		}
		kit := equipment[rng.Intn(len(equipment))]
		begin := n.DarkFrom.Add(time.Duration(rng.Intn(90)-30) * time.Minute).In(tz)
		end := begin.Add(time.Duration(60+rng.Intn(150)) * time.Minute)
		o := store.Observation{UserID: uid, LocationID: &pl.ID, Place: pl.Label(), Lat: &pl.Lat, Lon: &pl.Lon, Night: date,
			StartedAt: begin.Format("2006-01-02T15:04"), EndedAt: end.Format("2006-01-02T15:04"), Notes: notes[rng.Intn(len(notes))],
			Seeing: 2 + rng.Intn(4), Transparency: 2 + rng.Intn(4), Bortle: pl.Bortle, Equipment: kit, Rating: 2 + rng.Intn(4)}
		c := rng.Intn(35)
		o.Cloud = &c
		mp := n.MoonPhase
		o.MoonPhase = &mp
		// only what was really up while out there
		mid := begin.Add(end.Sub(begin) / 2)
		if m := astro.At("moon", wh.Obs, mid); m.Alt > 10 && m.Phase > 0.15 {
			o.Objects = append(o.Objects, store.ObsObject{Key: "moon", Name: "Moon", Note: itemNotes["moon"]})
		}
		for _, p := range planetsTonight(wh.Obs, n, tz) {
			if p.Visible && p.Mag < 2 && astro.At(p.Key, wh.Obs, mid).Alt > 12 && rng.Intn(3) > 0 {
				o.Objects = append(o.Objects, store.ObsObject{Key: p.Key, Name: p.Name, Note: itemNotes[p.Key]})
			}
		}
		sights := sightsTonight(wh.Obs, n, pl.Bortle, nil, tz)
		rng.Shuffle(len(sights), func(i, j int) { sights[i], sights[j] = sights[j], sights[i] })
		picked := 0
		for _, x := range sights {
			obj, _ := objectFor(x.Key)
			if a, _ := objAltAz(x.Key, obj, wh.Obs, mid); a < 20 {
				continue
			}
			if picked < 2+rng.Intn(4) && (kit != "Eyes" || x.Kit == "eyes") {
				o.Objects = append(o.Objects, store.ObsObject{Key: x.Key, Name: x.Name, Note: itemNotes[x.Key]})
				picked++
			}
		}
		if len(o.Objects) == 0 {
			continue
		}
		o.Title = fmt.Sprintf("%s with the %s", map[int]string{0: "Balcony evening", 1: "Dark night on the mountain", 2: "At the cottage"}[pi], kit)
		if kit == "Eyes" {
			o.Title = map[int]string{0: "A look from the balcony", 1: "Stargazing on the mountain", 2: "Stars over the lake"}[pi]
		}
		made = append(made, o)
	}
	for i := range made {
		if err := s.store.CreateObservation(ctx, &made[i]); err != nil {
			return err
		}
	}
	for _, t := range []store.Target{
		{Key: "dso:M51", Name: "M51 Whirlpool Galaxy", Note: "Needs the mountain and the Dobsonian."},
		{Key: "dso:M13", Name: "M13 Great Hercules Cluster"},
		{Key: "dso:double-cluster", Name: "Double Cluster"},
		{Key: "saturn", Name: "Saturn", Note: "Show the rings to the kids."},
		{Key: "dso:M33", Name: "M33 Triangulum Galaxy", Note: "Only from a really dark sky."},
		{Key: "dso:omega-centauri", Name: "Omega Centauri", Note: "On the trip south one day."},
	} {
		t := t
		_ = s.store.AddTarget(ctx, uid, &t)
	}
	return nil
}
