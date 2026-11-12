// Package space fetches what the cloud forecast does not say: how steady and
// how clear the air will be for the stars (7Timer! ASTRO, free, no key), and
// the northern (and southern) lights: NOAA SWPC's Kp forecast and its
// OVATION aurora map. Everything is cached by the caller.
package space

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Endpoints, variables so tests can point them at a fake server.
var (
	SeeingURL  = "https://www.7timer.info/bin/astro.php"
	KpURL      = "https://services.swpc.noaa.gov/products/noaa-planetary-k-index-forecast.json"
	OvationURL = "https://services.swpc.noaa.gov/json/ovation_aurora_latest.json"
)

func get(ctx context.Context, hc *http.Client, ua, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", ua)
	res, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d", strings.SplitN(strings.TrimPrefix(strings.TrimPrefix(url, "https://"), "http://"), "/", 2)[0], res.StatusCode)
	}
	return io.ReadAll(io.LimitReader(res.Body, limit))
}

// ---- seeing and transparency ----

// AirHour is how the air will be for looking at the stars, every three hours.
// Seeing is how steady (1 = stars boil … 5 = rock steady); Transparency is
// how clear (1 = hazy … 5 = crystal). Both are 7Timer!'s 8-step scales
// turned round and folded to 5 steps, so 5 is always best.
type AirHour struct {
	Time         time.Time `json:"time"`
	Seeing       int       `json:"seeing"`
	Transparency int       `json:"transparency"`
	Cloud        int       `json:"cloud"` // 7Timer!'s own 1–9
}

// Air is the next three days of seeing and transparency at a spot.
type Air struct {
	Fetched time.Time `json:"fetched_at"`
	Hours   []AirHour `json:"hours"`
}

// fold turns 7Timer!'s 1 (best) … 8 (worst) into 5 (best) … 1 (worst).
func fold(v int) int {
	switch {
	case v <= 0:
		return 0
	case v <= 2:
		return 5
	case v <= 3:
		return 4
	case v <= 5:
		return 3
	case v <= 6:
		return 2
	}
	return 1
}

// FetchAir asks 7Timer! for the seeing and transparency at a spot.
func FetchAir(ctx context.Context, hc *http.Client, ua string, lat, lon float64) (*Air, error) {
	url := fmt.Sprintf("%s?lon=%.3f&lat=%.3f&ac=0&unit=metric&output=json&tzshift=0", SeeingURL, lon, lat)
	b, err := get(ctx, hc, ua, url, 1<<20)
	if err != nil {
		return nil, err
	}
	return ParseAir(b)
}

// ParseAir reads 7Timer!'s ASTRO JSON.
func ParseAir(b []byte) (*Air, error) {
	var body struct {
		Init string `json:"init"` // 2026111212
		Data []struct {
			Timepoint    int `json:"timepoint"`
			Cloud        int `json:"cloudcover"`
			Seeing       int `json:"seeing"`
			Transparency int `json:"transparency"`
		} `json:"dataseries"`
	}
	if err := json.Unmarshal(b, &body); err != nil {
		return nil, err
	}
	init, err := time.Parse("2006010215", body.Init)
	if err != nil || len(body.Data) == 0 {
		return nil, errors.New("7timer: no forecast")
	}
	a := &Air{Fetched: time.Now().UTC()}
	for _, d := range body.Data {
		a.Hours = append(a.Hours, AirHour{Time: init.Add(time.Duration(d.Timepoint) * time.Hour), Seeing: fold(d.Seeing), Transparency: fold(d.Transparency), Cloud: d.Cloud})
	}
	return a, nil
}

// At is the three-hour step holding a moment (nil out of range).
func (a *Air) At(t time.Time) *AirHour {
	if a == nil {
		return nil
	}
	var best *AirHour
	for i := range a.Hours {
		h := &a.Hours[i]
		if d := t.Sub(h.Time); d > -90*time.Minute && d <= 90*time.Minute {
			best = h
		}
	}
	return best
}

// ---- aurora ----

// KpStep is one three-hour step of the planetary K index (0 quiet … 9 extreme storm).
type KpStep struct {
	Time     time.Time `json:"time"`
	Kp       float64   `json:"kp"`
	Observed bool      `json:"observed"`
}

// Aurora is the space weather now: the Kp steps around today and the
// OVATION map of where the aurora is likely in the next half hour.
type Aurora struct {
	Fetched time.Time `json:"fetched_at"`
	Kp      []KpStep  `json:"kp"`
	grid    map[[2]int]int
}

