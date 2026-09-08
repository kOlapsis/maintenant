-- Tracks whether a container has actually been through CVE analysis, and how
-- it went. Without this a container never analysed and a container analysed
-- with zero findings both read as "no known CVEs": the scorer needs to tell
-- them apart.
CREATE TABLE cve_evaluations (
    container_id    TEXT PRIMARY KEY NOT NULL,
    status           TEXT NOT NULL,
    evaluated_at     BIGINT NOT NULL,
    ecosystem        TEXT,
    package_name     TEXT,
    package_version  TEXT,
    error            TEXT
);
CREATE INDEX idx_cve_evaluations_status ON cve_evaluations(status);
