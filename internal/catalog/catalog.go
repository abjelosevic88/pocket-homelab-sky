// Package catalog holds what is fixed in the sky: 5,000 stars down to the
// faintest the eye can see, the 88 constellations with their lines and
// borders, the Messier list and a few other favourite sights, and the outline
// of the Milky Way. All positions are J2000.
//
// The data comes from d3-celestial (BSD 3-Clause; see scripts/catalog.py and
// NOTICE): stars from XHIP/Hipparcos, names from the IAU list and the
// HD/HIP cross index, Messier data from SEDS, the Milky Way by Jose R. Vieira.
package catalog

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

//go:embed data/*.json
var files embed.FS

// Star is one star.
type Star struct {
	RA    float64 `json:"ra"`
	Dec   float64 `json:"dec"`
	Mag   float64 `json:"mag"`
	BV    float64 `json:"bv"`
	HIP   int     `json:"hip"`
	Name  string  `json:"name"`
	Bayer string  `json:"bayer"` // Greek letter or Flamsteed number
	Con   string  `json:"con"`
}

// Label is the star's name, else its Bayer name ("α Orionis"), else its number.
func (s Star) Label() string {
	if s.Name != "" {
		return s.Name
	}
	if s.Bayer != "" && s.Con != "" {
		if c, ok := Constellation(s.Con); ok && c.Gen != "" {
			return s.Bayer + " " + c.Gen
		}
		return s.Bayer + " " + s.Con
	}
	return "HIP " + strconv.Itoa(s.HIP)
}

// Con is one constellation.
type Con struct {
	ID    string         `json:"id"`
	Name  string         `json:"name"`
	Gen   string         `json:"gen"`
	Rank  int            `json:"rank"` // 1 = well known … 3 = faint
	Label [2]float64     `json:"label"`
	Lines [][][2]float64 `json:"lines"`
	About string         `json:"about"`
	polys [][][2]float64 // borders
	box   [][4]float64   // each border's ra/dec box
}

// DSO is a deep-sky object: a cluster, nebula or galaxy.
type DSO struct {
	ID    string   `json:"id"` // M31, double-cluster
	NGC   string   `json:"ngc"`
	Name  string   `json:"name"`
	Type  string   `json:"type"`
	Kind  string   `json:"kind"` // plain words for Type
	Mag   *float64 `json:"mag"`
	Dim   string   `json:"dim"` // size in arc minutes
	RA    float64  `json:"ra"`
	Dec   float64  `json:"dec"`
	Con   string   `json:"con"`
	About string   `json:"about"`
}

// Label is "M31 Andromeda Galaxy" or the plain name.
func (d DSO) Label() string {
	if strings.HasPrefix(d.ID, "M") && d.Name != "" {
		return d.ID + " " + d.Name
	}
	if d.Name != "" {
		return d.Name
	}
	return d.ID
}

type data struct {
	stars    []Star
	cons     []Con
	conByID  map[string]int
	messier  []DSO
	high     []DSO
	starHIP  map[int]int
	client   []byte
	etag     string
	loadErr  error
	milkyWay json.RawMessage
	bounds   json.RawMessage
}

var (
	once sync.Once
	d    data
)

func load() *data {
	once.Do(func() { d.loadErr = d.load() })
	return &d
}

