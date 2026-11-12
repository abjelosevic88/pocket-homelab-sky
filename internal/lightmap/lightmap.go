// Package lightmap answers "how bright is the sky here from artificial light,
// and where is it darker nearby?" from David Lorenz's World Atlas of the
// Artificial Night Sky Brightness (2025 VIIRS data, after P. Cinzano;
// https://djlorenz.github.io/astronomy/lp/). The medium world map (1/40°,
// about 2.8 km; 75° N to 65° S) is downloaded once and kept as one byte per
// cell on disk (80 MB), read with ReadAt, so it costs no memory.
//
// The atlas gives the brightness straight up in 15 zones (0, 1a … 7b, 8);
// its author asks not to call them Bortle classes, so the app says "about"
// when it suggests a Bortle number from a zone.
package lightmap

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// URL of the medium-resolution world map.
var URL = "https://djlorenz.github.io/astronomy/lp2025/world2025_low3.png"

// The atlas covers 75° N to 65° S; the file starts with its width and
// height (two little-endian uint32s), then one zone byte per cell.
const top, bottom = 75.0, -65.0

// Zones are the atlas's zones by rank (0 darkest … 15 brightest).
var Zones = []string{"0", "1a", "1b", "2a", "2b", "3a", "3b", "4a", "4b", "5a", "5b", "6a", "6b", "7a", "7b", "8"}

// zoneOfColor maps the atlas colours to their rank.
var zoneOfColor = map[[3]uint8]int{
	{0, 0, 0}: 0, {34, 34, 34}: 1, {66, 66, 66}: 2, {20, 47, 114}: 3, {33, 84, 216}: 4, {15, 87, 20}: 5, {31, 161, 42}: 6,
	{110, 100, 30}: 7, {184, 166, 37}: 8, {191, 100, 30}: 9, {253, 150, 80}: 10, {251, 90, 73}: 11, {251, 153, 138}: 12,
	{160, 160, 160}: 13, {242, 242, 242}: 14, {255, 255, 255}: 15,
}

// Bortle is about the Bortle class of a zone rank (the atlas measures only
// straight up, so this is a starting point, not a measurement).
func Bortle(rank int) int {
	return []int{1, 1, 2, 3, 3, 4, 4, 5, 5, 6, 6, 7, 7, 8, 8, 9}[max(0, min(15, rank))]
}

// Words says what a zone means for the sky, plainly.
func Words(rank int) string {
	switch {
	case rank <= 2:
		return "a truly dark sky: the Milky Way in full detail"
	case rank <= 4:
		return "a dark country sky: the Milky Way clear"
	case rank <= 6:
		return "a rural sky with some glow on the horizon"
	case rank <= 8:
		return "a village or suburb sky: the Milky Way faint"
	case rank <= 10:
		return "a bright suburb: the Milky Way gone overhead"
	case rank <= 12:
		return "a town sky: only the brighter stars"
	}
	return "a city sky: few stars"
}

// Map is the atlas on disk.
type Map struct {
	path   string
	mu     sync.Mutex
	f      *os.File
	w, h   int
	perDeg float64
}

// Open opens a map built earlier (ErrMissing when there is none yet).
func Open(path string) (*Map, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, ErrMissing
	}
	var hdr [8]byte
	if _, err := io.ReadFull(f, hdr[:]); err != nil {
		f.Close()
		return nil, err
	}
	w, h := int(binary.LittleEndian.Uint32(hdr[:4])), int(binary.LittleEndian.Uint32(hdr[4:]))
	fi, _ := f.Stat()
	if w <= 0 || h <= 0 || fi.Size() != int64(8+w*h) {
		f.Close()
		return nil, fmt.Errorf("lightmap: %s is damaged", path)
	}
	return &Map{path: path, f: f, w: w, h: h, perDeg: float64(w) / 360}, nil
}

// ErrMissing means the map has not been downloaded yet.
var ErrMissing = errors.New("the light-pollution map is not downloaded yet")

// Build downloads the atlas image and writes the zone file (atomic rename).
func Build(ctx context.Context, hc *http.Client, ua, dst string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", ua)
	res, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("light-pollution atlas: HTTP %d", res.StatusCode)
	}
	return BuildFrom(io.LimitReader(res.Body, 64<<20), dst)
}

