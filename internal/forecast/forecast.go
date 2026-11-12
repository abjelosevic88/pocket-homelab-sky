// Package forecast talks to Open-Meteo (free, no key) for what matters to
// the night sky: cloud by the hour (low, middle and high), how far you can
// see, damp and dew, wind and the chance of rain, for the next week; and for
// place search, with OpenStreetMap's Nominatim for the name of a spot on the
// map ("where am I").
package forecast

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Endpoints, variables so tests can point them at a fake server.
var (
	ForecastURL = "https://api.open-meteo.com/v1/forecast"
	GeocodeURL  = "https://geocoding-api.open-meteo.com/v1/search"
	ReverseURL  = "https://nominatim.openstreetmap.org/reverse"
)

// Source is where forecasts and place names come from: Open-Meteo, or the
// demo's made-up skies.
type Source interface {
	Clouds(ctx context.Context, lat, lon float64) (*Forecast, error)
	Search(ctx context.Context, name, lang string, count int) ([]Place, error)
	Reverse(ctx context.Context, lat, lon float64, lang string) (*Place, error)
}

// Client fetches forecasts.
type Client struct {
	UserAgent string
	HTTP      *http.Client
}

// New creates a client.
func New(ua string) *Client {
	return &Client{UserAgent: ua, HTTP: &http.Client{Timeout: 20 * time.Second}}
}

// Hour is one hour of the sky forecast. Time is the wall clock where the
// place is (2026-11-12T21:00).
type Hour struct {
	Time       string   `json:"time"`
	Cloud      *float64 `json:"cloud"`      // % of the sky
	Low        *float64 `json:"low"`        // % low cloud (fog, stratus)
	Mid        *float64 `json:"mid"`        // % middle cloud
	High       *float64 `json:"high"`       // % high cloud (thin veils)
	Visibility *float64 `json:"visibility"` // metres
	Humidity   *float64 `json:"humidity"`   // %
	Temp       *float64 `json:"temp"`       // °C
	Dew        *float64 `json:"dew"`        // °C
	Wind       *float64 `json:"wind"`       // km/h
	PrecipProb *float64 `json:"precip_prob"`
}

// Forecast is the next week for one spot.
type Forecast struct {
	Lat       float64 `json:"lat"`
	Lon       float64 `json:"lon"`
	Elevation float64 `json:"elevation"`
	Timezone  string  `json:"timezone"`
	Fetched   string  `json:"fetched_at"`
	Hours     []Hour  `json:"hours"`
}

// Location is the forecast's time zone (UTC when unknown).
func (f *Forecast) Location() *time.Location {
	if f != nil && f.Timezone != "" {
		if l, err := time.LoadLocation(f.Timezone); err == nil {
			return l
		}
	}
	return time.UTC
}

// At is the forecast hour holding a moment (nil when out of range).
func (f *Forecast) At(t time.Time) *Hour {
	if f == nil {
		return nil
	}
	k := t.In(f.Location()).Format("2006-01-02T15") + ":00"
	for i := range f.Hours {
		if f.Hours[i].Time == k {
			return &f.Hours[i]
		}
	}
	return nil
}

type series []*float64

func (s series) at(i int) *float64 {
	if i < 0 || i >= len(s) {
		return nil
	}
	return s[i]
}

const hourlyVars = "cloud_cover,cloud_cover_low,cloud_cover_mid,cloud_cover_high,visibility,relative_humidity_2m,temperature_2m,dew_point_2m,wind_speed_10m,precipitation_probability"

// Clouds is the sky forecast for the next seven days, by the hour.
func (c *Client) Clouds(ctx context.Context, lat, lon float64) (*Forecast, error) {
	q := url.Values{}
	q.Set("latitude", coord(lat))
	q.Set("longitude", coord(lon))
	q.Set("hourly", hourlyVars)
	q.Set("timezone", "auto")
	q.Set("forecast_days", "8")
	q.Set("past_days", "1")
	var body struct {
		Lat       float64 `json:"latitude"`
		Lon       float64 `json:"longitude"`
		Elevation float64 `json:"elevation"`
		Timezone  string  `json:"timezone"`
		Hourly    struct {
			Time       []string `json:"time"`
			Cloud      series   `json:"cloud_cover"`
			Low        series   `json:"cloud_cover_low"`
			Mid        series   `json:"cloud_cover_mid"`
			High       series   `json:"cloud_cover_high"`
			Visibility series   `json:"visibility"`
			Humidity   series   `json:"relative_humidity_2m"`
			Temp       series   `json:"temperature_2m"`
			Dew        series   `json:"dew_point_2m"`
			Wind       series   `json:"wind_speed_10m"`
			PrecipProb series   `json:"precipitation_probability"`
		} `json:"hourly"`
	}
	if err := c.get(ctx, ForecastURL, q, &body); err != nil {
		return nil, err
	}
	h := body.Hourly
	if len(h.Time) == 0 {
		return nil, errors.New("open-meteo: no forecast for this place")
	}
	f := &Forecast{Lat: body.Lat, Lon: body.Lon, Elevation: body.Elevation, Timezone: body.Timezone, Fetched: time.Now().UTC().Format(time.RFC3339)}
	f.Hours = make([]Hour, 0, len(h.Time))
	for i, t := range h.Time {
		f.Hours = append(f.Hours, Hour{Time: t, Cloud: h.Cloud.at(i), Low: h.Low.at(i), Mid: h.Mid.at(i), High: h.High.at(i), Visibility: h.Visibility.at(i),
			Humidity: h.Humidity.at(i), Temp: h.Temp.at(i), Dew: h.Dew.at(i), Wind: h.Wind.at(i), PrecipProb: h.PrecipProb.at(i)})
	}
	return f, nil
}

