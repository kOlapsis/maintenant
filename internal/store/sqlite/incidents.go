// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See LICENSE-COMMERCIAL.md
//
// Source: https://github.com/kolapsis/maintenant

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/kolapsis/maintenant/internal/status"
)

// IncidentStoreImpl implements status.IncidentStore using SQLite.
type IncidentStoreImpl struct {
	db     *sql.DB
	writer *Writer
}

// NewIncidentStore creates a new SQLite-backed incident store.
func NewIncidentStore(d *DB) *IncidentStoreImpl {
	return &IncidentStoreImpl{
		db:     d.ReadDB(),
		writer: d.Writer(),
	}
}

func (s *IncidentStoreImpl) ListIncidents(ctx context.Context, opts status.ListIncidentsOpts) ([]status.Incident, int, error) {
	countQuery := `SELECT COUNT(*) FROM incidents WHERE 1=1`
	query := `SELECT id, title, severity, status, is_maintenance, maintenance_window_id,
		created_at, resolved_at, updated_at FROM incidents WHERE 1=1`
	var args []interface{}

	if opts.Status != "" {
		query += ` AND status = ?`
		countQuery += ` AND status = ?`
		args = append(args, opts.Status)
	}
	if opts.Severity != "" {
		query += ` AND severity = ?`
		countQuery += ` AND severity = ?`
		args = append(args, opts.Severity)
	}

	var total int
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count incidents: %w", err)
	}

	query += ` ORDER BY updated_at DESC`

	limit := opts.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := opts.Offset
	if offset < 0 {
		offset = 0
	}
	query += ` LIMIT ? OFFSET ?`
	queryArgs := append(args, limit, offset)

	rows, err := s.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list incidents: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)

	incidents, err := s.scanIncidents(ctx, rows)
	if err != nil {
		return nil, 0, err
	}
	return incidents, total, nil
}

func (s *IncidentStoreImpl) ListActiveIncidents(ctx context.Context) ([]status.Incident, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, title, severity, status, is_maintenance, maintenance_window_id,
			created_at, resolved_at, updated_at
		FROM incidents WHERE status != 'resolved'
		ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list active incidents: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)
	return s.scanIncidents(ctx, rows)
}

func (s *IncidentStoreImpl) ListRecentIncidents(ctx context.Context, days int) ([]status.Incident, error) {
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour).Unix()
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, title, severity, status, is_maintenance, maintenance_window_id,
			created_at, resolved_at, updated_at
		FROM incidents WHERE status = 'resolved' AND resolved_at >= ?
		ORDER BY resolved_at DESC`, cutoff)
	if err != nil {
		return nil, fmt.Errorf("list recent incidents: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)
	return s.scanIncidents(ctx, rows)
}

func (s *IncidentStoreImpl) GetIncident(ctx context.Context, id int64) (*status.Incident, error) {
	var inc status.Incident
	var isMaint, createdAt, updatedAt int64
	var resolvedAt sql.NullInt64
	var maintID sql.NullInt64

	err := s.db.QueryRowContext(ctx,
		`SELECT id, title, severity, status, is_maintenance, maintenance_window_id,
			created_at, resolved_at, updated_at
		FROM incidents WHERE id = ?`, id,
	).Scan(&inc.ID, &inc.Title, &inc.Severity, &inc.Status, &isMaint, &maintID,
		&createdAt, &resolvedAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get incident: %w", err)
	}

	inc.IsMaintenance = isMaint != 0
	if maintID.Valid {
		inc.MaintenanceWindowID = &maintID.Int64
	}
	inc.CreatedAt = time.Unix(createdAt, 0).UTC()
	if resolvedAt.Valid {
		t := time.Unix(resolvedAt.Int64, 0).UTC()
		inc.ResolvedAt = &t
	}
	inc.UpdatedAt = time.Unix(updatedAt, 0).UTC()

	if err := s.loadIncidentRelations(ctx, &inc); err != nil {
		return nil, err
	}
	return &inc, nil
}

func (s *IncidentStoreImpl) GetActiveIncidentByComponent(ctx context.Context, componentID int64) (*status.Incident, error) {
	var incID int64
	err := s.db.QueryRowContext(ctx,
		`SELECT i.id FROM incidents i
		JOIN incident_components ic ON ic.incident_id = i.id
		WHERE ic.component_id = ? AND i.status != 'resolved'
		ORDER BY i.created_at DESC LIMIT 1`, componentID,
	).Scan(&incID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get active incident by component: %w", err)
	}
	return s.GetIncident(ctx, incID)
}

func (s *IncidentStoreImpl) CreateIncident(ctx context.Context, inc *status.Incident, componentIDs []int64, initialMessage string) (int64, error) {
	now := time.Now().Unix()

	res, err := s.writer.Exec(ctx,
		`INSERT INTO incidents (title, severity, status, is_maintenance, maintenance_window_id, created_at, resolved_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		inc.Title, inc.Severity, inc.Status, boolToInt(inc.IsMaintenance), inc.MaintenanceWindowID,
		now, nil, now,
	)
	if err != nil {
		return 0, fmt.Errorf("create incident: %w", err)
	}
	inc.ID = res.LastInsertID
	inc.CreatedAt = time.Unix(now, 0).UTC()
	inc.UpdatedAt = inc.CreatedAt

	for _, cid := range componentIDs {
		if _, err := s.writer.Exec(ctx,
			`INSERT INTO incident_components (incident_id, component_id) VALUES (?, ?)`,
			inc.ID, cid,
		); err != nil {
			return 0, fmt.Errorf("link incident component: %w", err)
		}
	}

	if initialMessage != "" {
		if _, err := s.writer.Exec(ctx,
			`INSERT INTO incident_updates (incident_id, status, message, is_auto, created_at)
			VALUES (?, ?, ?, 0, ?)`,
			inc.ID, inc.Status, initialMessage, now,
		); err != nil {
			return 0, fmt.Errorf("create initial update: %w", err)
		}
	}

	return inc.ID, nil
}

