package sat

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// celestrakURL is CelesTrak's GP query (a variable so tests can point it at
// a local server).
var celestrakURL = "https://celestrak.org/NORAD/elements/gp.php"

// maxStarlinks is how many of the newest Starlinks are kept.
const maxStarlinks = 120

// Fetch downloads the elements from CelesTrak: the two crewed stations (ISS
// and Tiangong) from the stations list, the brightest satellites from the
// visual list and the Starlinks launched in the last 30 days (the newest
// 120). CelesTrak updates the elements every few hours and asks people not
// to download the same list more often than that, so callers should keep
// the catalogue and fetch again only every few hours.
//
// If some of the lists could not be had, Fetch returns the satellites of the
// others together with an error naming what failed; if none came, the
// catalogue is nil.
func Fetch(ctx context.Context, hc *http.Client, ua string) (*Catalog, error) {
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	cat := &Catalog{Fetched: time.Now().UTC()}
	seen := map[int]bool{}
	add := func(ss []Sat) {
		for _, s := range ss {
			if s.NoradID != 0 && seen[s.NoradID] {
				continue
			}
			seen[s.NoradID] = true
			cat.Sats = append(cat.Sats, s)
		}
	}
	var errs []error

	if text, err := get(ctx, hc, ua, "stations"); err != nil {
		errs = append(errs, err)
	} else {
		var keep []Sat
		for _, s := range ParseTLE(text, GroupStation) {
			n := strings.ToUpper(s.Name)
			if s.NoradID == noradISS || s.NoradID == noradCSS ||
				strings.HasPrefix(n, "ISS (ZARYA)") || strings.HasPrefix(n, "CSS (TIANHE)") {
				keep = append(keep, s)
			}
		}
		add(keep)
	}

	if text, err := get(ctx, hc, ua, "visual"); err != nil {
		errs = append(errs, err)
	} else {
		add(ParseTLE(text, GroupVisual))
	}

	if text, err := get(ctx, hc, ua, "last-30-days"); err != nil {
		errs = append(errs, err)
	} else {
		var st []Sat
		for _, s := range ParseTLE(text, GroupStarlink) {
			if strings.HasPrefix(strings.ToUpper(s.Name), "STARLINK") {
				// All of these went up in the last 30 days: still low,
				// close together and bright.
				s.StdMag = magNewStarlink
				st = append(st, s)
			}
		}
		// The newest first (higher catalogue numbers are later launches).
		sort.SliceStable(st, func(i, j int) bool { return st[i].NoradID > st[j].NoradID })
		if len(st) > maxStarlinks {
			st = st[:maxStarlinks]
		}
		add(st)
	}

	if len(errs) == 3 {
		return nil, errors.Join(errs...)
	}
	return cat, errors.Join(errs...)
}

// get is one CelesTrak list as TLE text.
func get(ctx context.Context, hc *http.Client, ua, group string) (string, error) {
	q := url.Values{"GROUP": {group}, "FORMAT": {"tle"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, celestrakURL+"?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	if ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	resp, err := hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("celestrak %s: %w", group, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return "", fmt.Errorf("celestrak %s: %w", group, err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("celestrak %s: %s: %s", group, resp.Status, firstLine(string(body)))
	}
	text := string(body)
	// CelesTrak answers some refusals with a plain sentence and 200.
	if !strings.Contains(text, "\n1 ") && !strings.HasPrefix(text, "1 ") {
		return "", fmt.Errorf("celestrak %s: no elements in the reply: %s", group, firstLine(text))
	}
	return text, nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}
