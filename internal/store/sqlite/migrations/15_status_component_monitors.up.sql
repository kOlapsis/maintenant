-- Phase A: create join table (CREATE IF NOT EXISTS — safe on re-run)
CREATE TABLE IF NOT EXISTS status_component_monitors (
    component_id  INTEGER NOT NULL REFERENCES status_components(id) ON DELETE CASCADE,
    monitor_type  TEXT NOT NULL,
    monitor_id    INTEGER NOT NULL,
    PRIMARY KEY (component_id, monitor_type, monitor_id)
);

CREATE INDEX IF NOT EXISTS idx_status_component_monitors_lookup
    ON status_component_monitors(monitor_type, monitor_id);

-- Phase B: populate join table from legacy schema.
-- Reads monitor_type/monitor_id which always exist at this point (old schema or dirty-state re-run).
-- INSERT OR IGNORE is safe when re-running after a partial failure.
INSERT OR IGNORE INTO status_component_monitors (component_id, monitor_type, monitor_id)
    SELECT id, monitor_type, monitor_id
    FROM status_components
    WHERE monitor_id != 0;

-- Phase C: create new status_components table with the final schema.
-- DROP IF EXISTS ensures a clean slate on re-runs (no partial table left over).
DROP TABLE IF EXISTS status_components_new;
CREATE TABLE status_components_new (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    composition_mode TEXT NOT NULL DEFAULT 'explicit',
    match_all_type   TEXT,
    display_name     TEXT NOT NULL,
    group_id         INTEGER REFERENCES component_groups(id) ON DELETE SET NULL,
    display_order    INTEGER NOT NULL DEFAULT 0,
    visible          INTEGER NOT NULL DEFAULT 1,
    status_override  TEXT,
    auto_incident    INTEGER NOT NULL DEFAULT 0,
    created_at       INTEGER NOT NULL,
    updated_at       INTEGER NOT NULL
);

-- Phase D: copy data, deriving composition_mode/match_all_type from the legacy monitor_id=0 convention.
-- monitor_type and monitor_id are always present here (old schema, or dirty-state re-run where
-- ALTER TABLE DROP COLUMN had not yet succeeded).
INSERT INTO status_components_new
    (id, composition_mode, match_all_type, display_name, group_id,
     display_order, visible, status_override, auto_incident, created_at, updated_at)
    SELECT
        id,
        CASE WHEN monitor_id = 0 THEN 'match-all' ELSE 'explicit' END,
        CASE WHEN monitor_id = 0 THEN monitor_type ELSE NULL END,
        display_name, group_id, display_order,
        visible, status_override, auto_incident, created_at, updated_at
    FROM status_components;

-- Phase E: swap tables.
-- ALTER TABLE DROP COLUMN cannot remove columns that are part of a UNIQUE constraint,
-- so we replace the table entirely.  SQLite allows DROP TABLE even when child tables
-- hold FK references to it; the rename restores the target immediately after.
DROP TABLE status_components;
ALTER TABLE status_components_new RENAME TO status_components;

-- Phase F: restore indexes.
CREATE INDEX idx_status_components_group_order ON status_components(group_id, display_order);
CREATE INDEX idx_status_components_visible ON status_components(visible);
