// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package swarm

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/moby/moby/api/types/system"
)

// InfoProvider abstracts docker info retrieval for the Swarm detector.
type InfoProvider interface {
	Info(ctx context.Context) (system.Info, error)
}

// DetectionResult holds the outcome of Swarm mode detection.
type DetectionResult struct {
	Active       bool      `json:"active"`
	IsManager    bool      `json:"is_manager"`
	ClusterID    string    `json:"cluster_id,omitempty"`
	CreatedAt    time.Time `json:"created_at,omitzero"`
	ManagerCount int       `json:"manager_count,omitempty"`
	WorkerCount  int       `json:"worker_count,omitempty"`
}

// Cluster returns the cluster this node manages, or nil when it is not an active manager.
func (r DetectionResult) Cluster() *SwarmCluster {
	if !r.Active || !r.IsManager {
		return nil
	}
	return &SwarmCluster{
		ID:           r.ClusterID,
		CreatedAt:    r.CreatedAt,
		ManagerCount: r.ManagerCount,
		WorkerCount:  r.WorkerCount,
		IsManager:    true,
	}
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
		result.CreatedAt = info.Swarm.Cluster.CreatedAt
	}
	result.ManagerCount = info.Swarm.Managers
	result.WorkerCount = info.Swarm.Nodes - info.Swarm.Managers
	if prev := d.Result(); result.IsManager && info.Swarm.Nodes == 0 && prev.IsManager {
		// A manager without a leader reads neither the cluster nor its nodes: keep what it read last.
		result = prev
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

func (d *Detector) setResult(r DetectionResult) {
	d.mu.Lock()
	d.result = r
	d.mu.Unlock()
}
