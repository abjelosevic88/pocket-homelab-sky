package forecast

import (
	"context"
	"math"
	"math/rand"
	"strings"
	"time"
)

// Demo is made-up but believable skies (DEMO_DATA) so `make run`, the CI
// smoke test and the screenshots work without the network: tonight and two
// more nights this week clear, the others cloudy or mixed, with a veil of
// high cloud now and then. Same place and hour, same sky.
type Demo struct{}

// demoNights is the shape of the coming nights: the cloud cover in the
// evening and in the small hours.
var demoNights = [][2]float64{{12, 6}, {8, 4}, {70, 85}, {35, 15}, {95, 90}, {20, 55}, {5, 10}, {60, 30}, {15, 10}}

// Clouds implements Source.
func (Demo) Clouds(_ context.Context, lat, lon float64) (*Forecast, error) {
	now := time.Now()
	loc := time.Local
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	f := &Forecast{Lat: lat, Lon: lon, Elevation: 518, Timezone: loc.String(), Fetched: now.UTC().Format(time.RFC3339)}
	if f.Timezone == "Local" {
		f.Timezone = ""
	}
	rng := rand.New(rand.NewSource(int64(lat*100)*7919 + int64(lon*100) + int64(today.YearDay())))
	for h := -24; h < 8*24; h++ {
		t := today.Add(time.Duration(h) * time.Hour)
		day := int(t.Sub(today).Hours()) / 24
		if t.Hour() < 12 {
			day-- // the small hours belong to the night before
		}
		if day < -1 {
			day = -1
		}
		n := demoNights[(day+1)%len(demoNights)]
		hr := t.Hour()
		// evening (18–24) leans to the first value, the small hours to the second, the day in between
		var c float64
		switch {
		case hr >= 18:
			c = n[0]
		case hr < 6:
			c = n[1]
		default:
			c = (n[0]+n[1])/2 + 15
		}
		c = math.Max(0, math.Min(100, c+rng.Float64()*14-7))
		high := math.Min(100, c*0.6+rng.Float64()*10)
		low := math.Max(0, c*0.5-10+rng.Float64()*8)
		mid := math.Max(0, c*0.4+rng.Float64()*6)
		temp := 6 - 5*math.Cos(2*math.Pi*float64((hr+9)%24)/24) - 0.3*float64(day)
		hum := math.Min(98, 60+c*0.3+float64((24-hr)%24)*0.6)
		dew := temp - (100-hum)/5
		vis := 24000 - c*120
		wind := 4 + rng.Float64()*10
		pp := math.Max(0, c-60)
		f.Hours = append(f.Hours, Hour{Time: t.Format("2006-01-02T15") + ":00", Cloud: r0(c), Low: r0(low), Mid: r0(mid), High: r0(high),
			Visibility: r0(vis), Humidity: r0(hum), Temp: r1(temp), Dew: r1(dew), Wind: r1(wind), PrecipProb: r0(pp)})
	}
	return f, nil
}

func r0(v float64) *float64 { r := math.Round(v); return &r }
func r1(v float64) *float64 { r := math.Round(v*10) / 10; return &r }

var demoPlaces = []Place{
	{Name: "Sarajevo", Region: "Federation of Bosnia and Herzegovina", Country: "Bosnia and Herzegovina", CountryCode: "BA", Lat: 43.8486, Lon: 18.3564, Elevation: 518, Timezone: "Europe/Sarajevo", Population: 696731},
	{Name: "Mostar", Region: "Federation of Bosnia and Herzegovina", Country: "Bosnia and Herzegovina", CountryCode: "BA", Lat: 43.3433, Lon: 17.8081, Elevation: 64, Timezone: "Europe/Sarajevo", Population: 104518},
	{Name: "Jahorina", Region: "Republika Srpska", Country: "Bosnia and Herzegovina", CountryCode: "BA", Lat: 43.7383, Lon: 18.5675, Elevation: 1557, Timezone: "Europe/Sarajevo"},
	{Name: "Vienna", Region: "Vienna", Country: "Austria", CountryCode: "AT", Lat: 48.2085, Lon: 16.3721, Elevation: 171, Timezone: "Europe/Vienna", Population: 1691468},
	{Name: "Zagreb", Region: "City of Zagreb", Country: "Croatia", CountryCode: "HR", Lat: 45.8144, Lon: 15.978, Elevation: 158, Timezone: "Europe/Zagreb", Population: 698966},
	{Name: "Split", Region: "Split-Dalmatia", Country: "Croatia", CountryCode: "HR", Lat: 43.5089, Lon: 16.4392, Elevation: 0, Timezone: "Europe/Zagreb", Population: 176314},
	{Name: "Munich", Region: "Bavaria", Country: "Germany", CountryCode: "DE", Lat: 48.1374, Lon: 11.5755, Elevation: 524, Timezone: "Europe/Berlin", Population: 1260391},
	{Name: "Berlin", Region: "Berlin", Country: "Germany", CountryCode: "DE", Lat: 52.5244, Lon: 13.4105, Elevation: 43, Timezone: "Europe/Berlin", Population: 3426354},
	{Name: "London", Region: "England", Country: "United Kingdom", CountryCode: "GB", Lat: 51.5085, Lon: -0.1257, Elevation: 25, Timezone: "Europe/London", Population: 7556900},
	{Name: "Lisbon", Region: "Lisbon", Country: "Portugal", CountryCode: "PT", Lat: 38.7167, Lon: -9.1333, Elevation: 45, Timezone: "Europe/Lisbon", Population: 517802},
	{Name: "New York", Region: "New York", Country: "United States", CountryCode: "US", Lat: 40.7143, Lon: -74.006, Elevation: 10, Timezone: "America/New_York", Population: 8175133},
}

// Search implements Source over a few well-known places.
func (Demo) Search(_ context.Context, name, _ string, count int) ([]Place, error) {
	n := strings.ToLower(strings.TrimSpace(name))
	out := []Place{}
	for _, p := range demoPlaces {
		if n != "" && strings.HasPrefix(strings.ToLower(p.Name), n) {
			out = append(out, p)
		}
	}
	if count > 0 && len(out) > count {
		out = out[:count]
	}
	return out, nil
}

// Reverse implements Source: the nearest known place.
func (Demo) Reverse(_ context.Context, lat, lon float64, _ string) (*Place, error) {
	best, dist := demoPlaces[0], math.MaxFloat64
	for _, p := range demoPlaces {
		if d := Distance(lat, lon, p.Lat, p.Lon); d < dist {
			best, dist = p, d
		}
	}
	best.Lat, best.Lon = lat, lon
	return &best, nil
}
