package astro

import (
	"math"
	"time"
)

// Double stars to split and variable stars to time.

// DoubleStar is a star that a telescope (or sharp eyes) shows as two.
// RA and Dec are J2000 degrees of the main star (precess with OfDate);
// SepArcsec is how far apart the two are, in seconds of arc (the Moon is
// 1800" across; steady air lets a small telescope split about 2"); Kit is
// what is needed to split them, Colors what they look like.
type DoubleStar struct {
	Name      string  `json:"name"`
	Con       string  `json:"con"` // constellation
	RA        float64 `json:"ra"`
	Dec       float64 `json:"dec"`
	Mag1      float64 `json:"mag1"`
	Mag2      float64 `json:"mag2"`
	SepArcsec float64 `json:"sep_arcsec"`
	Colors    string  `json:"colors"`
	Kit       string  `json:"kit"` // eyes | binoculars | small telescope | telescope
	About     string  `json:"about"`
}

// DoubleStars are famous double stars, a tour from the easy to the
// challenging. Pairs that orbit each other quickly (Castor, Alpha Centauri,
// Porrima) change their separation over the years; the figure is for the
// mid-2020s.
var DoubleStars = []DoubleStar{
	{"Albireo", "Cygnus", 292.680, 27.960, 3.1, 5.1, 34.4, "gold and blue", "small telescope",
		"The most colourful double in the sky: a golden star with a sapphire-blue companion, at the foot of the Northern Cross."},
	{"Mizar and Alcor", "Ursa Major", 200.981, 54.925, 2.2, 4.0, 708, "white and white", "eyes",
		"The middle star of the Big Dipper's handle has a faint partner, Alcor, that keen eyes can see; a small telescope then splits Mizar itself into two (14\")."},
	{"Epsilon Lyrae", "Lyra", 281.084, 39.670, 4.7, 4.7, 208, "white and white", "binoculars",
		"The Double Double: binoculars show two stars next to Vega, and a telescope at high power splits each of them again (about 2.3\")."},
	{"Castor", "Gemini", 113.650, 31.888, 1.9, 3.0, 5.5, "white and white", "small telescope",
		"The fainter of the Twins' heads is two bright white stars close together; in fact it is a family of six stars."},
	{"Almach", "Andromeda", 30.975, 42.330, 2.1, 5.0, 9.6, "orange and blue", "small telescope",
		"A bright orange star with a small blue-green companion, often called a winter Albireo."},
	{"Polaris", "Ursa Minor", 37.955, 89.264, 2.0, 8.7, 18.2, "yellow-white and white", "small telescope",
		"The Pole Star has a faint little companion that a small telescope picks out beside its glare."},
	{"Rigel", "Orion", 78.634, -8.202, 0.1, 6.8, 9.5, "blue-white and white", "small telescope",
		"Orion's brilliant blue-white foot hides a faint companion close in its glare; steady air helps."},
	{"Gamma Delphini", "Delphinus", 311.665, 16.124, 4.3, 5.1, 9.0, "yellow and green-white", "small telescope",
		"A pretty, nearly equal pair of yellowish and greenish stars at the nose of the little Dolphin."},
	{"Cor Caroli", "Canes Venatici", 194.007, 38.318, 2.9, 5.6, 19.3, "white and pale yellow", "small telescope",
		"An easy pair below the Big Dipper's handle; the brighter star has strong magnetic fields and odd chemistry."},
	{"Eta Cassiopeiae", "Cassiopeia", 12.276, 57.815, 3.5, 7.4, 13.4, "yellow and red-purple", "small telescope",
		"A Sun-like yellow star with a reddish companion, both just 19 light-years away."},
	{"Gamma Arietis", "Aries", 28.383, 19.294, 4.5, 4.6, 7.5, "white and white", "small telescope",
		"Mesarthim, two matching white stars side by side like a pair of headlights."},
	{"Alnitak", "Orion", 85.190, -1.943, 1.9, 3.7, 2.2, "blue-white and blue-white", "telescope",
		"The left star of Orion's Belt is a tight pair of hot blue giants; it needs steady air and high power."},
	{"Trapezium", "Orion", 83.819, -5.387, 5.1, 6.7, 13, "white", "small telescope",
		"Four young stars in a little trapezium at the heart of the Orion Nebula, lighting up the cloud they were born from."},
	{"Alpha Centauri", "Centaurus", 219.902, -60.834, 0.0, 1.3, 8, "yellow and orange", "small telescope",
		"The nearest star system to the Sun, 4.4 light-years away: two Sun-like stars circling each other every 80 years (southern sky)."},
	{"Acrux", "Crux", 186.650, -63.099, 1.3, 1.6, 4.0, "blue-white and blue-white", "small telescope",
		"The foot of the Southern Cross is two bright blue-white stars, easy in a small telescope (southern sky)."},
	{"Antares", "Scorpius", 247.352, -26.432, 1.0, 5.4, 2.6, "red and green", "telescope",
		"The red supergiant heart of the Scorpion has a small companion that looks green beside it; hard, it needs steady air."},
	{"Izar", "Boötes", 221.247, 27.074, 2.5, 4.8, 2.9, "orange and blue-green", "telescope",
		"Pulcherrima, 'the loveliest': an orange star with a close blue-green companion; needs high power."},
	{"Algieba", "Leo", 154.993, 19.842, 2.4, 3.6, 4.7, "golden and yellow", "small telescope",
		"Two golden-yellow giant stars in the Lion's mane, a fine sight at medium power."},
	{"Nu Draconis", "Draco", 263.044, 55.184, 4.9, 4.9, 62, "white and white", "binoculars",
		"Two identical white stars in the Dragon's head, wide enough for steady binoculars."},
	{"61 Cygni", "Cygnus", 316.725, 38.750, 5.2, 6.0, 31, "orange and orange", "small telescope",
		"Two orange dwarf stars; the first star whose distance was measured, by Bessel in 1838 (11.4 light-years)."},
	{"Beta Monocerotis", "Monoceros", 97.204, -7.033, 4.6, 5.0, 7.1, "white", "small telescope",
		"A small telescope shows two stars and a little more power shows the second is itself double: a fine triple."},
	{"Porrima", "Virgo", 190.415, -1.449, 3.5, 3.5, 3.1, "yellow-white and yellow-white", "telescope",
		"Two equal stars circling each other every 169 years; they were too close to split in 2005 and are opening up again."},
}

