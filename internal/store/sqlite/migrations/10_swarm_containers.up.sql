ALTER TABLE containers ADD COLUMN swarm_service_id TEXT DEFAULT '';
ALTER TABLE containers ADD COLUMN swarm_service_name TEXT DEFAULT '';
ALTER TABLE containers ADD COLUMN swarm_service_mode TEXT DEFAULT '';
ALTER TABLE containers ADD COLUMN swarm_node_id TEXT DEFAULT '';
ALTER TABLE containers ADD COLUMN swarm_task_slot INTEGER DEFAULT 0;
ALTER TABLE containers ADD COLUMN swarm_desired_replicas INTEGER DEFAULT 0;

CREATE INDEX idx_containers_swarm_service ON containers(swarm_service_id) WHERE swarm_service_id != '';
