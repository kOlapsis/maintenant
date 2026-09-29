// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package runtime

// HealthInfo holds runtime-agnostic health check information.
type HealthInfo struct {
	HasHealthCheck bool
	Status         string // "healthy", "unhealthy", "starting", "none"
	FailingStreak  int
	LastOutput     string
}
