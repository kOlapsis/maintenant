// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

// Package agentevent carries the per-event metadata the agent server hands to
// the domain services.
package agentevent

import "time"

// Meta is the observation time, the replay flag and the id of an event pushed by an agent.
type Meta struct {
	ObservedAt time.Time
	Replayed   bool
	EventID    string
}
