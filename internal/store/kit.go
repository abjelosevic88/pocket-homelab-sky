package store

import (
	"context"
	"database/sql"
	"strings"
)

// Kit is one piece of equipment.
type Kit struct {
	ID        int64   `json:"id"`
	Kind      string  `json:"kind"` // binoculars | telescope | eyepiece | camera
	Name      string  `json:"name"`
	Aperture  float64 `json:"aperture"`
	Focal     float64 `json:"focal"`
	AFOV      float64 `json:"afov"`
	TFOV      float64 `json:"tfov"`
	Power     float64 `json:"power"`
	SensorW   float64 `json:"sensor_w"`
	SensorH   float64 `json:"sensor_h"`
	Notes     string  `json:"notes"`
	CreatedAt string  `json:"created_at"`
}

const kitCols = `id, kind, name, aperture, focal, afov, tfov, power, sensor_w, sensor_h, notes, created_at`

func scanKit(sc interface{ Scan(...any) error }) (*Kit, error) {
	var k Kit
	err := sc.Scan(&k.ID, &k.Kind, &k.Name, &k.Aperture, &k.Focal, &k.AFOV, &k.TFOV, &k.Power, &k.SensorW, &k.SensorH, &k.Notes, &k.CreatedAt)
	return &k, err
}

// ListKit is the person's equipment.
func (s *Store) ListKit(ctx context.Context, uid int64) ([]Kit, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+kitCols+` FROM kit WHERE user_id = ? ORDER BY CASE kind WHEN 'binoculars' THEN 1 WHEN 'telescope' THEN 2 WHEN 'eyepiece' THEN 3 ELSE 4 END, focal DESC, name`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Kit{}
	for rows.Next() {
		k, err := scanKit(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *k)
	}
	return out, rows.Err()
}

// GetKit is one piece of the person's equipment.
func (s *Store) GetKit(ctx context.Context, uid, id int64) (*Kit, error) {
	k, err := scanKit(s.DB.QueryRowContext(ctx, `SELECT `+kitCols+` FROM kit WHERE user_id = ? AND id = ?`, uid, id))
	return k, notFound(err)
}

