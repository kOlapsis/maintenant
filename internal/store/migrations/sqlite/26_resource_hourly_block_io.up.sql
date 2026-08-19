-- Block I/O in the hourly rollup, so the 7-day resource chart can be served
-- from resource_hourly instead of scanning a week of raw snapshots. Without
-- these two columns the aggregate carries CPU, memory and network only, and
-- switching the read path would drop the Block I/O series.
--
-- Existing rows keep 0 until the rollup re-aggregates their bucket, which it
-- does for every bucket still covered by the raw retention window.
ALTER TABLE resource_hourly ADD COLUMN avg_block_read_bytes BIGINT NOT NULL DEFAULT 0;
ALTER TABLE resource_hourly ADD COLUMN avg_block_write_bytes BIGINT NOT NULL DEFAULT 0;
