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

	"github.com/kolapsis/maintenant/internal/certificate"
)

// CertificateStore implements certificate.CertificateStore using SQLite.
type CertificateStore struct {
	db     *sql.DB
	writer *Writer
}

// NewCertificateStore creates a new SQLite-backed certificate store.
func NewCertificateStore(d *DB) *CertificateStore {
	return &CertificateStore{
		db:     d.ReadDB(),
		writer: d.Writer(),
	}
}

const certMonitorColumns = `id, hostname, port, source, endpoint_id, status,
	check_interval_seconds, warning_thresholds_json, last_alerted_threshold,
	last_check_at, next_check_at, last_error, active, created_at, external_id`

func (s *CertificateStore) CreateMonitor(ctx context.Context, m *certificate.CertMonitor) (int64, error) {
	now := time.Now().Unix()
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
	}

	thresholdsJSON := m.WarningThresholdsJSON()

	var endpointID interface{}
	if m.EndpointID != nil {
		endpointID = *m.EndpointID
	}

	var nextCheckAt interface{}
	if m.NextCheckAt != nil {
		nextCheckAt = m.NextCheckAt.Unix()
	}

	res, err := s.writer.Exec(ctx,
		`INSERT INTO cert_monitors (hostname, port, source, endpoint_id, status,
			check_interval_seconds, warning_thresholds_json, last_alerted_threshold,
			last_check_at, next_check_at, last_error, active, created_at, external_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, NULL, NULL, ?, NULL, 1, ?, ?)`,
		m.Hostname, m.Port, string(m.Source), endpointID, string(m.Status),
		m.CheckIntervalSeconds, thresholdsJSON,
		nextCheckAt, now, m.ExternalID,
	)
	if err != nil {
		return 0, fmt.Errorf("insert cert monitor: %w", err)
	}
	m.ID = res.LastInsertID
	return res.LastInsertID, nil
}

func (s *CertificateStore) GetMonitorByID(ctx context.Context, id int64) (*certificate.CertMonitor, error) {
	return s.scanMonitor(s.db.QueryRowContext(ctx,
		`SELECT `+certMonitorColumns+` FROM cert_monitors WHERE id=?`, id))
}

func (s *CertificateStore) GetMonitorByHostPort(ctx context.Context, hostname string, port int) (*certificate.CertMonitor, error) {
	return s.scanMonitor(s.db.QueryRowContext(ctx,
		`SELECT `+certMonitorColumns+` FROM cert_monitors WHERE hostname=? AND port=?`,
		hostname, port))
}

func (s *CertificateStore) GetMonitorByEndpointID(ctx context.Context, endpointID int64) (*certificate.CertMonitor, error) {
	return s.scanMonitor(s.db.QueryRowContext(ctx,
		`SELECT `+certMonitorColumns+` FROM cert_monitors WHERE endpoint_id=? AND active=1`,
		endpointID))
}

func (s *CertificateStore) ListMonitors(ctx context.Context, opts certificate.ListCertificatesOpts) ([]*certificate.CertMonitor, error) {
	query := `SELECT ` + certMonitorColumns + ` FROM cert_monitors WHERE active=1`
	var args []interface{}

	if opts.Status != "" {
		query += ` AND status=?`
		args = append(args, opts.Status)
	}
	if opts.Source != "" {
		query += ` AND source=?`
		args = append(args, opts.Source)
	}

	query += ` ORDER BY hostname, port`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list cert monitors: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)

	var monitors []*certificate.CertMonitor
	for rows.Next() {
		m, err := s.scanMonitorRow(rows)
		if err != nil {
			return nil, err
		}
		monitors = append(monitors, m)
	}
	return monitors, rows.Err()
}

func (s *CertificateStore) CountStandaloneMonitors(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM cert_monitors WHERE source='standalone' AND active=1`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count standalone cert monitors: %w", err)
	}
	return count, nil
}

func (s *CertificateStore) UpdateMonitor(ctx context.Context, m *certificate.CertMonitor) error {
	thresholdsJSON := m.WarningThresholdsJSON()

	var lastCheckAt interface{}
	if m.LastCheckAt != nil {
		lastCheckAt = m.LastCheckAt.Unix()
	}
	var nextCheckAt interface{}
	if m.NextCheckAt != nil {
		nextCheckAt = m.NextCheckAt.Unix()
	}

	_, err := s.writer.Exec(ctx,
		`UPDATE cert_monitors SET status=?, check_interval_seconds=?,
			warning_thresholds_json=?, last_alerted_threshold=?,
			last_check_at=?, next_check_at=?, last_error=?
		WHERE id=?`,
		string(m.Status), m.CheckIntervalSeconds,
		thresholdsJSON, m.LastAlertedThreshold,
		lastCheckAt, nextCheckAt, NullableString(m.LastError),
		m.ID,
	)
	if err != nil {
		return fmt.Errorf("update cert monitor %d: %w", m.ID, err)
	}
	return nil
}

func (s *CertificateStore) SoftDeleteMonitor(ctx context.Context, id int64) error {
	_, err := s.writer.Exec(ctx,
		`UPDATE cert_monitors SET active=0 WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("soft delete cert monitor %d: %w", id, err)
	}
	return nil
}

