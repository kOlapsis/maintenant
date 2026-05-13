-- Best-effort restore of the component_groups feature.
-- Component-to-group assignments are not recoverable (they were dropped in the up migration);
-- the column is recreated as NULL for every existing component.

CREATE TABLE IF NOT EXISTS component_groups (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    name          TEXT NOT NULL UNIQUE,
    display_order INTEGER NOT NULL DEFAULT 0,
    created_at    INTEGER NOT NULL
);

ALTER TABLE status_components ADD COLUMN group_id INTEGER REFERENCES component_groups(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_status_components_group_order ON status_components(group_id, display_order);
