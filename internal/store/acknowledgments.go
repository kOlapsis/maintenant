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

package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kolapsis/maintenant/internal/security"
	"github.com/kolapsis/maintenant/internal/uid"
)

// AcknowledgmentStoreImpl implements security.AcknowledgmentStore using SQLite.
type AcknowledgmentStoreImpl struct {
	db     *Reader
	writer *Writer
}

// NewAcknowledgmentStore creates a new SQLite-backed acknowledgment store.
func NewAcknowledgmentStore(d *DB) *AcknowledgmentStoreImpl {
	return &AcknowledgmentStoreImpl{
		db:     d.Reader(),
		writer: d.Writer(),
	}
}

func (s *AcknowledgmentStoreImpl) InsertAcknowledgment(ctx context.Context, ack *security.RiskAcknowledgment) (string, error) {
	ack.ID = uid.New()
	_, err := s.writer.Exec(ctx,
		`INSERT INTO risk_acknowledgments (id, container_external_id, finding_type, finding_key, acknowledged_by, reason, acknowledged_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		ack.ID, ack.ContainerExternalID, ack.FindingType, ack.FindingKey,
		ack.AcknowledgedBy, ack.Reason, ack.AcknowledgedAt.Unix(),
	)
	if err != nil {
		return "", fmt.Errorf("insert acknowledgment: %w", err)
	}
	return ack.ID, nil
}

func (s *AcknowledgmentStoreImpl) DeleteAcknowledgment(ctx context.Context, id string) error {
	_, err := s.writer.Exec(ctx,
		`DELETE FROM risk_acknowledgments WHERE id = ?`, id,
	)
	if err != nil {
		return fmt.Errorf("delete acknowledgment: %w", err)
	}
	return nil
}

func (s *AcknowledgmentStoreImpl) ListAcknowledgments(ctx context.Context, containerExternalID string) ([]*security.RiskAcknowledgment, error) {
	query := `SELECT id, container_external_id, finding_type, finding_key, acknowledged_by, reason, acknowledged_at
		FROM risk_acknowledgments`
	var args []interface{}

	if containerExternalID != "" {
		query += ` WHERE container_external_id = ?`
		args = append(args, containerExternalID)
	}
	query += ` ORDER BY acknowledged_at DESC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list acknowledgments: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var result []*security.RiskAcknowledgment
	for rows.Next() {
		ack, err := scanAcknowledgmentRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, ack)
	}
	return result, rows.Err()
}

func (s *AcknowledgmentStoreImpl) GetAcknowledgment(ctx context.Context, id string) (*security.RiskAcknowledgment, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, container_external_id, finding_type, finding_key, acknowledged_by, reason, acknowledged_at
		FROM risk_acknowledgments WHERE id = ?`, id)

	ack := &security.RiskAcknowledgment{}
	var ackedAt int64
	err := row.Scan(&ack.ID, &ack.ContainerExternalID, &ack.FindingType, &ack.FindingKey,
		&ack.AcknowledgedBy, &ack.Reason, &ackedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get acknowledgment: %w", err)
	}
	ack.AcknowledgedAt = time.Unix(ackedAt, 0).UTC()
	return ack, nil
}

func (s *AcknowledgmentStoreImpl) IsAcknowledged(ctx context.Context, containerExternalID, findingType, findingKey string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM risk_acknowledgments WHERE container_external_id = ? AND finding_type = ? AND finding_key = ?`,
		containerExternalID, findingType, findingKey,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check acknowledgment: %w", err)
	}
	return count > 0, nil
}

func scanAcknowledgmentRow(rows *sql.Rows) (*security.RiskAcknowledgment, error) {
	ack := &security.RiskAcknowledgment{}
	var ackedAt int64
	err := rows.Scan(&ack.ID, &ack.ContainerExternalID, &ack.FindingType, &ack.FindingKey,
		&ack.AcknowledgedBy, &ack.Reason, &ackedAt)
	if err != nil {
		return nil, fmt.Errorf("scan acknowledgment row: %w", err)
	}
	ack.AcknowledgedAt = time.Unix(ackedAt, 0).UTC()
	return ack, nil
}
