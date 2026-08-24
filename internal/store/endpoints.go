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
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/kolapsis/maintenant/internal/endpoint"
	"github.com/kolapsis/maintenant/internal/uid"
)

// EndpointStore implements endpoint.EndpointStore using SQLite.
type EndpointStore struct {
	db     *Reader
	writer *Writer
}

// NewEndpointStore creates a new SQLite-backed endpoint store.
func NewEndpointStore(d *DB) *EndpointStore {
	return &EndpointStore{
		db:     d.Reader(),
		writer: d.Writer(),
	}
}

const endpointColumns = `id, agent_id, container_name, label_key, external_id, endpoint_type, target,
	status, alert_state, consecutive_failures, consecutive_successes,
	last_check_at, last_response_time_ms, last_http_status, last_error,
	config_json, active, first_seen_at, last_seen_at, source, name`

// UpsertEndpoint upserts a label-discovered endpoint. Its id is derived
// deterministically from (agent_id, container_name, label_key) so the agent and
// server mint the same id; a repeat report updates the existing row.
func (s *EndpointStore) UpsertEndpoint(ctx context.Context, e *endpoint.Endpoint) (string, error) {
	e.AgentID = uid.Agent(e.AgentID)
	e.ID = uid.EndpointLabel(e.AgentID, e.ContainerName, e.LabelKey)
	configJSON := e.ConfigJSON()
	now := time.Now().Unix()

	firstSeen := now
	if !e.FirstSeenAt.IsZero() {
		firstSeen = e.FirstSeenAt.Unix()
	}
	source := string(e.Source)
	if source == "" {
		source = string(endpoint.SourceLabel)
	}
	_, err := s.writer.Exec(ctx,
		`INSERT INTO endpoints (id, agent_id, container_name, label_key, external_id, endpoint_type, target,
			status, alert_state, consecutive_failures, consecutive_successes,
			config_json, active, first_seen_at, last_seen_at, source, name)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 0, ?, 1, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			external_id=excluded.external_id, endpoint_type=excluded.endpoint_type,
			target=excluded.target, config_json=excluded.config_json,
			active=1, last_seen_at=excluded.last_seen_at`,
		e.ID, e.AgentID, e.ContainerName, e.LabelKey, e.ExternalID, string(e.EndpointType), e.Target,
		string(endpoint.StatusUnknown), string(endpoint.AlertNormal),
		configJSON, firstSeen, now, source, e.Name,
	)
	if err != nil {
		return "", fmt.Errorf("upsert endpoint: %w", err)
	}
	return e.ID, nil
}

func (s *EndpointStore) GetEndpointByIdentity(ctx context.Context, containerName, labelKey string) (*endpoint.Endpoint, error) {
	return s.scanEndpoint(s.db.QueryRowContext(ctx,
		`SELECT `+endpointColumns+` FROM endpoints WHERE container_name=? AND label_key=?`,
		containerName, labelKey))
}

// GetActiveAgentEndpointByTarget resolves an active endpoint pushed by a remote
// agent, keyed by (agent_id, target). Used to attach a pushed probe result to
// the endpoint the server provisioned from that agent's container labels.
func (s *EndpointStore) GetActiveAgentEndpointByTarget(ctx context.Context, agentID, target string) (*endpoint.Endpoint, error) {
	return s.scanEndpoint(s.db.QueryRowContext(ctx,
		`SELECT `+endpointColumns+` FROM endpoints
		WHERE agent_id=? AND target=? AND active=1`,
		uid.Agent(agentID), target))
}

func (s *EndpointStore) GetEndpointByID(ctx context.Context, id string) (*endpoint.Endpoint, error) {
	return s.scanEndpoint(s.db.QueryRowContext(ctx,
		`SELECT `+endpointColumns+` FROM endpoints WHERE id=?`, id))
}

func (s *EndpointStore) ListEndpoints(ctx context.Context, opts endpoint.ListEndpointsOpts) ([]*endpoint.Endpoint, error) {
	query := `SELECT ` + endpointColumns + ` FROM endpoints WHERE 1=1`
	var args []interface{}

	if !opts.IncludeInactive {
		query += ` AND active=1`
	}
	if opts.Status != "" {
		query += ` AND status=?`
		args = append(args, opts.Status)
	}
	if opts.ContainerName != "" {
		query += ` AND container_name=?`
		args = append(args, opts.ContainerName)
	}
	if opts.EndpointType != "" {
		query += ` AND endpoint_type=?`
		args = append(args, opts.EndpointType)
	}
	if opts.Source != "" {
		query += ` AND source=?`
		args = append(args, opts.Source)
	}
	if opts.AgentFilter != nil {
		query += ` AND agent_id=?`
		if *opts.AgentFilter == "local" {
			args = append(args, uid.LocalAgent)
		} else {
			args = append(args, *opts.AgentFilter)
		}
	}

	query += ` ORDER BY container_name, label_key`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list endpoints: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)

	var endpoints []*endpoint.Endpoint
	for rows.Next() {
		e, err := s.scanEndpointRow(rows)
		if err != nil {
			return nil, err
		}
		endpoints = append(endpoints, e)
	}
	return endpoints, rows.Err()
}

