DROP TABLE IF EXISTS anomaly_settings;
ALTER TABLE anomaly_series_state DROP COLUMN global_mad;
ALTER TABLE anomaly_series_state DROP COLUMN global_median;
