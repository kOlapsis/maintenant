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

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/uid"
)

// AlertStoreImpl implements alert.AlertStore using SQLite.
type AlertStoreImpl struct {
	db     *sql.DB
	writer *Writer
}

// NewAlertStore creates a new SQLite-backed alert store.
func NewAlertStore(d *DB) *AlertStoreImpl {
	return &AlertStoreImpl{
		db:     d.ReadDB(),
		writer: d.Writer(),
	}
}

const alertColumns = `id, source, alert_type, severity, status, message,
	entity_type, entity_id, entity_name, details,
	resolved_by_id, fired_at, resolved_at,
	acknowledged_at, acknowledged_by, escalated_at, created_at`

func (s *AlertStoreImpl) InsertAlert(ctx context.Context, a *alert.Alert) (string, error) {
	a.ID = uid.New()
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}
	_, err := s.writer.Exec(ctx,
		`INSERT INTO alerts (id, source, alert_type, severity, status, message,
			entity_type, entity_id, entity_name, details,
			resolved_by_id, fired_at, resolved_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.Source, a.AlertType, a.Severity, a.Status, a.Message,
		a.EntityType, a.EntityID, a.EntityName, a.Details,
		nullableStrPtr(a.ResolvedByID), a.FiredAt.Unix(), nullableTime(a.ResolvedAt), a.CreatedAt.Unix(),
	)
	if err != nil {
		return "", fmt.Errorf("insert alert: %w", err)
	}
	return a.ID, nil
}

func (s *AlertStoreImpl) GetAlert(ctx context.Context, id string) (*alert.Alert, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+alertColumns+` FROM alerts WHERE id = ?`, id)
	return scanAlertFromRow(row)
}

func (s *AlertStoreImpl) ListAlerts(ctx context.Context, opts alert.ListAlertsOpts) ([]*alert.Alert, error) {
	query := `SELECT ` + alertColumns + ` FROM alerts WHERE 1=1`
	var args []interface{}

	if opts.Source != "" {
		query += ` AND source = ?`
		args = append(args, opts.Source)
	}
	if opts.Severity != "" {
		query += ` AND severity = ?`
		args = append(args, opts.Severity)
	}
	if opts.Status != "" {
		query += ` AND status = ?`
		args = append(args, opts.Status)
	}
	if opts.Before != nil {
		query += ` AND fired_at < ?`
		args = append(args, opts.Before.Unix())
	}

	query += ` ORDER BY fired_at DESC`

	limit := opts.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	query += ` LIMIT ?`
	args = append(args, limit+1) // fetch one extra to determine has_more

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list alerts: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)

	var alerts []*alert.Alert
	for rows.Next() {
		a, err := scanAlertFromRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan alert: %w", err)
		}
		alerts = append(alerts, a)
	}
	return alerts, rows.Err()
}

func (s *AlertStoreImpl) UpdateAlertStatus(ctx context.Context, id string, status string, resolvedAt *time.Time, resolvedByID *string) error {
	_, err := s.writer.Exec(ctx,
		`UPDATE alerts SET status = ?, resolved_at = ?, resolved_by_id = ? WHERE id = ?`,
		status, nullableTime(resolvedAt), nullableStrPtr(resolvedByID), id,
	)
	if err != nil {
		return fmt.Errorf("update alert status: %w", err)
	}
	return nil
}

func (s *AlertStoreImpl) UpdateAlertSeverity(ctx context.Context, id string, severity, message string) error {
	_, err := s.writer.Exec(ctx,
		`UPDATE alerts SET severity = ?, message = ? WHERE id = ?`,
		severity, message, id,
	)
	if err != nil {
		return fmt.Errorf("update alert severity: %w", err)
	}
	return nil
}

func (s *AlertStoreImpl) GetActiveAlert(ctx context.Context, source, alertType, entityType string, entityID string) (*alert.Alert, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+alertColumns+` FROM alerts
		WHERE source = ? AND alert_type = ? AND entity_type = ? AND entity_id = ? AND status = 'active'
		LIMIT 1`,
		source, alertType, entityType, entityID)
	a, err := scanAlertFromRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return a, err
}

func (s *AlertStoreImpl) ListActiveAlerts(ctx context.Context) ([]*alert.Alert, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+alertColumns+` FROM alerts WHERE status = 'active' ORDER BY fired_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list active alerts: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)

	var alerts []*alert.Alert
	for rows.Next() {
		a, err := scanAlertFromRow(rows)
		if err != nil {
			return nil, err
		}
		alerts = append(alerts, a)
	}
	return alerts, rows.Err()
}

