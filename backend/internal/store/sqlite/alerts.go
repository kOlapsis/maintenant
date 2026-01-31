package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kolapsis/pulseboard/internal/alert"
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
	resolved_by_id, fired_at, resolved_at, created_at`

func (s *AlertStoreImpl) InsertAlert(ctx context.Context, a *alert.Alert) (int64, error) {
	res, err := s.writer.Exec(ctx,
		`INSERT INTO alerts (source, alert_type, severity, status, message,
			entity_type, entity_id, entity_name, details,
			resolved_by_id, fired_at, resolved_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.Source, a.AlertType, a.Severity, a.Status, a.Message,
		a.EntityType, a.EntityID, a.EntityName, a.Details,
		a.ResolvedByID, a.FiredAt.UTC().Format(time.RFC3339), nullableTimeStr(a.ResolvedAt),
	)
	if err != nil {
		return 0, fmt.Errorf("insert alert: %w", err)
	}
	a.ID = res.LastInsertID
	return res.LastInsertID, nil
}

func (s *AlertStoreImpl) GetAlert(ctx context.Context, id int64) (*alert.Alert, error) {
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
		args = append(args, opts.Before.UTC().Format(time.RFC3339))
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
	defer rows.Close()

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

func (s *AlertStoreImpl) UpdateAlertStatus(ctx context.Context, id int64, status string, resolvedAt *time.Time, resolvedByID *int64) error {
	var resolvedAtStr *string
	if resolvedAt != nil {
		s := resolvedAt.UTC().Format(time.RFC3339)
		resolvedAtStr = &s
	}
	_, err := s.writer.Exec(ctx,
		`UPDATE alerts SET status = ?, resolved_at = ?, resolved_by_id = ? WHERE id = ?`,
		status, resolvedAtStr, resolvedByID, id,
	)
	if err != nil {
		return fmt.Errorf("update alert status: %w", err)
	}
	return nil
}

func (s *AlertStoreImpl) GetActiveAlert(ctx context.Context, source, alertType, entityType string, entityID int64) (*alert.Alert, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+alertColumns+` FROM alerts
		WHERE source = ? AND alert_type = ? AND entity_type = ? AND entity_id = ? AND status = 'active'
		LIMIT 1`,
		source, alertType, entityType, entityID)
	a, err := scanAlertFromRow(row)
	if err == sql.ErrNoRows {
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
	defer rows.Close()

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
		before.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return 0, fmt.Errorf("delete old alerts: %w", err)
	}
	return res.RowsAffected, nil
}

// scanAlertFromRow scans a single alert from any type implementing rowScanner.
func scanAlertFromRow(scanner rowScanner) (*alert.Alert, error) {
	a := &alert.Alert{}
	var firedAt, createdAt string
	var resolvedAt, details sql.NullString
	var resolvedByID sql.NullInt64

	err := scanner.Scan(
		&a.ID, &a.Source, &a.AlertType, &a.Severity, &a.Status, &a.Message,
		&a.EntityType, &a.EntityID, &a.EntityName, &details,
		&resolvedByID, &firedAt, &resolvedAt, &createdAt,
	)
	if err != nil {
		return nil, err
	}

	a.FiredAt, _ = time.Parse(time.RFC3339, firedAt)
	a.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	if details.Valid {
		a.Details = details.String
	}
	if resolvedAt.Valid {
		t, _ := time.Parse(time.RFC3339, resolvedAt.String)
		a.ResolvedAt = &t
	}
	if resolvedByID.Valid {
		v := resolvedByID.Int64
		a.ResolvedByID = &v
	}
	return a, nil
}

func nullableTimeStr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
}
