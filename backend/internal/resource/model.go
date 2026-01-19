package resource

import "time"

// AlertState represents the current alerting state for a container's resources.
type AlertState string

const (
	AlertStateNormal   AlertState = "normal"
	AlertStateCPU      AlertState = "cpu_alert"
	AlertStateMemory   AlertState = "mem_alert"
	AlertStateBoth     AlertState = "both_alert"
)

// ResourceSnapshot is a point-in-time measurement of a container's resource usage.
type ResourceSnapshot struct {
	ID              int64   `json:"id"`
	ContainerID     int64   `json:"container_id"`
	CPUPercent      float64 `json:"cpu_percent"`
	MemUsed         int64   `json:"mem_used"`
	MemLimit        int64   `json:"mem_limit"`
	NetRxBytes      int64   `json:"net_rx_bytes"`
	NetTxBytes      int64   `json:"net_tx_bytes"`
	BlockReadBytes  int64   `json:"block_read_bytes"`
	BlockWriteBytes int64   `json:"block_write_bytes"`
	Timestamp       time.Time `json:"timestamp"`
}

// ResourceAlertConfig holds per-container resource alert thresholds.
type ResourceAlertConfig struct {
	ID                     int64      `json:"id"`
	ContainerID            int64      `json:"container_id"`
	CPUThreshold           float64    `json:"cpu_threshold"`
	MemThreshold           float64    `json:"mem_threshold"`
	Enabled                bool       `json:"enabled"`
	AlertState             AlertState `json:"alert_state"`
	CPUConsecutiveBreaches int        `json:"cpu_consecutive_breaches"`
	MemConsecutiveBreaches int        `json:"mem_consecutive_breaches"`
	LastAlertedAt          *time.Time `json:"last_alerted_at"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}