func (s *EndpointStore) ListEndpointsByExternalID(ctx context.Context, externalID string) ([]*endpoint.Endpoint, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+endpointColumns+` FROM endpoints WHERE external_id=? AND active=1 ORDER BY label_key`,
		externalID)
	if err != nil {
		return nil, fmt.Errorf("list endpoints by external_id: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)

	var endpoints []*endpoint.Endpoint
	for rows.Next() {
		e, err := s.scanEndpointRow(rows)
		if err != nil {
			return nil, err
		}
		endpoints = append(endpoints, e)
	}
	return endpoints, rows.Err()
}

func (s *EndpointStore) CountActiveEndpoints(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM endpoints WHERE active=1`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count active endpoints: %w", err)
	}
	return count, nil
}

func (s *EndpointStore) DeactivateEndpoint(ctx context.Context, id string) error {
	_, err := s.writer.Exec(ctx,
		`UPDATE endpoints SET active=0 WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("deactivate endpoint %s: %w", id, err)
	}
	return nil
}

func (s *EndpointStore) UpdateCheckResult(ctx context.Context, id string, status endpoint.EndpointStatus,
	alertState endpoint.AlertState, consecutiveFailures, consecutiveSuccesses int,
	responseTimeMs int64, httpStatus *int, lastError string) error {

	now := time.Now().Unix()
	_, err := s.writer.Exec(ctx,
		`UPDATE endpoints SET status=?, alert_state=?,
			consecutive_failures=?, consecutive_successes=?,
			last_check_at=?, last_response_time_ms=?, last_http_status=?, last_error=?
		WHERE id=?`,
		string(status), string(alertState),
		consecutiveFailures, consecutiveSuccesses,
		now, responseTimeMs, httpStatus, NullableString(lastError),
		id,
	)
	if err != nil {
		return fmt.Errorf("update check result for endpoint %s: %w", id, err)
	}
	return nil
}

func (s *EndpointStore) InsertCheckResult(ctx context.Context, result *endpoint.CheckResult) (string, error) {
	result.ID = uid.New()
	_, err := s.writer.Exec(ctx,
		`INSERT INTO check_results (id, endpoint_id, success, response_time_ms, http_status, error_message, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		result.ID, result.EndpointID, boolToInt(result.Success), result.ResponseTimeMs,
		result.HTTPStatus, NullableString(result.ErrorMessage), result.Timestamp.Unix(),
	)
	if err != nil {
		return "", fmt.Errorf("insert check result: %w", err)
	}
	return result.ID, nil
}

func (s *EndpointStore) ListCheckResults(ctx context.Context, endpointID string, opts endpoint.ListChecksOpts) ([]*endpoint.CheckResult, int, error) {
	countQuery := `SELECT COUNT(*) FROM check_results WHERE endpoint_id=?`
	countArgs := []interface{}{endpointID}

	query := `SELECT id, endpoint_id, success, response_time_ms, http_status, error_message, timestamp
		FROM check_results WHERE endpoint_id=?`
	args := []interface{}{endpointID}

	if opts.Since != nil {
		query += ` AND timestamp>=?`
		args = append(args, opts.Since.Unix())
		countQuery += ` AND timestamp>=?`
		countArgs = append(countArgs, opts.Since.Unix())
	}

	var total int
	if err := s.db.QueryRowContext(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count check results: %w", err)
	}

	query += ` ORDER BY timestamp DESC`
	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	query += fmt.Sprintf(` LIMIT %d`, limit)
	if opts.Offset > 0 {
		query += fmt.Sprintf(` OFFSET %d`, opts.Offset) // #nosec G202 -- integer formatting of an int option.
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list check results: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)

	var results []*endpoint.CheckResult
	for rows.Next() {
		r, err := scanCheckResultRow(rows)
		if err != nil {
			return nil, 0, err
		}
		results = append(results, r)
	}
	return results, total, rows.Err()
}

func (s *EndpointStore) GetCheckResultsInWindow(ctx context.Context, endpointID string, from, to time.Time) (int, int, error) {
	var total, successes int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(success), 0) FROM check_results
		WHERE endpoint_id=? AND timestamp>=? AND timestamp<=?`,
		endpointID, from.Unix(), to.Unix(),
	).Scan(&total, &successes)
	if err != nil {
		return 0, 0, fmt.Errorf("get check results in window: %w", err)
	}
	return total, successes, nil
}

