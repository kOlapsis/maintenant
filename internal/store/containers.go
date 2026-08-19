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

	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/uid"
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

// InsertContainer upserts a container. Its id is derived deterministically from
// (agent_id, external_id) so the agent and server mint the same id; a repeat
// report updates the existing row.
func (s *ContainerStore) InsertContainer(ctx context.Context, c *container.Container) (string, error) {
	c.AgentID = uid.Agent(c.AgentID)
	c.ID = uid.Container(c.AgentID, c.ExternalID)
	_, err := s.writer.Exec(ctx,
		`INSERT INTO containers (id, agent_id, external_id, name, image, state, health_status, has_health_check,
			orchestration_group, orchestration_unit, custom_group, is_ignored, alert_severity,
			restart_threshold, alert_channels, archived, first_seen_at, last_state_change_at,
			runtime_type, error_detail, controller_kind, namespace, pod_count, ready_count,
			compose_working_dir,
			swarm_service_id, swarm_service_name, swarm_service_mode, swarm_node_id, swarm_task_slot, swarm_desired_replicas)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name, image=excluded.image, state=excluded.state, health_status=excluded.health_status,
			has_health_check=excluded.has_health_check, orchestration_group=excluded.orchestration_group,
			orchestration_unit=excluded.orchestration_unit, custom_group=excluded.custom_group,
			is_ignored=excluded.is_ignored, alert_severity=excluded.alert_severity,
			restart_threshold=excluded.restart_threshold, alert_channels=excluded.alert_channels,
			archived=excluded.archived, last_state_change_at=excluded.last_state_change_at,
			runtime_type=excluded.runtime_type, error_detail=excluded.error_detail,
			controller_kind=excluded.controller_kind, namespace=excluded.namespace,
			pod_count=excluded.pod_count, ready_count=excluded.ready_count,
			compose_working_dir=excluded.compose_working_dir,
			swarm_service_id=excluded.swarm_service_id, swarm_service_name=excluded.swarm_service_name,
			swarm_service_mode=excluded.swarm_service_mode, swarm_node_id=excluded.swarm_node_id,
			swarm_task_slot=excluded.swarm_task_slot, swarm_desired_replicas=excluded.swarm_desired_replicas`,
		c.ID, c.AgentID, c.ExternalID, c.Name, c.Image, string(c.State), nullableHealth(c.HealthStatus),
		boolToInt(c.HasHealthCheck), NullableString(c.OrchestrationGroup), NullableString(c.OrchestrationUnit),
		NullableString(c.CustomGroup), boolToInt(c.IsIgnored), string(c.AlertSeverity),
		c.RestartThreshold, NullableString(c.AlertChannels), boolToInt(c.Archived),
		c.FirstSeenAt.Unix(), c.LastStateChangeAt.Unix(),
		c.RuntimeType, c.ErrorDetail, c.ControllerKind, c.Namespace, c.PodCount, c.ReadyCount,
		c.ComposeWorkingDir,
		c.SwarmServiceID, c.SwarmServiceName, c.SwarmServiceMode, c.SwarmNodeID, c.SwarmTaskSlot, c.SwarmDesiredReplicas,
	)
	if err != nil {
		return "", fmt.Errorf("insert container: %w", err)
	}
	return c.ID, nil
}

