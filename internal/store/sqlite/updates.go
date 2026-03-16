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
	"strings"
	"time"

	"github.com/kolapsis/maintenant/internal/update"
)

// UpdateStore implements update.UpdateStore using SQLite.
type UpdateStore struct {
	db     *sql.DB
	writer *Writer
}

// NewUpdateStore creates a new SQLite-backed update store.
func NewUpdateStore(d *DB) *UpdateStore {
	return &UpdateStore{
		db:     d.ReadDB(),
		writer: d.Writer(),
	}
}

// --- Scan records ---

func (s *UpdateStore) InsertScanRecord(ctx context.Context, r *update.ScanRecord) (int64, error) {
	res, err := s.writer.Exec(ctx,
		`INSERT INTO image_update_scans (started_at, containers_scanned, updates_found, errors, status)
		VALUES (?, ?, ?, ?, ?)`,
		r.StartedAt.Unix(), r.ContainersScanned, r.UpdatesFound, r.Errors, string(r.Status),
	)
	if err != nil {
		return 0, fmt.Errorf("insert scan record: %w", err)
	}
	r.ID = res.LastInsertID
	return res.LastInsertID, nil
}

func (s *UpdateStore) UpdateScanRecord(ctx context.Context, r *update.ScanRecord) error {
	var completedAt *int64
	if r.CompletedAt != nil {
		v := r.CompletedAt.Unix()
		completedAt = &v
	}
	_, err := s.writer.Exec(ctx,
		`UPDATE image_update_scans SET completed_at=?, containers_scanned=?, updates_found=?, errors=?, status=? WHERE id=?`,
		completedAt, r.ContainersScanned, r.UpdatesFound, r.Errors, string(r.Status), r.ID,
	)
	if err != nil {
		return fmt.Errorf("update scan record: %w", err)
	}
	return nil
}

func (s *UpdateStore) GetScanRecord(ctx context.Context, id int64) (*update.ScanRecord, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, started_at, completed_at, containers_scanned, updates_found, errors, status
		FROM image_update_scans WHERE id = ?`, id)
	return scanScanRecord(row)
}

func (s *UpdateStore) GetLatestScanRecord(ctx context.Context) (*update.ScanRecord, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, started_at, completed_at, containers_scanned, updates_found, errors, status
		FROM image_update_scans ORDER BY started_at DESC LIMIT 1`)
	return scanScanRecord(row)
}

// --- Image updates ---

func (s *UpdateStore) InsertImageUpdate(ctx context.Context, u *update.ImageUpdate) (int64, error) {
	var publishedAt *int64
	if u.PublishedAt != nil {
		v := u.PublishedAt.Unix()
		publishedAt = &v
	}
	res, err := s.writer.Exec(ctx,
		`INSERT INTO image_updates
		(scan_id, container_id, container_name, image, current_tag, current_digest, registry,
		 latest_tag, latest_digest, update_type, risk_score, published_at,
		 changelog_url, changelog_summary, has_breaking_changes, previous_digest, source_url,
		 status, detected_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(container_name, image, latest_tag) DO UPDATE SET
			scan_id=excluded.scan_id,
			container_id=excluded.container_id,
			current_tag=excluded.current_tag,
			current_digest=excluded.current_digest,
			latest_digest=excluded.latest_digest,
			update_type=excluded.update_type,
			risk_score=excluded.risk_score,
			published_at=excluded.published_at,
			changelog_url=excluded.changelog_url,
			changelog_summary=excluded.changelog_summary,
			has_breaking_changes=excluded.has_breaking_changes,
			previous_digest=excluded.previous_digest,
			source_url=excluded.source_url,
			detected_at=excluded.detected_at`,
		u.ScanID, u.ContainerID, u.ContainerName, u.Image, u.CurrentTag, u.CurrentDigest, u.Registry,
		u.LatestTag, u.LatestDigest, string(u.UpdateType), u.RiskScore, publishedAt,
		u.ChangelogURL, u.ChangelogSummary, boolToInt(u.HasBreakingChanges), u.PreviousDigest, u.SourceURL,
		string(u.Status), u.DetectedAt.Unix(),
	)
	if err != nil {
		return 0, fmt.Errorf("insert image update: %w", err)
	}
	u.ID = res.LastInsertID
	return res.LastInsertID, nil
}

