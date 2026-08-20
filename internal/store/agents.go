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

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/uid"
)

// AgentStore handles persistence for agents and enrollment tokens. The agents
// table primary key is `id` (the agent-generated UUID); timestamps are stored as
// epoch-second BIGINTs.
type AgentStore struct {
	db     *Reader
	writer *Writer
}

// NewAgentStore creates a new AgentStore.
func NewAgentStore(d *DB) *AgentStore {
	return &AgentStore{
		db:     d.Reader(),
		writer: d.Writer(),
	}
}

// Insert persists a new agent record.
func (s *AgentStore) Insert(ctx context.Context, a *agent.Agent) error {
	_, err := s.writer.Exec(ctx,
		`INSERT INTO agents (id, public_key, hostname, label, os_arch, agent_version,
			detected_runtime, status, last_seen_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.AgentID, a.PublicKey, a.Hostname, a.Label, a.OSArch, a.AgentVersion,
		a.DetectedRuntime, a.Status, nullableTime(a.LastSeenAt), a.CreatedAt.Unix(),
	)
	if err != nil {
		return fmt.Errorf("insert agent: %w", err)
	}
	return nil
}

const agentColumns = `id, public_key, hostname, label, os_arch, agent_version,
	detected_runtime, status, last_seen_at, created_at, revoked_at, revoked_by`

// Get retrieves an agent by ID.
func (s *AgentStore) Get(ctx context.Context, agentID string) (*agent.Agent, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+agentColumns+` FROM agents WHERE id = ?`, agentID)
	a, err := scanAgent(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, agent.ErrAgentNotFound
	}
	return a, err
}

// List retrieves agents with an optional status filter ("" or "all" = all).
// The local sentinel agent is an internal FK anchor, not an enrolled agent, so
// it is never surfaced in listings.
func (s *AgentStore) List(ctx context.Context, statusFilter string) ([]*agent.Agent, error) {
	query := `SELECT ` + agentColumns + ` FROM agents WHERE id != ?`
	args := []any{uid.LocalAgent}
	if statusFilter != "" && statusFilter != "all" {
		query += " AND status = ?"
		args = append(args, statusFilter)
	}
	query += " ORDER BY created_at DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	defer func() { _ = rows.Close() }()

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
		`UPDATE agents SET label = ? WHERE id = ?`, label, agentID)
	if err != nil {
		return fmt.Errorf("update agent label: %w", err)
	}
	if res.RowsAffected == 0 {
		return agent.ErrAgentNotFound
	}
	return nil
}

// UpdateLastSeen updates the last_seen_at timestamp.
func (s *AgentStore) UpdateLastSeen(ctx context.Context, agentID string, t time.Time) error {
	_, err := s.writer.Exec(ctx,
		`UPDATE agents SET last_seen_at = ? WHERE id = ?`, t.Unix(), agentID)
	if err != nil {
		return fmt.Errorf("update agent last_seen: %w", err)
	}
	return nil
}

// UpdateAgentVersion records the build the agent is currently running. Enrollment
// only happens once, so this is the only way the stored version stays truthful
// across agent upgrades.
func (s *AgentStore) UpdateAgentVersion(ctx context.Context, agentID, version string) error {
	_, err := s.writer.Exec(ctx,
		`UPDATE agents SET agent_version = ? WHERE id = ?`, version, agentID)
	if err != nil {
		return fmt.Errorf("update agent version: %w", err)
	}
	return nil
}

// Revoke marks an agent as revoked.
func (s *AgentStore) Revoke(ctx context.Context, agentID, revokedBy string) error {
	res, err := s.writer.Exec(ctx,
		`UPDATE agents SET status = 'revoked', revoked_at = ?, revoked_by = ?
		WHERE id = ?`, time.Now().Unix(), revokedBy, agentID)
	if err != nil {
		return fmt.Errorf("revoke agent: %w", err)
	}
	if res.RowsAffected == 0 {
		return agent.ErrAgentNotFound
	}
	return nil
}

// Delete hard-deletes an agent and all its events via FK ON DELETE CASCADE.
func (s *AgentStore) Delete(ctx context.Context, agentID string) error {
	res, err := s.writer.Exec(ctx, `DELETE FROM agents WHERE id = ?`, agentID)
	if err != nil {
		return fmt.Errorf("delete agent: %w", err)
	}
	if res.RowsAffected == 0 {
		return agent.ErrAgentNotFound
	}
	return nil
}

// CountByStatus returns agent counts grouped by status.
func (s *AgentStore) CountByStatus(ctx context.Context) (active, revoked int, err error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT status, COUNT(*) FROM agents WHERE id != ? GROUP BY status`, uid.LocalAgent)
	if err != nil {
		return 0, 0, fmt.Errorf("count agents by status: %w", err)
	}
	defer func() { _ = rows.Close() }()
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

// CountByRuntime returns active agent counts grouped by detected_runtime.
func (s *AgentStore) CountByRuntime(ctx context.Context) (docker, swarm, kubernetes int, err error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT detected_runtime, COUNT(*) FROM agents WHERE status = 'active' AND id != ? GROUP BY detected_runtime`, uid.LocalAgent)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("count agents by runtime: %w", err)
	}
	defer func() { _ = rows.Close() }()
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

// InsertToken persists a new enrollment token. Only the hash and the display
// prefix are written — the caller holds the cleartext and must not pass it here.
func (s *AgentStore) InsertToken(ctx context.Context, t *agent.EnrollmentToken) error {
	_, err := s.writer.Exec(ctx,
		`INSERT INTO enrollment_tokens (id, token_hash, token_prefix, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?)`,
		t.TokenID, t.TokenHash, t.TokenPrefix, t.CreatedAt.Unix(), t.ExpiresAt.Unix(),
	)
	if err != nil {
		return fmt.Errorf("insert token: %w", err)
	}
	return nil
}

// EnrollAtomic enforces the per-edition host cap, consumes the one-time token,
// and inserts the agent as a single serialized transaction. Because the count,
// the token consume, and the insert run in one writer-goroutine transaction, no
// two concurrent enrollments can both pass the cap check and over-fill it. A
// limit < 0 means unlimited; the local sentinel is never counted. Returns
// ErrHostLimitReached, ErrTokenNotFound, ErrTokenAlreadyConsumed, or
// ErrTokenExpired.
func (s *AgentStore) EnrollAtomic(ctx context.Context, limit int, tokenCleartext string, a *agent.Agent) error {
	return s.writer.Tx(ctx, func(ctx context.Context, tx *Tx) error {
		if err := tx.Serialize(ctx, "agents"); err != nil {
			return err
		}
		if limit >= 0 {
			var active int
			if err := tx.QueryRowContext(ctx,
				`SELECT COUNT(*) FROM agents WHERE id != ? AND status = 'active'`, uid.LocalAgent,
			).Scan(&active); err != nil {
				return fmt.Errorf("count active agents: %w", err)
			}
			if active >= limit {
				return agent.ErrHostLimitReached
			}
		}

		now := time.Now().Unix()
		// The client sends the cleartext; the row is found by its hash.
		tokenHash := agent.HashToken(tokenCleartext)
		res, err := tx.ExecContext(ctx,
			`UPDATE enrollment_tokens
			SET consumed_at = ?, consumed_by_agent_id = ?
			WHERE token_hash = ? AND consumed_at IS NULL AND expires_at > ?`,
			now, a.AgentID, tokenHash, now,
		)
		if err != nil {
			return fmt.Errorf("consume token: %w", err)
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("consume token rows: %w", err)
		}
		if affected != 1 {
			return classifyTokenFailure(ctx, tx, tokenHash, now)
		}

		if _, err := tx.ExecContext(ctx,
			`INSERT INTO agents (id, public_key, hostname, label, os_arch, agent_version,
				detected_runtime, status, last_seen_at, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			a.AgentID, a.PublicKey, a.Hostname, a.Label, a.OSArch, a.AgentVersion,
			a.DetectedRuntime, a.Status, nullableTime(a.LastSeenAt), a.CreatedAt.Unix(),
		); err != nil {
			return fmt.Errorf("insert agent: %w", err)
		}
		return nil
	})
}

