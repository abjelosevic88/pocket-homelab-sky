CREATE TABLE users (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  email         TEXT NOT NULL UNIQUE,
  name          TEXT NOT NULL DEFAULT '',
  password_hash TEXT NOT NULL DEFAULT '',
  role          TEXT NOT NULL DEFAULT 'member',
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at    TEXT
);

CREATE TABLE sessions (
  id         TEXT PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE api_tokens (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         TEXT NOT NULL DEFAULT '',
  token_hash   TEXT NOT NULL UNIQUE,
  prefix       TEXT NOT NULL DEFAULT '',
  last_used_at TEXT,
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- Box-wide settings (title, setup flag); everything else is personal.
CREATE TABLE settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL DEFAULT ''
);

CREATE TABLE user_settings (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  key     TEXT NOT NULL,
  value   TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (user_id, key)
);

CREATE TABLE cache (
  key        TEXT PRIMARY KEY,
  value      TEXT NOT NULL DEFAULT '',
  fetched_at TEXT NOT NULL
);


-- ---------- Pocket Sky: everything below is per person ----------

-- A place to watch the sky from: home, the balcony, a dark spot in the hills.
-- One is home (the default everywhere: Tonight, the map, the messages).
CREATE TABLE locations (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         TEXT NOT NULL,                         -- "Home", "Bjelašnica"
  region       TEXT NOT NULL DEFAULT '',
  country      TEXT NOT NULL DEFAULT '',
  country_code TEXT NOT NULL DEFAULT '',
  lat          REAL NOT NULL,
  lon          REAL NOT NULL,
  elevation    REAL,
  timezone     TEXT NOT NULL DEFAULT '',              -- Europe/Sarajevo
  is_home      INTEGER NOT NULL DEFAULT 0,
  alerts       INTEGER NOT NULL DEFAULT 1,            -- tell me about clear nights here
  bortle       INTEGER NOT NULL DEFAULT 0,            -- how dark the sky is: 1 (pristine) … 9 (city centre), 0 = not known
  sort         INTEGER NOT NULL DEFAULT 0,
  note         TEXT NOT NULL DEFAULT '',              -- "trees to the east", "park by the lake"
  source       TEXT NOT NULL DEFAULT '',              -- '' by hand | import | <app slug>
  source_ref   TEXT NOT NULL DEFAULT '',
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX locations_user ON locations(user_id, sort, name);
CREATE UNIQUE INDEX locations_source ON locations(user_id, source, source_ref) WHERE source_ref <> '';

-- The observing log: one night (or a moment) out under the sky.
CREATE TABLE observations (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  location_id  INTEGER REFERENCES locations(id) ON DELETE SET NULL,
  place        TEXT NOT NULL DEFAULT '',              -- the place's name, kept when the place goes
  lat          REAL,
  lon          REAL,
  night        TEXT NOT NULL,                         -- YYYY-MM-DD: the evening's date
  started_at   TEXT NOT NULL DEFAULT '',              -- local wall clock, 2026-11-12T21:30
  ended_at     TEXT NOT NULL DEFAULT '',
  title        TEXT NOT NULL DEFAULT '',
  notes        TEXT NOT NULL DEFAULT '',
  seeing       INTEGER NOT NULL DEFAULT 0,            -- steadiness of the air 1 (boiling) … 5 (steady), 0 = not noted
  transparency INTEGER NOT NULL DEFAULT 0,            -- clarity 1 (hazy) … 5 (crystal), 0 = not noted
  cloud        INTEGER,                               -- % cloud, from the forecast or by hand
  moon_phase   REAL,                                  -- lit fraction of the Moon that night (worked out)
  bortle       INTEGER NOT NULL DEFAULT 0,
  equipment    TEXT NOT NULL DEFAULT '',              -- "Eyes", "10×50 binoculars", "8-inch Dobsonian"
  rating       INTEGER NOT NULL DEFAULT 0,            -- 0–5 stars, how good the night was
  source       TEXT NOT NULL DEFAULT '',              -- '' by hand | import | <app slug>
  source_ref   TEXT NOT NULL DEFAULT '',
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX observations_user ON observations(user_id, night DESC, id DESC);
CREATE UNIQUE INDEX observations_source ON observations(user_id, source, source_ref) WHERE source_ref <> '';

-- What was seen on a night: a planet, a star, a galaxy, the Moon, an eclipse.
CREATE TABLE observation_objects (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id        INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  observation_id INTEGER NOT NULL REFERENCES observations(id) ON DELETE CASCADE,
  object_key     TEXT NOT NULL,                       -- jupiter | moon | dso:M31 | star:32349 | con:Ori | event:<key> | other:<text>
  name           TEXT NOT NULL,
  note           TEXT NOT NULL DEFAULT '',
  sort           INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX observation_objects_obs ON observation_objects(user_id, observation_id, sort);
CREATE INDEX observation_objects_key ON observation_objects(user_id, object_key);

-- Things the person wants to see; ticked off by the log on its own.
CREATE TABLE targets (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  object_key  TEXT NOT NULL,
  name        TEXT NOT NULL,
  note        TEXT NOT NULL DEFAULT '',
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE UNIQUE INDEX targets_key ON targets(user_id, object_key);

-- Messages already sent, so each is sent once (clear:2026-11-12:3, event:<key>).
CREATE TABLE sent (
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  key        TEXT NOT NULL,
  sent_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  PRIMARY KEY (user_id, key)
);
