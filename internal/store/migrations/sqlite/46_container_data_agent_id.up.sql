-- Unqualified on purpose: before the UUID conversion containers has no agent_id, the subqueries then read the outer row's and every row stays on the local sentinel.

ALTER TABLE version_pins ADD COLUMN agent_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
DROP INDEX uq_version_pins_container;
INSERT INTO version_pins (id, container_id, image, pinned_tag, pinned_digest, reason, pinned_at, agent_id)
SELECT lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-7' || substr(lower(hex(randomblob(2))), 2) || '-' ||
       substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' || lower(hex(randomblob(6))),
       p.container_id, p.image, p.pinned_tag, p.pinned_digest, p.reason, p.pinned_at, a.id
FROM version_pins p, agents a
WHERE a.id <> '00000000-0000-0000-0000-000000000000'
  AND EXISTS (SELECT 1 FROM containers WHERE external_id = p.container_id AND agent_id = a.id);
DELETE FROM version_pins WHERE agent_id = '00000000-0000-0000-0000-000000000000'
  AND EXISTS (SELECT 1 FROM containers WHERE external_id = version_pins.container_id)
  AND NOT EXISTS (SELECT 1 FROM containers WHERE external_id = version_pins.container_id
                  AND agent_id = '00000000-0000-0000-0000-000000000000');
CREATE UNIQUE INDEX uq_version_pins_agent_container ON version_pins(agent_id, container_id);

ALTER TABLE container_cves ADD COLUMN agent_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
DROP INDEX uq_container_cves;
INSERT INTO container_cves (id, container_id, cve_id, severity, cvss_score, summary, fixed_in, first_detected_at, resolved_at, agent_id)
SELECT lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-7' || substr(lower(hex(randomblob(2))), 2) || '-' ||
       substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' || lower(hex(randomblob(6))),
       v.container_id, v.cve_id, v.severity, v.cvss_score, v.summary, v.fixed_in, v.first_detected_at, v.resolved_at, a.id
FROM container_cves v, agents a
WHERE a.id <> '00000000-0000-0000-0000-000000000000'
  AND EXISTS (SELECT 1 FROM containers WHERE external_id = v.container_id AND agent_id = a.id);
DELETE FROM container_cves WHERE agent_id = '00000000-0000-0000-0000-000000000000'
  AND EXISTS (SELECT 1 FROM containers WHERE external_id = container_cves.container_id)
  AND NOT EXISTS (SELECT 1 FROM containers WHERE external_id = container_cves.container_id
                  AND agent_id = '00000000-0000-0000-0000-000000000000');
CREATE UNIQUE INDEX uq_container_cves_agent ON container_cves(agent_id, container_id, cve_id);

ALTER TABLE risk_score_history ADD COLUMN agent_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
INSERT INTO risk_score_history (id, container_id, score, factors_json, recorded_at, agent_id)
SELECT lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-7' || substr(lower(hex(randomblob(2))), 2) || '-' ||
       substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' || lower(hex(randomblob(6))),
       h.container_id, h.score, h.factors_json, h.recorded_at, a.id
FROM risk_score_history h, agents a
WHERE a.id <> '00000000-0000-0000-0000-000000000000'
  AND EXISTS (SELECT 1 FROM containers WHERE external_id = h.container_id AND agent_id = a.id);
DELETE FROM risk_score_history WHERE agent_id = '00000000-0000-0000-0000-000000000000'
  AND EXISTS (SELECT 1 FROM containers WHERE external_id = risk_score_history.container_id)
  AND NOT EXISTS (SELECT 1 FROM containers WHERE external_id = risk_score_history.container_id
                  AND agent_id = '00000000-0000-0000-0000-000000000000');

CREATE TABLE cve_evaluations_new (
    container_id     TEXT NOT NULL,
    status           TEXT NOT NULL,
    evaluated_at     BIGINT NOT NULL,
    ecosystem        TEXT,
    package_name     TEXT,
    package_version  TEXT,
    error            TEXT,
    agent_id         TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    PRIMARY KEY (agent_id, container_id)
);
INSERT INTO cve_evaluations_new (container_id, status, evaluated_at, ecosystem, package_name, package_version, error)
SELECT container_id, status, evaluated_at, ecosystem, package_name, package_version, error FROM cve_evaluations;
INSERT INTO cve_evaluations_new (container_id, status, evaluated_at, ecosystem, package_name, package_version, error, agent_id)
SELECT e.container_id, e.status, e.evaluated_at, e.ecosystem, e.package_name, e.package_version, e.error, a.id
FROM cve_evaluations_new e, agents a
WHERE a.id <> '00000000-0000-0000-0000-000000000000'
  AND EXISTS (SELECT 1 FROM containers WHERE external_id = e.container_id AND agent_id = a.id);
