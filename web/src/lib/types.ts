export interface User { id: number; email: string; name: string; role: 'admin' | 'member'; created_at: string }
export interface APIToken { id: number; user_id: number; name: string; prefix: string; last_used_at: string | null; created_at: string }

export type ConnMode = 'core' | 'direct' | 'off'
export type ConnVia = '' | 'core' | 'direct'
export type AppSlug = 'calendar' | 'weather' | 'places' | 'notes'

type ConnFields = { [K in AppSlug as `${K}_mode`]: ConnMode } & { [K in AppSlug as `${K}_via`]: ConnVia } & { [K in AppSlug as `${K}_detail` | `${K}_url` | `${K}_token` | `${K}_public_url` | `${K}_open_url`]: string } & { [K in AppSlug as `${K}_configured`]: boolean }

export type EventKind = 'meteor_shower' | 'lunar_eclipse' | 'solar_eclipse' | 'conjunction' | 'opposition' | 'elongation' | 'moon_phase' | 'season' | 'aurora'

export type Settings = ConnFields & {
  core_configured: boolean
  core_registered?: boolean
  title: string; date_format: string; time_format: string; week_start: number
  temp_unit: 'c' | 'f'
  mag_limit: number; show_lines: boolean; show_con_names: boolean; show_star_names: boolean; show_dso: boolean; show_milky_way: boolean
  show_grid: boolean; show_equator: boolean; show_ecliptic: boolean; night_red: boolean; camera_fov: number; cloud_max: number
  notify_clear: boolean; notify_events: boolean; event_kinds: EventKind[]; event_kinds_all: EventKind[]; notify_hour: number
  notify_url: string; notify_format: string; notify_configured: boolean
  ai_plan: boolean
  ai_provider: 'auto' | 'pocket' | 'openai'; ai_pocket_model: string; openai_api_key: string; openai_model: string; openai_base_url: string
  setup_complete: boolean; base_url: string
}

export interface Place {
  id: number; name: string; region: string; country: string; country_code: string; lat: number; lon: number
  elevation: number | null; timezone: string; is_home: boolean; alerts: boolean; bortle: number; sort: number; note: string
  source: string; source_ref: string; created_at: string; updated_at: string
}

export interface PlaceRef { id?: number; name: string; lat: number; lon: number; timezone: string; bortle: number }

export interface Score { value: number; label: 'Great' | 'Good' | 'Fair' | 'Poor' | 'Cloudy'; words: string; cloud_avg: number | null; clear: boolean; forecast: boolean; fog: boolean }

export interface Night {
  date: string; sunset: string | null; civil_dusk: string | null; nautical_dusk: string | null; astro_dusk: string | null
  astro_dawn: string | null; nautical_dawn: string | null; civil_dawn: string | null; sunrise: string | null
  dark_from: string; dark_to: string; dark_kind: 'astro' | 'nautical' | 'civil' | 'none'; dark_hours: number
  moon_rise: string | null; moon_set: string | null; moon_phase: number; moon_age: number; moon_up_dark: number; moon_dark_hrs: number
}

export interface NightHour { seeing: number | null; transparency: number | null; time: string; cloud: number | null; low: number | null; mid: number | null; high: number | null; humidity: number | null; temp: number | null; dew: number | null; wind: number | null; sun_alt: number; moon_alt: number; dark: boolean }

export interface MoonView { phase: number; phase_name: string; age: number; waxing: boolean; rise: string | null; set: string | null; up_dark: number; con: string; dist_km: number; words: string; next_new: string | null; next_full: string | null }

export interface PlanetView { key: string; name: string; mag: number; phase: number; con: string; con_name: string; rise: string | null; set: string | null; best_at: string | null; best_alt: number; best_az: number; dir: string; visible: boolean; when: string; words: string; elong: number; size: number; dist_au: number }

export interface SightView { key: string; name: string; type: string; mag: number | null; con: string; con_name: string; best_at: string | null; best_alt: number; dir: string; words: string; seen: boolean; kit: 'eyes' | 'binoculars' | 'telescope' }

export interface SkyEvent {
  kind: EventKind; t: string; title: string; detail: string; bodies?: string[]; value?: number; sub?: string; key: string
  visible?: boolean; start?: string; end?: string; alt?: number
}

export interface Tonight {
  place: PlaceRef; date: string; night: Night; score: Score; moon: MoonView; planets: PlanetView[]; sights: SightView[]
  hours: NightHour[]; events: SkyEvent[]; stale: boolean; forecast_at: string
  air: { seeing: number; transparency: number; words: string } | null
  aurora: { kp_max: number; kp_needed: number; chance: number; likely: boolean; words: string; mag_lat: number } | null
  darkness: Darkness | null
}

export interface Darkness { zone: string; rank: number; bortle: number; words: string }
export interface DarkSpot { lat: number; lon: number; rank: number; zone: string; bortle: number; km: number; dir: number; dir_words: string; name: string; region: string; words: string }

export interface NightSummary { date: string; score: Score; dark_from: string; dark_to: string; dark_kind: string; dark_hours: number; moon_phase: number; moon_name: string; moon_up_dark: number }

export interface ObsObject { id?: number; key: string; name: string; note: string }
export interface Observation {
  id: number; location_id: number | null; place: string; lat: number | null; lon: number | null; night: string; started_at: string; ended_at: string
  title: string; notes: string; seeing: number; transparency: number; cloud: number | null; moon_phase: number | null; bortle: number
  equipment: string; rating: number; source: string; source_ref: string; created_at: string; updated_at: string; objects: ObsObject[]
}

export interface Target { id: number; key: string; name: string; note: string; created_at: string; seen_on: string }

export interface SkyObject { key: string; name: string; kind: 'star' | 'dso' | 'constellation' | 'planet' | 'moon' | 'sun'; type: string; ra: number; dec: number; mag: number | null; con: string; about: string; alt_name?: string }

export interface ObjectDetail extends SkyObject {
  con_name: string; ra_date: number; dec_date: number; alt: number; az: number; dir: string; up: boolean
  rise: string | null; transit: string | null; set: string | null; always?: 'up' | 'down'; best_at: string | null; best_alt: number; tonight: string
  body?: { mag: number; phase: number; elong: number; size: number; dist_au: number; dist_km: number }
  seen: { first: string; last: string; times: number } | null; target_id: number
}

export interface ProgressItem { key: string; name: string; type: string; con: string; mag: number | null; seen: string; times: number; visible_tonight: boolean }
export interface ProgressGroup { seen: number; total: number; items: ProgressItem[] }
export interface Progress {
  stats: { nights: number; entries: number; objects: number; hours: number; first: string; last: string }
  messier: ProgressGroup; highlights: ProgressGroup; planets: ProgressGroup; constellations: ProgressGroup
}

export interface Overview { places: (Place & { tonight: NightSummary | null })[]; next_event: SkyEvent | null }

export interface SatPass {
  name: string; norad: number; group: 'station' | 'visual' | 'starlink'; start: string; peak: string; end: string
  start_dir: string; peak_dir: string; end_dir: string; peak_az: number; peak_alt: number; mag: number
  visible: boolean; visible_from?: string; visible_to?: string; train: boolean; count: number; words: string
}
export interface SatNow { name: string; norad: number; group: string; alt: number; az: number; alt2: number; az2: number; mag: number; sunlit: boolean; km: number }
