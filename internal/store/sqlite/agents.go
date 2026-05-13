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

	"github.com/kolapsis/maintenant/internal/agent"
)

// AgentStore handles persistence for agents and enrollment tokens.
type AgentStore struct {
	db     *sql.DB
	writer *Writer
}

// NewAgentStore creates a new AgentStore.
func NewAgentStore(d *DB) *AgentStore {
	return &AgentStore{
		db:     d.ReadDB(),
		writer: d.Writer(),
	}
}

// Insert persists a new agent record.
func (s *AgentStore) Insert(ctx context.Context, a *agent.Agent) error {
	_, err := s.writer.Exec(ctx,
		`INSERT INTO agents (agent_id, public_key, hostname, label, os_arch, agent_version,
			detected_runtime, status, last_seen_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.AgentID, a.PublicKey, a.Hostname, a.Label, a.OSArch, a.AgentVersion,
		a.DetectedRuntime, a.Status, nullableTime(a.LastSeenAt), a.CreatedAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("insert agent: %w", err)
	}
	return nil
}

// Get retrieves an agent by ID.
func (s *AgentStore) Get(ctx context.Context, agentID string) (*agent.Agent, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT agent_id, public_key, hostname, label, os_arch, agent_version,
			detected_runtime, status, last_seen_at, created_at, revoked_at, revoked_by
		FROM agents WHERE agent_id = ?`, agentID)
	a, err := scanAgent(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, agent.ErrAgentNotFound
	}
	return a, err
}

// List retrieves agents with optional filters.
// status="" means all statuses. connectedIDs is the set of currently connected agent_ids (for connection_state filter).
func (s *AgentStore) List(ctx context.Context, statusFilter string) ([]*agent.Agent, error) {
	query := `SELECT agent_id, public_key, hostname, label, os_arch, agent_version,
		detected_runtime, status, last_seen_at, created_at, revoked_at, revoked_by
	FROM agents`
	args := []any{}
	if statusFilter != "" && statusFilter != "all" {
		query += " WHERE status = ?"
		args = append(args, statusFilter)
	}
	query += " ORDER BY created_at DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	defer rows.Close()

	var agents []*agent.Agent
	for rows.Next() {
		a, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		agents = append(agents, a)
	}
	return agents, rows.Err()
}

// UpdateLabel updates the display label for an agent.
func (s *AgentStore) UpdateLabel(ctx context.Context, agentID, label string) error {
	if len(label) > 64 {
		return agent.ErrLabelTooLong
	}
	res, err := s.writer.Exec(ctx,
		`UPDATE agents SET label = ? WHERE agent_id = ?`, label, agentID)
	if err != nil {
		return fmt.Errorf("update agent label: %w", err)
	}
	n := res.RowsAffected
	if n == 0 {
		return agent.ErrAgentNotFound
	}
	return nil
}

// UpdateLastSeen updates the last_seen_at timestamp.
func (s *AgentStore) UpdateLastSeen(ctx context.Context, agentID string, t time.Time) error {
	_, err := s.writer.Exec(ctx,
		`UPDATE agents SET last_seen_at = ? WHERE agent_id = ?`, t.UTC(), agentID)
	if err != nil {
		return fmt.Errorf("update agent last_seen: %w", err)
	}
	return nil
}

// Revoke marks an agent as revoked.
func (s *AgentStore) Revoke(ctx context.Context, agentID, revokedBy string) error {
	now := time.Now().UTC()
	res, err := s.writer.Exec(ctx,
		`UPDATE agents SET status = 'revoked', revoked_at = ?, revoked_by = ?
		WHERE agent_id = ?`, now, revokedBy, agentID)
	if err != nil {
		return fmt.Errorf("revoke agent: %w", err)
	}
	n := res.RowsAffected
	if n == 0 {
		return agent.ErrAgentNotFound
	}
	return nil
}

// Delete hard-deletes an agent and all its events via FK ON DELETE CASCADE.
func (s *AgentStore) Delete(ctx context.Context, agentID string) error {
	res, err := s.writer.Exec(ctx, `DELETE FROM agents WHERE agent_id = ?`, agentID)
	if err != nil {
		return fmt.Errorf("delete agent: %w", err)
	}
	n := res.RowsAffected
	if n == 0 {
		return agent.ErrAgentNotFound
	}
	return nil
}

