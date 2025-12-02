package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kolapsis/pulseboard/internal/container"
)

// ContainerStore implements container.ContainerStore using SQLite.
type ContainerStore struct {
	db     *sql.DB
	writer *Writer
}

// NewContainerStore creates a new SQLite-backed container store.
func NewContainerStore(d *DB) *ContainerStore {
	return &ContainerStore{
		db:     d.ReadDB(),
		writer: d.Writer(),
	}
}

func (s *ContainerStore) InsertContainer(ctx context.Context, c *container.Container) (int64, error) {
	res, err := s.writer.Exec(ctx,
		`INSERT INTO containers (external_id, name, image, state, health_status, has_health_check,
			orchestration_group, orchestration_unit, custom_group, is_ignored, alert_severity,
			restart_threshold, alert_channels, archived, first_seen_at, last_state_change_at,
			runtime_type, error_detail, controller_kind, namespace, pod_count, ready_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ExternalID, c.Name, c.Image, string(c.State), nullableHealth(c.HealthStatus),
		boolToInt(c.HasHealthCheck), NullableString(c.OrchestrationGroup), NullableString(c.OrchestrationUnit),
		NullableString(c.CustomGroup), boolToInt(c.IsIgnored), string(c.AlertSeverity),
		c.RestartThreshold, NullableString(c.AlertChannels), boolToInt(c.Archived),
		c.FirstSeenAt.Unix(), c.LastStateChangeAt.Unix(),
		c.RuntimeType, c.ErrorDetail, c.ControllerKind, c.Namespace, c.PodCount, c.ReadyCount,
	)
	if err != nil {
		return 0, fmt.Errorf("insert container: %w", err)
	}
	c.ID = res.LastInsertID
	return res.LastInsertID, nil
}

func (s *ContainerStore) UpdateContainer(ctx context.Context, c *container.Container) error {
	_, err := s.writer.Exec(ctx,
		`UPDATE containers SET name=?, image=?, state=?, health_status=?, has_health_check=?,
			orchestration_group=?, orchestration_unit=?, custom_group=?, is_ignored=?, alert_severity=?,
			restart_threshold=?, alert_channels=?, archived=?, last_state_change_at=?, archived_at=?,
			runtime_type=?, error_detail=?, controller_kind=?, namespace=?, pod_count=?, ready_count=?
		WHERE id=?`,
		c.Name, c.Image, string(c.State), nullableHealth(c.HealthStatus),
		boolToInt(c.HasHealthCheck), NullableString(c.OrchestrationGroup), NullableString(c.OrchestrationUnit),
		NullableString(c.CustomGroup), boolToInt(c.IsIgnored), string(c.AlertSeverity),
		c.RestartThreshold, NullableString(c.AlertChannels), boolToInt(c.Archived),
		c.LastStateChangeAt.Unix(), nullableTime(c.ArchivedAt),
		c.RuntimeType, c.ErrorDetail, c.ControllerKind, c.Namespace, c.PodCount, c.ReadyCount,
		c.ID,
	)
	if err != nil {
		return fmt.Errorf("update container %d: %w", c.ID, err)
	}
	return nil
}

func (s *ContainerStore) GetContainerByExternalID(ctx context.Context, externalID string) (*container.Container, error) {
	return s.scanContainer(s.db.QueryRowContext(ctx,
		`SELECT `+containerColumns+` FROM containers WHERE external_id=?`, externalID))
}

func (s *ContainerStore) GetContainerByID(ctx context.Context, id int64) (*container.Container, error) {
	return s.scanContainer(s.db.QueryRowContext(ctx,
		`SELECT `+containerColumns+` FROM containers WHERE id=?`, id))
}

func (s *ContainerStore) ListContainers(ctx context.Context, opts container.ListContainersOpts) ([]*container.Container, error) {
	query := `SELECT ` + containerColumns + ` FROM containers WHERE 1=1`
	var args []interface{}

	if !opts.IncludeArchived {
		query += ` AND archived=0`
	}
	if !opts.IncludeIgnored {
		query += ` AND is_ignored=0`
	}
	if opts.GroupFilter != "" {
		query += ` AND (custom_group=? OR orchestration_group=?)`
		args = append(args, opts.GroupFilter, opts.GroupFilter)
	}
	if opts.StateFilter != "" {
		query += ` AND state=?`
		args = append(args, opts.StateFilter)
	}

	query += ` ORDER BY name ASC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}
	defer rows.Close()

	var containers []*container.Container
	for rows.Next() {
		c, err := s.scanContainerRow(rows)
		if err != nil {
			return nil, err
		}
		containers = append(containers, c)
	}
	return containers, rows.Err()
}

