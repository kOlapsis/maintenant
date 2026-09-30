// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package container

import (
	"context"
	"sync"
	"time"
)

const uptimeCacheTTL = 60 * time.Second

// UptimeCalculator computes container uptime percentages from state transitions.
type UptimeCalculator struct {
	store ContainerStore
	cache sync.Map // map[cacheKey]*cacheEntry
}

type cacheKey struct {
	containerID string
	window      string
}

type cacheEntry struct {
	value     float64
	expiresAt time.Time
}

// NewUptimeCalculator creates a new uptime calculator.
func NewUptimeCalculator(store ContainerStore) *UptimeCalculator {
	return &UptimeCalculator{store: store}
}

// Calculate computes a container's uptime over 24h and over every longer window that fits in maxWindow.
func (u *UptimeCalculator) Calculate(ctx context.Context, containerID string, maxWindow time.Duration) (*UptimeResult, error) {
	result := &UptimeResult{}
	windows := []struct {
		name     string
		duration time.Duration
		value    **float64
	}{
		{"24h", 24 * time.Hour, &result.Hours24},
		{"7d", 7 * 24 * time.Hour, &result.Days7},
		{"30d", 30 * 24 * time.Hour, &result.Days30},
		{"90d", 90 * 24 * time.Hour, &result.Days90},
	}
	for i, w := range windows {
		if i > 0 && w.duration > maxWindow {
			break
		}
		pct, err := u.calculateWindow(ctx, containerID, w.duration, w.name)
		if err != nil {
			return nil, err
		}
		*w.value = pct
	}
	return result, nil
}

func (u *UptimeCalculator) calculateWindow(ctx context.Context, containerID string, window time.Duration, windowName string) (*float64, error) {
	key := cacheKey{containerID: containerID, window: windowName}

	// Check cache for 24h window
	if windowName == "24h" {
		if entry, ok := u.cache.Load(key); ok {
			ce := entry.(*cacheEntry)
			if time.Now().Before(ce.expiresAt) {
				return &ce.value, nil
			}
		}
	}

	now := time.Now()
	from := now.Add(-window)

	transitions, err := u.store.GetTransitionsInWindow(ctx, containerID, from, now)
	if err != nil {
		return nil, err
	}

	pct := computeUptime(transitions, from, now)

	// Cache 24h window
	if windowName == "24h" {
		u.cache.Store(key, &cacheEntry{
			value:     pct,
			expiresAt: time.Now().Add(uptimeCacheTTL),
		})
	}

	return &pct, nil
}

// computeUptime calculates uptime percentage from a list of transitions in a time window.
// Health-aware: running+healthy=up for containers with health checks.
func computeUptime(transitions []*StateTransition, from, to time.Time) float64 {
	totalSeconds := to.Sub(from).Seconds()
	if totalSeconds <= 0 {
		return 0
	}

	if len(transitions) == 0 {
		// No transitions: assume container was in its current state for the entire window
		return 100.0
	}

	var upSeconds float64

	for i, t := range transitions {
		var end time.Time
		if i+1 < len(transitions) {
			end = transitions[i+1].Timestamp
		} else {
			end = to
		}

		spanStart := t.Timestamp
		if spanStart.Before(from) {
			spanStart = from
		}
		if end.After(to) {
			end = to
		}

		span := end.Sub(spanStart).Seconds()
		if span <= 0 {
			continue
		}

		if isUp(t) {
			upSeconds += span
		}
	}

	pct := (upSeconds / totalSeconds) * 100.0
	if pct > 100 {
		pct = 100
	}
	// Round to 2 decimal places
	return float64(int(pct*100)) / 100
}

// ComputeUptime is the exported entry point for the time-weighted uptime
// calculation, reused by the per-day uptime aggregation in package sqlite.
func ComputeUptime(transitions []*StateTransition, from, to time.Time) float64 {
	return computeUptime(transitions, from, to)
}

func isUp(t *StateTransition) bool {
	if t.NewState != StateRunning {
		return false
	}
	// Health-aware: if container has health info and is unhealthy, it's not "up"
	if t.NewHealth != nil && *t.NewHealth == HealthUnhealthy {
		return false
	}
	return true
}
