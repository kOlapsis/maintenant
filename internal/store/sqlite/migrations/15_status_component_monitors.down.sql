-- Best-effort restore — lossy for multi-monitor explicit bundles (only first ref preserved)
ALTER TABLE status_components ADD COLUMN monitor_type TEXT NOT NULL DEFAULT '';
ALTER TABLE status_components ADD COLUMN monitor_id   INTEGER NOT NULL DEFAULT 0;

UPDATE status_components
SET monitor_type = (
    SELECT m.monitor_type FROM status_component_monitors m
    WHERE m.component_id = status_components.id
    ORDER BY m.monitor_type ASC, m.monitor_id ASC LIMIT 1
),
monitor_id = (
    SELECT m.monitor_id FROM status_component_monitors m
    WHERE m.component_id = status_components.id
    ORDER BY m.monitor_type ASC, m.monitor_id ASC LIMIT 1
)
WHERE composition_mode = 'explicit';

UPDATE status_components
    SET monitor_type = match_all_type,
        monitor_id   = 0
    WHERE composition_mode = 'match-all';

ALTER TABLE status_components DROP COLUMN composition_mode;
ALTER TABLE status_components DROP COLUMN match_all_type;

DROP INDEX IF EXISTS idx_status_component_monitors_lookup;
DROP TABLE IF EXISTS status_component_monitors;
