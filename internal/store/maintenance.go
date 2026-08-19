// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/kolapsis/maintenant/internal/status"
	"github.com/kolapsis/maintenant/internal/uid"
)

// MaintenanceStoreImpl implements status.MaintenanceStore using SQLite.
type MaintenanceStoreImpl struct {
	db     *sql.DB
	writer *Writer
}

// NewMaintenanceStore creates a new SQLite-backed maintenance store.
func NewMaintenanceStore(d *DB) *MaintenanceStoreImpl {
	return &MaintenanceStoreImpl{
		db:     d.ReadDB(),
		writer: d.Writer(),
	}
}

func (s *MaintenanceStoreImpl) ListMaintenance(ctx context.Context, statusFilter string, limit int) ([]status.MaintenanceWindow, error) {
	query := `SELECT mw.id, mw.title, mw.description, mw.starts_at, mw.ends_at,
		mw.active, mw.incident_id, mw.created_at, mw.updated_at
		FROM maintenance_windows mw`
	var args []interface{}

	now := time.Now().Unix()
	switch statusFilter {
	case "upcoming":
		query += ` WHERE mw.active = 0 AND mw.starts_at > ?`
		args = append(args, now)
	case "active":
		query += ` WHERE mw.active = 1`
	case "completed":
		query += ` WHERE mw.active = 0 AND mw.ends_at <= ?`
		args = append(args, now)
	}

	query += ` ORDER BY mw.starts_at DESC`

	if limit <= 0 || limit > 100 {
		limit = 20
	}
	query += ` LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list maintenance: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)
	return s.scanMaintenanceWindows(ctx, rows)
}

func (s *MaintenanceStoreImpl) GetMaintenance(ctx context.Context, id string) (*status.MaintenanceWindow, error) {
	var mw status.MaintenanceWindow
	var active int
	var incidentID sql.NullString
	var startsAt, endsAt, createdAt, updatedAt int64

	err := s.db.QueryRowContext(ctx,
		`SELECT id, title, description, starts_at, ends_at, active, incident_id, created_at, updated_at
		FROM maintenance_windows WHERE id = ?`, id,
	).Scan(&mw.ID, &mw.Title, &mw.Description, &startsAt, &endsAt,
		&active, &incidentID, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get maintenance: %w", err)
	}

	mw.Active = active != 0
	if incidentID.Valid {
		mw.IncidentID = &incidentID.String
	}
	mw.StartsAt = time.Unix(startsAt, 0).UTC()
	mw.EndsAt = time.Unix(endsAt, 0).UTC()
	mw.CreatedAt = time.Unix(createdAt, 0).UTC()
	mw.UpdatedAt = time.Unix(updatedAt, 0).UTC()

	if err := s.loadMaintenanceComponents(ctx, &mw); err != nil {
		return nil, err
	}
	return &mw, nil
}

func (s *MaintenanceStoreImpl) CreateMaintenance(ctx context.Context, mw *status.MaintenanceWindow, componentIDs []string) (string, error) {
	now := time.Now().Unix()
	mw.ID = uid.New()
	_, err := s.writer.Exec(ctx,
		`INSERT INTO maintenance_windows (id, title, description, starts_at, ends_at, active, incident_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 0, NULL, ?, ?)`,
		mw.ID, mw.Title, mw.Description, mw.StartsAt.Unix(), mw.EndsAt.Unix(), now, now,
	)
	if err != nil {
		return "", fmt.Errorf("create maintenance: %w", err)
	}
	mw.CreatedAt = time.Unix(now, 0).UTC()
	mw.UpdatedAt = mw.CreatedAt

	for _, cid := range componentIDs {
		if _, err := s.writer.Exec(ctx,
			`INSERT INTO maintenance_components (maintenance_id, component_id) VALUES (?, ?)`,
			mw.ID, cid,
		); err != nil {
			return "", fmt.Errorf("link maintenance component: %w", err)
		}
	}

	return mw.ID, nil
}

func (s *MaintenanceStoreImpl) UpdateMaintenance(ctx context.Context, mw *status.MaintenanceWindow, componentIDs []string) error {
	now := time.Now().Unix()
	_, err := s.writer.Exec(ctx,
		`UPDATE maintenance_windows SET title = ?, description = ?, starts_at = ?, ends_at = ?, updated_at = ?
		WHERE id = ?`,
		mw.Title, mw.Description, mw.StartsAt.Unix(), mw.EndsAt.Unix(), now, mw.ID,
	)
	if err != nil {
		return fmt.Errorf("update maintenance: %w", err)
	}
	mw.UpdatedAt = time.Unix(now, 0).UTC()

	if componentIDs != nil {
		if _, err := s.writer.Exec(ctx,
			`DELETE FROM maintenance_components WHERE maintenance_id = ?`, mw.ID,
		); err != nil {
			return fmt.Errorf("clear maintenance components: %w", err)
		}
		for _, cid := range componentIDs {
			if _, err := s.writer.Exec(ctx,
				`INSERT INTO maintenance_components (maintenance_id, component_id) VALUES (?, ?)`,
				mw.ID, cid,
			); err != nil {
				return fmt.Errorf("link maintenance component: %w", err)
			}
		}
	}

	return nil
}

func (s *MaintenanceStoreImpl) DeleteMaintenance(ctx context.Context, id string) error {
	_, err := s.writer.Exec(ctx, `DELETE FROM maintenance_windows WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete maintenance: %w", err)
	}
	return nil
}