func (s *ContainerStore) ArchiveContainer(ctx context.Context, externalID string, archivedAt time.Time) error {
	_, err := s.writer.Exec(ctx,
		`UPDATE containers SET archived=1, archived_at=? WHERE external_id=? AND archived=0`,
		archivedAt.Unix(), externalID,
	)
	if err != nil {
		return fmt.Errorf("archive container %s: %w", externalID, err)
	}
	return nil
}

// InsertTransition records a state transition.
func (s *ContainerStore) InsertTransition(ctx context.Context, t *container.StateTransition) (int64, error) {
	res, err := s.writer.Exec(ctx,
		`INSERT INTO state_transitions (container_id, previous_state, new_state, previous_health, new_health, exit_code, log_snippet, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ContainerID, string(t.PreviousState), string(t.NewState),
		nullableHealth(t.PreviousHealth), nullableHealth(t.NewHealth),
		t.ExitCode, NullableString(t.LogSnippet), t.Timestamp.Unix(),
	)
	if err != nil {
		return 0, fmt.Errorf("insert transition: %w", err)
	}
	t.ID = res.LastInsertID
	return res.LastInsertID, nil
}

func (s *ContainerStore) ListTransitionsByContainer(ctx context.Context, containerID int64, opts container.ListTransitionsOpts) ([]*container.StateTransition, int, error) {
	countQuery := `SELECT COUNT(*) FROM state_transitions WHERE container_id=?`
	var countArgs []interface{}
	countArgs = append(countArgs, containerID)

	query := `SELECT ` + transitionColumns + ` FROM state_transitions WHERE container_id=?`
	var args []interface{}
	args = append(args, containerID)

	if opts.Since != nil {
		query += ` AND timestamp>=?`
		args = append(args, opts.Since.Unix())
		countQuery += ` AND timestamp>=?`
		countArgs = append(countArgs, opts.Since.Unix())
	}
	if opts.Until != nil {
		query += ` AND timestamp<=?`
		args = append(args, opts.Until.Unix())
		countQuery += ` AND timestamp<=?`
		countArgs = append(countArgs, opts.Until.Unix())
	}

	var total int
	if err := s.db.QueryRowContext(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count transitions: %w", err)
	}

	query += ` ORDER BY timestamp DESC`
	if opts.Limit > 0 {
		query += fmt.Sprintf(` LIMIT %d`, opts.Limit)
	}
	if opts.Offset > 0 {
		query += fmt.Sprintf(` OFFSET %d`, opts.Offset)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list transitions: %w", err)
	}
	defer rows.Close()

	var transitions []*container.StateTransition
	for rows.Next() {
		t, err := scanTransitionRow(rows)
		if err != nil {
			return nil, 0, err
		}
		transitions = append(transitions, t)
	}
	return transitions, total, rows.Err()
}

func (s *ContainerStore) CountRestartsSince(ctx context.Context, containerID int64, since time.Time) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM state_transitions
		WHERE container_id=? AND new_state='running' AND previous_state='restarting' AND timestamp>=?`,
		containerID, since.Unix(),
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count restarts: %w", err)
	}
	return count, nil
}

func (s *ContainerStore) GetTransitionsInWindow(ctx context.Context, containerID int64, from, to time.Time) ([]*container.StateTransition, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+transitionColumns+` FROM state_transitions
		WHERE container_id=? AND timestamp>=? AND timestamp<=?
		ORDER BY timestamp ASC`,
		containerID, from.Unix(), to.Unix(),
	)
	if err != nil {
		return nil, fmt.Errorf("get transitions in window: %w", err)
	}
	defer rows.Close()

	var transitions []*container.StateTransition
	for rows.Next() {
		t, err := scanTransitionRow(rows)
		if err != nil {
			return nil, err
		}
		transitions = append(transitions, t)
	}
	return transitions, rows.Err()
}

func (s *ContainerStore) DeleteTransitionsBefore(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	var totalDeleted int64
	for {
		res, err := s.writer.Exec(ctx,
			`DELETE FROM state_transitions WHERE rowid IN (
				SELECT rowid FROM state_transitions WHERE timestamp<? LIMIT ?
			)`, before.Unix(), batchSize)
		if err != nil {
			return totalDeleted, fmt.Errorf("delete transitions: %w", err)
		}
		totalDeleted += res.RowsAffected
		if res.RowsAffected < int64(batchSize) {
			break
		}
	}
	return totalDeleted, nil
}