func (s *ContainerStore) UpdateContainer(ctx context.Context, c *container.Container) error {
	c.AgentID = uid.Agent(c.AgentID)
	_, err := s.writer.Exec(ctx,
		`UPDATE containers SET name=?, image=?, state=?, health_status=?, has_health_check=?,
			orchestration_group=?, orchestration_unit=?, custom_group=?, is_ignored=?, alert_severity=?,
			restart_threshold=?, alert_channels=?, archived=?, last_state_change_at=?, archived_at=?,
			runtime_type=?, error_detail=?, controller_kind=?, namespace=?, pod_count=?, ready_count=?,
			compose_working_dir=?,
			swarm_service_id=?, swarm_service_name=?, swarm_service_mode=?, swarm_node_id=?, swarm_task_slot=?, swarm_desired_replicas=?,
			agent_id=?
		WHERE id=?`,
		c.Name, c.Image, string(c.State), nullableHealth(c.HealthStatus),
		boolToInt(c.HasHealthCheck), NullableString(c.OrchestrationGroup), NullableString(c.OrchestrationUnit),
		NullableString(c.CustomGroup), boolToInt(c.IsIgnored), string(c.AlertSeverity),
		c.RestartThreshold, NullableString(c.AlertChannels), boolToInt(c.Archived),
		c.LastStateChangeAt.Unix(), nullableTime(c.ArchivedAt),
		c.RuntimeType, c.ErrorDetail, c.ControllerKind, c.Namespace, c.PodCount, c.ReadyCount,
		c.ComposeWorkingDir,
		c.SwarmServiceID, c.SwarmServiceName, c.SwarmServiceMode, c.SwarmNodeID, c.SwarmTaskSlot, c.SwarmDesiredReplicas,
		c.AgentID,
		c.ID,
	)
	if err != nil {
		return fmt.Errorf("update container %s: %w", c.ID, err)
	}
	return nil
}

func (s *ContainerStore) GetContainerByExternalID(ctx context.Context, externalID string) (*container.Container, error) {
	return s.scanContainer(s.db.QueryRowContext(ctx,
		`SELECT `+containerColumns+` FROM containers WHERE external_id=?`, externalID))
}

func (s *ContainerStore) GetContainerByID(ctx context.Context, id string) (*container.Container, error) {
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
	if opts.AgentFilter != nil {
		query += ` AND agent_id=?`
		args = append(args, *opts.AgentFilter)
	}

	query += ` ORDER BY name ASC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)

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

func (s *ContainerStore) DeleteContainerByID(ctx context.Context, id string) error {
	// Look up the external_id for cleaning up tables keyed by it (soft refs,
	// no FK). The FK-constrained children (transitions, snapshots, alert configs)
	// are removed automatically by ON DELETE CASCADE.
	var externalID string
	err := s.db.QueryRowContext(ctx, `SELECT external_id FROM containers WHERE id = ?`, id).Scan(&externalID)
	if err != nil {
		return fmt.Errorf("get container external_id: %w", err)
	}

	for _, q := range []struct {
		sql  string
		desc string
	}{
		{`DELETE FROM image_updates WHERE container_id = ?`, "image updates"},
		{`DELETE FROM container_cves WHERE container_id = ?`, "container cves"},
		{`DELETE FROM version_pins WHERE container_id = ?`, "version pins"},
		{`DELETE FROM risk_score_history WHERE container_id = ?`, "risk score history"},
	} {
		if _, err := s.writer.Exec(ctx, q.sql, externalID); err != nil {
			return fmt.Errorf("delete container %s: %w", q.desc, err)
		}
	}

	if _, err := s.writer.Exec(ctx, `DELETE FROM containers WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete container: %w", err)
	}
	return nil
}

// InsertTransition records a state transition.
func (s *ContainerStore) InsertTransition(ctx context.Context, t *container.StateTransition) (string, error) {
	t.ID = uid.New()
	_, err := s.writer.Exec(ctx,
		`INSERT INTO state_transitions (id, container_id, previous_state, new_state, previous_health, new_health, exit_code, log_snippet, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.ContainerID, string(t.PreviousState), string(t.NewState),
		nullableHealth(t.PreviousHealth), nullableHealth(t.NewHealth),
		t.ExitCode, NullableString(t.LogSnippet), t.Timestamp.Unix(),
	)
	if err != nil {
		return "", fmt.Errorf("insert transition: %w", err)
	}
	return t.ID, nil
}

func (s *ContainerStore) ListTransitionsByContainer(ctx context.Context, containerID string, opts container.ListTransitionsOpts) ([]*container.StateTransition, int, error) {
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
		query += fmt.Sprintf(` OFFSET %d`, opts.Offset) // #nosec G202 -- integer formatting of an int option.
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list transitions: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)

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

func (s *ContainerStore) CountRestartsSince(ctx context.Context, containerID string, since time.Time) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM state_transitions
		WHERE container_id=? AND new_state='running' AND previous_state IN ('restarting','exited') AND timestamp>=?`,
		containerID, since.Unix(),
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count restarts: %w", err)
	}
	return count, nil
}