func (s *AlertStoreImpl) DeleteAlertsOlderThan(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.writer.Exec(ctx,
		`DELETE FROM alerts WHERE created_at < ?`,
		before.Unix(),
	)
	if err != nil {
		return 0, fmt.Errorf("delete old alerts: %w", err)
	}
	return res.RowsAffected, nil
}

// scanAlertFromRow scans a single alert from any type implementing rowScanner.
func scanAlertFromRow(scanner rowScanner) (*alert.Alert, error) {
	a := &alert.Alert{}
	var firedAt, createdAt int64
	var details, acknowledgedBy sql.NullString
	var resolvedByID sql.NullString
	var resolvedAt, acknowledgedAt, escalatedAt sql.NullInt64

	err := scanner.Scan(
		&a.ID, &a.Source, &a.AlertType, &a.Severity, &a.Status, &a.Message,
		&a.EntityType, &a.EntityID, &a.EntityName, &details,
		&resolvedByID, &firedAt, &resolvedAt,
		&acknowledgedAt, &acknowledgedBy, &escalatedAt, &createdAt,
	)
	if err != nil {
		return nil, err
	}

	a.FiredAt = time.Unix(firedAt, 0)
	a.CreatedAt = time.Unix(createdAt, 0)
	if details.Valid {
		a.Details = details.String
	}
	if resolvedAt.Valid {
		t := time.Unix(resolvedAt.Int64, 0)
		a.ResolvedAt = &t
	}
	if resolvedByID.Valid {
		v := resolvedByID.String
		a.ResolvedByID = &v
	}
	if acknowledgedAt.Valid {
		t := time.Unix(acknowledgedAt.Int64, 0)
		a.AcknowledgedAt = &t
	}
	if acknowledgedBy.Valid {
		a.AcknowledgedBy = acknowledgedBy.String
	}
	if escalatedAt.Valid {
		t := time.Unix(escalatedAt.Int64, 0)
		a.EscalatedAt = &t
	}
	return a, nil
}

func (s *AlertStoreImpl) AcknowledgeAlert(ctx context.Context, id string, by string, at time.Time) error {
	_, err := s.writer.Exec(ctx,
		`UPDATE alerts SET acknowledged_at = ?, acknowledged_by = ?
		WHERE id = ? AND status = 'active' AND acknowledged_at IS NULL`,
		at.Unix(), by, id,
	)
	if err != nil {
		return fmt.Errorf("acknowledge alert: %w", err)
	}
	return nil
}

func (s *AlertStoreImpl) SetEscalatedAt(ctx context.Context, id string, at time.Time) error {
	_, err := s.writer.Exec(ctx,
		`UPDATE alerts SET escalated_at = ? WHERE id = ?`,
		at.Unix(), id,
	)
	if err != nil {
		return fmt.Errorf("set escalated_at: %w", err)
	}
	return nil
}

func (s *AlertStoreImpl) ListUnacknowledgedActiveAlerts(ctx context.Context) ([]*alert.Alert, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+alertColumns+` FROM alerts
		WHERE status = 'active' AND acknowledged_at IS NULL AND escalated_at IS NULL
		ORDER BY fired_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("list unacknowledged active alerts: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)

	var alerts []*alert.Alert
	for rows.Next() {
		a, err := scanAlertFromRow(rows)
		if err != nil {
			return nil, err
		}
		alerts = append(alerts, a)
	}
	return alerts, rows.Err()
}

// nullableStrPtr binds a *string FK column: NULL when nil, otherwise the value.
func nullableStrPtr(s *string) interface{} {
	if s == nil {
		return nil
	}
	return *s
}