func (d *data) load() error {
	raw := func(name string) []byte {
		b, err := files.ReadFile("data/" + name)
		if err != nil {
			panic(err)
		}
		return b
	}
	var st [][]any
	if err := json.Unmarshal(raw("stars.json"), &st); err != nil {
		return err
	}
	d.starHIP = map[int]int{}
	for _, r := range st {
		s := Star{RA: num(r[0]), Dec: num(r[1]), Mag: num(r[2]), BV: num(r[3]), HIP: int(num(r[4])), Name: str(r[5]), Bayer: str(r[6]), Con: str(r[7])}
		d.starHIP[s.HIP] = len(d.stars)
		d.stars = append(d.stars, s)
	}
	if err := json.Unmarshal(raw("constellations.json"), &d.cons); err != nil {
		return err
	}
	d.conByID = map[string]int{}
	for i := range d.cons {
		c := &d.cons[i]
		c.About = conAbout[c.ID]
		if c.About == "" {
			c.About = conAbout[strings.TrimRight(c.ID, "12")]
		}
		d.conByID[c.ID] = i
	}
	var bounds []struct {
		ID    string         `json:"id"`
		Polys [][][2]float64 `json:"polys"`
	}
	d.bounds = raw("bounds.json")
	if err := json.Unmarshal(d.bounds, &bounds); err != nil {
		return err
	}
	for _, b := range bounds {
		i, ok := d.conByID[b.ID]
		if !ok {
			// Serpens' two halves are one constellation in the lines file.
			if i, ok = d.conByID[strings.TrimRight(b.ID, "12")]; !ok {
				continue
			}
		}
		for _, p := range b.Polys {
			ring, box := unwrapRing(p)
			d.cons[i].polys = append(d.cons[i].polys, ring)
			d.cons[i].box = append(d.cons[i].box, box)
		}
	}
	if err := json.Unmarshal(raw("messier.json"), &d.messier); err != nil {
		return err
	}
	for i := range d.messier {
		m := &d.messier[i]
		m.Kind, m.About = typeWords[m.Type], typeAbout[m.Type]
		if n, ok := betterNames[m.ID]; ok {
			m.Name = n
		}
		if a := messierAbout[m.ID]; a != "" {
			m.About = a
		}
		m.Name = strings.ReplaceAll(m.Name, "´", "'")
		m.Con = d.conAt(m.RA, m.Dec)
	}
	d.high = append([]DSO{}, highlightsData...)
	for i := range d.high {
		h := &d.high[i]
		h.Kind, h.About = typeWords[h.Type], typeAbout[h.Type]
		h.Con = d.conAt(h.RA, h.Dec)
	}
	d.milkyWay = raw("milkyway.json")
	// What the web app draws, in one file it caches.
	type clientCon struct {
		ID    string         `json:"id"`
		Name  string         `json:"name"`
		Gen   string         `json:"gen"`
		Rank  int            `json:"rank"`
		Label [2]float64     `json:"label"`
		Lines [][][2]float64 `json:"lines"`
		About string         `json:"about"`
	}
	cc := make([]clientCon, len(d.cons))
	for i, c := range d.cons {
		cc[i] = clientCon{c.ID, c.Name, c.Gen, c.Rank, c.Label, c.Lines, c.About}
	}
	b, err := json.Marshal(map[string]any{
		"stars": json.RawMessage(raw("stars.json")), "constellations": cc, "messier": d.messier, "highlights": d.high,
		"milkyway": d.milkyWay, "bounds": d.bounds,
	})
	if err != nil {
		return err
	}
	d.client = b
	sum := sha256.Sum256(b)
	d.etag = `"` + hex.EncodeToString(sum[:8]) + `"`
	return nil
}

func num(v any) float64 {
	f, _ := v.(float64)
	return f
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

// unwrapRing makes a border's right ascensions continuous (no jump at 0h)
// and closes rings around a pole over the pole itself.
func unwrapRing(p [][2]float64) ([][2]float64, [4]float64) {
	out := make([][2]float64, 0, len(p)+4)
	prev := 0.0
	for i, pt := range p {
		ra := pt[0]
		if i > 0 {
			for ra-prev > 180 {
				ra -= 360
			}
			for ra-prev < -180 {
				ra += 360
			}
		}
		out = append(out, [2]float64{ra, pt[1]})
		prev = ra
	}
	if len(out) > 2 {
		span := out[len(out)-1][0] - out[0][0]
		first := out[0]
		closing := out[len(out)-1]
		if math.Abs(span) > 180 {
			// A ring around a pole: go over the pole to close it.
			pole := 90.0
			if first[1] < 0 {
				pole = -90
			}
			out = append(out, [2]float64{closing[0], pole}, [2]float64{first[0], pole})
		}
	}
	box := [4]float64{math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)}
	for _, pt := range out {
		box[0] = math.Min(box[0], pt[0])
		box[1] = math.Max(box[1], pt[0])
		box[2] = math.Min(box[2], pt[1])
		box[3] = math.Max(box[3], pt[1])
	}
	return out, box
}

