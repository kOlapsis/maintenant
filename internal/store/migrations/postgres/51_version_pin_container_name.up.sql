ALTER TABLE version_pins ADD COLUMN container_name TEXT NOT NULL DEFAULT '';
UPDATE version_pins SET container_name = COALESCE((
    SELECT CASE WHEN c.swarm_service_name <> '' THEN c.swarm_service_name ELSE c.name END
    FROM containers c
    WHERE c.agent_id = version_pins.agent_id AND c.external_id = version_pins.container_id), '');
DELETE FROM version_pins WHERE container_name = '';
