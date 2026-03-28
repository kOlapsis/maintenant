// Copyright 2026 Benjamin Touchard (kOlapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package swarm

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/docker/docker/api/types/system"
)

// InfoProvider abstracts docker info retrieval for the Swarm detector.
type InfoProvider interface {
	Info(ctx context.Context) (system.Info, error)
}

// DetectionResult holds the outcome of Swarm mode detection.
type DetectionResult struct {
	Active    bool   `json:"active"`
	IsManager bool   `json:"is_manager"`
	ClusterID string `json:"cluster_id,omitempty"`
}

// Detector detects whether the Docker engine is part of a Swarm cluster
// and whether this node is a manager.
type Detector struct {
	provider InfoProvider
	logger   *slog.Logger

	mu     sync.RWMutex
	result DetectionResult
}

// NewDetector creates a new Swarm mode detector.
func NewDetector(provider InfoProvider, logger *slog.Logger) *Detector {
	return &Detector{
		provider: provider,
		logger:   logger,
	}
}

// Detect checks the Docker engine for Swarm mode status.
func (d *Detector) Detect(ctx context.Context) (DetectionResult, error) {
	info, err := d.provider.Info(ctx)
	if err != nil {
		return DetectionResult{}, fmt.Errorf("swarm detection: %w", err)
	}

	result := DetectionResult{}

	if info.Swarm.LocalNodeState != "active" {
		d.setResult(result)
		return result, nil
	}

	result.Active = true
	result.IsManager = info.Swarm.ControlAvailable
	if info.Swarm.Cluster != nil {
		result.ClusterID = info.Swarm.Cluster.ID
	}

	if result.IsManager {
		d.logger.Info("detected Swarm mode (manager node)",
			"cluster_id", result.ClusterID)
	} else {
		d.logger.Info("detected Swarm mode (worker node) — Swarm management APIs not available, falling back to container monitoring",
			"cluster_id", result.ClusterID)
	}

	d.setResult(result)
	return result, nil
}

// Result returns the cached detection result.
func (d *Detector) Result() DetectionResult {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.result
}

// Recheck re-checks the Swarm state and returns true if it changed.
func (d *Detector) Recheck(ctx context.Context) (changed bool, result DetectionResult, err error) {
	prev := d.Result()
	result, err = d.Detect(ctx)
	if err != nil {
		return false, result, err
	}
	changed = prev.Active != result.Active || prev.IsManager != result.IsManager
	return changed, result, nil
}

func (d *Detector) setResult(r DetectionResult) {
	d.mu.Lock()
	d.result = r
	d.mu.Unlock()
}
