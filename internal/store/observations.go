package store

import (
	"context"
	"database/sql"
	"strings"
)

const obsCols = `id, user_id, location_id, place, lat, lon, night, started_at, ended_at, title, notes, seeing, transparency, cloud, moon_phase, bortle, equipment, rating, source, source_ref, created_at, updated_at`

func scanObservation(sc interface{ Scan(...any) error }) (*Observation, error) {
	var o Observation
	var loc sql.NullInt64
	var lat, lon, moon sql.NullFloat64
	var cloud sql.NullInt64
	if err := sc.Scan(&o.ID, &o.UserID, &loc, &o.Place, &lat, &lon, &o.Night, &o.StartedAt, &o.EndedAt, &o.Title, &o.Notes, &o.Seeing, &o.Transparency,
		&cloud, &moon, &o.Bortle, &o.Equipment, &o.Rating, &o.Source, &o.SourceRef, &o.CreatedAt, &o.UpdatedAt); err != nil {
		return nil, err
	}
	if loc.Valid {
		v := loc.Int64
		o.LocationID = &v
	}
	if cloud.Valid {
		v := int(cloud.Int64)
		o.Cloud = &v
	}
	o.Lat, o.Lon, o.MoonPhase = optF(lat), optF(lon), optF(moon)
	o.Objects = []ObsObject{}
	return &o, nil
}

// ObsFilter narrows the log.
type ObsFilter struct {
	From, To string // nights, inclusive
	Q        string // words in the title, notes, place or objects
	Object   string // an object key
	Limit    int
}

// ListObservations returns one person's log, newest night first, with what was seen.
func (s *Store) ListObservations(ctx context.Context, uid int64, f ObsFilter) ([]Observation, error) {
	q := `SELECT ` + obsCols + ` FROM observations WHERE user_id = ?`
	args := []any{uid}
	if f.From != "" {
		q += ` AND night >= ?`
		args = append(args, f.From)
	}
	if f.To != "" {
		q += ` AND night <= ?`
		args = append(args, f.To)
	}
	if f.Object != "" {
		q += ` AND id IN (SELECT observation_id FROM observation_objects WHERE user_id = ? AND object_key = ?)`
		args = append(args, uid, f.Object)
	}
	if w := strings.TrimSpace(f.Q); w != "" {
		like := "%" + strings.ToLower(w) + "%"
		q += ` AND (lower(title) LIKE ? OR lower(notes) LIKE ? OR lower(place) LIKE ? OR lower(equipment) LIKE ?
			OR id IN (SELECT observation_id FROM observation_objects WHERE user_id = ? AND (lower(name) LIKE ? OR lower(note) LIKE ?)))`
		args = append(args, like, like, like, like, uid, like, like)
	}
	q += ` ORDER BY night DESC, started_at DESC, id DESC`
	if f.Limit > 0 {
		q += ` LIMIT ?`
		args = append(args, f.Limit)
	}
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	out := []Observation{}
	for rows.Next() {
		o, err := scanObservation(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, *o)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return out, nil
	}
	objs, err := s.objectsFor(ctx, uid, out)
	if err != nil {
		return nil, err
	}
	for i := range out {
		if o, ok := objs[out[i].ID]; ok {
			out[i].Objects = o
		}
	}
	return out, nil
}

