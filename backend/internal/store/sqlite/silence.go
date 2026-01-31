package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kolapsis/pulseboard/internal/alert"
)

// SilenceStoreImpl implements alert.SilenceStore using SQLite.
type SilenceStoreImpl struct {
	db     *sql.DB
	writer *Writer
}

// NewSilenceStore creates a new SQLite-backed silence store.
func NewSilenceStore(d *DB) *SilenceStoreImpl {
	return &SilenceStoreImpl{
		db:     d.ReadDB(),
		writer: d.Writer(),
	}
}

func (s *SilenceStoreImpl) InsertSilenceRule(ctx context.Context, rule *alert.SilenceRule) (int64, error) {
	var entityID *int64
	if rule.EntityID != nil {
		entityID = rule.EntityID
	}

	res, err := s.writer.Exec(ctx,
		`INSERT INTO silence_rules (entity_type, entity_id, source, reason, starts_at, duration_seconds)
		VALUES (?, ?, ?, ?, ?, ?)`,
		NullableString(rule.EntityType), entityID, NullableString(rule.Source),
		NullableString(rule.Reason),
		rule.StartsAt.UTC().Format(time.RFC3339), rule.DurationSeconds,
	)
	if err != nil {
		return 0, fmt.Errorf("insert silence rule: %w", err)
	}
	rule.ID = res.LastInsertID
	return res.LastInsertID, nil
}

func (s *SilenceStoreImpl) ListSilenceRules(ctx context.Context, activeOnly bool) ([]*alert.SilenceRule, error) {
	query := `SELECT id, entity_type, entity_id, source, reason, starts_at, duration_seconds, cancelled_at, created_at
		FROM silence_rules`

	if activeOnly {
		query += ` WHERE cancelled_at IS NULL
			AND datetime(starts_at, '+' || duration_seconds || ' seconds') > datetime('now')`
	}

	query += ` ORDER BY created_at DESC`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list silence rules: %w", err)
	}
	defer rows.Close()

	var rules []*alert.SilenceRule
	for rows.Next() {
		r, err := scanSilenceRow(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

func (s *SilenceStoreImpl) CancelSilenceRule(ctx context.Context, id int64) error {
	_, err := s.writer.Exec(ctx,
		`UPDATE silence_rules SET cancelled_at = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339), id,
	)
	if err != nil {
		return fmt.Errorf("cancel silence rule: %w", err)
	}
	return nil
}

func (s *SilenceStoreImpl) GetActiveSilenceRules(ctx context.Context) ([]*alert.SilenceRule, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, entity_type, entity_id, source, reason, starts_at, duration_seconds, cancelled_at, created_at
		FROM silence_rules
		WHERE cancelled_at IS NULL
			AND datetime(starts_at, '+' || duration_seconds || ' seconds') > datetime('now')`)
	if err != nil {
		return nil, fmt.Errorf("get active silence rules: %w", err)
	}
	defer rows.Close()

	var rules []*alert.SilenceRule
	for rows.Next() {
		r, err := scanSilenceRow(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

func scanSilenceRow(rows *sql.Rows) (*alert.SilenceRule, error) {
	r := &alert.SilenceRule{}
	var entityType, source, reason, cancelledAt sql.NullString
	var entityID sql.NullInt64
	var startsAt, createdAt string

	err := rows.Scan(&r.ID, &entityType, &entityID, &source, &reason,
		&startsAt, &r.DurationSeconds, &cancelledAt, &createdAt)
	if err != nil {
		return nil, err
	}

	if entityType.Valid {
		r.EntityType = entityType.String
	}
	if entityID.Valid {
		v := entityID.Int64
		r.EntityID = &v
	}
	if source.Valid {
		r.Source = source.String
	}
	if reason.Valid {
		r.Reason = reason.String
	}
	r.StartsAt, _ = time.Parse(time.RFC3339, startsAt)
	r.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	if cancelledAt.Valid {
		t, _ := time.Parse(time.RFC3339, cancelledAt.String)
		r.CancelledAt = &t
	}

	// Compute derived fields
	r.ExpiresAt = r.StartsAt.Add(time.Duration(r.DurationSeconds) * time.Second)
	r.IsActive = r.CancelledAt == nil && r.ExpiresAt.After(time.Now())

	return r, nil
}
