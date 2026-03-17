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
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/kolapsis/maintenant/internal/endpoint"
)

// EndpointStore implements endpoint.EndpointStore using SQLite.
type EndpointStore struct {
	db     *sql.DB
	writer *Writer
}

// NewEndpointStore creates a new SQLite-backed endpoint store.
func NewEndpointStore(d *DB) *EndpointStore {
	return &EndpointStore{
		db:     d.ReadDB(),
		writer: d.Writer(),
	}
}

const endpointColumns = `id, container_name, label_key, external_id, endpoint_type, target,
	status, alert_state, consecutive_failures, consecutive_successes,
	last_check_at, last_response_time_ms, last_http_status, last_error,
	config_json, active, first_seen_at, last_seen_at, source, name`

func (s *EndpointStore) UpsertEndpoint(ctx context.Context, e *endpoint.Endpoint) (int64, error) {
	configJSON := e.ConfigJSON()
	now := time.Now().Unix()

	// Try to find existing by identity
	existing, err := s.GetEndpointByIdentity(ctx, e.ContainerName, e.LabelKey)
	if err != nil {
		return 0, err
	}

	if existing != nil {
		// Update existing endpoint
		_, err := s.writer.Exec(ctx,
			`UPDATE endpoints SET external_id=?, endpoint_type=?, target=?, config_json=?,
				active=1, last_seen_at=?
			WHERE id=?`,
			e.ExternalID, string(e.EndpointType), e.Target, configJSON,
			now, existing.ID,
		)
		if err != nil {
			return 0, fmt.Errorf("update endpoint: %w", err)
		}
		e.ID = existing.ID
		return existing.ID, nil
	}

	// Insert new endpoint
	firstSeen := now
	if !e.FirstSeenAt.IsZero() {
		firstSeen = e.FirstSeenAt.Unix()
	}
	source := string(e.Source)
	if source == "" {
		source = string(endpoint.SourceLabel)
	}
	res, err := s.writer.Exec(ctx,
		`INSERT INTO endpoints (container_name, label_key, external_id, endpoint_type, target,
			status, alert_state, consecutive_failures, consecutive_successes,
			config_json, active, first_seen_at, last_seen_at, source, name)
		VALUES (?, ?, ?, ?, ?, ?, ?, 0, 0, ?, 1, ?, ?, ?, ?)`,
		e.ContainerName, e.LabelKey, e.ExternalID, string(e.EndpointType), e.Target,
		string(endpoint.StatusUnknown), string(endpoint.AlertNormal),
		configJSON, firstSeen, now, source, e.Name,
	)
	if err != nil {
		return 0, fmt.Errorf("insert endpoint: %w", err)
	}
	e.ID = res.LastInsertID
	return res.LastInsertID, nil
}

func (s *EndpointStore) GetEndpointByIdentity(ctx context.Context, containerName, labelKey string) (*endpoint.Endpoint, error) {
	return s.scanEndpoint(s.db.QueryRowContext(ctx,
		`SELECT `+endpointColumns+` FROM endpoints WHERE container_name=? AND label_key=?`,
		containerName, labelKey))
}