func (s *MaintenanceStoreImpl) GetPendingActivation(ctx context.Context, now int64) ([]status.MaintenanceWindow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, title, description, starts_at, ends_at, active, incident_id, created_at, updated_at
		FROM maintenance_windows
		WHERE active = 0 AND starts_at <= ? AND ends_at > ?`, now, now)
	if err != nil {
		return nil, fmt.Errorf("pending activation: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)
	return s.scanMaintenanceWindows(ctx, rows)
}

func (s *MaintenanceStoreImpl) GetPendingDeactivation(ctx context.Context, now int64) ([]status.MaintenanceWindow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, title, description, starts_at, ends_at, active, incident_id, created_at, updated_at
		FROM maintenance_windows
		WHERE active = 1 AND ends_at <= ?`, now)
	if err != nil {
		return nil, fmt.Errorf("pending deactivation: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)
	return s.scanMaintenanceWindows(ctx, rows)
}

func (s *MaintenanceStoreImpl) SetActive(ctx context.Context, id string, active bool, incidentID *string) error {
	now := time.Now().Unix()
	_, err := s.writer.Exec(ctx,
		`UPDATE maintenance_windows SET active = ?, incident_id = ?, updated_at = ? WHERE id = ?`,
		boolToInt(active), incidentID, now, id,
	)
	if err != nil {
		return fmt.Errorf("set active: %w", err)
	}
	return nil
}

// --- Scan helpers ---

func (s *MaintenanceStoreImpl) scanMaintenanceWindows(ctx context.Context, rows *sql.Rows) ([]status.MaintenanceWindow, error) {
	var windows []status.MaintenanceWindow
	for rows.Next() {
		var mw status.MaintenanceWindow
		var active int
		var incidentID sql.NullString
		var startsAt, endsAt, createdAt, updatedAt int64

		if err := rows.Scan(&mw.ID, &mw.Title, &mw.Description,
			&startsAt, &endsAt, &active, &incidentID, &createdAt, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan maintenance: %w", err)
		}

		mw.Active = active != 0
		if incidentID.Valid {
			mw.IncidentID = &incidentID.String
		}
		mw.StartsAt = time.Unix(startsAt, 0).UTC()
		mw.EndsAt = time.Unix(endsAt, 0).UTC()
		mw.CreatedAt = time.Unix(createdAt, 0).UTC()
		mw.UpdatedAt = time.Unix(updatedAt, 0).UTC()

		if err := s.loadMaintenanceComponents(ctx, &mw); err != nil {
			return nil, err
		}
		windows = append(windows, mw)
	}
	return windows, rows.Err()
}

func (s *MaintenanceStoreImpl) loadMaintenanceComponents(ctx context.Context, mw *status.MaintenanceWindow) error {
	rows, err := s.db.QueryContext(ctx,
		`SELECT sc.id, sc.display_name FROM status_components sc
		JOIN maintenance_components mc ON mc.component_id = sc.id
		WHERE mc.maintenance_id = ?`, mw.ID)
	if err != nil {
		return fmt.Errorf("load maintenance components: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)
	for rows.Next() {
		var ref status.IncidentCompRef
		if err := rows.Scan(&ref.ID, &ref.Name); err != nil {
			return fmt.Errorf("scan maintenance component: %w", err)
		}
		mw.Components = append(mw.Components, ref)
	}
	return rows.Err()
}

// IsEntitySuppressed returns true (with the matching window ID and end time) if at
// least one active maintenance window currently covers the given monitor. Active
// means starts_at ≤ now < ends_at regardless of the window's `active` flag (which
// controls Status Page display, not suppressor logic).
func (s *MaintenanceStoreImpl) IsEntitySuppressed(
	ctx context.Context, monitorType string, monitorID string, now time.Time,
) (matched bool, windowID string, endsAt time.Time, err error) {
	const q = `
SELECT mw.id, mw.ends_at
FROM maintenance_windows mw
JOIN maintenance_components mc ON mc.maintenance_id = mw.id
JOIN status_components sc ON sc.id = mc.component_id
LEFT JOIN status_component_monitors scm
    ON scm.component_id = sc.id
   AND scm.monitor_type = ?
   AND scm.monitor_id   = ?
WHERE mw.starts_at <= ?
  AND mw.ends_at   >  ?
  AND (
    (sc.composition_mode = 'match-all' AND sc.match_all_type = ?)
    OR
    (sc.composition_mode = 'explicit'  AND scm.component_id IS NOT NULL)
  )
LIMIT 1`

	nowUnix := now.Unix()
	var endsAtUnix int64
	scanErr := s.db.QueryRowContext(ctx, q,
		monitorType, monitorID,
		nowUnix, nowUnix,
		monitorType,
	).Scan(&windowID, &endsAtUnix)

	if errors.Is(scanErr, sql.ErrNoRows) {
		return false, "", time.Time{}, nil
	}
	if scanErr != nil {
		return false, "", time.Time{}, fmt.Errorf("maintenance: scan suppressed entity: %w", scanErr)
	}
	return true, windowID, time.Unix(endsAtUnix, 0), nil
}
