package astro

import (
	"math"
	"sort"
	"time"
)

// The Moon's face: well-known seas, craters and mountains, where the line
// between day and night (the terminator) runs, and which features stand out
// best tonight. Positions on the Moon are selenographic: latitude north
// positive, longitude EAST positive (east is the side of Mare Crisium, the
// IAU convention), 0,0 the middle of the face we see.

// MoonFeature is a named place on the Moon.
type MoonFeature struct {
	Name   string  `json:"name"`
	Kind   string  `json:"kind"` // sea | crater | mountains | landing
	Lat    float64 `json:"lat"`  // degrees, north positive
	Lon    float64 `json:"lon"`  // degrees, east positive
	SizeKm float64 `json:"size_km"`
	About  string  `json:"about"`
}

// moonRadiusKm is the Moon's mean radius.
const moonRadiusKm = 1737.4

// MoonFeatures are about fifty well-known places on the near side, with
// IAU positions (rounded to a tenth of a degree) and sizes.
var MoonFeatures = []MoonFeature{
	// Seas (maria): dark plains of old lava.
	{"Mare Imbrium", "sea", 32.8, -15.6, 1146, "The Sea of Showers, a huge lava plain filling a basin blasted out by an enormous impact."},
	{"Mare Serenitatis", "sea", 28.0, 17.5, 674, "The Sea of Serenity, a round dark plain that forms one 'eye' of the Man in the Moon."},
	{"Mare Tranquillitatis", "sea", 8.5, 31.4, 873, "The Sea of Tranquility, where Apollo 11 landed in 1969."},
	{"Mare Crisium", "sea", 17.0, 59.1, 556, "The Sea of Crises, a neat oval near the edge that is the first big dark patch after New Moon."},
	{"Mare Fecunditatis", "sea", -7.8, 51.3, 909, "The Sea of Fertility, a rambling plain where the Soviet Luna 16 robot scooped up soil in 1970."},
	{"Mare Nectaris", "sea", -15.2, 35.5, 333, "The Sea of Nectar, a small round sea with the three big craters Theophilus, Cyrillus and Catharina on its shore."},
	{"Mare Nubium", "sea", -21.3, -16.6, 715, "The Sea of Clouds, a wide plain south of the middle of the Moon, home of the Straight Wall."},
	{"Mare Humorum", "sea", -24.4, -38.6, 389, "The Sea of Moisture, a round sea with the large crater Gassendi on its northern shore."},
	{"Oceanus Procellarum", "sea", 18.4, -57.4, 2568, "The Ocean of Storms, the biggest dark plain of all, covering much of the western side."},
	{"Mare Frigoris", "sea", 56.0, 1.4, 1596, "The Sea of Cold, a long thin dark band across the far north of the face."},
	{"Mare Vaporum", "sea", 13.3, 3.6, 245, "The Sea of Vapours, a small dark plain just north of the centre, edged by the Apennine mountains."},
	{"Mare Cognitum", "sea", -10.0, -23.1, 376, "The Sea that has become Known, named after Ranger 7 sent back the first close-up pictures here in 1964."},
	{"Mare Insularum", "sea", 7.5, -30.9, 513, "The Sea of Islands, the plain around the bright craters Copernicus and Kepler."},
	{"Sinus Iridum", "sea", 44.1, -31.5, 236, "The Bay of Rainbows, a half-circle bay on Mare Imbrium whose rim of mountains catches the sunrise like a jewelled handle."},
	// Craters.
	{"Tycho", "crater", -43.3, -11.4, 85, "A young crater whose bright rays splash across much of the Moon, best seen near Full Moon."},
	{"Copernicus", "crater", 9.6, -20.1, 96, "The 'Monarch of the Moon': terraced walls and central peaks, with bright rays around it."},
	{"Kepler", "crater", 8.1, -38.0, 31, "A small, sharp, bright crater with its own spray of rays in the Ocean of Storms."},
	{"Aristarchus", "crater", 23.7, -47.4, 40, "The brightest spot on the Moon, it shines even when lit only by sunlight reflected off the Earth."},
	{"Plato", "crater", 51.6, -9.4, 101, "A crater with a dark, smooth, lava-filled floor, like a little lake north of Mare Imbrium."},
	{"Clavius", "crater", -58.4, -14.4, 231, "One of the largest craters on the near side, with a curved chain of smaller craters on its floor."},
	{"Grimaldi", "crater", -5.2, -68.6, 173, "A big crater near the western edge with one of the darkest floors on the Moon."},
	{"Langrenus", "crater", -8.9, 61.0, 132, "A large crater with terraced walls and a central peak near the eastern edge."},
	{"Petavius", "crater", -25.3, 60.4, 184, "A big crater near the eastern edge with a cracked floor and a central mountain cluster."},
	{"Theophilus", "crater", -11.4, 26.4, 100, "A deep, sharp crater with a big central peak, one of the finest sights at the edge of the light."},
	{"Cyrillus", "crater", -13.2, 24.0, 98, "An older, battered neighbour of Theophilus, partly overlapped by it."},
	{"Catharina", "crater", -18.0, 23.6, 99, "The oldest and most worn of the trio with Theophilus and Cyrillus."},
	{"Archimedes", "crater", 29.7, -4.0, 81, "A crater with a flat, lava-flooded floor on the plain of Mare Imbrium."},
	{"Ptolemaeus", "crater", -9.3, -1.9, 154, "A huge, flat-floored walled plain near the centre of the face; first in a famous line of three with Alphonsus and Arzachel."},
	{"Alphonsus", "crater", -13.4, -2.8, 108, "A big crater with a central peak and small dark spots on its floor; Ranger 9 crashed here in 1965."},
	{"Arzachel", "crater", -18.2, -1.9, 96, "The youngest and sharpest of the trio Ptolemaeus, Alphonsus and Arzachel."},
	{"Eratosthenes", "crater", 14.5, -11.3, 58, "A deep crater at the end of the Apennine mountains, striking when the Sun is low on it."},
	{"Posidonius", "crater", 31.9, 29.9, 95, "A crater on the shore of Mare Serenitatis with cracks (rilles) on its floor."},
	{"Gassendi", "crater", -17.6, -40.1, 110, "A crater on the shore of Mare Humorum whose floor is criss-crossed by fine cracks."},
	{"Schickard", "crater", -44.4, -54.6, 227, "A huge crater near the south-western edge, seen squashed by the curve of the Moon, with patches of dark lava on its floor."},
	{"Maurolycus", "crater", -41.8, 14.0, 114, "A big crater in the rugged, crater-packed southern highlands."},
	{"Proclus", "crater", 16.1, 46.8, 28, "A small, very bright crater west of Mare Crisium whose rays fan out unevenly, missing one side."},
	{"Pythagoras", "crater", 63.5, -63.0, 145, "A deep crater near the north-western edge, seen very foreshortened."},
	{"Albategnius", "crater", -11.2, 4.0, 136, "A big, old walled plain near the centre with a central peak."},
	{"Hipparchus", "crater", -5.1, 5.2, 138, "A large, worn walled plain just south-east of the centre of the face."},
	{"Aristoteles", "crater", 50.2, 17.4, 88, "A big crater with terraced walls in the north, next to its smaller partner Eudoxus."},
	{"Bullialdus", "crater", -20.7, -22.2, 61, "A textbook crater on Mare Nubium: terraced walls and central peaks."},
	{"Atlas", "crater", 46.7, 44.4, 87, "A large crater in the north-east, paired with its neighbour Hercules."},
	// Mountains, cliffs, valleys.
	{"Montes Apenninus", "mountains", 18.9, -3.7, 600, "The Apennines, the Moon's grandest mountain range, the south-eastern rim of the Imbrium basin."},
	{"Montes Alpes", "mountains", 46.4, -0.8, 281, "The lunar Alps, cut through by the long straight Alpine Valley."},
	{"Montes Caucasus", "mountains", 38.4, 10.0, 445, "The Caucasus mountains, between Mare Imbrium and Mare Serenitatis."},
	{"Vallis Alpes", "mountains", 48.5, 3.2, 166, "The Alpine Valley, a straight trench slicing through the lunar Alps."},
	{"Rupes Recta", "mountains", -22.1, -7.8, 110, "The Straight Wall, a long cliff on Mare Nubium that shows as a dark line a day after First Quarter and a bright one before Last Quarter."},
	{"Mons Pico", "mountains", 45.7, -8.9, 25, "A lone mountain on Mare Imbrium south of Plato, casting a long spike of shadow at sunrise."},
	// Landing sites.
	{"Apollo 11", "landing", 0.7, 23.5, 0, "Where Neil Armstrong and Buzz Aldrin became the first people to walk on the Moon, on 20 July 1969."},
	{"Apollo 15", "landing", 26.1, 3.6, 0, "Where astronauts first drove the lunar rover, beside the Apennine mountains and Hadley Rille, in 1971."},
	{"Apollo 17", "landing", 20.2, 30.8, 0, "The last time people walked on the Moon, in the Taurus-Littrow valley, in December 1972."},
}