func (s *EndpointStore) DeleteCheckResultsBefore(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	deleted, _, err := s.deleteCheckResultsBefore(ctx, before, batchOpts{batchSize: batchSize})
	return deleted, err
}

func (s *EndpointStore) deleteCheckResultsBefore(ctx context.Context, before time.Time, o batchOpts) (int64, bool, error) {
	return deleteRowsBefore(ctx, s.writer, o, "check_results", "timestamp", before)
}

func (s *EndpointStore) DeleteInactiveEndpointsBefore(ctx context.Context, before time.Time) (int64, error) {
	// First, delete check results for inactive endpoints
	_, err := s.writer.Exec(ctx,
		`DELETE FROM check_results WHERE endpoint_id IN (
			SELECT id FROM endpoints WHERE active=0 AND last_seen_at<?
		)`, before.Unix())
	if err != nil {
		return 0, fmt.Errorf("delete check results for inactive endpoints: %w", err)
	}

	res, err := s.writer.Exec(ctx,
		`DELETE FROM endpoints WHERE active=0 AND last_seen_at<?`, before.Unix())
	if err != nil {
		return 0, fmt.Errorf("delete inactive endpoints: %w", err)
	}
	return res.RowsAffected, nil
}

// InsertStandaloneEndpoint creates a manually-defined endpoint (not from container labels).
// Its identity is (agent_id, empty container_name, label_key=external_id) where the
// external_id is a freshly minted unique key, so the derived id is stable.
func (s *EndpointStore) InsertStandaloneEndpoint(ctx context.Context, e *endpoint.Endpoint) (string, error) {
	e.AgentID = uid.Agent(e.AgentID)
	configJSON := e.ConfigJSON()
	now := time.Now().Unix()
	externalID := uid.New()
	e.ID = uid.EndpointLabel(e.AgentID, "", externalID)

	_, err := s.writer.Exec(ctx,
		`INSERT INTO endpoints (id, agent_id, container_name, label_key, external_id, endpoint_type, target,
			status, alert_state, consecutive_failures, consecutive_successes,
			config_json, active, first_seen_at, last_seen_at, source, name)
		VALUES (?, ?, '', ?, ?, ?, ?, ?, ?, 0, 0, ?, 1, ?, ?, 'standalone', ?)
		ON CONFLICT(id) DO UPDATE SET
			endpoint_type=excluded.endpoint_type, target=excluded.target,
			config_json=excluded.config_json, active=1, last_seen_at=excluded.last_seen_at, name=excluded.name`,
		e.ID, e.AgentID, externalID, externalID, string(e.EndpointType), e.Target,
		string(endpoint.StatusUnknown), string(endpoint.AlertNormal),
		configJSON, now, now, e.Name,
	)
	if err != nil {
		return "", fmt.Errorf("insert standalone endpoint: %w", err)
	}
	return e.ID, nil
}

// UpdateStandaloneEndpoint updates a standalone endpoint's mutable fields.
func (s *EndpointStore) UpdateStandaloneEndpoint(ctx context.Context, id string, name, target string, endpointType endpoint.EndpointType, configJSON string) error {
	now := time.Now().Unix()
	res, err := s.writer.Exec(ctx,
		`UPDATE endpoints SET name=?, target=?, endpoint_type=?, config_json=?, last_seen_at=?
		WHERE id=? AND source='standalone' AND active=1`,
		name, target, string(endpointType), configJSON, now, id,
	)
	if err != nil {
		return fmt.Errorf("update standalone endpoint %s: %w", id, err)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("standalone endpoint %s not found or not standalone", id)
	}
	return nil
}