func (s *Store) objectsFor(ctx context.Context, uid int64, obs []Observation) (map[int64][]ObsObject, error) {
	ids := make([]any, 0, len(obs)+1)
	ids = append(ids, uid)
	ph := make([]string, 0, len(obs))
	for _, o := range obs {
		ids = append(ids, o.ID)
		ph = append(ph, "?")
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT observation_id, id, object_key, name, note FROM observation_objects WHERE user_id = ? AND observation_id IN (`+strings.Join(ph, ",")+`) ORDER BY sort, id`, ids...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64][]ObsObject{}
	for rows.Next() {
		var oid int64
		var x ObsObject
		if err := rows.Scan(&oid, &x.ID, &x.Key, &x.Name, &x.Note); err != nil {
			return nil, err
		}
		out[oid] = append(out[oid], x)
	}
	return out, rows.Err()
}

// GetObservation returns one log entry of the person's (ErrNotFound for anyone else's).
func (s *Store) GetObservation(ctx context.Context, uid, id int64) (*Observation, error) {
	o, err := scanObservation(s.DB.QueryRowContext(ctx, `SELECT `+obsCols+` FROM observations WHERE user_id = ? AND id = ?`, uid, id))
	if err != nil {
		return nil, notFound(err)
	}
	objs, err := s.objectsFor(ctx, uid, []Observation{*o})
	if err != nil {
		return nil, err
	}
	if v, ok := objs[o.ID]; ok {
		o.Objects = v
	}
	return o, nil
}

// ObservationBySource finds a log entry another app (or an import) added.
func (s *Store) ObservationBySource(ctx context.Context, uid int64, source, ref string) (*Observation, error) {
	var id int64
	err := s.DB.QueryRowContext(ctx, `SELECT id FROM observations WHERE user_id = ? AND source = ? AND source_ref = ?`, uid, source, ref).Scan(&id)
	if err != nil {
		return nil, notFound(err)
	}
	return s.GetObservation(ctx, uid, id)
}

func nullInt(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

// CreateObservation adds a log entry with its objects.
func (s *Store) CreateObservation(ctx context.Context, o *Observation) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO observations (user_id, location_id, place, lat, lon, night, started_at, ended_at, title, notes, seeing, transparency, cloud, moon_phase, bortle, equipment, rating, source, source_ref)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		o.UserID, nullI(o.LocationID), o.Place, nullF(o.Lat), nullF(o.Lon), o.Night, o.StartedAt, o.EndedAt, o.Title, o.Notes, o.Seeing, o.Transparency,
		nullInt(o.Cloud), nullF(o.MoonPhase), o.Bortle, o.Equipment, o.Rating, o.Source, o.SourceRef)
	if err != nil {
		return conflict(err)
	}
	o.ID, _ = res.LastInsertId()
	if err := writeObjects(ctx, tx, o.UserID, o.ID, o.Objects); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	got, err := s.GetObservation(ctx, o.UserID, o.ID)
	if err == nil {
		*o = *got
	}
	return err
}

func writeObjects(ctx context.Context, tx *sql.Tx, uid, oid int64, objs []ObsObject) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM observation_objects WHERE user_id = ? AND observation_id = ?`, uid, oid); err != nil {
		return err
	}
	for i, x := range objs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO observation_objects (user_id, observation_id, object_key, name, note, sort) VALUES (?, ?, ?, ?, ?, ?)`,
			uid, oid, x.Key, x.Name, x.Note, i); err != nil {
			return err
		}
	}
	return nil
}

// UpdateObservation saves a log entry; objects are replaced when given (nil keeps them).
func (s *Store) UpdateObservation(ctx context.Context, o *Observation, objects []ObsObject) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE observations SET location_id = ?, place = ?, lat = ?, lon = ?, night = ?, started_at = ?, ended_at = ?, title = ?, notes = ?,
		seeing = ?, transparency = ?, cloud = ?, moon_phase = ?, bortle = ?, equipment = ?, rating = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
		WHERE user_id = ? AND id = ?`,
		nullI(o.LocationID), o.Place, nullF(o.Lat), nullF(o.Lon), o.Night, o.StartedAt, o.EndedAt, o.Title, o.Notes, o.Seeing, o.Transparency,
		nullInt(o.Cloud), nullF(o.MoonPhase), o.Bortle, o.Equipment, o.Rating, o.UserID, o.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	if objects != nil {
		if err := writeObjects(ctx, tx, o.UserID, o.ID, objects); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	got, err := s.GetObservation(ctx, o.UserID, o.ID)
	if err == nil {
		*o = *got
	}
	return err
}

// DeleteObservation removes a log entry of the person's.
func (s *Store) DeleteObservation(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM observations WHERE user_id = ? AND id = ?`, uid, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Seen is the first and last night an object was logged, and how often.
type Seen struct {
	Key   string `json:"key"`
	Name  string `json:"name"`
	First string `json:"first"`
	Last  string `json:"last"`
	Times int    `json:"times"`
}

// SeenObjects is everything the person has logged, by object key.
func (s *Store) SeenObjects(ctx context.Context, uid int64) (map[string]Seen, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT x.object_key, MAX(x.name), MIN(o.night), MAX(o.night), COUNT(DISTINCT o.id)
		FROM observation_objects x JOIN observations o ON o.id = x.observation_id AND o.user_id = x.user_id
		WHERE x.user_id = ? GROUP BY x.object_key`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Seen{}
	for rows.Next() {
		var v Seen
		if err := rows.Scan(&v.Key, &v.Name, &v.First, &v.Last, &v.Times); err != nil {
			return nil, err
		}
		out[v.Key] = v
	}
	return out, rows.Err()
}

// LogStats are the totals of the log.
type LogStats struct {
	Nights  int     `json:"nights"`
	Entries int     `json:"entries"`
	Objects int     `json:"objects"` // different objects
	Hours   float64 `json:"hours"`
	First   string  `json:"first"`
	Last    string  `json:"last"`
}

// Stats sums the person's log.
func (s *Store) Stats(ctx context.Context, uid int64) (LogStats, error) {
	var st LogStats
	var first, last sql.NullString
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(DISTINCT night), COUNT(1), MIN(night), MAX(night),
		COALESCE(SUM(CASE WHEN started_at <> '' AND ended_at > started_at THEN (julianday(ended_at) - julianday(started_at)) * 24 ELSE 0 END), 0)
		FROM observations WHERE user_id = ?`, uid).Scan(&st.Nights, &st.Entries, &first, &last, &st.Hours)
	if err != nil {
		return st, err
	}
	st.First, st.Last = first.String, last.String
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(DISTINCT object_key) FROM observation_objects WHERE user_id = ?`, uid).Scan(&st.Objects)
	return st, nil
}