func (s *IncidentStoreImpl) UpdateIncident(ctx context.Context, inc *status.Incident, componentIDs []int64) error {
	now := time.Now().Unix()
	_, err := s.writer.Exec(ctx,
		`UPDATE incidents SET title = ?, severity = ?, updated_at = ? WHERE id = ?`,
		inc.Title, inc.Severity, now, inc.ID,
	)
	if err != nil {
		return fmt.Errorf("update incident: %w", err)
	}
	inc.UpdatedAt = time.Unix(now, 0).UTC()

	if componentIDs != nil {
		if _, err := s.writer.Exec(ctx,
			`DELETE FROM incident_components WHERE incident_id = ?`, inc.ID,
		); err != nil {
			return fmt.Errorf("clear incident components: %w", err)
		}
		for _, cid := range componentIDs {
			if _, err := s.writer.Exec(ctx,
				`INSERT INTO incident_components (incident_id, component_id) VALUES (?, ?)`,
				inc.ID, cid,
			); err != nil {
				return fmt.Errorf("link incident component: %w", err)
			}
		}
	}

	return nil
}

func (s *IncidentStoreImpl) DeleteIncident(ctx context.Context, id int64) error {
	_, err := s.writer.Exec(ctx, `DELETE FROM incidents WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete incident: %w", err)
	}
	return nil
}

// --- Incident Updates ---

func (s *IncidentStoreImpl) ListUpdates(ctx context.Context, incidentID int64) ([]status.IncidentUpdate, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, incident_id, status, message, is_auto, alert_id, created_at
		FROM incident_updates WHERE incident_id = ?
		ORDER BY created_at DESC`, incidentID)
	if err != nil {
		return nil, fmt.Errorf("list updates: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)

	var updates []status.IncidentUpdate
	for rows.Next() {
		u, err := scanIncidentUpdate(rows)
		if err != nil {
			return nil, err
		}
		updates = append(updates, u)
	}
	return updates, rows.Err()
}

func (s *IncidentStoreImpl) CreateUpdate(ctx context.Context, u *status.IncidentUpdate) (int64, error) {
	now := time.Now().Unix()

	res, err := s.writer.Exec(ctx,
		`INSERT INTO incident_updates (incident_id, status, message, is_auto, alert_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		u.IncidentID, u.Status, u.Message, boolToInt(u.IsAuto), u.AlertID, now,
	)
	if err != nil {
		return 0, fmt.Errorf("create update: %w", err)
	}
	u.ID = res.LastInsertID
	u.CreatedAt = time.Unix(now, 0).UTC()

	// Update parent incident status and timestamp
	resolvedAt := sql.NullInt64{}
	if u.Status == status.IncidentResolved {
		resolvedAt = sql.NullInt64{Int64: now, Valid: true}
	}
	if _, err := s.writer.Exec(ctx,
		`UPDATE incidents SET status = ?, resolved_at = COALESCE(?, resolved_at), updated_at = ? WHERE id = ?`,
		u.Status, resolvedAt, now, u.IncidentID,
	); err != nil {
		return 0, fmt.Errorf("update incident status: %w", err)
	}

	return u.ID, nil
}

func (s *IncidentStoreImpl) DeleteIncidentsOlderThan(ctx context.Context, days int) (int64, error) {
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour).Unix()
	res, err := s.writer.Exec(ctx,
		`DELETE FROM incidents WHERE status = 'resolved' AND resolved_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("delete old incidents: %w", err)
	}
	return res.RowsAffected, nil
}

// --- Scan helpers ---

func (s *IncidentStoreImpl) scanIncidents(ctx context.Context, rows *sql.Rows) ([]status.Incident, error) {
	var incidents []status.Incident
	for rows.Next() {
		var inc status.Incident
		var isMaint, createdAt, updatedAt int64
		var resolvedAt sql.NullInt64
		var maintID sql.NullInt64

		if err := rows.Scan(&inc.ID, &inc.Title, &inc.Severity, &inc.Status,
			&isMaint, &maintID, &createdAt, &resolvedAt, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan incident: %w", err)
		}

		inc.IsMaintenance = isMaint != 0
		if maintID.Valid {
			inc.MaintenanceWindowID = &maintID.Int64
		}
		inc.CreatedAt = time.Unix(createdAt, 0).UTC()
		if resolvedAt.Valid {
			t := time.Unix(resolvedAt.Int64, 0).UTC()
			inc.ResolvedAt = &t
		}
		inc.UpdatedAt = time.Unix(updatedAt, 0).UTC()

		if err := s.loadIncidentRelations(ctx, &inc); err != nil {
			return nil, err
		}
		incidents = append(incidents, inc)
	}
	return incidents, rows.Err()
}

func (s *IncidentStoreImpl) loadIncidentRelations(ctx context.Context, inc *status.Incident) error {
	// Load components
	compRows, err := s.db.QueryContext(ctx,
		`SELECT sc.id, sc.display_name FROM status_components sc
		JOIN incident_components ic ON ic.component_id = sc.id
		WHERE ic.incident_id = ?`, inc.ID)
	if err != nil {
		return fmt.Errorf("load incident components: %w", err)
	}
	defer func(compRows *sql.Rows) {
		_ = compRows.Close()
	}(compRows)
	for compRows.Next() {
		var ref status.IncidentCompRef
		if err := compRows.Scan(&ref.ID, &ref.Name); err != nil {
			return fmt.Errorf("scan incident component: %w", err)
		}
		inc.Components = append(inc.Components, ref)
	}
	if err := compRows.Err(); err != nil {
		return err
	}

	// Load updates
	updRows, err := s.db.QueryContext(ctx,
		`SELECT id, incident_id, status, message, is_auto, alert_id, created_at
		FROM incident_updates WHERE incident_id = ?
		ORDER BY created_at DESC`, inc.ID)
	if err != nil {
		return fmt.Errorf("load incident updates: %w", err)
	}
	defer func(updRows *sql.Rows) {
		_ = updRows.Close()
	}(updRows)
	for updRows.Next() {
		u, err := scanIncidentUpdate(updRows)
		if err != nil {
			return err
		}
		inc.Updates = append(inc.Updates, u)
	}
	return updRows.Err()
}

func scanIncidentUpdate(rows *sql.Rows) (status.IncidentUpdate, error) {
	var u status.IncidentUpdate
	var isAuto int
	var alertID sql.NullInt64
	var createdAt int64

	if err := rows.Scan(&u.ID, &u.IncidentID, &u.Status, &u.Message,
		&isAuto, &alertID, &createdAt,
	); err != nil {
		return u, fmt.Errorf("scan update: %w", err)
	}

	u.IsAuto = isAuto != 0
	if alertID.Valid {
		u.AlertID = &alertID.Int64
	}
	u.CreatedAt = time.Unix(createdAt, 0).UTC()
	return u, nil
}
