DROP INDEX IF EXISTS idx_cert_monitor_external_id;
-- SQLite does not support DROP COLUMN; recreating the table would be destructive.
-- This down migration only removes the index. The column remains but is unused.
