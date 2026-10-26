// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

// Package agentevent carries the per-event metadata the agent server hands to
// the domain services.
package agentevent

import (
	"errors"
	"time"
)

// Meta is the observation time, the replay flag and the id of an event pushed by an agent.
type Meta struct {
	ObservedAt time.Time
	Replayed   bool
	EventID    string
}

// ErrInvalid marks an event refused for its content, reported back to the agent as invalid_argument.
var ErrInvalid = errors.New("invalid agent event")
