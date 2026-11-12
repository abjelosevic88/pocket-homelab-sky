package sat

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// demoEpoch is when Demo's elements are taken.
var demoEpoch = time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)

// demoSat is one satellite of the demo set, as orbital elements.
type demoSat struct {
	name   string
	id     int
	intl   string
	group  string
	mag    float64
	ndot   float64 // revolutions a day²/2, as in the TLE
	bstar  float64
	inc    float64
	node   float64
	ecc    float64
	argp   float64
	ma     float64
	mm     float64 // revolutions a day
	revNum int
}

// The demo set: the two stations, five bright satellites and a train of
// eight Starlinks a few days after their launch, close behind each other.
var demoSats = func() []demoSat {
	ds := []demoSat{
		{"ISS (ZARYA)", 25544, "98067A", GroupStation, magISS, 0.00012345, 0.00021852, 51.6345, 302.1520, 0.0004713, 47.8152, 312.3274, 15.50083712, 54712},
		{"CSS (TIANHE)", 48274, "21035A", GroupStation, magCSS, 0.00022541, 0.00026391, 41.4651, 118.6047, 0.0005512, 312.0781, 47.9706, 15.60519382, 31247},
		{"HST", 20580, "90037B", GroupVisual, 2.2, 0.00004815, 0.00022463, 28.4698, 141.2964, 0.0002405, 98.4119, 261.6716, 15.29318705, 76802},
		{"SL-16 R/B", 22220, "92076B", GroupVisual, 2.5, 0.00000021, 0.000012046, 71.0062, 202.5461, 0.0007563, 276.3511, 83.6747, 14.14512483, 75561},
		{"ENVISAT", 27386, "02009A", GroupVisual, 3.7, 0.00000187, 0.000075014, 98.2153, 333.7462, 0.0001148, 91.7032, 268.4361, 14.38261974, 30219},
		{"TERRA", 25994, "99068A", GroupVisual, 3.9, 0.00000096, 0.000027533, 98.0582, 21.6184, 0.0002841, 84.1347, 33.4572, 14.59731104, 43671},
		{"SL-8 R/B", 11267, "79014B", GroupVisual, 4.0, 0.00000087, 0.000081114, 74.0436, 166.2318, 0.0019874, 104.7621, 255.5734, 14.35602215, 47715},
	}
	// The train: the same orbit, each a little behind the one before.
	for i := range 8 {
		ds = append(ds, demoSat{
			name: fmt.Sprintf("STARLINK-%d", 34811+i), id: 66501 + i, intl: fmt.Sprintf("26219%c", 'A'+i),
			group: GroupStarlink, mag: magNewStarlink,
			ndot: 0.00098211, bstar: 0.0017426, inc: 43.0027, node: 75.4129, ecc: 0.0001472,
			argp: 268.2104, ma: 91.8726 - 1.4*float64(i), mm: 15.83905127 + 0.00000731*float64(i), revNum: 52,
		})
	}
	return ds
}()

// Demo is a fixed set of satellites for tests and the demo mode, needing no
// network: ISS, Tiangong, five bright satellites and a Starlink train of
// eight. The elements are taken on 10 November 2026; for passes far from
// that date use DemoAt.
func Demo() *Catalog { return DemoAt(demoEpoch) }

// DemoAt is the demo set with its elements taken at a given moment, so that
// the demo mode shows believable passes on any date.
func DemoAt(epoch time.Time) *Catalog {
	epoch = epoch.UTC().Truncate(time.Second)
	cat := &Catalog{Fetched: epoch}
	for _, d := range demoSats {
		l1, l2 := formatTLE(d, epoch)
		cat.Sats = append(cat.Sats, Sat{Name: d.name, NoradID: d.id, Line1: l1, Line2: l2, StdMag: d.mag, Group: d.group})
	}
	return cat
}

// formatTLE writes the two lines of an element set, with their checksums.
func formatTLE(d demoSat, epoch time.Time) (string, string) {
	start := time.Date(epoch.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
	day := 1 + epoch.Sub(start).Hours()/24
	sign := " "
	if d.ndot < 0 {
		sign = "-"
	}
	ndot := sign + strings.TrimPrefix(fmt.Sprintf("%.8f", math.Abs(d.ndot)), "0")
	l1 := fmt.Sprintf("1 %05dU %-8s %02d%012.8f %s %s %s 0 %4d",
		d.id, d.intl, epoch.Year()%100, day, ndot, expFormat(0), expFormat(d.bstar), 999)
	l2 := fmt.Sprintf("2 %05d %8.4f %8.4f %07d %8.4f %8.4f %11.8f%5d",
		d.id, d.inc, d.node, int(math.Round(d.ecc*1e7)), d.argp, d.ma, d.mm, d.revNum%100000)
	return l1 + checksum(l1), l2 + checksum(l2)
}

// expFormat writes a number the TLE way: " 12345-4" is 0.12345e-4.
func expFormat(x float64) string {
	if x == 0 {
		return " 00000-0"
	}
	sign := ' '
	if x < 0 {
		sign, x = '-', -x
	}
	e := int(math.Floor(math.Log10(x))) + 1
	m := int(math.Round(x / math.Pow(10, float64(e)) * 1e5))
	if m >= 100000 {
		m /= 10
		e++
	}
	return fmt.Sprintf("%c%05d%+d", sign, m, e)
}

// checksum is the last digit of a TLE line: the digits added up, a minus
// sign counting one.
func checksum(line string) string {
	sum := 0
	for _, c := range line {
		switch {
		case c >= '0' && c <= '9':
			sum += int(c - '0')
		case c == '-':
			sum++
		}
	}
	return fmt.Sprint(sum % 10)
}
