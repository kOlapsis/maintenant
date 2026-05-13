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
	"fmt"
	"strings"
	"time"

	"github.com/kolapsis/maintenant/internal/status"
)

// StatusComponentStoreImpl implements status.ComponentStore using SQLite.
type StatusComponentStoreImpl struct {
	db     *sql.DB
	writer *Writer
}

// NewStatusComponentStore creates a new SQLite-backed component store.
func NewStatusComponentStore(d *DB) *StatusComponentStoreImpl {
	return &StatusComponentStoreImpl{
		db:     d.ReadDB(),
		writer: d.Writer(),
	}
}

// --- Components ---

const componentSelectCols = `SELECT sc.id, sc.composition_mode, sc.match_all_type, sc.display_name,
	sc.display_order, sc.visible,
	sc.status_override, sc.auto_incident, sc.created_at, sc.updated_at
FROM status_components sc`

func (s *StatusComponentStoreImpl) ListComponents(ctx context.Context) ([]status.Component, error) {
	rows, err := s.db.QueryContext(ctx,
		componentSelectCols+`
		ORDER BY sc.display_order, sc.display_name`)
	if err != nil {
		return nil, fmt.Errorf("list components: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)

	comps, err := scanComponents(rows)
	if err != nil {
		return nil, err
	}
	if err := s.hydrateMonitorRefs(ctx, comps); err != nil {
		return nil, err
	}
	setNeedsAttention(comps)
	return comps, nil
}

func (s *StatusComponentStoreImpl) ListVisibleComponents(ctx context.Context) ([]status.Component, error) {
	rows, err := s.db.QueryContext(ctx,
		componentSelectCols+`
		WHERE sc.visible = 1
		ORDER BY sc.display_order, sc.display_name`)
	if err != nil {
		return nil, fmt.Errorf("list visible components: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)

	comps, err := scanComponents(rows)
	if err != nil {
		return nil, err
	}
	if err := s.hydrateMonitorRefs(ctx, comps); err != nil {
		return nil, err
	}
	setNeedsAttention(comps)
	return comps, nil
}

func (s *StatusComponentStoreImpl) GetComponent(ctx context.Context, id int64) (*status.Component, error) {
	rows, err := s.db.QueryContext(ctx,
		componentSelectCols+`
		WHERE sc.id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("get component: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)

	comps, err := scanComponents(rows)
	if err != nil {
		return nil, err
	}
	if len(comps) == 0 {
		return nil, nil
	}
	if err := s.hydrateMonitorRefs(ctx, comps); err != nil {
		return nil, err
	}
	setNeedsAttention(comps)
	return &comps[0], nil
}

func (s *StatusComponentStoreImpl) ListComponentsByMonitor(ctx context.Context, monitorType string, monitorID int64) ([]status.Component, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT sc.id, sc.composition_mode, sc.match_all_type, sc.display_name,
			sc.display_order, sc.visible,
			sc.status_override, sc.auto_incident, sc.created_at, sc.updated_at
		FROM status_components sc
		LEFT JOIN status_component_monitors m ON m.component_id = sc.id
		WHERE (m.monitor_type = ? AND m.monitor_id = ?)
		   OR (sc.composition_mode = 'match-all' AND sc.match_all_type = ?)
		ORDER BY sc.display_order, sc.id`,
		monitorType, monitorID, monitorType)
	if err != nil {
		return nil, fmt.Errorf("list components by monitor: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)

	comps, err := scanComponents(rows)
	if err != nil {
		return nil, err
	}
	if err := s.hydrateMonitorRefs(ctx, comps); err != nil {
		return nil, err
	}
	setNeedsAttention(comps)
	return comps, nil
}

func (s *StatusComponentStoreImpl) RemoveDanglingMonitorRefs(ctx context.Context, monitorType string, monitorID int64) error {
	_, err := s.writer.Exec(ctx,
		`DELETE FROM status_component_monitors WHERE monitor_type = ? AND monitor_id = ?`,
		monitorType, monitorID,
	)
	if err != nil {
		return fmt.Errorf("remove dangling monitor refs: %w", err)
	}
	return nil
}

func (s *StatusComponentStoreImpl) CreateComponent(ctx context.Context, c *status.Component) (int64, error) {
	now := time.Now().Unix()

	if c.CompositionMode == "" {
		c.CompositionMode = status.CompositionExplicit
	}

	var matchAllType any
	if c.MatchAllType != "" {
		matchAllType = c.MatchAllType
	}

	res, err := s.writer.Exec(ctx,
		`INSERT INTO status_components (composition_mode, match_all_type, display_name,
			display_order, visible, status_override, auto_incident, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		string(c.CompositionMode), matchAllType, c.DisplayName,
		c.DisplayOrder, boolToInt(c.Visible), c.StatusOverride, boolToInt(c.AutoIncident),
		now, now,
	)
	if err != nil {
		return 0, fmt.Errorf("create component: %w", err)
	}
	c.ID = res.LastInsertID
	c.CreatedAt = time.Unix(now, 0).UTC()
	c.UpdatedAt = c.CreatedAt

	// Insert monitor refs for explicit mode.
	if c.CompositionMode == status.CompositionExplicit {
		for _, ref := range c.Monitors {
			if _, err := s.writer.Exec(ctx,
				`INSERT INTO status_component_monitors (component_id, monitor_type, monitor_id) VALUES (?, ?, ?)`,
				c.ID, ref.Type, ref.ID,
			); err != nil {
				return 0, fmt.Errorf("insert monitor ref: %w", err)
			}
		}
	}

	return res.LastInsertID, nil
}

func (s *StatusComponentStoreImpl) UpdateComponent(ctx context.Context, c *status.Component) error {
	now := time.Now().Unix()

	var matchAllType any
	if c.MatchAllType != "" {
		matchAllType = c.MatchAllType
	}

	_, err := s.writer.Exec(ctx,
		`UPDATE status_components SET display_name = ?, display_order = ?,
			visible = ?, status_override = ?, auto_incident = ?, match_all_type = ?, updated_at = ?
		WHERE id = ?`,
		c.DisplayName, c.DisplayOrder,
		boolToInt(c.Visible), c.StatusOverride, boolToInt(c.AutoIncident),
		matchAllType, now, c.ID,
	)
	if err != nil {
		return fmt.Errorf("update component: %w", err)
	}
	c.UpdatedAt = time.Unix(now, 0).UTC()

	// Delta-apply monitor refs for explicit mode.
	if c.CompositionMode == status.CompositionExplicit {
		if err := s.deltaApplyMonitorRefs(ctx, c.ID, c.Monitors); err != nil {
			return err
		}
	}

	return nil
}

// deltaApplyMonitorRefs computes the diff between stored refs and desired refs,
// deletes removed entries, and inserts new ones.
func (s *StatusComponentStoreImpl) deltaApplyMonitorRefs(ctx context.Context, componentID int64, desired []status.MonitorRef) error {
	// Load current refs.
	rows, err := s.db.QueryContext(ctx,
		`SELECT monitor_type, monitor_id FROM status_component_monitors WHERE component_id = ?`,
		componentID)
	if err != nil {
		return fmt.Errorf("load current monitor refs: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)

	type refKey struct {
		t string
		i int64
	}
	current := make(map[refKey]struct{})
	for rows.Next() {
		var k refKey
		if err := rows.Scan(&k.t, &k.i); err != nil {
			return fmt.Errorf("scan monitor ref: %w", err)
		}
		current[k] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate monitor refs: %w", err)
	}

	desiredSet := make(map[refKey]struct{}, len(desired))
	for _, ref := range desired {
		desiredSet[refKey{ref.Type, ref.ID}] = struct{}{}
	}

	// Delete refs no longer desired.
	for k := range current {
		if _, ok := desiredSet[k]; !ok {
			if _, err := s.writer.Exec(ctx,
				`DELETE FROM status_component_monitors WHERE component_id = ? AND monitor_type = ? AND monitor_id = ?`,
				componentID, k.t, k.i,
			); err != nil {
				return fmt.Errorf("delete monitor ref: %w", err)
			}
		}
	}

	// Insert new refs.
	for k := range desiredSet {
		if _, ok := current[k]; !ok {
			if _, err := s.writer.Exec(ctx,
				`INSERT INTO status_component_monitors (component_id, monitor_type, monitor_id) VALUES (?, ?, ?)`,
				componentID, k.t, k.i,
			); err != nil {
				return fmt.Errorf("insert monitor ref: %w", err)
			}
		}
	}

	return nil
}

func (s *StatusComponentStoreImpl) DeleteComponent(ctx context.Context, id int64) error {
	_, err := s.writer.Exec(ctx, `DELETE FROM status_components WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete component: %w", err)
	}
	return nil
}

// --- Hydration ---

// hydrateMonitorRefs fetches monitor refs for a batch of components in a single query.
// It mutates the slice in-place.
func (s *StatusComponentStoreImpl) hydrateMonitorRefs(ctx context.Context, comps []status.Component) error {
	if len(comps) == 0 {
		return nil
	}

	ids := make([]any, len(comps))
	for i, c := range comps {
		ids[i] = c.ID
	}
	placeholders := strings.Repeat("?,", len(ids))
	placeholders = placeholders[:len(placeholders)-1]

	rows, err := s.db.QueryContext(ctx,
		"SELECT component_id, monitor_type, monitor_id FROM status_component_monitors WHERE component_id IN ("+placeholders+") ORDER BY component_id, monitor_type, monitor_id",
		ids...)
	if err != nil {
		return fmt.Errorf("hydrate monitor refs: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)

	idx := make(map[int64]int, len(comps))
	for i, c := range comps {
		idx[c.ID] = i
		// Initialize to empty slice (not nil) for explicit mode so JSON encodes as [].
		if comps[i].CompositionMode == status.CompositionExplicit {
			comps[i].Monitors = []status.MonitorRef{}
		}
	}

	for rows.Next() {
		var compID int64
		var ref status.MonitorRef
		if err := rows.Scan(&compID, &ref.Type, &ref.ID); err != nil {
			return fmt.Errorf("scan monitor ref: %w", err)
		}
		if i, ok := idx[compID]; ok {
			comps[i].Monitors = append(comps[i].Monitors, ref)
		}
	}

	// For match-all components, set Monitors to nil (dynamic, not stored).
	for i := range comps {
		if comps[i].CompositionMode == status.CompositionMatchAll {
			comps[i].Monitors = nil
		}
	}

	return rows.Err()
}

// setNeedsAttention marks explicit-mode components with no monitors as needing attention.
func setNeedsAttention(comps []status.Component) {
	for i := range comps {
		comps[i].NeedsAttention = comps[i].CompositionMode == status.CompositionExplicit && len(comps[i].Monitors) == 0
	}
}

// --- Scan helpers ---

func scanComponents(rows *sql.Rows) ([]status.Component, error) {
	var components []status.Component
	for rows.Next() {
		var c status.Component
		var compositionMode string
		var matchAllType sql.NullString
		var override sql.NullString
		var visible, autoInc int
		var createdAt, updatedAt int64

		if err := rows.Scan(
			&c.ID, &compositionMode, &matchAllType, &c.DisplayName,
			&c.DisplayOrder, &visible,
			&override, &autoInc, &createdAt, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan component: %w", err)
		}

		c.CompositionMode = status.CompositionMode(compositionMode)
		if matchAllType.Valid {
			c.MatchAllType = matchAllType.String
		}
		if override.Valid {
			c.StatusOverride = &override.String
		}
		c.Visible = visible != 0
		c.AutoIncident = autoInc != 0
		c.CreatedAt = time.Unix(createdAt, 0).UTC()
		c.UpdatedAt = time.Unix(updatedAt, 0).UTC()
		components = append(components, c)
	}
	return components, rows.Err()
}

// CountConfigured returns the number of operator-configured status-page
// components. The table uses hard-delete only, so no soft-delete filter
// is needed. Used by the telemetry subsystem; see specs/015-shm-telemetry.
func (s *StatusComponentStoreImpl) CountConfigured(ctx context.Context) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM status_components`,
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("count configured status components: %w", err)
	}
	return count, nil
}