func (s *UpdateStore) UpdateImageUpdate(ctx context.Context, u *update.ImageUpdate) error {
	var publishedAt *int64
	if u.PublishedAt != nil {
		v := u.PublishedAt.Unix()
		publishedAt = &v
	}
	_, err := s.writer.Exec(ctx,
		`UPDATE image_updates SET
		 scan_id=?, container_name=?, image=?, current_tag=?, current_digest=?, registry=?,
		 latest_tag=?, latest_digest=?, update_type=?, risk_score=?, published_at=?,
		 changelog_url=?, changelog_summary=?, has_breaking_changes=?, previous_digest=?, source_url=?,
		 status=?
		WHERE id=?`,
		u.ScanID, u.ContainerName, u.Image, u.CurrentTag, u.CurrentDigest, u.Registry,
		u.LatestTag, u.LatestDigest, string(u.UpdateType), u.RiskScore, publishedAt,
		u.ChangelogURL, u.ChangelogSummary, boolToInt(u.HasBreakingChanges), u.PreviousDigest, u.SourceURL,
		string(u.Status), u.ID,
	)
	if err != nil {
		return fmt.Errorf("update image update: %w", err)
	}
	return nil
}

func (s *UpdateStore) GetImageUpdate(ctx context.Context, id int64) (*update.ImageUpdate, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, scan_id, container_id, container_name, image, current_tag, current_digest, registry,
		 latest_tag, latest_digest, update_type, risk_score, published_at,
		 changelog_url, changelog_summary, has_breaking_changes, previous_digest, source_url,
		 status, detected_at
		FROM image_updates WHERE id = ?`, id)
	return scanImageUpdate(row)
}

func (s *UpdateStore) GetImageUpdateByContainer(ctx context.Context, containerID string) (*update.ImageUpdate, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, scan_id, container_id, container_name, image, current_tag, current_digest, registry,
		 latest_tag, latest_digest, update_type, risk_score, published_at,
		 changelog_url, changelog_summary, has_breaking_changes, previous_digest, source_url,
		 status, detected_at
		FROM image_updates WHERE container_id = ? ORDER BY detected_at DESC LIMIT 1`, containerID)
	return scanImageUpdate(row)
}

func (s *UpdateStore) ListImageUpdates(ctx context.Context, opts update.ListImageUpdatesOpts) ([]*update.ImageUpdate, error) {
	query := `SELECT id, scan_id, container_id, container_name, image, current_tag, current_digest, registry,
		 latest_tag, latest_digest, update_type, risk_score, published_at,
		 changelog_url, changelog_summary, has_breaking_changes, previous_digest, source_url,
		 status, detected_at
		FROM image_updates WHERE 1=1`
	var args []interface{}

	if opts.Status != "" {
		query += " AND status = ?"
		args = append(args, opts.Status)
	}
	if opts.UpdateType != "" {
		query += " AND update_type = ?"
		args = append(args, opts.UpdateType)
	}
	if opts.MinRisk > 0 {
		query += " AND risk_score >= ?"
		args = append(args, opts.MinRisk)
	}
	query += " ORDER BY detected_at DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list image updates: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)
	return collectImageUpdates(rows)
}

