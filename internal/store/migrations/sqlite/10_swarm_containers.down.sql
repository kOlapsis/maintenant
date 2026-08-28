DROP INDEX IF EXISTS idx_containers_swarm_service;
-- SQLite pre-3.35 does not support DROP COLUMN; use rebuild if needed.
-- For development, recreating the table is acceptable.