// Place is a search result.
type Place struct {
	Name        string  `json:"name"`
	Region      string  `json:"region"`
	Country     string  `json:"country"`
	CountryCode string  `json:"country_code"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
	Elevation   float64 `json:"elevation"`
	Timezone    string  `json:"timezone"`
	Population  int     `json:"population"`
}

// Search finds places by name (cities, towns, villages).
func (c *Client) Search(ctx context.Context, name, lang string, count int) ([]Place, error) {
	if count <= 0 || count > 20 {
		count = 8
	}
	q := url.Values{}
	q.Set("name", name)
	q.Set("count", fmt.Sprint(count))
	q.Set("format", "json")
	if lang != "" {
		q.Set("language", lang)
	}
	var body struct {
		Results []struct {
			Name        string  `json:"name"`
			Admin1      string  `json:"admin1"`
			Country     string  `json:"country"`
			CountryCode string  `json:"country_code"`
			Lat         float64 `json:"latitude"`
			Lon         float64 `json:"longitude"`
			Elevation   float64 `json:"elevation"`
			Timezone    string  `json:"timezone"`
			Population  int     `json:"population"`
		} `json:"results"`
	}
	if err := c.get(ctx, GeocodeURL, q, &body); err != nil {
		return nil, err
	}
	out := []Place{}
	for _, r := range body.Results {
		out = append(out, Place{Name: r.Name, Region: r.Admin1, Country: r.Country, CountryCode: strings.ToUpper(r.CountryCode), Lat: r.Lat, Lon: r.Lon,
			Elevation: r.Elevation, Timezone: r.Timezone, Population: r.Population})
	}
	return out, nil
}

// Reverse names a spot on the map: the town or city it is in.
func (c *Client) Reverse(ctx context.Context, lat, lon float64, lang string) (*Place, error) {
	q := url.Values{}
	q.Set("lat", coord(lat))
	q.Set("lon", coord(lon))
	q.Set("format", "jsonv2")
	q.Set("zoom", "10")
	if lang != "" {
		q.Set("accept-language", lang)
	}
	var body struct {
		Name    string `json:"name"`
		Address struct {
			City        string `json:"city"`
			Town        string `json:"town"`
			Village     string `json:"village"`
			Municipalty string `json:"municipality"`
			County      string `json:"county"`
			State       string `json:"state"`
			Country     string `json:"country"`
			CountryCode string `json:"country_code"`
		} `json:"address"`
	}
	if err := c.get(ctx, ReverseURL, q, &body); err != nil {
		return nil, err
	}
	a := body.Address
	// a city split into municipalities (Sarajevo, Belgrade) names the
	// municipality first; the county "City of Sarajevo" is what people say
	county := ""
	for _, p := range []string{"City of ", "Grad ", "Stadt "} {
		if strings.HasPrefix(a.County, p) {
			county = strings.TrimPrefix(a.County, p)
		}
	}
	muni := strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(a.Municipalty, " Municipality"), " municipality"))
	name := firstNonEmpty(a.City, a.Town, a.Village, county, muni, body.Name, a.County)
	if name == "" {
		return nil, errors.New("no name for this spot")
	}
	return &Place{Name: name, Region: a.State, Country: a.Country, CountryCode: strings.ToUpper(a.CountryCode), Lat: lat, Lon: lon}, nil
}

func (c *Client) get(ctx context.Context, base string, q url.Values, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", host(base), err)
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		var e struct {
			Reason string `json:"reason"`
			Error  any    `json:"error"`
		}
		b, _ := io.ReadAll(io.LimitReader(res.Body, 4000))
		_ = json.Unmarshal(b, &e)
		if e.Reason == "" {
			e.Reason = res.Status
		}
		return fmt.Errorf("%s: %s", host(base), e.Reason)
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 16<<20)).Decode(v); err != nil {
		return fmt.Errorf("%s: %w", host(base), err)
	}
	return nil
}

func host(u string) string {
	if p, err := url.Parse(u); err == nil {
		return p.Host
	}
	return u
}

func coord(v float64) string { return fmt.Sprintf("%.4f", v) }

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

// ---- words ----

// Distance is the great-circle distance in km.
func Distance(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371.0
	p1, p2 := lat1*math.Pi/180, lat2*math.Pi/180
	dp, dl := (lat2-lat1)*math.Pi/180, (lon2-lon1)*math.Pi/180
	a := math.Sin(dp/2)*math.Sin(dp/2) + math.Cos(p1)*math.Cos(p2)*math.Sin(dl/2)*math.Sin(dl/2)
	return 2 * r * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