// Equipment is what the person has written as equipment, most used first.
func (s *Store) Equipment(ctx context.Context, uid int64) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT equipment FROM observations WHERE user_id = ? AND equipment <> '' GROUP BY equipment ORDER BY COUNT(1) DESC, MAX(night) DESC LIMIT 20`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ---- targets ----

// ListTargets returns what the person wants to see, with when each was first logged.
func (s *Store) ListTargets(ctx context.Context, uid int64) ([]Target, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT t.id, t.object_key, t.name, t.note, t.created_at,
		COALESCE((SELECT MIN(o.night) FROM observation_objects x JOIN observations o ON o.id = x.observation_id WHERE x.user_id = t.user_id AND x.object_key = t.object_key), '')
		FROM targets t WHERE t.user_id = ? ORDER BY t.created_at, t.id`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Target{}
	for rows.Next() {
		var t Target
		if err := rows.Scan(&t.ID, &t.Key, &t.Name, &t.Note, &t.CreatedAt, &t.SeenOn); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// GetTarget returns one of the person's targets.
func (s *Store) GetTarget(ctx context.Context, uid, id int64) (*Target, error) {
	ts, err := s.ListTargets(ctx, uid)
	if err != nil {
		return nil, err
	}
	for _, t := range ts {
		if t.ID == id {
			return &t, nil
		}
	}
	return nil, ErrNotFound
}

// AddTarget adds something to see (ErrConflict when it is already there).
func (s *Store) AddTarget(ctx context.Context, uid int64, t *Target) error {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO targets (user_id, object_key, name, note) VALUES (?, ?, ?, ?)`, uid, t.Key, t.Name, t.Note)
	if err != nil {
		return conflict(err)
	}
	id, _ := res.LastInsertId()
	got, err := s.GetTarget(ctx, uid, id)
	if err == nil {
		*t = *got
	}
	return err
}

// UpdateTarget changes a target's note.
func (s *Store) UpdateTarget(ctx context.Context, uid, id int64, note string) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE targets SET note = ? WHERE user_id = ? AND id = ?`, note, uid, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteTarget removes a target.
func (s *Store) DeleteTarget(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM targets WHERE user_id = ? AND id = ?`, uid, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- sent messages ----

// MarkSent records a message as sent; false when it already was.
func (s *Store) MarkSent(ctx context.Context, uid int64, key string) (bool, error) {
	res, err := s.DB.ExecContext(ctx, `INSERT OR IGNORE INTO sent (user_id, key) VALUES (?, ?)`, uid, key)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// PurgeSent forgets messages older than a time (RFC 3339).
func (s *Store) PurgeSent(ctx context.Context, before string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM sent WHERE sent_at < ?`, before)
	return err
}
