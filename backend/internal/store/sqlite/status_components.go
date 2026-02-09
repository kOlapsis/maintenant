package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kolapsis/pulseboard/internal/status"
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

// --- Groups ---

func (s *StatusComponentStoreImpl) ListGroups(ctx context.Context) ([]status.ComponentGroup, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT g.id, g.name, g.display_order, g.created_at,
			COUNT(sc.id) AS component_count
		FROM component_groups g
		LEFT JOIN status_components sc ON sc.group_id = g.id
		GROUP BY g.id
		ORDER BY g.display_order, g.name`)
	if err != nil {
		return nil, fmt.Errorf("list groups: %w", err)
	}
	defer rows.Close()

	var groups []status.ComponentGroup
	for rows.Next() {
		var g status.ComponentGroup
		var createdAt int64
		if err := rows.Scan(&g.ID, &g.Name, &g.DisplayOrder, &createdAt, &g.ComponentCount); err != nil {
			return nil, fmt.Errorf("scan group: %w", err)
		}
		g.CreatedAt = time.Unix(createdAt, 0).UTC()
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

func (s *StatusComponentStoreImpl) GetGroup(ctx context.Context, id int64) (*status.ComponentGroup, error) {
	var g status.ComponentGroup
	var createdAt int64
	err := s.db.QueryRowContext(ctx,
		`SELECT g.id, g.name, g.display_order, g.created_at,
			(SELECT COUNT(*) FROM status_components WHERE group_id = g.id) AS component_count
		FROM component_groups g WHERE g.id = ?`, id,
	).Scan(&g.ID, &g.Name, &g.DisplayOrder, &createdAt, &g.ComponentCount)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get group: %w", err)
	}
	g.CreatedAt = time.Unix(createdAt, 0).UTC()
	return &g, nil
}

func (s *StatusComponentStoreImpl) CreateGroup(ctx context.Context, g *status.ComponentGroup) (int64, error) {
	now := time.Now().Unix()
	res, err := s.writer.Exec(ctx,
		`INSERT INTO component_groups (name, display_order, created_at) VALUES (?, ?, ?)`,
		g.Name, g.DisplayOrder, now,
	)
	if err != nil {
		return 0, fmt.Errorf("create group: %w", err)
	}
	g.ID = res.LastInsertID
	g.CreatedAt = time.Unix(now, 0).UTC()
	return res.LastInsertID, nil
}

func (s *StatusComponentStoreImpl) UpdateGroup(ctx context.Context, g *status.ComponentGroup) error {
	_, err := s.writer.Exec(ctx,
		`UPDATE component_groups SET name = ?, display_order = ? WHERE id = ?`,
		g.Name, g.DisplayOrder, g.ID,
	)
	if err != nil {
		return fmt.Errorf("update group: %w", err)
	}
	return nil
}

func (s *StatusComponentStoreImpl) DeleteGroup(ctx context.Context, id int64) error {
	_, err := s.writer.Exec(ctx,
		`DELETE FROM component_groups WHERE id = ?`, id,
	)
	if err != nil {
		return fmt.Errorf("delete group: %w", err)
	}
	return nil
}

// --- Components ---

func (s *StatusComponentStoreImpl) ListComponents(ctx context.Context) ([]status.StatusComponent, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT sc.id, sc.monitor_type, sc.monitor_id, sc.display_name,
			sc.group_id, g.name, sc.display_order, sc.visible,
			sc.status_override, sc.auto_incident, sc.created_at, sc.updated_at
		FROM status_components sc
		LEFT JOIN component_groups g ON g.id = sc.group_id
		ORDER BY COALESCE(g.display_order, 999999), sc.display_order, sc.display_name`)
	if err != nil {
		return nil, fmt.Errorf("list components: %w", err)
	}
	defer rows.Close()
	return scanComponents(rows)
}

func (s *StatusComponentStoreImpl) ListVisibleComponents(ctx context.Context) ([]status.StatusComponent, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT sc.id, sc.monitor_type, sc.monitor_id, sc.display_name,
			sc.group_id, g.name, sc.display_order, sc.visible,
			sc.status_override, sc.auto_incident, sc.created_at, sc.updated_at
		FROM status_components sc
		LEFT JOIN component_groups g ON g.id = sc.group_id
		WHERE sc.visible = 1
		ORDER BY COALESCE(g.display_order, 999999), sc.display_order, sc.display_name`)
	if err != nil {
		return nil, fmt.Errorf("list visible components: %w", err)
	}
	defer rows.Close()
	return scanComponents(rows)
}

func (s *StatusComponentStoreImpl) GetComponent(ctx context.Context, id int64) (*status.StatusComponent, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT sc.id, sc.monitor_type, sc.monitor_id, sc.display_name,
			sc.group_id, g.name, sc.display_order, sc.visible,
			sc.status_override, sc.auto_incident, sc.created_at, sc.updated_at
		FROM status_components sc
		LEFT JOIN component_groups g ON g.id = sc.group_id
		WHERE sc.id = ?`, id)
	return scanComponent(row)
}