func (s *UpdateStore) GetUpdateSummary(ctx context.Context) (*update.UpdateSummary, error) {
	summary := &update.UpdateSummary{}

	rows, err := s.db.QueryContext(ctx, `SELECT status, risk_score FROM image_updates`)
	if err != nil {
		return nil, fmt.Errorf("get update summary: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)

	tracked := 0
	for rows.Next() {
		var st string
		var riskScore int
		if err := rows.Scan(&st, &riskScore); err != nil {
			return nil, err
		}
		tracked++
		switch st {
		case "pinned":
			summary.Pinned++
		case "available", "dismissed":
			switch {
			case riskScore >= 81:
				summary.Critical++
			case riskScore >= 31:
				summary.Recommended++
			default:
				summary.Available++
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Count active containers that have no pending update (up to date).
	var totalContainers int
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM containers WHERE state = 'running'`).Scan(&totalContainers)
	if err != nil {
		return nil, fmt.Errorf("count containers for up_to_date: %w", err)
	}
	upToDate := totalContainers - tracked
	if upToDate < 0 {
		upToDate = 0
	}
	summary.UpToDate = upToDate

	return summary, nil
}

func (s *UpdateStore) DeleteImageUpdatesByContainer(ctx context.Context, containerID string) error {
	_, err := s.writer.Exec(ctx, `DELETE FROM image_updates WHERE container_id = ?`, containerID)
	if err != nil {
		return fmt.Errorf("delete image updates by container: %w", err)
	}
	return nil
}

func (s *UpdateStore) DeleteStaleImageUpdates(ctx context.Context, scanID int64, scannedContainerNames []string) (int64, error) {
	if len(scannedContainerNames) == 0 {
		return 0, nil
	}
	placeholders := make([]string, len(scannedContainerNames))
	args := make([]interface{}, 0, len(scannedContainerNames)+1)
	args = append(args, scanID)
	for i, name := range scannedContainerNames {
		placeholders[i] = "?"
		args = append(args, name)
	}
	query := fmt.Sprintf(
		`DELETE FROM image_updates WHERE scan_id != ? AND container_name IN (%s)`,
		strings.Join(placeholders, ","),
	)
	res, err := s.writer.Exec(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("delete stale image updates: %w", err)
	}
	return res.RowsAffected, nil
}

// --- CVE cache ---

func (s *UpdateStore) InsertCVECacheEntry(ctx context.Context, e *update.CVECacheEntry) (int64, error) {
	res, err := s.writer.Exec(ctx,
		`INSERT OR REPLACE INTO cve_cache
		(ecosystem, package_name, package_version, cve_id, cvss_score, cvss_vector, severity,
		 summary, fixed_in, references_json, fetched_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.Ecosystem, e.PackageName, e.PackageVersion, e.CVEID, e.CVSSScore, e.CVSSVector,
		string(e.Severity), e.Summary, e.FixedIn, e.ReferencesJSON,
		e.FetchedAt.Unix(), e.ExpiresAt.Unix(),
	)
	if err != nil {
		return 0, fmt.Errorf("insert cve cache entry: %w", err)
	}
	e.ID = res.LastInsertID
	return res.LastInsertID, nil
}

func (s *UpdateStore) GetCVECacheEntries(ctx context.Context, ecosystem, packageName, packageVersion string) ([]*update.CVECacheEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, ecosystem, package_name, package_version, cve_id, cvss_score, cvss_vector,
		 severity, summary, fixed_in, references_json, fetched_at, expires_at
		FROM cve_cache WHERE ecosystem = ? AND package_name = ? AND package_version = ?`,
		ecosystem, packageName, packageVersion)
	if err != nil {
		return nil, fmt.Errorf("get cve cache entries: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)

	var result []*update.CVECacheEntry
	for rows.Next() {
		e, err := scanCVECacheEntry(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

func (s *UpdateStore) IsCVECacheFresh(ctx context.Context, ecosystem, packageName, packageVersion string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM cve_cache
		WHERE ecosystem = ? AND package_name = ? AND package_version = ? AND expires_at > ?`,
		ecosystem, packageName, packageVersion, time.Now().Unix(),
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check cve cache freshness: %w", err)
	}
	return count > 0, nil
}

// --- Container CVEs ---

func (s *UpdateStore) UpsertContainerCVE(ctx context.Context, c *update.ContainerCVE) error {
	_, err := s.writer.Exec(ctx,
		`INSERT INTO container_cves (container_id, cve_id, severity, cvss_score, summary, fixed_in, first_detected_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(container_id, cve_id) DO UPDATE SET
			severity=excluded.severity, cvss_score=excluded.cvss_score,
			summary=excluded.summary, fixed_in=excluded.fixed_in`,
		c.ContainerID, c.CVEID, string(c.Severity), c.CVSSScore, c.Summary, c.FixedIn,
		c.FirstDetectedAt.Unix(),
	)
	if err != nil {
		return fmt.Errorf("upsert container cve: %w", err)
	}
	return nil
}

func (s *UpdateStore) ListContainerCVEs(ctx context.Context, containerID string) ([]*update.ContainerCVE, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, container_id, cve_id, severity, cvss_score, summary, fixed_in, first_detected_at, resolved_at
		FROM container_cves WHERE container_id = ? AND resolved_at IS NULL ORDER BY cvss_score DESC`, containerID)
	if err != nil {
		return nil, fmt.Errorf("list container cves: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)
	return collectContainerCVEs(rows)
}

func (s *UpdateStore) ListAllActiveCVEs(ctx context.Context, opts update.ListCVEsOpts) ([]*update.ContainerCVE, error) {
	query := `SELECT id, container_id, cve_id, severity, cvss_score, summary, fixed_in, first_detected_at, resolved_at
		FROM container_cves WHERE resolved_at IS NULL`
	var args []interface{}

	if opts.Severity != "" {
		query += " AND severity = ?"
		args = append(args, opts.Severity)
	}
	if opts.ContainerID != "" {
		query += " AND container_id = ?"
		args = append(args, opts.ContainerID)
	}
	query += " ORDER BY cvss_score DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list all active cves: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)
	return collectContainerCVEs(rows)
}

func (s *UpdateStore) ResolveContainerCVE(ctx context.Context, containerID, cveID string) error {
	_, err := s.writer.Exec(ctx,
		`UPDATE container_cves SET resolved_at = ? WHERE container_id = ? AND cve_id = ? AND resolved_at IS NULL`,
		time.Now().Unix(), containerID, cveID,
	)
	if err != nil {
		return fmt.Errorf("resolve container cve: %w", err)
	}
	return nil
}

func (s *UpdateStore) DeleteContainerCVEs(ctx context.Context, containerID string) error {
	_, err := s.writer.Exec(ctx, `DELETE FROM container_cves WHERE container_id = ?`, containerID)
	if err != nil {
		return fmt.Errorf("delete container cves: %w", err)
	}
	return nil
}

func (s *UpdateStore) GetCVESummaryCounts(ctx context.Context) (map[string]int, error) {
	counts := map[string]int{"critical": 0, "high": 0, "medium": 0, "low": 0}
	rows, err := s.db.QueryContext(ctx,
		`SELECT severity, COUNT(*) FROM container_cves WHERE resolved_at IS NULL GROUP BY severity`)
	if err != nil {
		return nil, fmt.Errorf("get cve summary counts: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)

	for rows.Next() {
		var sev string
		var count int
		if err := rows.Scan(&sev, &count); err != nil {
			return nil, err
		}
		counts[sev] = count
	}
	return counts, rows.Err()
}

// --- Version pins ---

func (s *UpdateStore) InsertVersionPin(ctx context.Context, p *update.VersionPin) (int64, error) {
	res, err := s.writer.Exec(ctx,
		`INSERT OR REPLACE INTO version_pins (container_id, image, pinned_tag, pinned_digest, reason, pinned_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		p.ContainerID, p.Image, p.PinnedTag, p.PinnedDigest, p.Reason, p.PinnedAt.Unix(),
	)
	if err != nil {
		return 0, fmt.Errorf("insert version pin: %w", err)
	}
	p.ID = res.LastInsertID
	return res.LastInsertID, nil
}

func (s *UpdateStore) GetVersionPin(ctx context.Context, containerID string) (*update.VersionPin, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, container_id, image, pinned_tag, pinned_digest, reason, pinned_at
		FROM version_pins WHERE container_id = ?`, containerID)
	return scanVersionPin(row)
}

func (s *UpdateStore) DeleteVersionPin(ctx context.Context, containerID string) error {
	_, err := s.writer.Exec(ctx, `DELETE FROM version_pins WHERE container_id = ?`, containerID)
	if err != nil {
		return fmt.Errorf("delete version pin: %w", err)
	}
	return nil
}

// --- Update exclusions ---

func (s *UpdateStore) InsertExclusion(ctx context.Context, e *update.UpdateExclusion) (int64, error) {
	res, err := s.writer.Exec(ctx,
		`INSERT INTO update_exclusions (pattern, pattern_type, created_at)
		VALUES (?, ?, ?)`,
		e.Pattern, string(e.PatternType), e.CreatedAt.Unix(),
	)
	if err != nil {
		return 0, fmt.Errorf("insert exclusion: %w", err)
	}
	e.ID = res.LastInsertID
	return res.LastInsertID, nil
}

func (s *UpdateStore) ListExclusions(ctx context.Context) ([]*update.UpdateExclusion, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, pattern, pattern_type, created_at FROM update_exclusions ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list exclusions: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)

	var result []*update.UpdateExclusion
	for rows.Next() {
		var e update.UpdateExclusion
		var createdAt int64
		if err := rows.Scan(&e.ID, &e.Pattern, &e.PatternType, &createdAt); err != nil {
			return nil, err
		}
		e.CreatedAt = time.Unix(createdAt, 0)
		result = append(result, &e)
	}
	return result, rows.Err()
}

func (s *UpdateStore) DeleteExclusion(ctx context.Context, id int64) error {
	_, err := s.writer.Exec(ctx, `DELETE FROM update_exclusions WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete exclusion: %w", err)
	}
	return nil
}

// --- Risk score history ---

func (s *UpdateStore) InsertRiskScoreRecord(ctx context.Context, r *update.RiskScoreRecord) (int64, error) {
	res, err := s.writer.Exec(ctx,
		`INSERT INTO risk_score_history (container_id, score, factors_json, recorded_at)
		VALUES (?, ?, ?, ?)`,
		r.ContainerID, r.Score, r.FactorsJSON, r.RecordedAt.Unix(),
	)
	if err != nil {
		return 0, fmt.Errorf("insert risk score record: %w", err)
	}
	r.ID = res.LastInsertID
	return res.LastInsertID, nil
}

func (s *UpdateStore) ListRiskScoreHistory(ctx context.Context, containerID string, from, to time.Time) ([]*update.RiskScoreRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, container_id, score, factors_json, recorded_at
		FROM risk_score_history
		WHERE container_id = ? AND recorded_at >= ? AND recorded_at <= ?
		ORDER BY recorded_at ASC`,
		containerID, from.Unix(), to.Unix(),
	)
	if err != nil {
		return nil, fmt.Errorf("list risk score history: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)

	var result []*update.RiskScoreRecord
	for rows.Next() {
		var r update.RiskScoreRecord
		var recordedAt int64
		if err := rows.Scan(&r.ID, &r.ContainerID, &r.Score, &r.FactorsJSON, &recordedAt); err != nil {
			return nil, err
		}
		r.RecordedAt = time.Unix(recordedAt, 0)
		result = append(result, &r)
	}
	return result, rows.Err()
}

// --- Retention cleanup ---

func (s *UpdateStore) CleanupExpired(ctx context.Context, olderThan time.Time) (int64, error) {
	var totalDeleted int64
	ts := olderThan.Unix()

	res, err := s.writer.Exec(ctx,
		`DELETE FROM image_update_scans WHERE started_at < ?`, ts)
	if err != nil {
		return 0, fmt.Errorf("cleanup scan records: %w", err)
	}
	totalDeleted += res.RowsAffected

	res, err = s.writer.Exec(ctx,
		`DELETE FROM image_updates WHERE detected_at < ?`, ts)
	if err != nil {
		return totalDeleted, fmt.Errorf("cleanup image updates: %w", err)
	}
	totalDeleted += res.RowsAffected

	// CVE cache uses expiry-based cleanup
	res, err = s.writer.Exec(ctx,
		`DELETE FROM cve_cache WHERE expires_at < ?`, time.Now().Unix())
	if err != nil {
		return totalDeleted, fmt.Errorf("cleanup cve cache: %w", err)
	}
	totalDeleted += res.RowsAffected

	return totalDeleted, nil
}

// --- Row scanners ---

type updateRowScanner interface {
	Scan(dest ...interface{}) error
}

func scanScanRecord(row updateRowScanner) (*update.ScanRecord, error) {
	var r update.ScanRecord
	var startedAt int64
	var completedAt sql.NullInt64
	var status string
	err := row.Scan(&r.ID, &startedAt, &completedAt, &r.ContainersScanned, &r.UpdatesFound, &r.Errors, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.StartedAt = time.Unix(startedAt, 0)
	if completedAt.Valid {
		t := time.Unix(completedAt.Int64, 0)
		r.CompletedAt = &t
	}
	r.Status = update.ScanStatus(status)
	return &r, nil
}

func scanImageUpdate(row updateRowScanner) (*update.ImageUpdate, error) {
	var u update.ImageUpdate
	var publishedAt sql.NullInt64
	var detectedAt int64
	var updateType, status sql.NullString
	var latestTag, latestDigest sql.NullString
	var changelogURL, changelogSummary, previousDigest, sourceURL sql.NullString
	var hasBreakingChanges sql.NullBool

	err := row.Scan(
		&u.ID, &u.ScanID, &u.ContainerID, &u.ContainerName, &u.Image,
		&u.CurrentTag, &u.CurrentDigest, &u.Registry,
		&latestTag, &latestDigest, &updateType, &u.RiskScore, &publishedAt,
		&changelogURL, &changelogSummary, &hasBreakingChanges, &previousDigest, &sourceURL,
		&status, &detectedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	u.DetectedAt = time.Unix(detectedAt, 0)
	if publishedAt.Valid {
		t := time.Unix(publishedAt.Int64, 0)
		u.PublishedAt = &t
	}
	if latestTag.Valid {
		u.LatestTag = latestTag.String
	}
	if latestDigest.Valid {
		u.LatestDigest = latestDigest.String
	}
	if updateType.Valid {
		u.UpdateType = update.UpdateType(updateType.String)
	}
	if status.Valid {
		u.Status = update.Status(status.String)
	}
	if changelogURL.Valid {
		u.ChangelogURL = changelogURL.String
	}
	if changelogSummary.Valid {
		u.ChangelogSummary = changelogSummary.String
	}
	if hasBreakingChanges.Valid {
		u.HasBreakingChanges = hasBreakingChanges.Bool
	}
	if previousDigest.Valid {
		u.PreviousDigest = previousDigest.String
	}
	if sourceURL.Valid {
		u.SourceURL = sourceURL.String
	}
	return &u, nil
}

func collectImageUpdates(rows *sql.Rows) ([]*update.ImageUpdate, error) {
	var result []*update.ImageUpdate
	for rows.Next() {
		u, err := scanImageUpdate(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, u)
	}
	return result, rows.Err()
}

func scanCVECacheEntry(row updateRowScanner) (*update.CVECacheEntry, error) {
	var e update.CVECacheEntry
	var fetchedAt, expiresAt int64
	var cvssVector, summary, fixedIn, referencesJSON sql.NullString
	var cvssScore sql.NullFloat64

	err := row.Scan(
		&e.ID, &e.Ecosystem, &e.PackageName, &e.PackageVersion, &e.CVEID,
		&cvssScore, &cvssVector, &e.Severity, &summary, &fixedIn, &referencesJSON,
		&fetchedAt, &expiresAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	e.FetchedAt = time.Unix(fetchedAt, 0)
	e.ExpiresAt = time.Unix(expiresAt, 0)
	if cvssScore.Valid {
		e.CVSSScore = cvssScore.Float64
	}
	if cvssVector.Valid {
		e.CVSSVector = cvssVector.String
	}
	if summary.Valid {
		e.Summary = summary.String
	}
	if fixedIn.Valid {
		e.FixedIn = fixedIn.String
	}
	if referencesJSON.Valid {
		e.ReferencesJSON = referencesJSON.String
	}
	return &e, nil
}

func scanContainerCVE(row updateRowScanner) (*update.ContainerCVE, error) {
	var c update.ContainerCVE
	var firstDetectedAt int64
	var resolvedAt sql.NullInt64
	var cvssScore sql.NullFloat64
	var summary, fixedIn sql.NullString

	err := row.Scan(
		&c.ID, &c.ContainerID, &c.CVEID, &c.Severity,
		&cvssScore, &summary, &fixedIn, &firstDetectedAt, &resolvedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	c.FirstDetectedAt = time.Unix(firstDetectedAt, 0)
	if resolvedAt.Valid {
		t := time.Unix(resolvedAt.Int64, 0)
		c.ResolvedAt = &t
	}
	if cvssScore.Valid {
		c.CVSSScore = cvssScore.Float64
	}
	if summary.Valid {
		c.Summary = summary.String
	}
	if fixedIn.Valid {
		c.FixedIn = fixedIn.String
	}
	return &c, nil
}

func collectContainerCVEs(rows *sql.Rows) ([]*update.ContainerCVE, error) {
	var result []*update.ContainerCVE
	for rows.Next() {
		c, err := scanContainerCVE(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

func scanVersionPin(row updateRowScanner) (*update.VersionPin, error) {
	var p update.VersionPin
	var pinnedAt int64
	var reason sql.NullString

	err := row.Scan(&p.ID, &p.ContainerID, &p.Image, &p.PinnedTag, &p.PinnedDigest, &reason, &pinnedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	p.PinnedAt = time.Unix(pinnedAt, 0)
	if reason.Valid {
		p.Reason = reason.String
	}
	return &p, nil
}

// UpsertDigestBaseline inserts or updates the digest baseline for a container.
func (s *UpdateStore) UpsertDigestBaseline(ctx context.Context, b *update.DigestBaseline) error {
	_, err := s.writer.Exec(ctx,
		`INSERT INTO digest_baselines (container_id, image, tag, remote_digest, checked_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(container_id) DO UPDATE SET
		   image = excluded.image,
		   tag = excluded.tag,
		   remote_digest = excluded.remote_digest,
		   checked_at = excluded.checked_at`,
		b.ContainerID, b.Image, b.Tag, b.RemoteDigest, b.CheckedAt.Unix())
	return err
}

// GetDigestBaseline returns the stored digest baseline for a container.
func (s *UpdateStore) GetDigestBaseline(ctx context.Context, containerID string) (*update.DigestBaseline, error) {
	var b update.DigestBaseline
	var checkedAt int64
	err := s.db.QueryRowContext(ctx,
		`SELECT container_id, image, tag, remote_digest, checked_at
		 FROM digest_baselines WHERE container_id = ?`, containerID).
		Scan(&b.ContainerID, &b.Image, &b.Tag, &b.RemoteDigest, &checkedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	b.CheckedAt = time.Unix(checkedAt, 0)
	return &b, nil
}
