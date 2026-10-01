ALTER TABLE containers ADD COLUMN swarm_desired_replicas INTEGER NOT NULL DEFAULT 0;
ALTER TABLE containers ADD COLUMN swarm_service_mode TEXT NOT NULL DEFAULT '';
