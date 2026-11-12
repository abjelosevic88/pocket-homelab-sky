// Package pocket talks to sibling Pocket apps: Pocket Calendar (sky events
// and observing nights), Pocket Notes (a night of the log as a note), Pocket
// Weather and Pocket Places (places to watch the sky from). Each call is a
// bearer-token JSON request; the token comes from Pocket Core (an app link
// acting for one person) or from the person's own settings.
package pocket

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is a bearer-token JSON client for one Pocket app.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// New creates a client.
func New(baseURL, token string, timeout time.Duration) *Client {
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), Token: token, HTTP: &http.Client{Timeout: timeout}}
}

// Configured reports whether URL and token are set.
func (c *Client) Configured() bool { return c != nil && c.BaseURL != "" && c.Token != "" }

// ErrNotFound is a 404 from the sibling app.
var ErrNotFound = errors.New("not found")

func (c *Client) do(ctx context.Context, method, path string, q url.Values, body any, v any) error {
	if !c.Configured() {
		return errors.New("not configured")
	}
	u := c.BaseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized {
		return errors.New("token rejected (401)")
	}
	if res.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if res.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		b, _ := io.ReadAll(io.LimitReader(res.Body, 2000))
		_ = json.Unmarshal(b, &e)
		if e.Error != "" {
			return fmt.Errorf("HTTP %d: %s", res.StatusCode, e.Error)
		}
		return fmt.Errorf("HTTP %d", res.StatusCode)
	}
	if v == nil || res.StatusCode == http.StatusNoContent {
		return nil
	}
	return json.NewDecoder(res.Body).Decode(v)
}

// Health is the connection test for any Pocket app: /healthz plus a token check.
func (c *Client) Health(ctx context.Context) (string, error) {
	var out struct {
		Status  string `json:"status"`
		Version string `json:"version"`
	}
	if !c.Configured() {
		return "", errors.New("not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/healthz", nil)
	if err != nil {
		return "", err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", res.StatusCode)
	}
	_ = json.NewDecoder(res.Body).Decode(&out)
	var me map[string]any
	if err := c.do(ctx, http.MethodGet, "/api/v1/auth/me", nil, nil, &me); err != nil {
		return out.Version, err
	}
	return out.Version, nil
}

// Do is a raw JSON call (for a connection test).
func (c *Client) Do(ctx context.Context, method, path string, q url.Values, body, v any) error {
	return c.do(ctx, method, path, q, body, v)
}

// list decodes a list that may be bare or wrapped in {key:[…]} / {results:[…]}.
func list[T any](raw json.RawMessage, keys ...string) ([]T, error) {
	var out []T
	if err := json.Unmarshal(raw, &out); err == nil {
		return out, nil
	}
	var wrap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, err
	}
	for _, k := range append(keys, "results", "items") {
		if b, ok := wrap[k]; ok {
			if err := json.Unmarshal(b, &out); err != nil {
				return nil, err
			}
			return out, nil
		}
	}
	return []T{}, nil
}

// raw downloads a body as bytes (an attachment), at most limit bytes.
func (c *Client) raw(ctx context.Context, path string, q url.Values, limit int64) ([]byte, string, error) {
	if !c.Configured() {
		return nil, "", errors.New("not configured")
	}
	u := c.BaseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == http.StatusUnauthorized:
		return nil, "", errors.New("token rejected (401)")
	case res.StatusCode == http.StatusNotFound:
		return nil, "", ErrNotFound
	case res.StatusCode >= 300:
		return nil, "", fmt.Errorf("HTTP %d", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(b)) > limit {
		return nil, "", errors.New("too large")
	}
	return b, res.Header.Get("Content-Type"), nil
}

// Event is an event to put on the person's calendar.
type Event struct {
	Title       string
	Description string
	Location    string
	URL         string
	Start       string // YYYY-MM-DDTHH:MM (local), or YYYY-MM-DD for all day
	End         string // YYYY-MM-DDTHH:MM (local); empty = Minutes after Start
	Minutes     int
}

func (e Event) body() map[string]any {
	b := map[string]any{"title": e.Title, "description": e.Description, "location": e.Location, "url": e.URL, "categories": []string{"Sky"}}
	start, err := time.Parse("2006-01-02T15:04", e.Start)
	if err != nil {
		b["all_day"], b["start"] = true, e.Start
		return b
	}
	end := start.Add(time.Duration(max(e.Minutes, 60)) * time.Minute)
	if t, err := time.Parse("2006-01-02T15:04", e.End); err == nil && t.After(start) {
		end = t
	}
	b["all_day"], b["start"], b["end"] = false, start.Format("2006-01-02T15:04"), end.Format("2006-01-02T15:04")
	return b
}

// CreateEvent adds an event and returns its id.
func (c *Client) CreateEvent(ctx context.Context, e Event) (int64, error) {
	var out struct {
		EventID int64 `json:"event_id"`
		ID      any   `json:"id"`
	}
	if err := c.do(ctx, http.MethodPost, "/api/v1/events", nil, e.body(), &out); err != nil {
		return 0, err
	}
	if out.EventID > 0 {
		return out.EventID, nil
	}
	// a bare {"id": 12} or an occurrence id like "12:20261104"
	var id int64
	_, _ = fmt.Sscan(strings.SplitN(fmt.Sprint(out.ID), ":", 2)[0], &id)
	return id, nil
}

// Note is a note in Pocket Notes.
type Note struct {
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	Category string `json:"category"`
}

// CreateNote adds a note in a folder.
func (c *Client) CreateNote(ctx context.Context, title, folder, content string) (*Note, error) {
	var out Note
	if err := c.do(ctx, http.MethodPost, "/api/v1/notes", nil, map[string]any{"title": title, "category": folder, "content": content}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Spot is a named place on the map from another app.
type Spot struct {
	ID        int64    `json:"id"`
	Name      string   `json:"name"`
	Region    string   `json:"region"`
	Country   string   `json:"country"`
	City      string   `json:"city"`
	Lat       *float64 `json:"lat"`
	Lon       *float64 `json:"lon"`
	Lng       *float64 `json:"lng"`
	Elevation *float64 `json:"elevation"`
	Timezone  string   `json:"timezone"`
	Kind      string   `json:"kind"`
}

// Longitude is lon or lng, whichever the app uses.
func (p Spot) Longitude() *float64 {
	if p.Lon != nil {
		return p.Lon
	}
	return p.Lng
}

// WeatherPlaces lists the places the person follows in Pocket Weather.
func (c *Client) WeatherPlaces(ctx context.Context) ([]Spot, error) {
	raw, _, err := c.raw(ctx, "/api/v1/locations", nil, 4<<20)
	if err != nil {
		return nil, err
	}
	return list[Spot](raw, "locations", "items")
}

// SavedPlaces lists the person's places in Pocket Places.
func (c *Client) SavedPlaces(ctx context.Context) ([]Spot, error) {
	raw, _, err := c.raw(ctx, "/api/v1/places", nil, 8<<20)
	if err != nil {
		return nil, err
	}
	return list[Spot](raw, "places", "items")
}
