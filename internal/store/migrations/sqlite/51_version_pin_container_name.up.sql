ALTER TABLE version_pins ADD COLUMN container_name TEXT NOT NULL DEFAULT '';
-- agent_id is unqualified on purpose: before the UUID conversion containers has none, and the name falls back to version_pins.agent_id.
UPDATE version_pins SET container_name = COALESCE((
    SELECT CASE WHEN swarm_service_name <> '' THEN swarm_service_name ELSE name END
    FROM containers
    WHERE external_id = version_pins.container_id AND agent_id = version_pins.agent_id), '');
DELETE FROM version_pins WHERE container_name = '';
