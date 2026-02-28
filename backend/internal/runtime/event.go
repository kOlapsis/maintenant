package runtime

import "time"

// RuntimeEvent is a normalized state change from any runtime.
type RuntimeEvent struct {
	Action       string
	ExternalID   string
	Name         string
	ExitCode     string
	HealthStatus string
	ErrorDetail  string
	Timestamp    time.Time
	Labels       map[string]string
}
