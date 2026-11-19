// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kolapsis/maintenant/internal/status"
	"github.com/kolapsis/maintenant/internal/uid"
)

// IncidentStoreImpl implements status.IncidentStore using SQLite.
type IncidentStoreImpl struct {
	db     *Reader
	writer *Writer
}

// NewIncidentStore creates a new SQLite-backed incident store.
func NewIncidentStore(d *DB) *IncidentStoreImpl {
	return &IncidentStoreImpl{
		db:     d.Reader(),
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

func (s *IncidentStoreImpl) GetIncident(ctx context.Context, id string) (*status.Incident, error) {
	var inc status.Incident
	var isMaint, createdAt, updatedAt int64
	var resolvedAt sql.NullInt64
	var maintID sql.NullString

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
		inc.MaintenanceWindowID = &maintID.String
	}
	inc.CreatedAt = time.Unix(createdAt, 0).UTC()
	if resolvedAt.Valid {
		t := time.Unix(resolvedAt.Int64, 0).UTC()
		inc.ResolvedAt = &t
	}
	inc.UpdatedAt = time.Unix(updatedAt, 0).UTC()

	incs := []status.Incident{inc}
	if err := s.hydrateIncidents(ctx, incs); err != nil {
		return nil, err
	}
	return &incs[0], nil
}

func (s *IncidentStoreImpl) GetActiveIncidentByComponent(ctx context.Context, componentID string) (*status.Incident, error) {
	var incID string
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

func (s *IncidentStoreImpl) CreateIncident(ctx context.Context, inc *status.Incident, componentIDs []string, initialMessage string) (string, error) {
	now := time.Now().Unix()

	inc.ID = uid.New()
	err := s.writer.Tx(ctx, func(ctx context.Context, tx *Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO incidents (id, title, severity, status, is_maintenance, maintenance_window_id, created_at, resolved_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			inc.ID, inc.Title, inc.Severity, inc.Status, boolToInt(inc.IsMaintenance), inc.MaintenanceWindowID,
			now, nil, now,
		); err != nil {
			return fmt.Errorf("create incident: %w", err)
		}
		for _, cid := range componentIDs {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO incident_components (incident_id, component_id) VALUES (?, ?)`,
				inc.ID, cid,
			); err != nil {
				return fmt.Errorf("link incident component: %w", err)
			}
		}
		if initialMessage != "" {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO incident_updates (id, incident_id, status, message, is_auto, created_at)
				VALUES (?, ?, ?, ?, 0, ?)`,
				uid.New(), inc.ID, inc.Status, initialMessage, now,
			); err != nil {
				return fmt.Errorf("create initial update: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	inc.CreatedAt = time.Unix(now, 0).UTC()
	inc.UpdatedAt = inc.CreatedAt
	return inc.ID, nil
}

func (s *IncidentStoreImpl) UpdateIncident(ctx context.Context, inc *status.Incident, componentIDs []string) error {
	now := time.Now().Unix()
	err := s.writer.Tx(ctx, func(ctx context.Context, tx *Tx) error {
		if _, err := tx.ExecContext(ctx,
			`UPDATE incidents SET title = ?, severity = ?, updated_at = ? WHERE id = ?`,
			inc.Title, inc.Severity, now, inc.ID,
		); err != nil {
			return fmt.Errorf("update incident: %w", err)
		}
		if componentIDs == nil {
			return nil
		}
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM incident_components WHERE incident_id = ?`, inc.ID,
		); err != nil {
			return fmt.Errorf("clear incident components: %w", err)
		}
		for _, cid := range componentIDs {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO incident_components (incident_id, component_id) VALUES (?, ?)`,
				inc.ID, cid,
			); err != nil {
				return fmt.Errorf("link incident component: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	inc.UpdatedAt = time.Unix(now, 0).UTC()
	return nil
}

func (s *IncidentStoreImpl) DeleteIncident(ctx context.Context, id string) error {
	_, err := s.writer.Exec(ctx, `DELETE FROM incidents WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete incident: %w", err)
	}
	return nil
}

// --- Incident Updates ---

func (s *IncidentStoreImpl) ListUpdates(ctx context.Context, incidentID string) ([]status.IncidentUpdate, error) {
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

func (s *IncidentStoreImpl) CreateUpdate(ctx context.Context, u *status.IncidentUpdate) (string, error) {
	now := time.Now().Unix()

	u.ID = uid.New()
	resolvedAt := sql.NullInt64{}
	if u.Status == status.IncidentResolved {
		resolvedAt = sql.NullInt64{Int64: now, Valid: true}
	}
	err := s.writer.Tx(ctx, func(ctx context.Context, tx *Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO incident_updates (id, incident_id, status, message, is_auto, alert_id, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			u.ID, u.IncidentID, u.Status, u.Message, boolToInt(u.IsAuto), u.AlertID, now,
		); err != nil {
			return fmt.Errorf("create update: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE incidents SET status = ?, resolved_at = COALESCE(?, resolved_at), updated_at = ? WHERE id = ?`,
			u.Status, resolvedAt, now, u.IncidentID,
		); err != nil {
			return fmt.Errorf("update incident status: %w", err)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	u.CreatedAt = time.Unix(now, 0).UTC()
	return u.ID, nil
}

// --- Scan helpers ---

func (s *IncidentStoreImpl) scanIncidents(ctx context.Context, rows *sql.Rows) ([]status.Incident, error) {
	var incidents []status.Incident
	for rows.Next() {
		var inc status.Incident
		var isMaint, createdAt, updatedAt int64
		var resolvedAt sql.NullInt64
		var maintID sql.NullString

		if err := rows.Scan(&inc.ID, &inc.Title, &inc.Severity, &inc.Status,
			&isMaint, &maintID, &createdAt, &resolvedAt, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan incident: %w", err)
		}

		inc.IsMaintenance = isMaint != 0
		if maintID.Valid {
			inc.MaintenanceWindowID = &maintID.String
		}
		inc.CreatedAt = time.Unix(createdAt, 0).UTC()
		if resolvedAt.Valid {
			t := time.Unix(resolvedAt.Int64, 0).UTC()
			inc.ResolvedAt = &t
		}
		inc.UpdatedAt = time.Unix(updatedAt, 0).UTC()
		incidents = append(incidents, inc)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := s.hydrateIncidents(ctx, incidents); err != nil {
		return nil, err
	}
	return incidents, nil
}

// hydrateIncidents loads the components and updates of a batch of incidents in
// place, one query per relation. The caller must have closed its own cursor:
// the pool is small and shared with the writer.
func (s *IncidentStoreImpl) hydrateIncidents(ctx context.Context, incs []status.Incident) error {
	if len(incs) == 0 {
		return nil
	}
	idx := make(map[string]int, len(incs))
	args := make([]any, len(incs))
	for i := range incs {
		idx[incs[i].ID] = i
		args[i] = incs[i].ID
	}
	in := placeholderList(len(args))

	// #nosec G202 -- only generated "?" placeholders are concatenated; ids are bound.
	compRows, err := s.db.QueryContext(ctx,
		`SELECT ic.incident_id, sc.id, sc.display_name, sc.visible FROM status_components sc
		JOIN incident_components ic ON ic.component_id = sc.id
		WHERE ic.incident_id IN (`+in+`)`, args...)
	if err != nil {
		return fmt.Errorf("load incident components: %w", err)
	}
	defer func(compRows *sql.Rows) {
		_ = compRows.Close()
	}(compRows)
	for compRows.Next() {
		var incID string
		var ref status.IncidentCompRef
		var visible int
		if err := compRows.Scan(&incID, &ref.ID, &ref.Name, &visible); err != nil {
			return fmt.Errorf("scan incident component: %w", err)
		}
		ref.Visible = visible != 0
		incs[idx[incID]].Components = append(incs[idx[incID]].Components, ref)
	}
	if err := compRows.Err(); err != nil {
		return err
	}
	if err := compRows.Close(); err != nil {
		return err
	}

	// #nosec G202 -- only generated "?" placeholders are concatenated; ids are bound.
	updRows, err := s.db.QueryContext(ctx,
		`SELECT id, incident_id, status, message, is_auto, alert_id, created_at
		FROM incident_updates WHERE incident_id IN (`+in+`)
		ORDER BY created_at DESC`, args...)
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
		incs[idx[u.IncidentID]].Updates = append(incs[idx[u.IncidentID]].Updates, u)
	}
	return updRows.Err()
}

// placeholderList returns n comma-separated "?" placeholders.
func placeholderList(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

func scanIncidentUpdate(rows *sql.Rows) (status.IncidentUpdate, error) {
	var u status.IncidentUpdate
	var isAuto int
	var alertID sql.NullString
	var createdAt int64

	if err := rows.Scan(&u.ID, &u.IncidentID, &u.Status, &u.Message,
		&isAuto, &alertID, &createdAt,
	); err != nil {
		return u, fmt.Errorf("scan update: %w", err)
	}

	u.IsAuto = isAuto != 0
	if alertID.Valid {
		u.AlertID = &alertID.String
	}
	u.CreatedAt = time.Unix(createdAt, 0).UTC()
	return u, nil
}
