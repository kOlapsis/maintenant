-- Per-agent Swarm topology (services + tasks). swarm_nodes already exists
-- (migration 11 / uuid_schema). These two tables let the server serve the
-- Services/Tasks views for any agent's swarm — local runtime under the
-- LocalAgent sentinel, remote agents under their own id — instead of only the
-- server's live Docker runtime. Reconciled per agent: rows absent from an
-- agent's latest snapshot are hard-deleted.
--
-- Forward migration: runs before the one-time UUID conversion on
-- fresh/unconverted databases and stands alone on already-converted ones.
-- The agents parent is recreated by the conversion; the FK resolves by name.

CREATE TABLE swarm_services (
    id               TEXT PRIMARY KEY NOT NULL,                 -- uid.SwarmService(agent, service_id)
    agent_id         TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000' REFERENCES agents(id) ON DELETE CASCADE,
    service_id       TEXT NOT NULL,
    name             TEXT NOT NULL,
    image            TEXT NOT NULL DEFAULT '',
    mode             TEXT NOT NULL DEFAULT '',
    desired_replicas INTEGER NOT NULL DEFAULT 0,
    running_replicas INTEGER NOT NULL DEFAULT 0,
    labels           TEXT NOT NULL DEFAULT '{}',
    stack_name       TEXT NOT NULL DEFAULT '',
    created_at       BIGINT NOT NULL DEFAULT 0,
    UNIQUE(agent_id, service_id)
);
CREATE INDEX idx_swarm_services_agent ON swarm_services(agent_id);
CREATE INDEX idx_swarm_services_stack ON swarm_services(agent_id, stack_name);

CREATE TABLE swarm_tasks (
    id            TEXT PRIMARY KEY NOT NULL,                    -- uid.SwarmTask(agent, task_id)
    agent_id      TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000' REFERENCES agents(id) ON DELETE CASCADE,
    task_id       TEXT NOT NULL,
    service_id    TEXT NOT NULL,
    node_id       TEXT NOT NULL DEFAULT '',
    slot          INTEGER NOT NULL DEFAULT 0,
    state         TEXT NOT NULL DEFAULT '',
    desired_state TEXT NOT NULL DEFAULT '',
    container_id  TEXT NOT NULL DEFAULT '',
    error         TEXT NOT NULL DEFAULT '',
    exit_code     INTEGER,
    timestamp     BIGINT NOT NULL DEFAULT 0,
    node_hostname TEXT NOT NULL DEFAULT '',
    UNIQUE(agent_id, task_id)
);
CREATE INDEX idx_swarm_tasks_agent ON swarm_tasks(agent_id);
CREATE INDEX idx_swarm_tasks_service ON swarm_tasks(agent_id, service_id);
