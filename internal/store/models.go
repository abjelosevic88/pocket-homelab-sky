package store

import "strings"

// User is an account on this box.
type User struct {
	ID           int64  `json:"id"`
	Email        string `json:"email"`
	Name         string `json:"name"`
	PasswordHash string `json:"-"`
	Role         string `json:"role"` // admin | member
	CreatedAt    string `json:"created_at"`
}

// APIToken is a personal access token (hash stored, plaintext shown once).
type APIToken struct {
	ID         int64   `json:"id"`
	UserID     int64   `json:"user_id"`
	Name       string  `json:"name"`
	Prefix     string  `json:"prefix"`
	LastUsedAt *string `json:"last_used_at"`
	CreatedAt  string  `json:"created_at"`
}

// Location is a place the person watches the sky from.
type Location struct {
	ID          int64    `json:"id"`
	UserID      int64    `json:"-"`
	Name        string   `json:"name"`
	Region      string   `json:"region"`
	Country     string   `json:"country"`
	CountryCode string   `json:"country_code"`
	Lat         float64  `json:"lat"`
	Lon         float64  `json:"lon"`
	Elevation   *float64 `json:"elevation"`
	Timezone    string   `json:"timezone"`
	IsHome      bool     `json:"is_home"`
	Alerts      bool     `json:"alerts"`
	Bortle      int      `json:"bortle"`
	Sort        int      `json:"sort"`
	Note        string   `json:"note"`
	Source      string   `json:"source"`
	SourceRef   string   `json:"source_ref"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
}

// Label is the name with its country when that helps ("Vienna, Austria").
func (l Location) Label() string {
	if l.Country != "" && !strings.Contains(l.Name, l.Country) {
		return l.Name + ", " + l.Country
	}
	return l.Name
}

// Observation is one entry of the observing log: a night (or a moment)
// out under the sky and what was seen.
type Observation struct {
	ID           int64       `json:"id"`
	UserID       int64       `json:"-"`
	LocationID   *int64      `json:"location_id"`
	Place        string      `json:"place"`
	Lat          *float64    `json:"lat"`
	Lon          *float64    `json:"lon"`
	Night        string      `json:"night"`
	StartedAt    string      `json:"started_at"`
	EndedAt      string      `json:"ended_at"`
	Title        string      `json:"title"`
	Notes        string      `json:"notes"`
	Seeing       int         `json:"seeing"`
	Transparency int         `json:"transparency"`
	Cloud        *int        `json:"cloud"`
	MoonPhase    *float64    `json:"moon_phase"`
	Bortle       int         `json:"bortle"`
	Equipment    string      `json:"equipment"`
	Rating       int         `json:"rating"`
	Source       string      `json:"source"`
	SourceRef    string      `json:"source_ref"`
	CreatedAt    string      `json:"created_at"`
	UpdatedAt    string      `json:"updated_at"`
	Objects      []ObsObject `json:"objects"`
}

// ObsObject is one thing seen on a night.
type ObsObject struct {
	ID   int64  `json:"id"`
	Key  string `json:"key"`
	Name string `json:"name"`
	Note string `json:"note"`
}

// Target is something the person wants to see.
type Target struct {
	ID        int64  `json:"id"`
	Key       string `json:"key"`
	Name      string `json:"name"`
	Note      string `json:"note"`
	CreatedAt string `json:"created_at"`
	// Filled on the way out: the first night it was logged.
	SeenOn string `json:"seen_on"`
}

// Conn is how a person reaches one sibling Pocket app: "core" (an app link
// through Pocket Core), "direct" (URL + token) or "off"; "" = not chosen yet.
type Conn struct {
	Mode      string `json:"mode"`
	URL       string `json:"url"`
	Token     string `json:"token"`
	PublicURL string `json:"public_url"`
}

// Settings are the box-wide values plus one person's preferences and their
// own connections.
type Settings struct {
	Title         string `json:"title"`
	SetupComplete bool   `json:"setup_complete"`

	DateFormat string `json:"date_format"`
	TimeFormat string `json:"time_format"`
	WeekStart  int    `json:"week_start"` // 1 = Monday, 7 = Sunday
	TempUnit   string `json:"temp_unit"`  // c | f, for the forecast

	// The sky map.
	MagLimit      float64 `json:"mag_limit"`      // faintest stars drawn (3–6.5)
	ShowLines     bool    `json:"show_lines"`     // constellation lines
	ShowConNames  bool    `json:"show_con_names"` // constellation names
	ShowStarNames bool    `json:"show_star_names"`
	ShowDSO       bool    `json:"show_dso"` // clusters, nebulae, galaxies
	ShowMilkyWay  bool    `json:"show_milky_way"`
	ShowGrid      bool    `json:"show_grid"`     // altitude/azimuth grid
	ShowEquator   bool    `json:"show_equator"`  // the sky's equator and its grid
	ShowEcliptic  bool    `json:"show_ecliptic"` // the Sun's path
	NightRed      bool    `json:"night_red"`     // everything red, to keep your night vision
	CameraFOV     float64 `json:"camera_fov"`    // how much of the sky the phone's camera sees, top to bottom (degrees)

	// What counts as a clear night.
	CloudMax int `json:"cloud_max"` // % cloud at most, on average over the dark hours

	// Messages.
	NotifyClear  bool     `json:"notify_clear"`  // tonight looks clear and dark
	NotifyEvents bool     `json:"notify_events"` // a sky event tonight
	EventKinds   []string `json:"event_kinds"`   // which events
	NotifyHour   int      `json:"notify_hour"`
	NotifyURL    string   `json:"notify_url"`
	NotifyFormat string   `json:"notify_format"` // apprise | ntfy | json

	// AI: the night in plain words on Tonight.
	AIPlan bool `json:"ai_plan"`

	// Each sibling Pocket app, by slug.
	Conns map[string]Conn `json:"conns"`

	// Pocket AI through Core, or an OpenAI-compatible API with the person's own key.
	AIProvider    string `json:"ai_provider"` // auto | pocket | openai
	AIPocketModel string `json:"ai_pocket_model"`
	OpenAIKey     string `json:"openai_api_key"`
	OpenAIModel   string `json:"openai_model"`
	OpenAIBaseURL string `json:"openai_base_url"`
}

// SplitTags turns "a, b ,c" into clean lowercase tags.
func SplitTags(s string) []string {
	var out []string
	seen := map[string]bool{}
	for _, t := range strings.Split(s, ",") {
		t = strings.ToLower(strings.TrimSpace(t))
		if t != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

// JoinTags is the stored form of tags.
func JoinTags(s string) string { return strings.Join(SplitTags(s), ", ") }