// CountByStatus returns agent counts grouped by status and runtime.
func (s *AgentStore) CountByStatus(ctx context.Context) (active, revoked int, err error) {
	rows, err := s.db.QueryContext(ctx, `SELECT status, COUNT(*) FROM agents GROUP BY status`)
	if err != nil {
		return 0, 0, fmt.Errorf("count agents by status: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return 0, 0, err
		}
		switch status {
		case "active":
			active = count
		case "revoked":
			revoked = count
		}
	}
	return active, revoked, rows.Err()
}

// CountByRuntime returns agent counts grouped by detected_runtime.
func (s *AgentStore) CountByRuntime(ctx context.Context) (docker, swarm, kubernetes int, err error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT detected_runtime, COUNT(*) FROM agents WHERE status = 'active' GROUP BY detected_runtime`)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("count agents by runtime: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var rt string
		var count int
		if err := rows.Scan(&rt, &count); err != nil {
			return 0, 0, 0, err
		}
		switch rt {
		case "docker":
			docker = count
		case "swarm":
			swarm = count
		case "kubernetes":
			kubernetes = count
		}
	}
	return docker, swarm, kubernetes, rows.Err()
}

// InsertToken persists a new enrollment token.
func (s *AgentStore) InsertToken(ctx context.Context, t *agent.EnrollmentToken) error {
	_, err := s.writer.Exec(ctx,
		`INSERT INTO enrollment_tokens (token_id, token, created_at, expires_at)
		VALUES (?, ?, ?, ?)`,
		t.TokenID, t.Token, t.CreatedAt.UTC(), t.ExpiresAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("insert token: %w", err)
	}
	return nil
}

// ConsumeAtomic atomically marks a token as consumed. Returns ErrTokenNotFound,
// ErrTokenAlreadyConsumed, or ErrTokenExpired on failure.
func (s *AgentStore) ConsumeAtomic(ctx context.Context, tokenCleartext, agentID string) error {
	now := time.Now().UTC()
	res, err := s.writer.Exec(ctx,
		`UPDATE enrollment_tokens
		SET consumed_at = ?, consumed_by_agent_id = ?
		WHERE token = ? AND consumed_at IS NULL AND expires_at > ?`,
		now, agentID, tokenCleartext, now,
	)
	if err != nil {
		return fmt.Errorf("consume token: %w", err)
	}
	n := res.RowsAffected
	if n == 1 {
		return nil
	}
	// Determine why it failed
	var consumed sql.NullTime
	var expiresAt time.Time
	err = s.db.QueryRowContext(ctx,
		`SELECT consumed_at, expires_at FROM enrollment_tokens WHERE token = ?`, tokenCleartext,
	).Scan(&consumed, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return agent.ErrTokenNotFound
	}
	if err != nil {
		return fmt.Errorf("check token: %w", err)
	}
	if consumed.Valid {
		return agent.ErrTokenAlreadyConsumed
	}
	if expiresAt.Before(now) {
		return agent.ErrTokenExpired
	}
	return agent.ErrTokenNotFound
}

// GetByToken retrieves a token by its cleartext value.
func (s *AgentStore) GetByToken(ctx context.Context, tokenCleartext string) (*agent.EnrollmentToken, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT token_id, token, created_at, expires_at, consumed_at, consumed_by_agent_id
		FROM enrollment_tokens WHERE token = ?`, tokenCleartext)
	t, err := scanToken(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, agent.ErrTokenNotFound
	}
	return t, err
}

