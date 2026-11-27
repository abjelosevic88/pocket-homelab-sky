# API

Everything is under `/api/v1`, JSON in and out. Authenticate with the session cookie, or a personal token (Settings → API & shortcuts) as `Authorization: Bearer psky_…`. A Pocket Core app link (`pcx_…`) reaches only the routes of its scopes (`sky:read`, `sky:write`, and one route each for `pocket:search`, `pocket:day`, `pocket:widget` and `pocket:capture`).

Places are given as `?location=ID` (one of yours; home when left out) or `?lat=&lon=` (any spot, with `&tz=Europe/Sarajevo` for its time zone). Times are RFC 3339; positions are degrees.

## The sky

| | |
|---|---|
| `GET /summary` | Tonight at home in a few words: rating, cloud, darkness, the Moon, the planets up, the next event (Homepage, Shortcuts, Pocket AI) |
| `GET /tonight[?date=YYYY-MM-DD]` | One night: sunset, twilight, the dark hours, the Moon, every planet (when, where, how bright), the best sights for how dark the place is, cloud by the hour, the rating and the week's events |
| `GET /nights[?days=7]` | The coming nights rated (cloud over the dark hours, the Moon, darkness) |
| `GET /bodies[?t=]` | The Sun, the Moon and the planets at a moment: altitude, azimuth, direction, magnitude, phase, distance, constellation, next rise and set |
| `GET /events[?from=&to=&kinds=&visible=1]` | Sky events (a year by default): `moon_phase`, `season`, `conjunction`, `opposition`, `elongation`, `meteor_shower`, `lunar_eclipse`, `solar_eclipse` — eclipses with their times and whether they can be seen from the place |
| `GET /objects/search?q=` | Planets, stars, Messier objects, favourites and constellations by name or number |
| `GET /objects/{key}[?t=]` | One object at the place: where it is now, rise, highest and set today, the best time tonight, how often you logged it. Keys: `jupiter`, `moon`, `star:32349`, `dso:M31`, `dso:double-cluster`, `con:Ori` |
| `GET /catalog` | The fixed sky the map draws (stars, lines, borders, deep-sky objects, Milky Way), cached with an ETag |
| `GET /overview` | Your places, each with tonight's rating, and the next event at home |
| `GET /day/YYYY-MM-DD` | One night for Pocket Day: its sky, events and your log entries |
| `GET /search?q=` | Objects and log entries, for Pocket Search: `{"results":[{"id","kind","title","snippet","date","url"}]}` |
| `POST /calendar` | Put an event on your Pocket Calendar: `{"title","description","start":"YYYY-MM-DDTHH:MM","end"}` |

## The observing log

| | |
|---|---|
| `GET /observations[?from=&to=&q=&object=&limit=]` | Your nights, newest first, with what was seen |
| `POST /observations` | `{"night":"2026-11-12","location_id":1,"started_at":"2026-11-12T21:00","ended_at":"2026-11-12T23:30","title":"","equipment":"10×50 binoculars","seeing":4,"transparency":3,"cloud":10,"rating":4,"notes":"","objects":[{"key":"jupiter","note":"four moons"},{"name":"A bright satellite"}]}` — a linked app may add `source_ref` to make it idempotent |
| `GET/PUT/DELETE /observations/{id}` | One night (a linked app changes only its own) |
| `POST /observations/{id}/note` | Save it in Pocket Notes (folder Sky) |
| `GET /equipment` | What you have logged with, most used first |
| `GET/POST /targets`, `PUT/DELETE /targets/{id}` | The want-to-see list (`{"key":"dso:M51","note":""}`), each with the night it was first logged |
| `GET /progress` | The Messier list, the planets, the constellations and favourites: what you have seen, and what is up tonight |

## Places

`GET/POST /locations`, `GET/PUT/DELETE /locations/{id}` (`name, lat, lon, timezone, bortle 0–9, alerts, is_home, note`), `POST /locations/order {"ids":[…]}`, `POST /locations/import {"from":"weather"|"places","ids":[…]}` (list first without ids), `GET /geo/search?q=`, `GET /geo/reverse?lat=&lon=`.

## Everything else

`/auth/*`, `/settings`, `/tokens`, `/users`, `/ai/status|models|plan|ask|explain`, `/notify/test|check`, `/export.json`, `/export.csv`, `/import`, `/backup` (admins), `/healthz`, `/metrics` (when `METRICS_ENABLED`).

## For Pocket Search, Pocket Day and Pocket Hub

Four routes in the shape every Pocket app answers in (Pocket Core's capabilities). Each has its own link scope, which opens that one route and nothing else; you can call them with your own session or token too. They never ask an outside service: the sky is worked out on the box, and the cloud and northern-lights forecasts are used only when the app already keeps a recent copy.

| | | |
|---|---|---|
| `GET /pocket/search?q=&limit=` | `pocket:search` | Your log entries, plans, want-to-see list and equipment, and the planets, stars, constellations and deep-sky objects by name: `{"items":[{"id":"observation:12","title","snippet","kind","date","url"}]}`. The object the words name comes first, then your own things |
| `GET /pocket/day/YYYY-MM-DD[?tz=]` | `pocket:day` | The nights you logged for that date and what you saw (`observed`), the plans for that night (`planned`) and up to three sky events of the day at your home place (`event`), with a line like "Observed Jupiter and M42 Orion Nebula" |
| `GET /pocket/widget` | `pocket:widget` | Tonight at your home place: a line like "Dark from 18:40, Moon 12%", then a sky event, the planets that are up, the best sights and your next plan (at most eight rows). Empty until you add a place |
| `POST /pocket/capture` | `pocket:capture` | `{"request_id":"…","kind":"observation","text":"Saw Jupiter's moons, clear sky","date":"2026-11-12"}` adds an entry to the observing log: the first line is its title, the rest the notes, the night is `date` or tonight, the place your home place. The same `request_id` again answers the same entry and adds nothing |

Pocket Move can copy your places, observing log and want-to-see list out and bring them in: `GET /pocket/portable` describes them for `move:export` / `move:import` links (`/export.json`, `/export.csv` out; `/import` in, adding only what is not here yet).
