-- What the person looks with: binoculars, telescopes, eyepieces, cameras.
CREATE TABLE kit (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  kind        TEXT NOT NULL,                  -- binoculars | telescope | eyepiece | camera
  name        TEXT NOT NULL,
  aperture    REAL NOT NULL DEFAULT 0,        -- mm (binoculars, telescopes)
  focal       REAL NOT NULL DEFAULT 0,        -- mm (telescopes, eyepieces, camera lenses)
  afov        REAL NOT NULL DEFAULT 0,        -- eyepiece apparent field, degrees
  tfov        REAL NOT NULL DEFAULT 0,        -- binoculars' true field, degrees
  power       REAL NOT NULL DEFAULT 0,        -- binoculars' magnification
  sensor_w    REAL NOT NULL DEFAULT 0,        -- camera sensor, mm
  sensor_h    REAL NOT NULL DEFAULT 0,
  notes       TEXT NOT NULL DEFAULT '',
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX kit_user ON kit(user_id, kind, name);

-- An observing plan: a list of things to look at on a night, in order.
CREATE TABLE plans (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name        TEXT NOT NULL,
  night       TEXT NOT NULL,                  -- YYYY-MM-DD
  location_id INTEGER REFERENCES locations(id) ON DELETE SET NULL,
  setup       TEXT NOT NULL DEFAULT '',       -- what it is for: "10×50 binoculars"
  notes       TEXT NOT NULL DEFAULT '',
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX plans_user ON plans(user_id, night DESC);

CREATE TABLE plan_items (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  plan_id     INTEGER NOT NULL REFERENCES plans(id) ON DELETE CASCADE,
  object_key  TEXT NOT NULL,
  name        TEXT NOT NULL,
  note        TEXT NOT NULL DEFAULT '',
  sort        INTEGER NOT NULL DEFAULT 0,
  done        INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX plan_items_plan ON plan_items(user_id, plan_id, sort);

-- Photos of the sky: kept with a night of the log, maybe solved (what is in them).
CREATE TABLE photos (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id        INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  observation_id INTEGER REFERENCES observations(id) ON DELETE SET NULL,
  object_key     TEXT NOT NULL DEFAULT '',
  caption        TEXT NOT NULL DEFAULT '',
  file           TEXT NOT NULL,               -- under DATA_DIR/photos
  mime           TEXT NOT NULL DEFAULT '',
  size           INTEGER NOT NULL DEFAULT 0,
  width          INTEGER NOT NULL DEFAULT 0,
  height         INTEGER NOT NULL DEFAULT 0,
  source         TEXT NOT NULL DEFAULT '',    -- '' uploaded | photos (Pocket Photos)
  source_ref     TEXT NOT NULL DEFAULT '',
  solve          TEXT NOT NULL DEFAULT '',    -- JSON: astrometry.net state and result, or Pocket AI's description
  created_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX photos_user ON photos(user_id, observation_id, id);