func inRing(ring [][2]float64, ra, dec float64) bool {
	in := false
	for i, j := 0, len(ring)-1; i < len(ring); j, i = i, i+1 {
		xi, yi := ring[i][0], ring[i][1]
		xj, yj := ring[j][0], ring[j][1]
		if (yi > dec) != (yj > dec) && ra < (xj-xi)*(dec-yi)/(yj-yi)+xi {
			in = !in
		}
	}
	return in
}

func (d *data) conAt(ra, dec float64) string {
	for _, c := range d.cons {
		for k, ring := range c.polys {
			b := c.box[k]
			if dec < b[2] || dec > b[3] {
				continue
			}
			for _, r := range []float64{ra, ra - 360, ra + 360, ra - 720} {
				if r >= b[0] && r <= b[1] && inRing(ring, r, dec) {
					return c.ID
				}
			}
		}
	}
	return ""
}

// ConstellationAt is the constellation a J2000 position lies in ("" when unknown).
func ConstellationAt(ra, dec float64) string { return load().conAt(ra, dec) }

// Stars are all the stars, brightest first.
func Stars() []Star { return load().stars }

// StarByHIP is a star by its Hipparcos number.
func StarByHIP(hip int) (Star, bool) {
	dd := load()
	i, ok := dd.starHIP[hip]
	if !ok {
		return Star{}, false
	}
	return dd.stars[i], true
}

// Constellations are the 88 constellations, by name.
func Constellations() []Con { return load().cons }

// Constellation is one constellation by its three-letter id.
func Constellation(id string) (Con, bool) {
	dd := load()
	i, ok := dd.conByID[id]
	if !ok {
		return Con{}, false
	}
	return dd.cons[i], true
}

// ConName is a constellation's name ("" when unknown).
func ConName(id string) string {
	if c, ok := Constellation(id); ok {
		return c.Name
	}
	return ""
}

// Messier is the Messier list, M1 to M110.
func Messier() []DSO { return load().messier }

// Highlights are favourite sights outside the Messier list.
func Highlights() []DSO { return load().high }

// ClientJSON is what the web app draws the map from, and its ETag.
func ClientJSON() ([]byte, string, error) {
	dd := load()
	return dd.client, dd.etag, dd.loadErr
}

// ---- objects by key, and search ----

// Object is anything in the catalogue, by a stable key: star:32349,
// dso:M31, dso:double-cluster, con:Ori. (The Sun, the Moon and the planets
// are keyed by name — sun, moon, jupiter — and live in package astro.)
type Object struct {
	Key   string   `json:"key"`
	Name  string   `json:"name"`
	Kind  string   `json:"kind"` // star | dso | constellation | planet | moon | sun
	Type  string   `json:"type"` // plain words: "Spiral galaxy", "Star", "Constellation"
	RA    float64  `json:"ra"`   // J2000 (planets: of date, filled by the caller)
	Dec   float64  `json:"dec"`
	Mag   *float64 `json:"mag"`
	Con   string   `json:"con"`
	About string   `json:"about"`
	Alt   string   `json:"alt_name,omitempty"`
}

// StarObject turns a star into an Object.
func StarObject(s Star) Object {
	m := s.Mag
	o := Object{Key: "star:" + strconv.Itoa(s.HIP), Name: s.Label(), Kind: "star", Type: starType(s), RA: s.RA, Dec: s.Dec, Mag: &m, Con: s.Con}
	if s.Name != "" && s.Bayer != "" {
		if c, ok := Constellation(s.Con); ok {
			o.Alt = s.Bayer + " " + c.Gen
		}
	}
	o.About = fmt.Sprintf("A %s star in %s.", colourWord(s.BV), firstNonEmpty(ConName(s.Con), "the sky"))
	return o
}

func starType(s Star) string {
	switch {
	case s.Mag < 1.5:
		return "One of the brightest stars"
	case s.Mag < 3:
		return "Bright star"
	case s.Mag < 4.5:
		return "Star"
	}
	return "Faint star"
}

