// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package anomaly

// Scope types.
const (
	ScopeTypeContainer = "container"
	ScopeTypeHost      = "host"
)

// Metrics a series can carry.
const (
	MetricCPU       = "cpu"
	MetricMemory    = "memory"
	MetricNetworkIO = "network_io"
	MetricDiskIO    = "disk_io"
	MetricLoad      = "load"
	MetricSwap      = "swap"
	MetricDiskSpace = "disk_space"
)

// Series lifecycle states.
const (
	StateLearning   = "learning"
	StateReady      = "ready"
	StateRelearning = "relearning"
	StateDisabled   = "disabled"
)

// Detector kinds.
const (
	DetectorSpike       = "spike"
	DetectorDrift       = "drift"
	DetectorChangePoint = "change_point"
)

// Anomaly tiers: a passive anomaly is only shown, an active one also raises an alert.
const (
	TierPassive = "passive"
	TierActive  = "active"
)

// Sensitivity presets.
const (
	SensitivityLow    = "low"
	SensitivityMedium = "medium"
	SensitivityHigh   = "high"
)

// Reasons recorded when a series restarts learning.
const (
	ResetReasonImageDigestChange = "image_digest_change"
	ResetReasonManual            = "manual"
)

// SeriesKey identifies one behavioral series.
type SeriesKey struct {
	ScopeType string `json:"scope_type"`
	ScopeID   string `json:"scope_id"`
	Metric    string `json:"metric"`
	Dimension string `json:"dimension"`
}

// Baseline is the learned median and MAD of one series at one hour of the week.
type Baseline struct {
	SeriesKey
	Bucket      int     `json:"bucket"`
	Median      float64 `json:"median"`
	MAD         float64 `json:"mad"`
	SampleCount int     `json:"sample_count"`
	UpdatedAt   int64   `json:"updated_at"`
}

// SeriesState is the lifecycle and score record of one series.
type SeriesState struct {
	SeriesKey
	NodeID          string  `json:"node_id"`
	State           string  `json:"state"`
	FirstSeenAt     int64   `json:"first_seen_at"`
	DaysObserved    float64 `json:"days_observed"`
	ActiveBuckets   int     `json:"active_buckets"`
	ReadyBuckets    int     `json:"ready_buckets"`
	Progress        float64 `json:"progress"`
	ReadyAt         *int64  `json:"ready_at"`
	LastResetAt     *int64  `json:"last_reset_at"`
	LastResetReason string  `json:"last_reset_reason"`
	Sensitivity     string  `json:"sensitivity"`
	CurrentScore    float64 `json:"current_score"`
	ScoreUpdatedAt  *int64  `json:"score_updated_at"`
	GlobalMedian    float64 `json:"global_median"`
	GlobalMAD       float64 `json:"global_mad"`
	UpdatedAt       int64   `json:"updated_at"`
}

// AnomalyEvent is one detected anomaly window, open while EndedAt is nil.
type AnomalyEvent struct {
	ID string `json:"id"`
	SeriesKey
	NodeID         string  `json:"node_id"`
	Detector       string  `json:"detector"`
	Tier           string  `json:"tier"`
	StartedAt      int64   `json:"started_at"`
	EndedAt        *int64  `json:"ended_at"`
	PeakValue      float64 `json:"peak_value"`
	BaselineMedian float64 `json:"baseline_median"`
	PeakDeviation  float64 `json:"peak_deviation"`
	AlertID        string  `json:"alert_id,omitempty"`
	SuppressedBy   string  `json:"suppressed_by,omitempty"`
	CreatedAt      int64   `json:"created_at"`
}

// Settings is the operator-tunable part of detection.
type Settings struct {
	BucketPull int   `json:"bucket_pull"`
	UpdatedAt  int64 `json:"updated_at"`
}

// ScopeIdentity is the part of a container identity a stable scope id is derived from.
type ScopeIdentity struct {
	Name               string
	RuntimeType        string
	OrchestrationGroup string
	OrchestrationUnit  string
	ControllerKind     string
	Namespace          string
	SwarmServiceName   string
}

// HourlyResourceRow is one container hourly rollup with the identity of its container.
type HourlyResourceRow struct {
	ScopeIdentity
	AgentID     string
	Bucket      int64
	CPUPercent  float64
	MemUsed     float64
	NetRxBytes  float64
	NetTxBytes  float64
	SampleCount int
}

// SnapshotRow is one raw container resource snapshot with the identity of its container.
type SnapshotRow struct {
	ScopeIdentity
	AgentID    string
	Timestamp  int64
	CPUPercent float64
	MemUsed    float64
	NetRxBytes float64
	NetTxBytes float64
}

// AnomalyEventFilter narrows a listing of anomaly events; zero fields do not filter.
type AnomalyEventFilter struct {
	ScopeType string
	ScopeID   string
	NodeID    string
	Metric    string
	Tier      string
	Active    *bool
	Limit     int
}