// FetchAurora gets the Kp forecast and the OVATION map.
func FetchAurora(ctx context.Context, hc *http.Client, ua string) (*Aurora, error) {
	b, err := get(ctx, hc, ua, KpURL, 1<<20)
	if err != nil {
		return nil, err
	}
	kp, err := ParseKp(b)
	if err != nil {
		return nil, err
	}
	a := &Aurora{Fetched: time.Now().UTC(), Kp: kp}
	if ob, err := get(ctx, hc, ua, OvationURL, 8<<20); err == nil {
		a.grid, _ = ParseOvation(ob)
	}
	return a, nil
}

// ParseKp reads SWPC's Kp forecast (a list of objects, or of rows in older versions).
func ParseKp(b []byte) ([]KpStep, error) {
	var objs []struct {
		Time     string  `json:"time_tag"`
		Kp       float64 `json:"kp"`
		Observed string  `json:"observed"`
	}
	out := []KpStep{}
	parse := func(s string) (time.Time, error) {
		for _, f := range []string{"2006-01-02T15:04:05", "2006-01-02 15:04:05"} {
			if t, err := time.Parse(f, s); err == nil {
				return t, nil
			}
		}
		return time.Time{}, errors.New("bad time " + s)
	}
	if err := json.Unmarshal(b, &objs); err == nil {
		for _, o := range objs {
			if t, err := parse(o.Time); err == nil {
				out = append(out, KpStep{Time: t, Kp: o.Kp, Observed: o.Observed == "observed"})
			}
		}
		return out, nil
	}
	var rows [][]string
	if err := json.Unmarshal(b, &rows); err != nil {
		return nil, err
	}
	for i, r := range rows {
		if i == 0 || len(r) < 3 {
			continue
		}
		t, err := parse(r[0])
		kp, err2 := strconv.ParseFloat(r[1], 64)
		if err == nil && err2 == nil {
			out = append(out, KpStep{Time: t, Kp: kp, Observed: r[2] == "observed"})
		}
	}
	return out, nil
}

// ParseOvation reads the OVATION map: [lon 0–359, lat −90–90, chance %].
func ParseOvation(b []byte) (map[[2]int]int, error) {
	var body struct {
		Coords [][3]float64 `json:"coordinates"`
	}
	if err := json.Unmarshal(b, &body); err != nil {
		return nil, err
	}
	g := make(map[[2]int]int, len(body.Coords))
	for _, c := range body.Coords {
		if c[2] > 0 {
			g[[2]int{int(c[0]), int(c[1])}] = int(c[2])
		}
	}
	return g, nil
}

// Chance is OVATION's chance (%) of aurora overhead at a spot right now.
func (a *Aurora) Chance(lat, lon float64) int {
	if a == nil || a.grid == nil {
		return 0
	}
	x := int(math.Round(lon))
	if x < 0 {
		x += 360
	}
	return a.grid[[2]int{x % 360, int(math.Round(lat))}]
}

// MaxKp is the highest Kp forecast between two moments.
func (a *Aurora) MaxKp(from, to time.Time) float64 {
	m := 0.0
	if a == nil {
		return m
	}
	for _, s := range a.Kp {
		end := s.Time.Add(3 * time.Hour)
		if end.After(from) && s.Time.Before(to) && s.Kp > m {
			m = s.Kp
		}
	}
	return m
}

// MagLat is the geomagnetic latitude of a place (a tilted dipole, pole at 80.8° N 72.8° W).
func MagLat(lat, lon float64) float64 {
	const plat, plon = 80.8, -72.8
	r := math.Pi / 180
	s := math.Sin(lat*r)*math.Sin(plat*r) + math.Cos(lat*r)*math.Cos(plat*r)*math.Cos((lon-plon)*r)
	return math.Asin(math.Max(-1, math.Min(1, s))) / r
}

// KpNeeded is about the Kp at which the aurora shows low on the poleward
// horizon from a place: the oval reaches ~66.5° − 2·Kp magnetic latitude
// overhead and is seen up to ~10° further. Capped at 9; 0 = most nights.
func KpNeeded(lat, lon float64) float64 {
	m := math.Abs(MagLat(lat, lon))
	k := (56.5 - m) / 2
	return math.Max(0, math.Min(9, math.Round(k*2)/2))
}