// periodicTimes are the moments epoch + k·period (k whole) between from
// and to. epoch is a Julian day, period days.
func periodicTimes(epoch, period float64, from, to time.Time) []time.Time {
	var out []time.Time
	// Start one cycle early: rounding must not skip a moment right at from.
	k := math.Floor((JD(from)-epoch)/period) - 1
	for {
		t := FromJD(epoch + k*period)
		if !t.Before(to) {
			return out
		}
		if !t.Before(from) {
			out = append(out, t)
		}
		k++
	}
}

// AlgolMinima are the middles of Algol's eclipses between from and to.
// Algol, the "Demon Star" in Perseus, is two stars circling each other
// every 2 days 21 hours; when the dim one passes in front of the bright one,
// Algol fades from magnitude 2.1 to 3.4 for about ten hours, faintest at
// these moments.
//
// Elements of the GCVS (General Catalogue of Variable Stars): minimum =
// HJD 2445641.5135 + 2.8673043·E. The heliocentric correction (light's
// trip across the Earth's orbit, up to about ±8 minutes for Algol) is
// left out, and the real period wanders a little, so take the times as
// good to a quarter of an hour: fine for a fade that takes five hours.
func AlgolMinima(from, to time.Time) []time.Time {
	return periodicTimes(2445641.5135, 2.8673043, from, to)
}

// DeltaCepheiMaxima are the moments Delta Cephei is at its brightest
// between from and to. It is the star that gave its name to the Cepheids,
// stars that swell and shrink: it brightens from magnitude 4.4 to 3.5 in
// about a day and a half, then fades slowly over four days.
//
// GCVS elements: maximum = JD 2436075.445 + 5.366249·E (heliocentric
// correction left out, as for Algol; good to within an hour or so).
func DeltaCepheiMaxima(from, to time.Time) []time.Time {
	return periodicTimes(2436075.445, 5.366249, from, to)
}