func (s *ContainerStore) DeleteArchivedContainersBefore(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.writer.Exec(ctx,
		`DELETE FROM containers WHERE archived=1 AND archived_at<? AND archived_at IS NOT NULL`,
		before.Unix(),
	)
	if err != nil {
		return 0, fmt.Errorf("delete archived containers: %w", err)
	}
	return res.RowsAffected, nil
}

// --- Column lists and scanners ---

const containerColumns = `id, external_id, name, image, state, health_status, has_health_check,
	orchestration_group, orchestration_unit, custom_group, is_ignored, alert_severity,
	restart_threshold, alert_channels, archived, first_seen_at, last_state_change_at, archived_at,
	runtime_type, error_detail, controller_kind, namespace, pod_count, ready_count`

const transitionColumns = `id, container_id, previous_state, new_state, previous_health, new_health, exit_code, log_snippet, timestamp`

type rowScanner interface {
	Scan(dest ...interface{}) error
}

func (s *ContainerStore) scanContainer(row rowScanner) (*container.Container, error) {
	var c container.Container
	var healthStatus, orchestrationGroup, orchestrationUnit, customGroup, alertChannels sql.NullString
	var hasHealthCheck, isIgnored, archived int
	var firstSeen, lastChange int64
	var archivedAt sql.NullInt64

	err := row.Scan(
		&c.ID, &c.ExternalID, &c.Name, &c.Image, &c.State,
		&healthStatus, &hasHealthCheck,
		&orchestrationGroup, &orchestrationUnit, &customGroup,
		&isIgnored, &c.AlertSeverity,
		&c.RestartThreshold, &alertChannels,
		&archived, &firstSeen, &lastChange, &archivedAt,
		&c.RuntimeType, &c.ErrorDetail, &c.ControllerKind, &c.Namespace, &c.PodCount, &c.ReadyCount,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("scan container: %w", err)
	}

	c.HasHealthCheck = hasHealthCheck != 0
	c.IsIgnored = isIgnored != 0
	c.Archived = archived != 0
	c.FirstSeenAt = time.Unix(firstSeen, 0)
	c.LastStateChangeAt = time.Unix(lastChange, 0)

	if healthStatus.Valid {
		hs := container.HealthStatus(healthStatus.String)
		c.HealthStatus = &hs
	}
	if orchestrationGroup.Valid {
		c.OrchestrationGroup = orchestrationGroup.String
	}
	if orchestrationUnit.Valid {
		c.OrchestrationUnit = orchestrationUnit.String
	}
	if customGroup.Valid {
		c.CustomGroup = customGroup.String
	}
	if alertChannels.Valid {
		c.AlertChannels = alertChannels.String
	}
	if archivedAt.Valid {
		t := time.Unix(archivedAt.Int64, 0)
		c.ArchivedAt = &t
	}

	return &c, nil
}

func (s *ContainerStore) scanContainerRow(rows *sql.Rows) (*container.Container, error) {
	return s.scanContainer(rows)
}

func scanTransitionRow(rows rowScanner) (*container.StateTransition, error) {
	var t container.StateTransition
	var prevHealth, newHealth sql.NullString
	var exitCode sql.NullInt64
	var logSnippet sql.NullString
	var ts int64

	err := rows.Scan(
		&t.ID, &t.ContainerID, &t.PreviousState, &t.NewState,
		&prevHealth, &newHealth, &exitCode, &logSnippet, &ts,
	)
	if err != nil {
		return nil, fmt.Errorf("scan transition: %w", err)
	}

	t.Timestamp = time.Unix(ts, 0)
	if prevHealth.Valid {
		hs := container.HealthStatus(prevHealth.String)
		t.PreviousHealth = &hs
	}
	if newHealth.Valid {
		hs := container.HealthStatus(newHealth.String)
		t.NewHealth = &hs
	}
	if exitCode.Valid {
		ec := int(exitCode.Int64)
		t.ExitCode = &ec
	}
	if logSnippet.Valid {
		t.LogSnippet = logSnippet.String
	}

	return &t, nil
}

// --- Helpers ---

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullableHealth(h *container.HealthStatus) interface{} {
	if h == nil {
		return nil
	}
	return string(*h)
}

func nullableTime(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return t.Unix()
}