// classifyTokenFailure determines why a token UPDATE matched no rows.
func classifyTokenFailure(ctx context.Context, tx *Tx, tokenHash string, now int64) error {
	var consumed sql.NullInt64
	var expiresAt int64
	err := tx.QueryRowContext(ctx,
		`SELECT consumed_at, expires_at FROM enrollment_tokens WHERE token_hash = ?`, tokenHash,
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
	if expiresAt < now {
		return agent.ErrTokenExpired
	}
	return agent.ErrTokenNotFound
}

const tokenColumns = `id, token_hash, token_prefix, created_at, expires_at, consumed_at, consumed_by_agent_id` // #nosec G101 -- column names, not a credential.

// GetByToken retrieves a token from its cleartext value, which it hashes to
// find the row. Nothing it returns can reconstruct the cleartext.
func (s *AgentStore) GetByToken(ctx context.Context, tokenCleartext string) (*agent.EnrollmentToken, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+tokenColumns+` FROM enrollment_tokens WHERE token_hash = ?`, agent.HashToken(tokenCleartext))
	t, err := scanToken(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, agent.ErrTokenNotFound
	}
	return t, err
}

// GetTokenByID retrieves a token by its opaque id.
func (s *AgentStore) GetTokenByID(ctx context.Context, tokenID string) (*agent.EnrollmentToken, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+tokenColumns+` FROM enrollment_tokens WHERE id = ?`, tokenID)
	t, err := scanToken(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, agent.ErrTokenNotFound
	}
	return t, err
}

