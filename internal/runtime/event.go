// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package runtime

import "time"

// Resource types for RuntimeEvent.
const (
	ResourceContainer = "container"
	ResourceService   = "service"
	ResourceNode      = "node"
)

// RuntimeEvent is a normalized state change from any runtime.
type RuntimeEvent struct {
	Action       string
	ExternalID   string
	Name         string
	Image        string
	ExitCode     string
	HealthStatus string
	ErrorDetail  string
	ResourceType string // "container", "service", or "node"
	Timestamp    time.Time
	Labels       map[string]string
}
