package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kolapsis/pulseboard/internal/update"
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
		 latest_tag, latest_digest, update_type, risk_score, published_at, status, detected_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(container_name, image, latest_tag) DO UPDATE SET
			scan_id=excluded.scan_id,
			container_id=excluded.container_id,
			current_digest=excluded.current_digest,
			latest_digest=excluded.latest_digest,
			update_type=excluded.update_type,
			risk_score=excluded.risk_score,
			published_at=excluded.published_at,
			detected_at=excluded.detected_at`,
		u.ScanID, u.ContainerID, u.ContainerName, u.Image, u.CurrentTag, u.CurrentDigest, u.Registry,
		u.LatestTag, u.LatestDigest, string(u.UpdateType), u.RiskScore, publishedAt,
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
		 latest_tag=?, latest_digest=?, update_type=?, risk_score=?, published_at=?, status=?
		WHERE id=?`,
		u.ScanID, u.ContainerName, u.Image, u.CurrentTag, u.CurrentDigest, u.Registry,
		u.LatestTag, u.LatestDigest, string(u.UpdateType), u.RiskScore, publishedAt,
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
		 latest_tag, latest_digest, update_type, risk_score, published_at, status, detected_at
		FROM image_updates WHERE id = ?`, id)
	return scanImageUpdate(row)
}

func (s *UpdateStore) GetImageUpdateByContainer(ctx context.Context, containerID string) (*update.ImageUpdate, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, scan_id, container_id, container_name, image, current_tag, current_digest, registry,
		 latest_tag, latest_digest, update_type, risk_score, published_at, status, detected_at
		FROM image_updates WHERE container_id = ? ORDER BY detected_at DESC LIMIT 1`, containerID)
	return scanImageUpdate(row)
}

func (s *UpdateStore) ListImageUpdates(ctx context.Context, opts update.ListImageUpdatesOpts) ([]*update.ImageUpdate, error) {
	query := `SELECT id, scan_id, container_id, container_name, image, current_tag, current_digest, registry,
		 latest_tag, latest_digest, update_type, risk_score, published_at, status, detected_at
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
	query += " ORDER BY detected_at DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list image updates: %w", err)
	}
	defer rows.Close()
	return collectImageUpdates(rows)
}

func (s *UpdateStore) GetUpdateSummary(ctx context.Context) (*update.UpdateSummary, error) {
	summary := &update.UpdateSummary{}

	rows, err := s.db.QueryContext(ctx, `SELECT status, update_type FROM image_updates`)
	if err != nil {
		return nil, fmt.Errorf("get update summary: %w", err)
	}
	defer rows.Close()

	tracked := 0
	for rows.Next() {
		var st string
		var updateType sql.NullString
		if err := rows.Scan(&st, &updateType); err != nil {
			return nil, err
		}
		tracked++
		switch st {
		case "pinned":
			summary.Pinned++
		case "available", "dismissed":
			ut := ""
			if updateType.Valid {
				ut = updateType.String
			}
			switch ut {
			case "major":
				summary.Critical++
			case "minor":
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
	defer rows.Close()

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
	if err == sql.ErrNoRows {
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

	err := row.Scan(
		&u.ID, &u.ScanID, &u.ContainerID, &u.ContainerName, &u.Image,
		&u.CurrentTag, &u.CurrentDigest, &u.Registry,
		&latestTag, &latestDigest, &updateType, &u.RiskScore, &publishedAt,
		&status, &detectedAt,
	)
	if err == sql.ErrNoRows {
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
		u.Status = update.UpdateStatus(status.String)
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

func scanVersionPin(row updateRowScanner) (*update.VersionPin, error) {
	var p update.VersionPin
	var pinnedAt int64
	var reason sql.NullString

	err := row.Scan(&p.ID, &p.ContainerID, &p.Image, &p.PinnedTag, &p.PinnedDigest, &reason, &pinnedAt)
	if err == sql.ErrNoRows {
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
