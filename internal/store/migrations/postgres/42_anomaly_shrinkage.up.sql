ALTER TABLE anomaly_series_state ADD COLUMN global_median DOUBLE PRECISION NOT NULL DEFAULT 0;
ALTER TABLE anomaly_series_state ADD COLUMN global_mad DOUBLE PRECISION NOT NULL DEFAULT 0;

CREATE TABLE anomaly_settings (
    id          INTEGER PRIMARY KEY CHECK(id = 1),
    bucket_pull INTEGER NOT NULL,
    updated_at  BIGINT NOT NULL
);