// SaveKit adds (ID 0) or changes a piece of equipment.
func (s *Store) SaveKit(ctx context.Context, uid int64, k *Kit) error {
	k.Name = strings.TrimSpace(k.Name)
	var res sql.Result
	var err error
	if k.ID == 0 {
		res, err = s.DB.ExecContext(ctx, `INSERT INTO kit (user_id, kind, name, aperture, focal, afov, tfov, power, sensor_w, sensor_h, notes) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			uid, k.Kind, k.Name, k.Aperture, k.Focal, k.AFOV, k.TFOV, k.Power, k.SensorW, k.SensorH, k.Notes)
		if err == nil {
			k.ID, _ = res.LastInsertId()
		}
	} else {
		res, err = s.DB.ExecContext(ctx, `UPDATE kit SET kind = ?, name = ?, aperture = ?, focal = ?, afov = ?, tfov = ?, power = ?, sensor_w = ?, sensor_h = ?, notes = ? WHERE user_id = ? AND id = ?`,
			k.Kind, k.Name, k.Aperture, k.Focal, k.AFOV, k.TFOV, k.Power, k.SensorW, k.SensorH, k.Notes, uid, k.ID)
		if err == nil {
			if n, _ := res.RowsAffected(); n == 0 {
				return ErrNotFound
			}
		}
	}
	if err != nil {
		return err
	}
	got, err := s.GetKit(ctx, uid, k.ID)
	if err == nil {
		*k = *got
	}
	return err
}

// DeleteKit removes a piece of equipment.
func (s *Store) DeleteKit(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM kit WHERE user_id = ? AND id = ?`, uid, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- plans ----

// Plan is an observing plan.
type Plan struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	Night      string     `json:"night"`
	LocationID *int64     `json:"location_id"`
	Setup      string     `json:"setup"`
	Notes      string     `json:"notes"`
	CreatedAt  string     `json:"created_at"`
	UpdatedAt  string     `json:"updated_at"`
	Items      []PlanItem `json:"items"`
}

// PlanItem is one thing to look at in a plan.
type PlanItem struct {
	ID   int64  `json:"id"`
	Key  string `json:"key"`
	Name string `json:"name"`
	Note string `json:"note"`
	Done bool   `json:"done"`
}

const planCols = `id, name, night, location_id, setup, notes, created_at, updated_at`

func scanPlan(sc interface{ Scan(...any) error }) (*Plan, error) {
	var p Plan
	var loc sql.NullInt64
	if err := sc.Scan(&p.ID, &p.Name, &p.Night, &loc, &p.Setup, &p.Notes, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}
	if loc.Valid {
		v := loc.Int64
		p.LocationID = &v
	}
	p.Items = []PlanItem{}
	return &p, nil
}

// ListPlans is the person's plans, newest night first (without items).
func (s *Store) ListPlans(ctx context.Context, uid int64) ([]Plan, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+planCols+`, (SELECT COUNT(1) FROM plan_items i WHERE i.plan_id = plans.id) FROM plans WHERE user_id = ? ORDER BY night DESC, id DESC`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Plan{}
	for rows.Next() {
		var p Plan
		var loc sql.NullInt64
		var n int
		if err := rows.Scan(&p.ID, &p.Name, &p.Night, &loc, &p.Setup, &p.Notes, &p.CreatedAt, &p.UpdatedAt, &n); err != nil {
			return nil, err
		}
		if loc.Valid {
			v := loc.Int64
			p.LocationID = &v
		}
		p.Items = make([]PlanItem, n)
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetPlan is one plan of the person's with its items in order.
func (s *Store) GetPlan(ctx context.Context, uid, id int64) (*Plan, error) {
	p, err := scanPlan(s.DB.QueryRowContext(ctx, `SELECT `+planCols+` FROM plans WHERE user_id = ? AND id = ?`, uid, id))
	if err != nil {
		return nil, notFound(err)
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT id, object_key, name, note, done FROM plan_items WHERE user_id = ? AND plan_id = ? ORDER BY sort, id`, uid, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var it PlanItem
		var done int
		if err := rows.Scan(&it.ID, &it.Key, &it.Name, &it.Note, &done); err != nil {
			return nil, err
		}
		it.Done = done == 1
		p.Items = append(p.Items, it)
	}
	return p, rows.Err()
}

// SavePlan adds (ID 0) or changes a plan; items are replaced when given.
func (s *Store) SavePlan(ctx context.Context, uid int64, p *Plan, items []PlanItem) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if p.ID == 0 {
		res, err := tx.ExecContext(ctx, `INSERT INTO plans (user_id, name, night, location_id, setup, notes) VALUES (?, ?, ?, ?, ?, ?)`, uid, p.Name, p.Night, nullI(p.LocationID), p.Setup, p.Notes)
		if err != nil {
			return err
		}
		p.ID, _ = res.LastInsertId()
	} else {
		res, err := tx.ExecContext(ctx, `UPDATE plans SET name = ?, night = ?, location_id = ?, setup = ?, notes = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE user_id = ? AND id = ?`,
			p.Name, p.Night, nullI(p.LocationID), p.Setup, p.Notes, uid, p.ID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
	}
	if items != nil {
		if _, err := tx.ExecContext(ctx, `DELETE FROM plan_items WHERE user_id = ? AND plan_id = ?`, uid, p.ID); err != nil {
			return err
		}
		for i, it := range items {
			if _, err := tx.ExecContext(ctx, `INSERT INTO plan_items (user_id, plan_id, object_key, name, note, sort, done) VALUES (?, ?, ?, ?, ?, ?, ?)`, uid, p.ID, it.Key, it.Name, it.Note, i, boolInt(it.Done)); err != nil {
				return err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	got, err := s.GetPlan(ctx, uid, p.ID)
	if err == nil {
		*p = *got
	}
	return err
}

// DeletePlan removes a plan.
func (s *Store) DeletePlan(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM plans WHERE user_id = ? AND id = ?`, uid, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- photos ----

// Photo is a photo of the sky.
type Photo struct {
	ID            int64  `json:"id"`
	ObservationID *int64 `json:"observation_id"`
	Key           string `json:"key"`
	Caption       string `json:"caption"`
	File          string `json:"-"`
	Mime          string `json:"mime"`
	Size          int64  `json:"size"`
	Width         int    `json:"width"`
	Height        int    `json:"height"`
	Source        string `json:"source"`
	SourceRef     string `json:"source_ref"`
	Solve         string `json:"-"`
	CreatedAt     string `json:"created_at"`
}

const photoCols = `id, observation_id, object_key, caption, file, mime, size, width, height, source, source_ref, solve, created_at`

func scanPhoto(sc interface{ Scan(...any) error }) (*Photo, error) {
	var p Photo
	var obs sql.NullInt64
	if err := sc.Scan(&p.ID, &obs, &p.Key, &p.Caption, &p.File, &p.Mime, &p.Size, &p.Width, &p.Height, &p.Source, &p.SourceRef, &p.Solve, &p.CreatedAt); err != nil {
		return nil, err
	}
	if obs.Valid {
		v := obs.Int64
		p.ObservationID = &v
	}
	return &p, nil
}

// ListPhotos is the person's photos (of one night when obs > 0), newest first.
func (s *Store) ListPhotos(ctx context.Context, uid, obs int64) ([]Photo, error) {
	q := `SELECT ` + photoCols + ` FROM photos WHERE user_id = ?`
	args := []any{uid}
	if obs > 0 {
		q += ` AND observation_id = ?`
		args = append(args, obs)
	}
	rows, err := s.DB.QueryContext(ctx, q+` ORDER BY id DESC LIMIT 500`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Photo{}
	for rows.Next() {
		p, err := scanPhoto(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// GetPhoto is one photo of the person's.
func (s *Store) GetPhoto(ctx context.Context, uid, id int64) (*Photo, error) {
	p, err := scanPhoto(s.DB.QueryRowContext(ctx, `SELECT `+photoCols+` FROM photos WHERE user_id = ? AND id = ?`, uid, id))
	return p, notFound(err)
}

// AddPhoto records a photo already written to disk.
func (s *Store) AddPhoto(ctx context.Context, uid int64, p *Photo) error {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO photos (user_id, observation_id, object_key, caption, file, mime, size, width, height, source, source_ref) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		uid, nullI(p.ObservationID), p.Key, p.Caption, p.File, p.Mime, p.Size, p.Width, p.Height, p.Source, p.SourceRef)
	if err != nil {
		return err
	}
	p.ID, _ = res.LastInsertId()
	got, err := s.GetPhoto(ctx, uid, p.ID)
	if err == nil {
		*p = *got
	}
	return err
}

// UpdatePhoto saves the caption, the object, the night and the solve.
func (s *Store) UpdatePhoto(ctx context.Context, uid int64, p *Photo) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE photos SET observation_id = ?, object_key = ?, caption = ?, solve = ? WHERE user_id = ? AND id = ?`, nullI(p.ObservationID), p.Key, p.Caption, p.Solve, uid, p.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeletePhoto removes a photo's record (the caller removes the file).
func (s *Store) DeletePhoto(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM photos WHERE user_id = ? AND id = ?`, uid, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
