DROP INDEX uq_version_pins_agent_container;
DELETE FROM version_pins WHERE EXISTS (SELECT 1 FROM version_pins o
    WHERE o.container_id = version_pins.container_id AND o.agent_id < version_pins.agent_id);
ALTER TABLE version_pins DROP COLUMN agent_id;
CREATE UNIQUE INDEX uq_version_pins_container ON version_pins(container_id);

DROP INDEX uq_container_cves_agent;
DELETE FROM container_cves WHERE EXISTS (SELECT 1 FROM container_cves o
    WHERE o.container_id = container_cves.container_id AND o.cve_id = container_cves.cve_id
      AND o.agent_id < container_cves.agent_id);
ALTER TABLE container_cves DROP COLUMN agent_id;
CREATE UNIQUE INDEX uq_container_cves ON container_cves(container_id, cve_id);

DELETE FROM risk_score_history WHERE EXISTS (SELECT 1 FROM risk_score_history o
    WHERE o.container_id = risk_score_history.container_id AND o.recorded_at = risk_score_history.recorded_at
      AND o.agent_id < risk_score_history.agent_id);
ALTER TABLE risk_score_history DROP COLUMN agent_id;

CREATE TABLE cve_evaluations_old (
    container_id    TEXT PRIMARY KEY NOT NULL,
    status           TEXT NOT NULL,
    evaluated_at     BIGINT NOT NULL,
    ecosystem        TEXT,
    package_name     TEXT,
    package_version  TEXT,
    error            TEXT
);
INSERT INTO cve_evaluations_old (container_id, status, evaluated_at, ecosystem, package_name, package_version, error)
SELECT container_id, status, evaluated_at, ecosystem, package_name, package_version, error FROM cve_evaluations e
WHERE NOT EXISTS (SELECT 1 FROM cve_evaluations o WHERE o.container_id = e.container_id AND o.agent_id < e.agent_id);
DROP TABLE cve_evaluations;
ALTER TABLE cve_evaluations_old RENAME TO cve_evaluations;
CREATE INDEX idx_cve_evaluations_status ON cve_evaluations(status);

CREATE TABLE risk_acknowledgments_old (
    id            TEXT PRIMARY KEY NOT NULL,
    container_external_id TEXT NOT NULL,
    finding_type  TEXT NOT NULL,
    finding_key   TEXT NOT NULL DEFAULT '',
    acknowledged_by TEXT NOT NULL DEFAULT '',
    reason        TEXT NOT NULL DEFAULT '',
    acknowledged_at BIGINT NOT NULL,
    UNIQUE(container_external_id, finding_type, finding_key)
);
INSERT INTO risk_acknowledgments_old (id, container_external_id, finding_type, finding_key, acknowledged_by, reason, acknowledged_at)
SELECT id, container_external_id, finding_type, finding_key, acknowledged_by, reason, acknowledged_at FROM risk_acknowledgments r
WHERE NOT EXISTS (SELECT 1 FROM risk_acknowledgments o
    WHERE o.container_external_id = r.container_external_id AND o.finding_type = r.finding_type
      AND o.finding_key = r.finding_key AND o.agent_id < r.agent_id);
DROP TABLE risk_acknowledgments;
ALTER TABLE risk_acknowledgments_old RENAME TO risk_acknowledgments;
CREATE INDEX idx_risk_ack_container ON risk_acknowledgments(container_external_id);

CREATE TABLE digest_baselines_old (
    container_id  TEXT PRIMARY KEY NOT NULL,
    image         TEXT NOT NULL,
    tag           TEXT NOT NULL,
    remote_digest TEXT NOT NULL,
    checked_at    BIGINT NOT NULL DEFAULT 0
);
INSERT INTO digest_baselines_old (container_id, image, tag, remote_digest, checked_at)
SELECT container_id, image, tag, remote_digest, checked_at FROM digest_baselines d
WHERE NOT EXISTS (SELECT 1 FROM digest_baselines o WHERE o.container_id = d.container_id AND o.agent_id < d.agent_id);
DROP TABLE digest_baselines;
ALTER TABLE digest_baselines_old RENAME TO digest_baselines;
