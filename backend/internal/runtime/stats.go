package runtime

import "time"

// RawStats is a runtime-agnostic resource snapshot.
// Fields set to -1 indicate the metric is unavailable for this runtime.
type RawStats struct {
	CPUPercent      float64
	MemUsed         int64
	MemLimit        int64
	NetRxBytes      int64 // -1 if unavailable
	NetTxBytes      int64 // -1 if unavailable
	BlockReadBytes  int64 // -1 if unavailable
	BlockWriteBytes int64 // -1 if unavailable
	Timestamp       time.Time
}
