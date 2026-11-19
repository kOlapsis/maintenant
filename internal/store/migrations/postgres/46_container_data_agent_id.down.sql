DROP INDEX uq_version_pins_agent_container;
DELETE FROM version_pins p WHERE EXISTS (SELECT 1 FROM version_pins o
    WHERE o.container_id = p.container_id AND o.agent_id < p.agent_id);
ALTER TABLE version_pins DROP COLUMN agent_id;
CREATE UNIQUE INDEX uq_version_pins_container ON version_pins(container_id);

DROP INDEX uq_container_cves_agent;
DELETE FROM container_cves v WHERE EXISTS (SELECT 1 FROM container_cves o
    WHERE o.container_id = v.container_id AND o.cve_id = v.cve_id AND o.agent_id < v.agent_id);
ALTER TABLE container_cves DROP COLUMN agent_id;
CREATE UNIQUE INDEX uq_container_cves ON container_cves(container_id, cve_id);

DELETE FROM risk_score_history h WHERE EXISTS (SELECT 1 FROM risk_score_history o
    WHERE o.container_id = h.container_id AND o.recorded_at = h.recorded_at AND o.agent_id < h.agent_id);
ALTER TABLE risk_score_history DROP COLUMN agent_id;

ALTER TABLE cve_evaluations DROP CONSTRAINT cve_evaluations_pkey;
DELETE FROM cve_evaluations e WHERE EXISTS (SELECT 1 FROM cve_evaluations o
    WHERE o.container_id = e.container_id AND o.agent_id < e.agent_id);
ALTER TABLE cve_evaluations DROP COLUMN agent_id;
ALTER TABLE cve_evaluations ADD PRIMARY KEY (container_id);

ALTER TABLE risk_acknowledgments DROP CONSTRAINT uq_risk_ack_agent_finding;
DELETE FROM risk_acknowledgments r WHERE EXISTS (SELECT 1 FROM risk_acknowledgments o
    WHERE o.container_external_id = r.container_external_id AND o.finding_type = r.finding_type
      AND o.finding_key = r.finding_key AND o.agent_id < r.agent_id);
ALTER TABLE risk_acknowledgments DROP COLUMN agent_id;
ALTER TABLE risk_acknowledgments ADD UNIQUE (container_external_id, finding_type, finding_key);

ALTER TABLE digest_baselines DROP CONSTRAINT digest_baselines_pkey;
DELETE FROM digest_baselines d WHERE EXISTS (SELECT 1 FROM digest_baselines o
    WHERE o.container_id = d.container_id AND o.agent_id < d.agent_id);
ALTER TABLE digest_baselines DROP COLUMN agent_id;
ALTER TABLE digest_baselines ADD PRIMARY KEY (container_id);
