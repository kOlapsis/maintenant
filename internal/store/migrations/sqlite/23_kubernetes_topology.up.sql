-- Per-agent Kubernetes topology: namespaces, workloads, pods, nodes. Lets the
-- server serve the Workloads/Pods/Namespaces/Nodes views for any agent's
-- cluster (local runtime under the LocalAgent sentinel, remote agents under
-- their own id) instead of only the server's own live cluster API. Reconciled
-- per agent: rows absent from an agent's latest snapshot are hard-deleted.
--
-- Forward migration: runs before the one-time UUID conversion on
-- fresh/unconverted databases and stands alone on already-converted ones.

CREATE TABLE kubernetes_namespaces (
    id        TEXT PRIMARY KEY NOT NULL,                       -- uid.Namespace(agent, name)
    agent_id  TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000' REFERENCES agents(id) ON DELETE CASCADE,
    name      TEXT NOT NULL,
    UNIQUE(agent_id, name)
);
CREATE INDEX idx_k8s_namespaces_agent ON kubernetes_namespaces(agent_id);

CREATE TABLE kubernetes_workloads (
    id               TEXT PRIMARY KEY NOT NULL,                -- uid.K8sWorkload(agent, workload_id)
    agent_id         TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000' REFERENCES agents(id) ON DELETE CASCADE,
    workload_id      TEXT NOT NULL,                            -- "{namespace}/{kind}/{name}"
    name             TEXT NOT NULL,
    namespace        TEXT NOT NULL,
    kind             TEXT NOT NULL,                            -- Deployment, StatefulSet, DaemonSet, Job
    images           TEXT NOT NULL DEFAULT '[]',
    ready_replicas   INTEGER NOT NULL DEFAULT 0,
    desired_replicas INTEGER NOT NULL DEFAULT 0,
    status           TEXT NOT NULL DEFAULT '',                 -- healthy, degraded, progressing, failed
    created_at       BIGINT NOT NULL DEFAULT 0,
    UNIQUE(agent_id, workload_id)
);
CREATE INDEX idx_k8s_workloads_agent ON kubernetes_workloads(agent_id);
CREATE INDEX idx_k8s_workloads_ns ON kubernetes_workloads(agent_id, namespace);

CREATE TABLE kubernetes_pods (
    id            TEXT PRIMARY KEY NOT NULL,                   -- uid.Pod(agent, namespace, name)
    agent_id      TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000' REFERENCES agents(id) ON DELETE CASCADE,
    namespace     TEXT NOT NULL,
    name          TEXT NOT NULL,
    status        TEXT NOT NULL DEFAULT '',
    status_reason TEXT NOT NULL DEFAULT '',
    restart_count INTEGER NOT NULL DEFAULT 0,
    node_name     TEXT NOT NULL DEFAULT '',
    pod_ip        TEXT NOT NULL DEFAULT '',
    host_ip       TEXT NOT NULL DEFAULT '',
    workload_ref  TEXT NOT NULL DEFAULT '',
    containers    TEXT NOT NULL DEFAULT '[]',
    created_at    BIGINT NOT NULL DEFAULT 0,
    UNIQUE(agent_id, namespace, name)
);
CREATE INDEX idx_k8s_pods_agent ON kubernetes_pods(agent_id);
CREATE INDEX idx_k8s_pods_ns ON kubernetes_pods(agent_id, namespace);
CREATE INDEX idx_k8s_pods_workload ON kubernetes_pods(agent_id, workload_ref);

CREATE TABLE kubernetes_nodes (
    id                         TEXT PRIMARY KEY NOT NULL,      -- uid.K8sNode(agent, name)
    agent_id                   TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000' REFERENCES agents(id) ON DELETE CASCADE,
    name                       TEXT NOT NULL,
    roles                      TEXT NOT NULL DEFAULT '[]',
    status                     TEXT NOT NULL DEFAULT '',       -- ready, not-ready, unknown
    running_pods               INTEGER NOT NULL DEFAULT 0,
    kubernetes_version         TEXT NOT NULL DEFAULT '',
    os_image                   TEXT NOT NULL DEFAULT '',
    architecture               TEXT NOT NULL DEFAULT '',
    capacity_cpu_millicores    BIGINT NOT NULL DEFAULT 0,
    capacity_memory_bytes      BIGINT NOT NULL DEFAULT 0,
    capacity_pods              BIGINT NOT NULL DEFAULT 0,
    allocatable_cpu_millicores BIGINT NOT NULL DEFAULT 0,
    allocatable_memory_bytes   BIGINT NOT NULL DEFAULT 0,
    allocatable_pods           BIGINT NOT NULL DEFAULT 0,
    created_at                 BIGINT NOT NULL DEFAULT 0,
    UNIQUE(agent_id, name)
);
CREATE INDEX idx_k8s_nodes_agent ON kubernetes_nodes(agent_id);
