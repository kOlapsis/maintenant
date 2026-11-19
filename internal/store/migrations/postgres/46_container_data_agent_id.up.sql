ALTER TABLE version_pins ADD COLUMN agent_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
DROP INDEX uq_version_pins_container;
INSERT INTO version_pins (id, container_id, image, pinned_tag, pinned_digest, reason, pinned_at, agent_id)
SELECT gen_random_uuid()::text, p.container_id, p.image, p.pinned_tag, p.pinned_digest, p.reason, p.pinned_at, c.agent_id
FROM version_pins p JOIN (SELECT DISTINCT agent_id, external_id FROM containers) c ON c.external_id = p.container_id
WHERE c.agent_id <> '00000000-0000-0000-0000-000000000000';
DELETE FROM version_pins p WHERE p.agent_id = '00000000-0000-0000-0000-000000000000'
  AND EXISTS (SELECT 1 FROM containers c WHERE c.external_id = p.container_id)
  AND NOT EXISTS (SELECT 1 FROM containers c WHERE c.external_id = p.container_id
                  AND c.agent_id = '00000000-0000-0000-0000-000000000000');
CREATE UNIQUE INDEX uq_version_pins_agent_container ON version_pins(agent_id, container_id);

ALTER TABLE container_cves ADD COLUMN agent_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
DROP INDEX uq_container_cves;
INSERT INTO container_cves (id, container_id, cve_id, severity, cvss_score, summary, fixed_in, first_detected_at, resolved_at, agent_id)
SELECT gen_random_uuid()::text, v.container_id, v.cve_id, v.severity, v.cvss_score, v.summary, v.fixed_in,
       v.first_detected_at, v.resolved_at, c.agent_id
FROM container_cves v JOIN (SELECT DISTINCT agent_id, external_id FROM containers) c ON c.external_id = v.container_id
WHERE c.agent_id <> '00000000-0000-0000-0000-000000000000';
DELETE FROM container_cves v WHERE v.agent_id = '00000000-0000-0000-0000-000000000000'
  AND EXISTS (SELECT 1 FROM containers c WHERE c.external_id = v.container_id)
  AND NOT EXISTS (SELECT 1 FROM containers c WHERE c.external_id = v.container_id
                  AND c.agent_id = '00000000-0000-0000-0000-000000000000');
CREATE UNIQUE INDEX uq_container_cves_agent ON container_cves(agent_id, container_id, cve_id);

ALTER TABLE risk_score_history ADD COLUMN agent_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
INSERT INTO risk_score_history (id, container_id, score, factors_json, recorded_at, agent_id)
SELECT gen_random_uuid()::text, h.container_id, h.score, h.factors_json, h.recorded_at, c.agent_id
FROM risk_score_history h JOIN (SELECT DISTINCT agent_id, external_id FROM containers) c ON c.external_id = h.container_id
WHERE c.agent_id <> '00000000-0000-0000-0000-000000000000';
DELETE FROM risk_score_history h WHERE h.agent_id = '00000000-0000-0000-0000-000000000000'
  AND EXISTS (SELECT 1 FROM containers c WHERE c.external_id = h.container_id)
  AND NOT EXISTS (SELECT 1 FROM containers c WHERE c.external_id = h.container_id
                  AND c.agent_id = '00000000-0000-0000-0000-000000000000');

ALTER TABLE cve_evaluations ADD COLUMN agent_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE cve_evaluations DROP CONSTRAINT cve_evaluations_pkey;
INSERT INTO cve_evaluations (container_id, status, evaluated_at, ecosystem, package_name, package_version, error, agent_id)
SELECT e.container_id, e.status, e.evaluated_at, e.ecosystem, e.package_name, e.package_version, e.error, c.agent_id
FROM cve_evaluations e JOIN (SELECT DISTINCT agent_id, external_id FROM containers) c ON c.external_id = e.container_id
WHERE c.agent_id <> '00000000-0000-0000-0000-000000000000';
DELETE FROM cve_evaluations e WHERE e.agent_id = '00000000-0000-0000-0000-000000000000'
  AND EXISTS (SELECT 1 FROM containers c WHERE c.external_id = e.container_id)
  AND NOT EXISTS (SELECT 1 FROM containers c WHERE c.external_id = e.container_id
                  AND c.agent_id = '00000000-0000-0000-0000-000000000000');
ALTER TABLE cve_evaluations ADD PRIMARY KEY (agent_id, container_id);

ALTER TABLE risk_acknowledgments ADD COLUMN agent_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE risk_acknowledgments DROP CONSTRAINT risk_acknowledgments_container_external_id_finding_type_fin_key;
INSERT INTO risk_acknowledgments (id, container_external_id, finding_type, finding_key, acknowledged_by, reason, acknowledged_at, agent_id)
SELECT gen_random_uuid()::text, r.container_external_id, r.finding_type, r.finding_key, r.acknowledged_by, r.reason,
       r.acknowledged_at, c.agent_id
FROM risk_acknowledgments r JOIN (SELECT DISTINCT agent_id, external_id FROM containers) c ON c.external_id = r.container_external_id
WHERE c.agent_id <> '00000000-0000-0000-0000-000000000000';
DELETE FROM risk_acknowledgments r WHERE r.agent_id = '00000000-0000-0000-0000-000000000000'
  AND EXISTS (SELECT 1 FROM containers c WHERE c.external_id = r.container_external_id)
  AND NOT EXISTS (SELECT 1 FROM containers c WHERE c.external_id = r.container_external_id
                  AND c.agent_id = '00000000-0000-0000-0000-000000000000');
ALTER TABLE risk_acknowledgments ADD CONSTRAINT uq_risk_ack_agent_finding
    UNIQUE (agent_id, container_external_id, finding_type, finding_key);

ALTER TABLE digest_baselines ADD COLUMN agent_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE digest_baselines DROP CONSTRAINT digest_baselines_pkey;
INSERT INTO digest_baselines (container_id, image, tag, remote_digest, checked_at, agent_id)
SELECT d.container_id, d.image, d.tag, d.remote_digest, d.checked_at, c.agent_id
FROM digest_baselines d JOIN (SELECT DISTINCT agent_id, external_id FROM containers) c ON c.external_id = d.container_id
WHERE c.agent_id <> '00000000-0000-0000-0000-000000000000';
DELETE FROM digest_baselines d WHERE d.agent_id = '00000000-0000-0000-0000-000000000000'
  AND EXISTS (SELECT 1 FROM containers c WHERE c.external_id = d.container_id)
  AND NOT EXISTS (SELECT 1 FROM containers c WHERE c.external_id = d.container_id
                  AND c.agent_id = '00000000-0000-0000-0000-000000000000');
ALTER TABLE digest_baselines ADD PRIMARY KEY (agent_id, container_id);
