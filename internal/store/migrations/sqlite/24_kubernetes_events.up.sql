-- Per-agent Kubernetes events, tagged with the object they concern, so the
-- server can serve them on workload/pod detail views for any agent's cluster.
-- Events are ephemeral: each snapshot fully replaces an agent's rows (the agent
-- reports the current event window), so there is no natural key — ids are minted.

CREATE TABLE kubernetes_events (
    id                 TEXT PRIMARY KEY NOT NULL,
    agent_id           TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000' REFERENCES agents(id) ON DELETE CASCADE,
    involved_kind      TEXT NOT NULL DEFAULT '',
    involved_namespace TEXT NOT NULL DEFAULT '',
    involved_name      TEXT NOT NULL DEFAULT '',
    type               TEXT NOT NULL DEFAULT '',
    reason             TEXT NOT NULL DEFAULT '',
    message            TEXT NOT NULL DEFAULT '',
    source             TEXT NOT NULL DEFAULT '',
    first_seen         BIGINT NOT NULL DEFAULT 0,
    last_seen          BIGINT NOT NULL DEFAULT 0,
    count              INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_k8s_events_object ON kubernetes_events(agent_id, involved_kind, involved_namespace, involved_name);
