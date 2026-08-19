CREATE TABLE resource_hourly (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  container_id INTEGER NOT NULL,
  bucket INTEGER NOT NULL,
  avg_cpu_percent REAL NOT NULL,
  avg_mem_used INTEGER NOT NULL,
  avg_mem_limit INTEGER NOT NULL,
  avg_net_rx_bytes INTEGER NOT NULL,
  avg_net_tx_bytes INTEGER NOT NULL,
  sample_count INTEGER NOT NULL,
  UNIQUE(container_id, bucket)
);
CREATE INDEX idx_resource_hourly_lookup ON resource_hourly(container_id, bucket);

CREATE TABLE resource_daily (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  container_id INTEGER NOT NULL,
  bucket INTEGER NOT NULL,
  avg_cpu_percent REAL NOT NULL,
  avg_mem_used INTEGER NOT NULL,
  avg_mem_limit INTEGER NOT NULL,
  avg_net_rx_bytes INTEGER NOT NULL,
  avg_net_tx_bytes INTEGER NOT NULL,
  sample_count INTEGER NOT NULL,
  UNIQUE(container_id, bucket)
);
CREATE INDEX idx_resource_daily_lookup ON resource_daily(container_id, bucket);