// moonAngles are Meeus' mean argument of latitude F and mean longitude of
// the ascending node Ω of the Moon (ch. 47), T Julian centuries from J2000.
func moonAngles(T float64) (F, node float64) {
	F = 93.2720950 + 483202.0175233*T - 0.0036539*T*T
	node = 125.0445479 - 1934.1362891*T + 0.0020754*T*T
	return F, node
}

// selenographic is the point of the Moon under a direction given by
// ecliptic longitude and latitude of date seen from the Moon's centre
// (Meeus 53, the optical libration formulae, physical libration left out:
// it is under 0.04°).
func selenographic(lon, lat, T float64) (sLat, sLon float64) {
	const I = 1.54242 // the Moon's equator to the ecliptic
	F, node := moonAngles(T)
	W := lon - node
	A := atan2d(sind(W)*cosd(lat)*cosd(I)-sind(lat)*sind(I), cosd(W)*cosd(lat))
	sLon = Wrap180(A - F)
	sLat = asind(-sind(W)*cosd(lat)*sind(I) - sind(lat)*cosd(I))
	return sLat, sLon
}

func julianCenturies(t time.Time) float64 { return (JD(t) - 2451545.0) / 36525 }

// Libration is the Moon's optical libration at a moment (Meeus 53): the
// point of the Moon's surface right in the middle of the face we see, in
// selenographic latitude and longitude. The Moon rocks by up to about 7°
// north–south and 8° east–west over the month, showing us a little round
// each edge in turn: lon > 0 brings the eastern edge (Mare Crisium) into
// view, lat > 0 the north.
func Libration(t time.Time) (lat, lon float64) {
	m := Body("moon", t)
	return selenographic(m.Lon, m.Lat, julianCenturies(t))
}