func (s *StatusComponentStoreImpl) GetComponentByMonitor(ctx context.Context, monitorType string, monitorID int64) (*status.StatusComponent, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT sc.id, sc.monitor_type, sc.monitor_id, sc.display_name,
			sc.group_id, g.name, sc.display_order, sc.visible,
			sc.status_override, sc.auto_incident, sc.created_at, sc.updated_at
		FROM status_components sc
		LEFT JOIN component_groups g ON g.id = sc.group_id
		WHERE sc.monitor_type = ? AND sc.monitor_id = ?`, monitorType, monitorID)
	c, err := scanComponent(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return c, err
}

func (s *StatusComponentStoreImpl) CreateComponent(ctx context.Context, c *status.StatusComponent) (int64, error) {
	now := time.Now().Unix()
	res, err := s.writer.Exec(ctx,
		`INSERT INTO status_components (monitor_type, monitor_id, display_name, group_id,
			display_order, visible, status_override, auto_incident, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.MonitorType, c.MonitorID, c.DisplayName, c.GroupID,
		c.DisplayOrder, boolToInt(c.Visible), c.StatusOverride, boolToInt(c.AutoIncident),
		now, now,
	)
	if err != nil {
		return 0, fmt.Errorf("create component: %w", err)
	}
	c.ID = res.LastInsertID
	c.CreatedAt = time.Unix(now, 0).UTC()
	c.UpdatedAt = c.CreatedAt
	return res.LastInsertID, nil
}

func (s *StatusComponentStoreImpl) UpdateComponent(ctx context.Context, c *status.StatusComponent) error {
	now := time.Now().Unix()
	_, err := s.writer.Exec(ctx,
		`UPDATE status_components SET display_name = ?, group_id = ?, display_order = ?,
			visible = ?, status_override = ?, auto_incident = ?, updated_at = ?
		WHERE id = ?`,
		c.DisplayName, c.GroupID, c.DisplayOrder,
		boolToInt(c.Visible), c.StatusOverride, boolToInt(c.AutoIncident),
		now, c.ID,
	)
	if err != nil {
		return fmt.Errorf("update component: %w", err)
	}
	c.UpdatedAt = time.Unix(now, 0).UTC()
	return nil
}

func (s *StatusComponentStoreImpl) DeleteComponent(ctx context.Context, id int64) error {
	_, err := s.writer.Exec(ctx, `DELETE FROM status_components WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete component: %w", err)
	}
	return nil
}

// --- Scan helpers ---

func scanComponents(rows *sql.Rows) ([]status.StatusComponent, error) {
	var components []status.StatusComponent
	for rows.Next() {
		var c status.StatusComponent
		var groupID sql.NullInt64
		var groupName sql.NullString
		var override sql.NullString
		var visible, autoInc int
		var createdAt, updatedAt int64

		if err := rows.Scan(
			&c.ID, &c.MonitorType, &c.MonitorID, &c.DisplayName,
			&groupID, &groupName, &c.DisplayOrder, &visible,
			&override, &autoInc, &createdAt, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan component: %w", err)
		}

		if groupID.Valid {
			c.GroupID = &groupID.Int64
		}
		if groupName.Valid {
			c.GroupName = groupName.String
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

func scanComponent(row *sql.Row) (*status.StatusComponent, error) {
	var c status.StatusComponent
	var groupID sql.NullInt64
	var groupName sql.NullString
	var override sql.NullString
	var visible, autoInc int
	var createdAt, updatedAt int64

	err := row.Scan(
		&c.ID, &c.MonitorType, &c.MonitorID, &c.DisplayName,
		&groupID, &groupName, &c.DisplayOrder, &visible,
		&override, &autoInc, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}

	if groupID.Valid {
		c.GroupID = &groupID.Int64
	}
	if groupName.Valid {
		c.GroupName = groupName.String
	}
	if override.Valid {
		c.StatusOverride = &override.String
	}
	c.Visible = visible != 0
	c.AutoIncident = autoInc != 0
	c.CreatedAt = time.Unix(createdAt, 0).UTC()
	c.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	return &c, nil
}