func (s *EndpointStore) GetEndpointByID(ctx context.Context, id int64) (*endpoint.Endpoint, error) {
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

func (s *EndpointStore) DeactivateEndpoint(ctx context.Context, id int64) error {
	_, err := s.writer.Exec(ctx,
		`UPDATE endpoints SET active=0 WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("deactivate endpoint %d: %w", id, err)
	}
	return nil
}

func (s *EndpointStore) UpdateCheckResult(ctx context.Context, id int64, status endpoint.EndpointStatus,
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
		return fmt.Errorf("update check result for endpoint %d: %w", id, err)
	}
	return nil
}

func (s *EndpointStore) InsertCheckResult(ctx context.Context, result *endpoint.CheckResult) (int64, error) {
	res, err := s.writer.Exec(ctx,
		`INSERT INTO check_results (endpoint_id, success, response_time_ms, http_status, error_message, timestamp)
		VALUES (?, ?, ?, ?, ?, ?)`,
		result.EndpointID, boolToInt(result.Success), result.ResponseTimeMs,
		result.HTTPStatus, NullableString(result.ErrorMessage), result.Timestamp.Unix(),
	)
	if err != nil {
		return 0, fmt.Errorf("insert check result: %w", err)
	}
	result.ID = res.LastInsertID
	return res.LastInsertID, nil
}

func (s *EndpointStore) ListCheckResults(ctx context.Context, endpointID int64, opts endpoint.ListChecksOpts) ([]*endpoint.CheckResult, int, error) {
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
		query += fmt.Sprintf(` OFFSET %d`, opts.Offset)
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

func (s *EndpointStore) GetCheckResultsInWindow(ctx context.Context, endpointID int64, from, to time.Time) (int, int, error) {
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
	var totalDeleted int64
	for {
		res, err := s.writer.Exec(ctx,
			`DELETE FROM check_results WHERE rowid IN (
				SELECT rowid FROM check_results WHERE timestamp<? LIMIT ?
			)`, before.Unix(), batchSize)
		if err != nil {
			return totalDeleted, fmt.Errorf("delete check results: %w", err)
		}
		totalDeleted += res.RowsAffected
		if res.RowsAffected < int64(batchSize) {
			break
		}
	}
	return totalDeleted, nil
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
func (s *EndpointStore) InsertStandaloneEndpoint(ctx context.Context, e *endpoint.Endpoint) (int64, error) {
	configJSON := e.ConfigJSON()
	now := time.Now().Unix()

	res, err := s.writer.Exec(ctx,
		`INSERT INTO endpoints (container_name, label_key, external_id, endpoint_type, target,
			status, alert_state, consecutive_failures, consecutive_successes,
			config_json, active, first_seen_at, last_seen_at, source, name)
		VALUES ('', '', '', ?, ?, ?, ?, 0, 0, ?, 1, ?, ?, 'standalone', ?)`,
		string(e.EndpointType), e.Target,
		string(endpoint.StatusUnknown), string(endpoint.AlertNormal),
		configJSON, now, now, e.Name,
	)
	if err != nil {
		return 0, fmt.Errorf("insert standalone endpoint: %w", err)
	}
	return res.LastInsertID, nil
}

// UpdateStandaloneEndpoint updates a standalone endpoint's mutable fields.
func (s *EndpointStore) UpdateStandaloneEndpoint(ctx context.Context, id int64, name, target string, endpointType endpoint.EndpointType, configJSON string) error {
	now := time.Now().Unix()
	res, err := s.writer.Exec(ctx,
		`UPDATE endpoints SET name=?, target=?, endpoint_type=?, config_json=?, last_seen_at=?
		WHERE id=? AND source='standalone' AND active=1`,
		name, target, string(endpointType), configJSON, now, id,
	)
	if err != nil {
		return fmt.Errorf("update standalone endpoint %d: %w", id, err)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("standalone endpoint %d not found or not standalone", id)
	}
	return nil
}

// DeleteStandaloneEndpoint permanently removes a standalone endpoint and its check results.
func (s *EndpointStore) DeleteStandaloneEndpoint(ctx context.Context, id int64) error {
	// Delete check results first
	_, err := s.writer.Exec(ctx, `DELETE FROM check_results WHERE endpoint_id=?`, id)
	if err != nil {
		return fmt.Errorf("delete check results for standalone endpoint %d: %w", id, err)
	}

	res, err := s.writer.Exec(ctx,
		`DELETE FROM endpoints WHERE id=? AND source='standalone'`, id)
	if err != nil {
		return fmt.Errorf("delete standalone endpoint %d: %w", id, err)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("standalone endpoint %d not found or not standalone", id)
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
		&e.ID, &e.ContainerName, &e.LabelKey, &e.ExternalID,
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

	// Parse config JSON
	e.Config = endpoint.DefaultConfig()
	if configJSON != "" && configJSON != "{}" {
		if err := json.Unmarshal([]byte(configJSON), &e.Config); err != nil {
			// Use defaults on parse error
		}
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
func (s *EndpointStore) GetSparklineData(ctx context.Context, limit int) (map[int64][]float64, error) {
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

	result := make(map[int64][]float64)
	for rows.Next() {
		var epID int64
		var ms float64
		if err := rows.Scan(&epID, &ms); err != nil {
			return nil, fmt.Errorf("scan sparkline row: %w", err)
		}
		result[epID] = append(result[epID], ms)
	}
	return result, rows.Err()
}