// subsolar is the point of the Moon with the Sun overhead (Meeus 53: the
// Sun's direction seen from the Moon, from the Earth-based positions).
func subsolar(t time.Time) (lat, lon float64) {
	m := Body("moon", t)
	s := Body("sun", t)
	ratio := m.Dist / s.Dist
	lonH := s.Lon + 180 + ratio*deg*cosd(m.Lat)*sind(s.Lon-m.Lon)
	latH := ratio * m.Lat
	return selenographic(lonH, latH, julianCenturies(t))
}

// SunColongitude is the Sun's selenographic colongitude (Meeus 53): the
// longitude of the morning terminator counted westwards, the number lunar
// observers use. About 270° at New Moon, 0° at First Quarter, 90° at Full,
// 180° at Last Quarter.
func SunColongitude(t time.Time) float64 {
	_, l0 := subsolar(t)
	return Rev(90 - l0)
}

// Terminator is where the line between day and night on the Moon runs,
// as a selenographic longitude at the Moon's equator (east positive).
// While the Moon waxes (New to Full) this is the sunrise (morning)
// terminator, moving from the eastern edge across to the western one, and
// morning is true; while it wanes it is the sunset (evening) terminator.
// About 0° at First and Last Quarter, ±90° at Full.
func Terminator(t time.Time) (lon float64, morning bool) {
	_, l0 := subsolar(t)
	if moonElong(t) < 180 {
		return Wrap180(l0 - 90), true
	}
	return Wrap180(l0 + 90), false
}

// sunAltOnMoon is how high the Sun stands over a place on the Moon
// (degrees; negative: night).
func sunAltOnMoon(lat, lon, sLat, sLon float64) float64 {
	return asind(sind(sLat)*sind(lat) + cosd(sLat)*cosd(lat)*cosd(lon-sLon))
}

// BestOnMoon are up to n features that stand out best at a moment: those
// where the Sun is low, from just risen to about 12° up (or about to set),
// so their walls and peaks cast long shadows and the relief shows, on the
// side of the Moon facing us. The best is where the Sun stands about 5°
// high; for a big sea or range, a part of it in that band counts.
// Landing sites are left out (there is nothing to see at them).
func BestOnMoon(t time.Time, n int) []MoonFeature {
	sLat, sLon := subsolar(t)
	eLat, eLon := Libration(t)
	type scored struct {
		f     MoonFeature
		score float64
	}
	var list []scored
	for _, f := range MoonFeatures {
		if f.Kind == "landing" {
			continue
		}
		// Facing us (with the libration of the moment).
		if sunAltOnMoon(f.Lat, f.Lon, eLat, eLon) < 5 {
			continue
		}
		h := sunAltOnMoon(f.Lat, f.Lon, sLat, sLon)
		// Half its width in degrees, capped so the far edge of a huge sea
		// round the limb doesn't count.
		half := math.Min(15, f.SizeKm/2/moonRadiusKm*deg)
		if h+half <= 0 || h-half >= 12 {
			continue
		}
		list = append(list, scored{f, math.Max(0, math.Abs(h-5)-half)})
	}
	rank := map[string]int{"crater": 0, "mountains": 0, "sea": 1}
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i], list[j]
		// Within half a degree of each other counts as equally good.
		if ka, kb := math.Round(a.score*2), math.Round(b.score*2); ka != kb {
			return ka < kb
		}
		if rank[a.f.Kind] != rank[b.f.Kind] {
			return rank[a.f.Kind] < rank[b.f.Kind]
		}
		return a.f.SizeKm > b.f.SizeKm
	})
	if n > len(list) || n < 0 {
		n = len(list)
	}
	out := make([]MoonFeature, n)
	for i := range out {
		out[i] = list[i].f
	}
	return out
}
