-- Drop the component_groups feature entirely.
-- Order matters: drop the index first (it references group_id), then the column,
-- then the now-orphaned table. SQLite >= 3.35 supports ALTER TABLE DROP COLUMN
-- for columns that are not part of a UNIQUE/PRIMARY KEY constraint or referenced
-- by an index. The inline FK on group_id is dropped together with the column.

DROP INDEX IF EXISTS idx_status_components_group_order;
ALTER TABLE status_components DROP COLUMN group_id;
DROP TABLE IF EXISTS component_groups;