DELETE FROM cve_evaluations_new WHERE agent_id = '00000000-0000-0000-0000-000000000000'
  AND EXISTS (SELECT 1 FROM containers WHERE external_id = cve_evaluations_new.container_id)
  AND NOT EXISTS (SELECT 1 FROM containers WHERE external_id = cve_evaluations_new.container_id
                  AND agent_id = '00000000-0000-0000-0000-000000000000');
DROP TABLE cve_evaluations;
ALTER TABLE cve_evaluations_new RENAME TO cve_evaluations;
CREATE INDEX idx_cve_evaluations_status ON cve_evaluations(status);

CREATE TABLE risk_acknowledgments_new (
    id            TEXT PRIMARY KEY NOT NULL,
    container_external_id TEXT NOT NULL,
    finding_type  TEXT NOT NULL,
    finding_key   TEXT NOT NULL DEFAULT '',
    acknowledged_by TEXT NOT NULL DEFAULT '',
    reason        TEXT NOT NULL DEFAULT '',
    acknowledged_at BIGINT NOT NULL,
    agent_id      TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    UNIQUE(agent_id, container_external_id, finding_type, finding_key)
);
INSERT INTO risk_acknowledgments_new (id, container_external_id, finding_type, finding_key, acknowledged_by, reason, acknowledged_at)
SELECT id, container_external_id, finding_type, finding_key, acknowledged_by, reason, acknowledged_at FROM risk_acknowledgments;
INSERT INTO risk_acknowledgments_new (id, container_external_id, finding_type, finding_key, acknowledged_by, reason, acknowledged_at, agent_id)
SELECT lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-7' || substr(lower(hex(randomblob(2))), 2) || '-' ||
       substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' || lower(hex(randomblob(6))),
       r.container_external_id, r.finding_type, r.finding_key, r.acknowledged_by, r.reason, r.acknowledged_at, a.id
FROM risk_acknowledgments_new r, agents a
WHERE a.id <> '00000000-0000-0000-0000-000000000000'
  AND EXISTS (SELECT 1 FROM containers WHERE external_id = r.container_external_id AND agent_id = a.id);
DELETE FROM risk_acknowledgments_new WHERE agent_id = '00000000-0000-0000-0000-000000000000'
  AND EXISTS (SELECT 1 FROM containers WHERE external_id = risk_acknowledgments_new.container_external_id)
  AND NOT EXISTS (SELECT 1 FROM containers WHERE external_id = risk_acknowledgments_new.container_external_id
                  AND agent_id = '00000000-0000-0000-0000-000000000000');
DROP TABLE risk_acknowledgments;
ALTER TABLE risk_acknowledgments_new RENAME TO risk_acknowledgments;
CREATE INDEX idx_risk_ack_container ON risk_acknowledgments(container_external_id);

CREATE TABLE digest_baselines_new (
    container_id  TEXT NOT NULL,
    image         TEXT NOT NULL,
    tag           TEXT NOT NULL,
    remote_digest TEXT NOT NULL,
    checked_at    BIGINT NOT NULL DEFAULT 0,
    agent_id      TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    PRIMARY KEY (agent_id, container_id)
);
INSERT INTO digest_baselines_new (container_id, image, tag, remote_digest, checked_at)
SELECT container_id, image, tag, remote_digest, checked_at FROM digest_baselines;
INSERT INTO digest_baselines_new (container_id, image, tag, remote_digest, checked_at, agent_id)
SELECT d.container_id, d.image, d.tag, d.remote_digest, d.checked_at, a.id
FROM digest_baselines_new d, agents a
WHERE a.id <> '00000000-0000-0000-0000-000000000000'
  AND EXISTS (SELECT 1 FROM containers WHERE external_id = d.container_id AND agent_id = a.id);
DELETE FROM digest_baselines_new WHERE agent_id = '00000000-0000-0000-0000-000000000000'
  AND EXISTS (SELECT 1 FROM containers WHERE external_id = digest_baselines_new.container_id)
  AND NOT EXISTS (SELECT 1 FROM containers WHERE external_id = digest_baselines_new.container_id
                  AND agent_id = '00000000-0000-0000-0000-000000000000');
DROP TABLE digest_baselines;
ALTER TABLE digest_baselines_new RENAME TO digest_baselines;