func (s *CertificateStore) ReactivateMonitor(ctx context.Context, id int64, m *certificate.CertMonitor) error {
	thresholdsJSON := m.WarningThresholdsJSON()
	var endpointID interface{}
	if m.EndpointID != nil {
		endpointID = *m.EndpointID
	}
	_, err := s.writer.Exec(ctx,
		`UPDATE cert_monitors SET active=1, source=?, endpoint_id=?, status=?,
			check_interval_seconds=?, warning_thresholds_json=?,
			last_alerted_threshold=NULL, last_check_at=NULL, next_check_at=NULL,
			last_error=NULL
		WHERE id=?`,
		string(m.Source), endpointID, string(m.Status),
		m.CheckIntervalSeconds, thresholdsJSON,
		id)
	if err != nil {
		return fmt.Errorf("reactivate cert monitor %d: %w", id, err)
	}
	return nil
}

// --- Check results ---

func (s *CertificateStore) InsertCheckResult(ctx context.Context, result *certificate.CertCheckResult) (int64, error) {
	var notBefore, notAfter interface{}
	if result.NotBefore != nil {
		notBefore = result.NotBefore.Unix()
	}
	if result.NotAfter != nil {
		notAfter = result.NotAfter.Unix()
	}

	var chainValid, hostnameMatch interface{}
	if result.ChainValid != nil {
		chainValid = boolToInt(*result.ChainValid)
	}
	if result.HostnameMatch != nil {
		hostnameMatch = boolToInt(*result.HostnameMatch)
	}

	sansJSON := result.SANsJSON()

	res, err := s.writer.Exec(ctx,
		`INSERT INTO cert_check_results (monitor_id, subject_cn, issuer_cn, issuer_org,
			sans_json, serial_number, signature_algorithm, not_before, not_after,
			chain_valid, chain_error, hostname_match, error_message, checked_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		result.MonitorID, NullableString(result.SubjectCN), NullableString(result.IssuerCN),
		NullableString(result.IssuerOrg), sansJSON, NullableString(result.SerialNumber),
		NullableString(result.SignatureAlgorithm), notBefore, notAfter,
		chainValid, NullableString(result.ChainError), hostnameMatch,
		NullableString(result.ErrorMessage), result.CheckedAt.Unix(),
	)
	if err != nil {
		return 0, fmt.Errorf("insert cert check result: %w", err)
	}
	result.ID = res.LastInsertID
	return res.LastInsertID, nil
}

func (s *CertificateStore) GetLatestCheckResult(ctx context.Context, monitorID int64) (*certificate.CertCheckResult, error) {
	return s.scanCheckResult(s.db.QueryRowContext(ctx,
		`SELECT id, monitor_id, subject_cn, issuer_cn, issuer_org, sans_json,
			serial_number, signature_algorithm, not_before, not_after,
			chain_valid, chain_error, hostname_match, error_message, checked_at
		FROM cert_check_results WHERE monitor_id=? ORDER BY checked_at DESC LIMIT 1`,
		monitorID))
}

func (s *CertificateStore) ListCheckResults(ctx context.Context, monitorID int64, opts certificate.ListChecksOpts) ([]*certificate.CertCheckResult, int, error) {
	var total int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM cert_check_results WHERE monitor_id=?`, monitorID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count cert check results: %w", err)
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	query := `SELECT id, monitor_id, subject_cn, issuer_cn, issuer_org, sans_json,
		serial_number, signature_algorithm, not_before, not_after,
		chain_valid, chain_error, hostname_match, error_message, checked_at
	FROM cert_check_results WHERE monitor_id=? ORDER BY checked_at DESC`
	query += fmt.Sprintf(` LIMIT %d`, limit)
	if opts.Offset > 0 {
		query += fmt.Sprintf(` OFFSET %d`, opts.Offset)
	}

	rows, err := s.db.QueryContext(ctx, query, monitorID)
	if err != nil {
		return nil, 0, fmt.Errorf("list cert check results: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)

	var results []*certificate.CertCheckResult
	for rows.Next() {
		r, err := s.scanCheckResultRow(rows)
		if err != nil {
			return nil, 0, err
		}
		results = append(results, r)
	}
	return results, total, rows.Err()
}

// --- Chain entries ---

func (s *CertificateStore) InsertChainEntries(ctx context.Context, entries []*certificate.CertChainEntry) error {
	for _, e := range entries {
		_, err := s.writer.Exec(ctx,
			`INSERT INTO cert_chain_entries (check_result_id, position, subject_cn, issuer_cn, not_before, not_after)
			VALUES (?, ?, ?, ?, ?, ?)`,
			e.CheckResultID, e.Position, e.SubjectCN, e.IssuerCN,
			e.NotBefore.Unix(), e.NotAfter.Unix(),
		)
		if err != nil {
			return fmt.Errorf("insert chain entry: %w", err)
		}
	}
	return nil
}

func (s *CertificateStore) GetChainEntries(ctx context.Context, checkResultID int64) ([]*certificate.CertChainEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, check_result_id, position, subject_cn, issuer_cn, not_before, not_after
		FROM cert_chain_entries WHERE check_result_id=? ORDER BY position`,
		checkResultID)
	if err != nil {
		return nil, fmt.Errorf("get chain entries: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)

	var entries []*certificate.CertChainEntry
	for rows.Next() {
		var e certificate.CertChainEntry
		var notBefore, notAfter int64
		if err := rows.Scan(&e.ID, &e.CheckResultID, &e.Position, &e.SubjectCN, &e.IssuerCN, &notBefore, &notAfter); err != nil {
			return nil, fmt.Errorf("scan chain entry: %w", err)
		}
		e.NotBefore = time.Unix(notBefore, 0)
		e.NotAfter = time.Unix(notAfter, 0)
		entries = append(entries, &e)
	}
	return entries, rows.Err()
}

// --- Scheduler ---

func (s *CertificateStore) ListDueScheduledMonitors(ctx context.Context, now time.Time) ([]*certificate.CertMonitor, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+certMonitorColumns+` FROM cert_monitors
		WHERE source IN ('standalone','label') AND active=1 AND (next_check_at IS NULL OR next_check_at<=?)
		ORDER BY next_check_at`,
		now.Unix())
	if err != nil {
		return nil, fmt.Errorf("list due scheduled monitors: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)

	var monitors []*certificate.CertMonitor
	for rows.Next() {
		m, err := s.scanMonitorRow(rows)
		if err != nil {
			return nil, err
		}
		monitors = append(monitors, m)
	}
	return monitors, rows.Err()
}

// --- Label-discovered monitors ---

func (s *CertificateStore) ListMonitorsByExternalID(ctx context.Context, externalID string) ([]*certificate.CertMonitor, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+certMonitorColumns+` FROM cert_monitors WHERE external_id=? AND source='label' AND active=1`,
		externalID)
	if err != nil {
		return nil, fmt.Errorf("list monitors by external_id: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)

	var monitors []*certificate.CertMonitor
	for rows.Next() {
		m, err := s.scanMonitorRow(rows)
		if err != nil {
			return nil, err
		}
		monitors = append(monitors, m)
	}
	return monitors, rows.Err()
}

func (s *CertificateStore) DeactivateMonitor(ctx context.Context, id int64) error {
	_, err := s.writer.Exec(ctx,
		`UPDATE cert_monitors SET active=0 WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("deactivate cert monitor %d: %w", id, err)
	}
	return nil
}

// --- Retention ---

func (s *CertificateStore) DeleteCheckResultsBefore(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	var totalDeleted int64
	for {
		// First, delete chain entries for the check results we're about to delete
		_, err := s.writer.Exec(ctx,
			`DELETE FROM cert_chain_entries WHERE check_result_id IN (
				SELECT id FROM cert_check_results WHERE checked_at<? LIMIT ?
			)`, before.Unix(), batchSize)
		if err != nil {
			return totalDeleted, fmt.Errorf("delete cert chain entries: %w", err)
		}

		res, err := s.writer.Exec(ctx,
			`DELETE FROM cert_check_results WHERE rowid IN (
				SELECT rowid FROM cert_check_results WHERE checked_at<? LIMIT ?
			)`, before.Unix(), batchSize)
		if err != nil {
			return totalDeleted, fmt.Errorf("delete cert check results: %w", err)
		}
		totalDeleted += res.RowsAffected
		if res.RowsAffected < int64(batchSize) {
			break
		}
	}
	return totalDeleted, nil
}

// --- Scanners ---

func (s *CertificateStore) scanMonitor(row rowScanner) (*certificate.CertMonitor, error) {
	var m certificate.CertMonitor
	var endpointID, lastAlertedThreshold, lastCheckAt, nextCheckAt sql.NullInt64
	var lastError sql.NullString
	var thresholdsJSON string
	var active int
	var createdAt int64

	err := row.Scan(
		&m.ID, &m.Hostname, &m.Port, &m.Source, &endpointID, &m.Status,
		&m.CheckIntervalSeconds, &thresholdsJSON, &lastAlertedThreshold,
		&lastCheckAt, &nextCheckAt, &lastError, &active, &createdAt,
		&m.ExternalID,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("scan cert monitor: %w", err)
	}

	m.Active = active != 0
	m.CreatedAt = time.Unix(createdAt, 0)

	if endpointID.Valid {
		v := endpointID.Int64
		m.EndpointID = &v
	}
	if lastAlertedThreshold.Valid {
		v := int(lastAlertedThreshold.Int64)
		m.LastAlertedThreshold = &v
	}
	if lastCheckAt.Valid {
		t := time.Unix(lastCheckAt.Int64, 0)
		m.LastCheckAt = &t
	}
	if nextCheckAt.Valid {
		t := time.Unix(nextCheckAt.Int64, 0)
		m.NextCheckAt = &t
	}
	if lastError.Valid {
		m.LastError = lastError.String
	}

	// Parse warning thresholds
	m.WarningThresholds = certificate.DefaultWarningThresholds()
	if thresholdsJSON != "" {
		if err := json.Unmarshal([]byte(thresholdsJSON), &m.WarningThresholds); err != nil {
			// Use defaults on parse error
		}
	}

	return &m, nil
}

func (s *CertificateStore) scanMonitorRow(rows *sql.Rows) (*certificate.CertMonitor, error) {
	return s.scanMonitor(rows)
}

func (s *CertificateStore) scanCheckResult(row rowScanner) (*certificate.CertCheckResult, error) {
	var r certificate.CertCheckResult
	var subjectCN, issuerCN, issuerOrg, sansJSON, serialNumber, sigAlgo sql.NullString
	var notBefore, notAfter sql.NullInt64
	var chainValid, hostnameMatch sql.NullInt64
	var chainError, errorMessage sql.NullString
	var checkedAt int64

	err := row.Scan(
		&r.ID, &r.MonitorID, &subjectCN, &issuerCN, &issuerOrg, &sansJSON,
		&serialNumber, &sigAlgo, &notBefore, &notAfter,
		&chainValid, &chainError, &hostnameMatch, &errorMessage, &checkedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("scan cert check result: %w", err)
	}

	r.CheckedAt = time.Unix(checkedAt, 0)

	if subjectCN.Valid {
		r.SubjectCN = subjectCN.String
	}
	if issuerCN.Valid {
		r.IssuerCN = issuerCN.String
	}
	if issuerOrg.Valid {
		r.IssuerOrg = issuerOrg.String
	}
	if sansJSON.Valid && sansJSON.String != "" {
		_ = json.Unmarshal([]byte(sansJSON.String), &r.SANs)
	}
	if serialNumber.Valid {
		r.SerialNumber = serialNumber.String
	}
	if sigAlgo.Valid {
		r.SignatureAlgorithm = sigAlgo.String
	}
	if notBefore.Valid {
		t := time.Unix(notBefore.Int64, 0)
		r.NotBefore = &t
	}
	if notAfter.Valid {
		t := time.Unix(notAfter.Int64, 0)
		r.NotAfter = &t
	}
	if chainValid.Valid {
		v := chainValid.Int64 != 0
		r.ChainValid = &v
	}
	if chainError.Valid {
		r.ChainError = chainError.String
	}
	if hostnameMatch.Valid {
		v := hostnameMatch.Int64 != 0
		r.HostnameMatch = &v
	}
	if errorMessage.Valid {
		r.ErrorMessage = errorMessage.String
	}

	return &r, nil
}

func (s *CertificateStore) scanCheckResultRow(rows *sql.Rows) (*certificate.CertCheckResult, error) {
	return s.scanCheckResult(rows)
}