// BuildFrom turns an atlas PNG into the zone file.
func BuildFrom(r io.Reader, dst string) error {
	img, err := png.Decode(r)
	if err != nil {
		return err
	}
	b := img.Bounds()
	width, height := b.Dx(), b.Dy()
	if width%360 != 0 || height*360 != width*int(top-bottom) {
		return fmt.Errorf("light-pollution atlas: unexpected size %dx%d", width, height)
	}
	pal, ok := img.(*image.Paletted)
	if !ok {
		return errors.New("light-pollution atlas: not a palette image")
	}
	ranks := make([]byte, len(pal.Palette))
	for i, c := range pal.Palette {
		r, g, bl, _ := c.RGBA()
		rank, ok := zoneOfColor[[3]uint8{uint8(r >> 8), uint8(g >> 8), uint8(bl >> 8)}]
		if !ok {
			rank = 0
		}
		ranks[i] = byte(rank)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp := dst + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	var hdr [8]byte
	binary.LittleEndian.PutUint32(hdr[:4], uint32(width))
	binary.LittleEndian.PutUint32(hdr[4:], uint32(height))
	if _, err := f.Write(hdr[:]); err != nil {
		f.Close()
		return err
	}
	row := make([]byte, width)
	for y := 0; y < height; y++ {
		off := y * pal.Stride
		for x := 0; x < width; x++ {
			row[x] = ranks[pal.Pix[off+x]]
		}
		if _, err := f.Write(row); err != nil {
			f.Close()
			return err
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

func (m *Map) cell(lat, lon float64) (int, int, bool) {
	if lat > top || lat < bottom {
		return 0, 0, false
	}
	lon = math.Mod(lon+180, 360)
	if lon < 0 {
		lon += 360
	}
	x := int(lon * m.perDeg)
	y := int((top - lat) * m.perDeg)
	if x >= m.w {
		x = m.w - 1
	}
	if y >= m.h {
		y = m.h - 1
	}
	return x, y, true
}

// Zone is the zone rank at a spot (false outside the map: the far north and south).
func (m *Map) Zone(lat, lon float64) (int, bool) {
	x, y, ok := m.cell(lat, lon)
	if !ok {
		return 0, false
	}
	var b [1]byte
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := m.f.ReadAt(b[:], int64(8+y*m.w+x)); err != nil {
		return 0, false
	}
	return int(b[0]), true
}

// Spot is a darker place found near a spot.
type Spot struct {
	Lat    float64 `json:"lat"`
	Lon    float64 `json:"lon"`
	Rank   int     `json:"rank"`
	Zone   string  `json:"zone"`
	Bortle int     `json:"bortle"`
	Km     float64 `json:"km"`
	Dir    float64 `json:"dir"` // bearing, degrees from north
}

// Darkest finds up to n of the darkest places within km of a spot, nearest
// first among the darkest, spread apart (at least a fifth of km between them).
func (m *Map) Darkest(lat, lon, km float64, n int) []Spot {
	dLat := km / 111.2
	dLon := km / (111.2 * math.Max(0.1, math.Cos(lat*math.Pi/180)))
	var cands []Spot
	m.mu.Lock()
	for la := lat - dLat; la <= lat+dLat; la += 1.0 / m.perDeg {
		x0, y, ok := m.cell(la, lon-dLon)
		if !ok {
			continue
		}
		cols := int(2*dLon*m.perDeg) + 1
		buf := make([]byte, cols)
		if x0+cols <= m.w {
			if _, err := m.f.ReadAt(buf, int64(8+y*m.w+x0)); err != nil {
				continue
			}
		} else {
			for i := range buf {
				_, _ = m.f.ReadAt(buf[i:i+1], int64(8+y*m.w+(x0+i)%m.w))
			}
		}
		for i, r := range buf {
			lo := lon - dLon + float64(i)/m.perDeg
			d := distKm(lat, lon, la, lo)
			if d <= km {
				cands = append(cands, Spot{Lat: la, Lon: lo, Rank: int(r), Km: d})
			}
		}
	}
	m.mu.Unlock()
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].Rank != cands[j].Rank {
			return cands[i].Rank < cands[j].Rank
		}
		return cands[i].Km < cands[j].Km
	})
	var out []Spot
	for _, c := range cands {
		far := true
		for _, o := range out {
			if distKm(c.Lat, c.Lon, o.Lat, o.Lon) < km/5 {
				far = false
				break
			}
		}
		if !far {
			continue
		}
		c.Zone, c.Bortle, c.Dir = Zones[c.Rank], Bortle(c.Rank), bearing(lat, lon, c.Lat, c.Lon)
		c.Km = math.Round(c.Km*10) / 10
		c.Lat, c.Lon = math.Round(c.Lat*1e4)/1e4, math.Round(c.Lon*1e4)/1e4
		out = append(out, c)
		if len(out) >= n {
			break
		}
	}
	return out
}

func distKm(lat1, lon1, lat2, lon2 float64) float64 {
	r := math.Pi / 180
	dl := (lat2 - lat1) * r
	dn := (lon2 - lon1) * r
	a := math.Sin(dl/2)*math.Sin(dl/2) + math.Cos(lat1*r)*math.Cos(lat2*r)*math.Sin(dn/2)*math.Sin(dn/2)
	return 6371 * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

func bearing(lat1, lon1, lat2, lon2 float64) float64 {
	r := math.Pi / 180
	y := math.Sin((lon2-lon1)*r) * math.Cos(lat2*r)
	x := math.Cos(lat1*r)*math.Sin(lat2*r) - math.Sin(lat1*r)*math.Cos(lat2*r)*math.Cos((lon2-lon1)*r)
	b := math.Atan2(y, x) / r
	if b < 0 {
		b += 360
	}
	return math.Round(b)
}
