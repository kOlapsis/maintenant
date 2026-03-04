// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See LICENSE-COMMERCIAL.md
//
// Source: https://github.com/kolapsis/maintenant

package resource

import "time"

// AlertState represents the current alerting state for a container's resources.
type AlertState string

const (
	AlertStateNormal AlertState = "normal"
	AlertStateCPU    AlertState = "cpu_alert"
	AlertStateMemory AlertState = "mem_alert"
	AlertStateBoth   AlertState = "both_alert"
)

// ResourceSnapshot is a point-in-time measurement of a container's resource usage.
type ResourceSnapshot struct {
	ID              int64     `json:"id"`
	ContainerID     int64     `json:"container_id"`
	CPUPercent      float64   `json:"cpu_percent"`
	MemUsed         int64     `json:"mem_used"`
	MemLimit        int64     `json:"mem_limit"`
	NetRxBytes      int64     `json:"net_rx_bytes"`
	NetTxBytes      int64     `json:"net_tx_bytes"`
	BlockReadBytes  int64     `json:"block_read_bytes"`
	BlockWriteBytes int64     `json:"block_write_bytes"`
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

// RollupRow represents an aggregated resource measurement for a time bucket.
type RollupRow struct {
	ContainerID   int64
	Bucket        time.Time
	AvgCPUPercent float64
	AvgMemUsed    int64
	AvgMemLimit   int64
	AvgNetRx      int64
	AvgNetTx      int64
	SampleCount   int
}

// TopConsumerRow represents a container's average resource usage over a period.
type TopConsumerRow struct {
	ContainerID   int64
	ContainerName string
	AvgValue      float64
	AvgPercent    float64
}