func colourWord(bv float64) string {
	switch {
	case bv < 0:
		return "blue-white"
	case bv < 0.3:
		return "white"
	case bv < 0.6:
		return "yellow-white"
	case bv < 1.0:
		return "yellow"
	case bv < 1.4:
		return "orange"
	}
	return "red"
}

// DSOObject turns a deep-sky object into an Object.
func DSOObject(x DSO) Object {
	o := Object{Key: "dso:" + x.ID, Name: x.Label(), Kind: "dso", Type: firstNonEmpty(x.Kind, "Deep-sky object"), RA: x.RA, Dec: x.Dec, Mag: x.Mag, Con: x.Con, About: x.About}
	if x.NGC != "" {
		o.Alt = x.NGC
	}
	return o
}

// ConObject turns a constellation into an Object (at its label).
func ConObject(c Con) Object {
	return Object{Key: "con:" + c.ID, Name: c.Name, Kind: "constellation", Type: "Constellation", RA: c.Label[0], Dec: c.Label[1], Con: c.ID, About: c.About}
}

// Lookup finds a catalogue object by key.
func Lookup(k string) (Object, bool) {
	kind, id, ok := strings.Cut(k, ":")
	if !ok {
		return Object{}, false
	}
	switch kind {
	case "star":
		n, err := strconv.Atoi(id)
		if err != nil {
			return Object{}, false
		}
		if s, ok := StarByHIP(n); ok {
			return StarObject(s), true
		}
	case "dso":
		for _, m := range Messier() {
			if strings.EqualFold(m.ID, id) {
				return DSOObject(m), true
			}
		}
		for _, h := range Highlights() {
			if h.ID == id {
				return DSOObject(h), true
			}
		}
	case "con":
		if c, ok := Constellation(id); ok {
			return ConObject(c), true
		}
	}
	return Object{}, false
}

// Fold lowercases and strips accents ("Boötes" → "bootes").
func Fold(s string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	out, _, err := transform.String(t, strings.ToLower(strings.TrimSpace(s)))
	if err != nil {
		return strings.ToLower(s)
	}
	return out
}

// Search finds catalogue objects by name, Messier or NGC number, or a
// constellation's name or id; best matches first.
func Search(q string, limit int) []Object {
	q = Fold(q)
	if q == "" {
		return nil
	}
	type hit struct {
		o     Object
		score int
	}
	var hits []hit
	match := func(name string) int {
		n := Fold(name)
		switch {
		case n == "":
			return 0
		case n == q:
			return 100
		case strings.HasPrefix(n, q):
			return 70
		case strings.Contains(n, " "+q):
			return 50
		case strings.Contains(n, q):
			return 30
		}
		return 0
	}
	// "m 31", "m31"
	mq := strings.ReplaceAll(q, " ", "")
	for _, m := range Messier() {
		s := 0
		if strings.EqualFold(mq, m.ID) {
			s = 110
		}
		if v := match(m.Name); v > s {
			s = v
		}
		if v := match(strings.ReplaceAll(m.NGC, " ", "")); v > 0 && strings.HasPrefix(mq, "ngc") && v > s {
			s = v
		}
		if s > 0 {
			hits = append(hits, hit{DSOObject(m), s + 5})
		}
	}
	for _, h := range Highlights() {
		if s := match(h.Name); s > 0 {
			hits = append(hits, hit{DSOObject(h), s + 5})
		}
	}
	for _, c := range Constellations() {
		s := match(c.Name)
		if Fold(c.ID) == q {
			s = 100
		}
		if s > 0 {
			hits = append(hits, hit{ConObject(c), s + 8 - c.Rank})
		}
	}
	for _, st := range Stars() {
		if st.Name == "" {
			continue
		}
		if s := match(st.Name); s > 0 {
			hits = append(hits, hit{StarObject(st), s + int(math.Max(0, 6-st.Mag))})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	if limit <= 0 || limit > len(hits) {
		limit = len(hits)
	}
	out := make([]Object, 0, limit)
	for _, h := range hits[:limit] {
		out = append(out, h.o)
	}
	return out
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}