// DeleteEndpoint permanently removes an endpoint and its check results,
// whatever its source. The caller decides what may be deleted; see
// endpoint.Service.Delete.
func (s *EndpointStore) DeleteEndpoint(ctx context.Context, id string) error {
	if _, err := s.writer.Exec(ctx, `DELETE FROM check_results WHERE endpoint_id=?`, id); err != nil {
		return fmt.Errorf("delete check results for endpoint %s: %w", id, err)
	}

	res, err := s.writer.Exec(ctx, `DELETE FROM endpoints WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("delete endpoint %s: %w", id, err)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("endpoint %s not found", id)
	}
	return nil
}

// DeleteStandaloneEndpoint permanently removes a standalone endpoint and its check results.
func (s *EndpointStore) DeleteStandaloneEndpoint(ctx context.Context, id string) error {
	// Delete check results first
	_, err := s.writer.Exec(ctx, `DELETE FROM check_results WHERE endpoint_id=?`, id)
	if err != nil {
		return fmt.Errorf("delete check results for standalone endpoint %s: %w", id, err)
	}

	res, err := s.writer.Exec(ctx,
		`DELETE FROM endpoints WHERE id=? AND source='standalone'`, id)
	if err != nil {
		return fmt.Errorf("delete standalone endpoint %s: %w", id, err)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("standalone endpoint %s not found or not standalone", id)
	}
	return nil
}

// --- Scanners ---

func (s *EndpointStore) scanEndpoint(row rowScanner) (*endpoint.Endpoint, error) {
	var e endpoint.Endpoint
	var lastCheckAt, lastResponseTimeMs, lastHTTPStatus sql.NullInt64
	var lastError sql.NullString
	var configJSON string
	var active int
	var firstSeen, lastSeen int64
	var source, name string

	err := row.Scan(
		&e.ID, &e.AgentID, &e.ContainerName, &e.LabelKey, &e.ExternalID,
		&e.EndpointType, &e.Target,
		&e.Status, &e.AlertState,
		&e.ConsecutiveFailures, &e.ConsecutiveSuccesses,
		&lastCheckAt, &lastResponseTimeMs, &lastHTTPStatus, &lastError,
		&configJSON, &active, &firstSeen, &lastSeen,
		&source, &name,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("scan endpoint: %w", err)
	}

	e.Active = active != 0
	e.FirstSeenAt = time.Unix(firstSeen, 0)
	e.LastSeenAt = time.Unix(lastSeen, 0)
	e.Source = endpoint.EndpointSource(source)
	e.Name = name

	if lastCheckAt.Valid {
		t := time.Unix(lastCheckAt.Int64, 0)
		e.LastCheckAt = &t
	}
	if lastResponseTimeMs.Valid {
		v := lastResponseTimeMs.Int64
		e.LastResponseTimeMs = &v
	}
	if lastHTTPStatus.Valid {
		v := int(lastHTTPStatus.Int64)
		e.LastHTTPStatus = &v
	}
	if lastError.Valid {
		e.LastError = lastError.String
	}

	// Parse config JSON (best-effort: keep the defaults set above on parse error).
	e.Config = endpoint.DefaultConfig()
	if configJSON != "" && configJSON != "{}" {
		_ = json.Unmarshal([]byte(configJSON), &e.Config)
	}

	return &e, nil
}

func (s *EndpointStore) scanEndpointRow(rows *sql.Rows) (*endpoint.Endpoint, error) {
	return s.scanEndpoint(rows)
}

func scanCheckResultRow(row rowScanner) (*endpoint.CheckResult, error) {
	var r endpoint.CheckResult
	var success int
	var httpStatus sql.NullInt64
	var errorMessage sql.NullString
	var ts int64

	err := row.Scan(
		&r.ID, &r.EndpointID, &success, &r.ResponseTimeMs,
		&httpStatus, &errorMessage, &ts,
	)
	if err != nil {
		return nil, fmt.Errorf("scan check result: %w", err)
	}

	r.Success = success != 0
	r.Timestamp = time.Unix(ts, 0)
	if httpStatus.Valid {
		v := int(httpStatus.Int64)
		r.HTTPStatus = &v
	}
	if errorMessage.Valid {
		r.ErrorMessage = errorMessage.String
	}

	return &r, nil
}

// GetSparklineData returns the last N response_time_ms values per active endpoint.
func (s *EndpointStore) GetSparklineData(ctx context.Context, limit int) (map[string][]float64, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT endpoint_id, response_time_ms
		FROM (
			SELECT endpoint_id, response_time_ms,
				ROW_NUMBER() OVER (PARTITION BY endpoint_id ORDER BY timestamp DESC) AS rn
			FROM check_results
			WHERE endpoint_id IN (SELECT id FROM endpoints WHERE active=1)
				AND response_time_ms IS NOT NULL
		)
		WHERE rn <= ?
		ORDER BY endpoint_id, rn DESC
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("get sparkline data: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)

	result := make(map[string][]float64)
	for rows.Next() {
		var epID string
		var ms float64
		if err := rows.Scan(&epID, &ms); err != nil {
			return nil, fmt.Errorf("scan sparkline row: %w", err)
		}
		result[epID] = append(result[epID], ms)
	}
	return result, rows.Err()
}

// CountConfigured returns the number of operator-configured active endpoints.
// Soft-deleted (active=0) entries pending retention cleanup are excluded.
// Used by the telemetry subsystem; see specs/015-shm-telemetry.
func (s *EndpointStore) CountConfigured(ctx context.Context) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM endpoints WHERE active = 1`,
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("count configured endpoints: %w", err)
	}
	return count, nil
}