// CountConfigured returns the number of non-archived auto-discovered
// containers. Used by the telemetry subsystem; see specs/015-shm-telemetry.
func (s *ContainerStore) CountConfigured(ctx context.Context) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM containers WHERE archived = 0`,
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("count configured containers: %w", err)
	}
	return count, nil
}

func (s *ContainerStore) GetTransitionsInWindow(ctx context.Context, containerID string, from, to time.Time) ([]*container.StateTransition, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+transitionColumns+` FROM state_transitions
		WHERE container_id=? AND timestamp>=? AND timestamp<=?
		ORDER BY timestamp ASC`,
		containerID, from.Unix(), to.Unix(),
	)
	if err != nil {
		return nil, fmt.Errorf("get transitions in window: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)

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
	deleted, _, err := s.deleteTransitionsBefore(ctx, before, batchOpts{batchSize: batchSize})
	return deleted, err
}

func (s *ContainerStore) deleteTransitionsBefore(ctx context.Context, before time.Time, o batchOpts) (int64, bool, error) {
	return deleteRowsBefore(ctx, s.writer, o, "state_transitions", "timestamp", before)
}

func (s *ContainerStore) DeleteArchivedContainersBefore(ctx context.Context, before time.Time) (int64, error) {
	cutoff := before.Unix()

	// Tables keyed by external_id have no FK to cascade through, so they must go
	// first: once the container row is gone, nothing ties them back to anything
	// and they stay in the database for good.
	for _, q := range []struct {
		sql  string
		desc string
	}{
		{`DELETE FROM image_updates WHERE container_id IN (
			SELECT external_id FROM containers WHERE archived=1 AND archived_at<? AND archived_at IS NOT NULL)`, "image updates"},
		{`DELETE FROM container_cves WHERE container_id IN (
			SELECT external_id FROM containers WHERE archived=1 AND archived_at<? AND archived_at IS NOT NULL)`, "container cves"},
		{`DELETE FROM version_pins WHERE container_id IN (
			SELECT external_id FROM containers WHERE archived=1 AND archived_at<? AND archived_at IS NOT NULL)`, "version pins"},
		{`DELETE FROM risk_score_history WHERE container_id IN (
			SELECT external_id FROM containers WHERE archived=1 AND archived_at<? AND archived_at IS NOT NULL)`, "risk score history"},
	} {
		if _, err := s.writer.Exec(ctx, q.sql, cutoff); err != nil {
			return 0, fmt.Errorf("delete archived containers %s: %w", q.desc, err)
		}
	}

	res, err := s.writer.Exec(ctx,
		`DELETE FROM containers WHERE archived=1 AND archived_at<? AND archived_at IS NOT NULL`,
		cutoff,
	)
	if err != nil {
		return 0, fmt.Errorf("delete archived containers: %w", err)
	}
	return res.RowsAffected, nil
}

// --- Column lists and scanners ---

const containerColumns = `id, agent_id, external_id, name, image, state, health_status, has_health_check,
	orchestration_group, orchestration_unit, custom_group, is_ignored, alert_severity,
	restart_threshold, alert_channels, archived, first_seen_at, last_state_change_at, archived_at,
	runtime_type, error_detail, controller_kind, namespace, pod_count, ready_count,
	compose_working_dir,
	swarm_service_id, swarm_service_name, swarm_service_mode, swarm_node_id, swarm_task_slot, swarm_desired_replicas`

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
		&c.ID, &c.AgentID, &c.ExternalID, &c.Name, &c.Image, &c.State,
		&healthStatus, &hasHealthCheck,
		&orchestrationGroup, &orchestrationUnit, &customGroup,
		&isIgnored, &c.AlertSeverity,
		&c.RestartThreshold, &alertChannels,
		&archived, &firstSeen, &lastChange, &archivedAt,
		&c.RuntimeType, &c.ErrorDetail, &c.ControllerKind, &c.Namespace, &c.PodCount, &c.ReadyCount,
		&c.ComposeWorkingDir,
		&c.SwarmServiceID, &c.SwarmServiceName, &c.SwarmServiceMode, &c.SwarmNodeID, &c.SwarmTaskSlot, &c.SwarmDesiredReplicas,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
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
