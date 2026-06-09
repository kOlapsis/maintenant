// Copyright 2026 Benjamin Touchard (kOlapsis)
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
	"fmt"
	"time"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/uid"
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

func (s *SilenceStoreImpl) InsertSilenceRule(ctx context.Context, rule *alert.SilenceRule) (string, error) {
	rule.ID = uid.New()
	if rule.CreatedAt.IsZero() {
		rule.CreatedAt = time.Now().UTC()
	}
	_, err := s.writer.Exec(ctx,
		`INSERT INTO silence_rules (id, entity_type, entity_id, source, reason, starts_at, duration_seconds, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		rule.ID, NullableString(rule.EntityType), nullableStrPtr(rule.EntityID), NullableString(rule.Source),
		NullableString(rule.Reason),
		rule.StartsAt.Unix(), rule.DurationSeconds, rule.CreatedAt.Unix(),
	)
	if err != nil {
		return "", fmt.Errorf("insert silence rule: %w", err)
	}
	return rule.ID, nil
}

func (s *SilenceStoreImpl) ListSilenceRules(ctx context.Context, activeOnly bool) ([]*alert.SilenceRule, error) {
	query := `SELECT id, entity_type, entity_id, source, reason, starts_at, duration_seconds, cancelled_at, created_at
		FROM silence_rules`

	if activeOnly {
		query += ` WHERE cancelled_at IS NULL
			AND (starts_at + duration_seconds) > strftime('%s','now')`
	}

	query += ` ORDER BY created_at DESC`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list silence rules: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)

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

func (s *SilenceStoreImpl) CancelSilenceRule(ctx context.Context, id string) error {
	_, err := s.writer.Exec(ctx,
		`UPDATE silence_rules SET cancelled_at = ? WHERE id = ?`,
		time.Now().Unix(), id,
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
			AND (starts_at + duration_seconds) > strftime('%s','now')`)
	if err != nil {
		return nil, fmt.Errorf("get active silence rules: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)

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
	var entityType, source, reason, entityID sql.NullString
	var startsAt, createdAt int64
	var cancelledAt sql.NullInt64

	err := rows.Scan(&r.ID, &entityType, &entityID, &source, &reason,
		&startsAt, &r.DurationSeconds, &cancelledAt, &createdAt)
	if err != nil {
		return nil, err
	}

	if entityType.Valid {
		r.EntityType = entityType.String
	}
	if entityID.Valid {
		v := entityID.String
		r.EntityID = &v
	}
	if source.Valid {
		r.Source = source.String
	}
	if reason.Valid {
		r.Reason = reason.String
	}
	r.StartsAt = time.Unix(startsAt, 0)
	r.CreatedAt = time.Unix(createdAt, 0)
	if cancelledAt.Valid {
		t := time.Unix(cancelledAt.Int64, 0)
		r.CancelledAt = &t
	}

	// Compute derived fields
	r.ExpiresAt = r.StartsAt.Add(time.Duration(r.DurationSeconds) * time.Second)
	r.IsActive = r.CancelledAt == nil && r.ExpiresAt.After(time.Now())

	return r, nil
}