// ListTokens returns all tokens with optional filters.
func (s *AgentStore) ListTokens(ctx context.Context, includeExpired, includeConsumed bool) ([]*agent.EnrollmentToken, error) {
	now := time.Now().Unix()
	query := `SELECT ` + tokenColumns + ` FROM enrollment_tokens WHERE 1=1`
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
	defer func() { _ = rows.Close() }()

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
		`DELETE FROM enrollment_tokens WHERE id = ? AND consumed_at IS NULL`, tokenID)
	if err != nil {
		return fmt.Errorf("delete token: %w", err)
	}
	if res.RowsAffected == 0 {
		var count int
		queryErr := s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM enrollment_tokens WHERE id = ?`, tokenID,
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

// StaleAgents returns IDs of active agents whose last_seen_at is older than threshold.
func (s *AgentStore) StaleAgents(ctx context.Context, threshold time.Duration) ([]string, error) {
	cutoff := time.Now().Add(-threshold).Unix()
	rows, err := s.db.QueryContext(ctx,
		`SELECT id FROM agents WHERE status = 'active' AND last_seen_at < ?`, cutoff)
	if err != nil {
		return nil, fmt.Errorf("stale agents: %w", err)
	}
	defer func() { _ = rows.Close() }()
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

// GcExpiredTokens removes unconsumed tokens that expired more than 7 days ago.
func (s *AgentStore) GcExpiredTokens(ctx context.Context) error {
	cutoff := time.Now().Add(-7 * 24 * time.Hour).Unix()
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
	var lastSeen, revokedAt sql.NullInt64
	var revokedBy sql.NullString
	var createdAt int64
	err := s.Scan(
		&a.AgentID, &a.PublicKey, &a.Hostname, &a.Label, &a.OSArch, &a.AgentVersion,
		&a.DetectedRuntime, &a.Status, &lastSeen, &createdAt, &revokedAt, &revokedBy,
	)
	if err != nil {
		return nil, err
	}
	a.CreatedAt = time.Unix(createdAt, 0)
	if lastSeen.Valid {
		t := time.Unix(lastSeen.Int64, 0)
		a.LastSeenAt = &t
	}
	if revokedAt.Valid {
		t := time.Unix(revokedAt.Int64, 0)
		a.RevokedAt = &t
	}
	if revokedBy.Valid {
		a.RevokedBy = &revokedBy.String
	}
	return a, nil
}

func scanToken(s scanner) (*agent.EnrollmentToken, error) {
	t := &agent.EnrollmentToken{}
	var consumedAt sql.NullInt64
	var consumedBy sql.NullString
	var createdAt, expiresAt int64
	err := s.Scan(
		&t.TokenID, &t.TokenHash, &t.TokenPrefix, &createdAt, &expiresAt, &consumedAt, &consumedBy,
	)
	if err != nil {
		return nil, err
	}
	t.CreatedAt = time.Unix(createdAt, 0)
	t.ExpiresAt = time.Unix(expiresAt, 0)
	if consumedAt.Valid {
		ts := time.Unix(consumedAt.Int64, 0)
		t.ConsumedAt = &ts
	}
	if consumedBy.Valid {
		t.ConsumedByAgentID = &consumedBy.String
	}
	return t, nil
}
