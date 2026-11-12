package store

import (
	"context"
	"database/sql"
	"strings"
)

const locationCols = `id, user_id, name, region, country, country_code, lat, lon, elevation, timezone, is_home, alerts, bortle, sort, note, source, source_ref, created_at, updated_at`

func scanLocation(sc interface{ Scan(...any) error }) (*Location, error) {
	var l Location
	var elev sql.NullFloat64
	var home, alerts int
	if err := sc.Scan(&l.ID, &l.UserID, &l.Name, &l.Region, &l.Country, &l.CountryCode, &l.Lat, &l.Lon, &elev, &l.Timezone, &home, &alerts, &l.Bortle, &l.Sort, &l.Note,
		&l.Source, &l.SourceRef, &l.CreatedAt, &l.UpdatedAt); err != nil {
		return nil, err
	}
	if elev.Valid {
		v := elev.Float64
		l.Elevation = &v
	}
	l.IsHome, l.Alerts = home == 1, alerts == 1
	return &l, nil
}

// ListLocations returns one person's places, home first, then in their order.
func (s *Store) ListLocations(ctx context.Context, uid int64) ([]Location, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+locationCols+` FROM locations WHERE user_id = ? ORDER BY is_home DESC, source = 'here' DESC, sort, name COLLATE NOCASE`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Location{}
	for rows.Next() {
		l, err := scanLocation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *l)
	}
	return out, rows.Err()
}

// GetLocation returns one of the person's places (ErrNotFound for anyone else's).
func (s *Store) GetLocation(ctx context.Context, uid, id int64) (*Location, error) {
	l, err := scanLocation(s.DB.QueryRowContext(ctx, `SELECT `+locationCols+` FROM locations WHERE user_id = ? AND id = ?`, uid, id))
	return l, notFound(err)
}

// HomeLocation is the person's home place (else their first one).
func (s *Store) HomeLocation(ctx context.Context, uid int64) (*Location, error) {
	l, err := scanLocation(s.DB.QueryRowContext(ctx, `SELECT `+locationCols+` FROM locations WHERE user_id = ? AND source <> 'here' ORDER BY is_home DESC, sort, id LIMIT 1`, uid))
	if err == nil {
		return l, nil
	}
	l, err = scanLocation(s.DB.QueryRowContext(ctx, `SELECT `+locationCols+` FROM locations WHERE user_id = ? ORDER BY id LIMIT 1`, uid))
	return l, notFound(err)
}

// LocationBySource finds a place another app (or "here") added.
func (s *Store) LocationBySource(ctx context.Context, uid int64, source, ref string) (*Location, error) {
	l, err := scanLocation(s.DB.QueryRowContext(ctx, `SELECT `+locationCols+` FROM locations WHERE user_id = ? AND source = ? AND source_ref = ?`, uid, source, ref))
	return l, notFound(err)
}

// CreateLocation adds a place; the first one becomes home.
func (s *Store) CreateLocation(ctx context.Context, l *Location) error {
	l.Name = strings.TrimSpace(l.Name)
	var n int
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM locations WHERE user_id = ? AND source <> 'here'`, l.UserID).Scan(&n)
	if n == 0 && l.Source != "here" {
		l.IsHome = true
	}
	var maxSort int
	_ = s.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(sort), 0) FROM locations WHERE user_id = ?`, l.UserID).Scan(&maxSort)
	if l.Sort == 0 {
		l.Sort = maxSort + 1
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if l.IsHome {
		if _, err := tx.ExecContext(ctx, `UPDATE locations SET is_home = 0 WHERE user_id = ?`, l.UserID); err != nil {
			return err
		}
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO locations (user_id, name, region, country, country_code, lat, lon, elevation, timezone, is_home, alerts, bortle, sort, note, source, source_ref)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		l.UserID, l.Name, l.Region, l.Country, l.CountryCode, l.Lat, l.Lon, nullF(l.Elevation), l.Timezone, boolInt(l.IsHome), boolInt(l.Alerts), l.Bortle, l.Sort, l.Note, l.Source, l.SourceRef)
	if err != nil {
		return conflict(err)
	}
	l.ID, _ = res.LastInsertId()
	if err := tx.Commit(); err != nil {
		return err
	}
	got, err := s.GetLocation(ctx, l.UserID, l.ID)
	if err == nil {
		*l = *got
	}
	return err
}

// UpdateLocation saves a place; making it home takes home from the others.
func (s *Store) UpdateLocation(ctx context.Context, l *Location) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if l.IsHome {
		if _, err := tx.ExecContext(ctx, `UPDATE locations SET is_home = 0 WHERE user_id = ? AND id <> ?`, l.UserID, l.ID); err != nil {
			return err
		}
	}
	res, err := tx.ExecContext(ctx, `UPDATE locations SET name = ?, region = ?, country = ?, country_code = ?, lat = ?, lon = ?, elevation = ?, timezone = ?, is_home = ?, alerts = ?, bortle = ?, sort = ?, note = ?,
		updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE user_id = ? AND id = ?`,
		strings.TrimSpace(l.Name), l.Region, l.Country, l.CountryCode, l.Lat, l.Lon, nullF(l.Elevation), l.Timezone, boolInt(l.IsHome), boolInt(l.Alerts), l.Bortle, l.Sort, l.Note, l.UserID, l.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

// DeleteLocation removes a place (log entries keep its name); when it was
// home, the next place becomes home.
func (s *Store) DeleteLocation(ctx context.Context, uid, id int64) error {
	l, err := s.GetLocation(ctx, uid, id)
	if err != nil {
		return err
	}
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM locations WHERE user_id = ? AND id = ?`, uid, id); err != nil {
		return err
	}
	if l.IsHome {
		_, _ = s.DB.ExecContext(ctx, `UPDATE locations SET is_home = 1 WHERE id = (SELECT id FROM locations WHERE user_id = ? AND source <> 'here' ORDER BY sort, id LIMIT 1)`, uid)
	}
	return nil
}

// ReorderLocations sets the order of the person's places (ids not theirs are skipped).
func (s *Store) ReorderLocations(ctx context.Context, uid int64, ids []int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx, `UPDATE locations SET sort = ? WHERE user_id = ? AND id = ?`, i+1, uid, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// LocationUsers lists everyone who follows at least one place.
func (s *Store) LocationUsers(ctx context.Context) ([]int64, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT DISTINCT user_id FROM locations ORDER BY user_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func nullF(f *float64) any {
	if f == nil {
		return nil
	}
	return *f
}

func optF(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	f := v.Float64
	return &f
}

// CachePurge drops cached entries with the prefix fetched before a time (RFC 3339).
func (s *Store) CachePurge(ctx context.Context, prefix, before string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM cache WHERE key LIKE ? || '%' AND fetched_at < ?`, prefix, before)
	return err
}
