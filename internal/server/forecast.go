package server

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/abjelosevic88/pocket-homelab-sky/internal/forecast"
)

// ---- the forecast, cached ----
//
// Forecasts are public data about a spot on the map, so they are cached by
// rounded coordinates (about a kilometre) and shared by everyone watching the
// sky there: 30 minutes for the forecast, a month for place names. When
// Open-Meteo does not answer, the last copy is served (stale) rather than
// nothing.

const (
	forecastTTL = 30 * time.Minute
	nameTTL     = 30 * 24 * time.Hour
)

// wxLocks serialises fetches of the same spot so a page full of places
// asking at once fetches each one only once.
type wxLocks struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

func (l *wxLocks) lock(key string) func() {
	l.mu.Lock()
	if l.locks == nil {
		l.locks = map[string]*sync.Mutex{}
	}
	m, ok := l.locks[key]
	if !ok {
		m = &sync.Mutex{}
		l.locks[key] = m
	}
	l.mu.Unlock()
	m.Lock()
	return m.Unlock
}

func spot(lat, lon float64) string {
	return fmt.Sprintf("%.2f:%.2f", math.Round(lat*100)/100, math.Round(lon*100)/100)
}

// cached returns a cached value younger than ttl, else fetches a new one;
// a fetch that fails falls back to any old copy (stale = true).
func cached[T any](ctx context.Context, s *Server, key string, ttl time.Duration, fetch func() (T, error)) (T, bool, error) {
	var zero T
	unlock := s.wxLocks.lock(key)
	defer unlock()
	raw, at, ok := s.store.CacheGet(ctx, key)
	if ok {
		if t, err := time.Parse(time.RFC3339Nano, at); err == nil && time.Since(t) < ttl {
			var v T
			if json.Unmarshal([]byte(raw), &v) == nil {
				return v, false, nil
			}
		}
	}
	v, err := fetch()
	if err != nil {
		if ok {
			var old T
			if json.Unmarshal([]byte(raw), &old) == nil {
				s.log.Debug("forecast: serving stale copy", "key", key, "err", err)
				return old, true, nil
			}
		}
		return zero, false, err
	}
	if b, err := json.Marshal(v); err == nil {
		_ = s.store.CachePut(ctx, key, string(b))
	}
	return v, false, nil
}

// forecastAt is the sky forecast for a spot.
func (s *Server) forecastAt(ctx context.Context, lat, lon float64) (*forecast.Forecast, bool, error) {
	return cached(ctx, s, "fc:"+spot(lat, lon), forecastTTL, func() (*forecast.Forecast, error) { return s.wx.Clouds(ctx, lat, lon) })
}

// placeName names a spot on the map ("Vienna"), cached for a month.
func (s *Server) placeName(ctx context.Context, lat, lon float64, lang string) (*forecast.Place, error) {
	key := fmt.Sprintf("rev:%.2f:%.2f:%s", math.Round(lat*20)/20, math.Round(lon*20)/20, lang)
	p, _, err := cached(ctx, s, key, nameTTL, func() (*forecast.Place, error) { return s.wx.Reverse(ctx, lat, lon, lang) })
	return p, err
}