// GetTokenByID retrieves a token by its opaque token_id.
func (s *AgentStore) GetTokenByID(ctx context.Context, tokenID string) (*agent.EnrollmentToken, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT token_id, token, created_at, expires_at, consumed_at, consumed_by_agent_id
		FROM enrollment_tokens WHERE token_id = ?`, tokenID)
	t, err := scanToken(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, agent.ErrTokenNotFound
	}
	return t, err
}

// ListTokens returns all tokens with optional filters.
// When includeExpired=false, only non-expired (or consumed) tokens are returned.
// When includeConsumed=false, consumed tokens are excluded.
func (s *AgentStore) ListTokens(ctx context.Context, includeExpired, includeConsumed bool) ([]*agent.EnrollmentToken, error) {
	now := time.Now().UTC()
	query := `SELECT token_id, token, created_at, expires_at, consumed_at, consumed_by_agent_id
	FROM enrollment_tokens WHERE 1=1`
	args := []any{}
	if !includeExpired {
		query += " AND (expires_at > ? OR consumed_at IS NOT NULL)"
		args = append(args, now)
	}
	if !includeConsumed {
		query += " AND consumed_at IS NULL"
	}
	query += " ORDER BY created_at DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list tokens: %w", err)
	}
	defer rows.Close()

	var tokens []*agent.EnrollmentToken
	for rows.Next() {
		t, err := scanToken(rows)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

// DeleteToken removes an unconsumed token.
func (s *AgentStore) DeleteToken(ctx context.Context, tokenID string) error {
	res, err := s.writer.Exec(ctx,
		`DELETE FROM enrollment_tokens WHERE token_id = ? AND consumed_at IS NULL`, tokenID)
	if err != nil {
		return fmt.Errorf("delete token: %w", err)
	}
	n := res.RowsAffected
	if n == 0 {
		// Distinguish between not found and already consumed
		var count int
		queryErr := s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM enrollment_tokens WHERE token_id = ?`, tokenID,
		).Scan(&count)
		if queryErr != nil {
			return fmt.Errorf("check token existence: %w", queryErr)
		}
		if count == 0 {
			return agent.ErrTokenNotFound
		}
		return agent.ErrTokenAlreadyConsumed
	}
	return nil
}

// GcExpiredTokens removes unconsumed tokens that expired more than 7 days ago.
// StaleAgents returns IDs of active agents whose last_seen_at is older than threshold.
func (s *AgentStore) StaleAgents(ctx context.Context, threshold time.Duration) ([]string, error) {
	cutoff := time.Now().UTC().Add(-threshold)
	rows, err := s.db.QueryContext(ctx,
		`SELECT agent_id FROM agents WHERE status = 'active' AND last_seen_at < ?`, cutoff)
	if err != nil {
		return nil, fmt.Errorf("stale agents: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *AgentStore) GcExpiredTokens(ctx context.Context) error {
	cutoff := time.Now().UTC().Add(-7 * 24 * time.Hour)
	_, err := s.writer.Exec(ctx,
		`DELETE FROM enrollment_tokens WHERE expires_at < ? AND consumed_at IS NULL`, cutoff)
	if err != nil {
		return fmt.Errorf("gc expired tokens: %w", err)
	}
	return nil
}

// scanner is satisfied by both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanAgent(s scanner) (*agent.Agent, error) {
	a := &agent.Agent{}
	var lastSeen, revokedAt sql.NullTime
	var revokedBy sql.NullString
	err := s.Scan(
		&a.AgentID, &a.PublicKey, &a.Hostname, &a.Label, &a.OSArch, &a.AgentVersion,
		&a.DetectedRuntime, &a.Status, &lastSeen, &a.CreatedAt, &revokedAt, &revokedBy,
	)
	if err != nil {
		return nil, err
	}
	if lastSeen.Valid {
		a.LastSeenAt = &lastSeen.Time
	}
	if revokedAt.Valid {
		a.RevokedAt = &revokedAt.Time
	}
	if revokedBy.Valid {
		a.RevokedBy = &revokedBy.String
	}
	return a, nil
}

func scanToken(s scanner) (*agent.EnrollmentToken, error) {
	t := &agent.EnrollmentToken{}
	var consumedAt sql.NullTime
	var consumedBy sql.NullString
	err := s.Scan(
		&t.TokenID, &t.Token, &t.CreatedAt, &t.ExpiresAt, &consumedAt, &consumedBy,
	)
	if err != nil {
		return nil, err
	}
	if consumedAt.Valid {
		t.ConsumedAt = &consumedAt.Time
	}
	if consumedBy.Valid {
		t.ConsumedByAgentID = &consumedBy.String
	}
	return t, nil
}
