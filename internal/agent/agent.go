// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/kolapsis/maintenant/internal/docker"
	"github.com/kolapsis/maintenant/internal/runtime"
	"github.com/kolapsis/maintenant/internal/swarm"
)

// Runtime labels reported by the agent during enrollment.
// "docker" / "kubernetes" come from runtime.Runtime.Name(); "swarm" is derived
// from the swarm.Detector check applied to the docker runtime.
const (
	RuntimeDocker     = "docker"
	RuntimeSwarm      = "swarm"
	RuntimeKubernetes = "kubernetes"
)

// AgentConfig holds runtime configuration for an agent process.
type AgentConfig struct {
	DataDir            string
	ServerURL          string
	EnrollmentToken    string
	RuntimeOverride    string
	Label              string
	AgentVersion       string
	InsecureSkipVerify bool
}

// Run is the main agent entry point (mode=agent).
// It detects the local runtime, loads or creates the agent identity, enrolls if needed,
// then enters the long-lived Push streaming loop (US2).
func Run(ctx context.Context, cfg AgentConfig, logger *slog.Logger) error {
	rt, rtLabel, err := resolveRuntime(ctx, cfg.RuntimeOverride, logger)
	if err != nil {
		return fmt.Errorf("runtime detection: %w", err)
	}
	defer func() { _ = rt.Close() }()
	logger.Info("runtime detected", "runtime", rtLabel)

	id, err := LoadOrCreate(cfg.DataDir)
	if err != nil {
		return fmt.Errorf("identity: %w", err)
	}

	grpcClient, err := NewClient(ctx, cfg.ServerURL, cfg.InsecureSkipVerify, logger)
	if err != nil {
		return fmt.Errorf("connect to server: %w", err)
	}
	defer func() { _ = grpcClient.Close() }()

	// Let the server read logs of containers only this host can see.
	grpcClient.EnableCommands(rt, cfg.AgentVersion, logger)

	if !id.Registered {
		if cfg.EnrollmentToken == "" {
			return fmt.Errorf("agent is not enrolled and --enrollment-token is empty")
		}
		if err := RunEnrollment(ctx, id, cfg.DataDir, cfg.EnrollmentToken, rtLabel, cfg.Label, cfg.AgentVersion, grpcClient); err != nil {
			return fmt.Errorf("enrollment: %w", err)
		}
		logger.Info("agent enrolled successfully", "agent_id", id.AgentID, "runtime", rtLabel)
	} else {
		logger.Info("agent already enrolled", "agent_id", id.AgentID)
	}

	err = RunWithReconnect(ctx, grpcClient, id, logger, func(ctx context.Context, stream *PushStream) error {
		logger.Info("agent: stream authenticated, starting collector", "agent_id", id.AgentID)
		return RunCollector(ctx, id, rt, rtLabel, stream, logger)
	})
	if errors.Is(err, ErrAgentRevokedServer) {
		return fmt.Errorf("agent has been revoked by the server — re-enroll to reconnect")
	}
	return err
}

// resolveRuntime detects (or uses the override for) the local container runtime,
// connects to it, and returns it along with a label ("docker"/"swarm"/"kubernetes")
// suitable for the gRPC enrollment payload. Swarm is derived from the swarm.Detector
// applied to the docker runtime — there is no separate "swarm" factory.
func resolveRuntime(ctx context.Context, override string, logger *slog.Logger) (runtime.Runtime, string, error) {
	forceLabel := ""
	rtOverride := override
	switch override {
	case "":
		// auto-detect
	case RuntimeDocker, RuntimeKubernetes:
		// passthrough
	case RuntimeSwarm:
		rtOverride = RuntimeDocker
		forceLabel = RuntimeSwarm
	default:
		return nil, "", fmt.Errorf("unknown runtime override %q (valid: docker, swarm, kubernetes)", override)
	}

	rt, err := runtime.DetectWithOverride(ctx, logger, rtOverride)
	if err != nil {
		return nil, "", err
	}
	if err := rt.Connect(ctx); err != nil {
		_ = rt.Close()
		return nil, "", fmt.Errorf("connect to runtime %s: %w", rt.Name(), err)
	}

	label := rt.Name()
	if forceLabel != "" {
		label = forceLabel
	} else if dr, ok := rt.(*docker.Runtime); ok {
		det := swarm.NewDetector(dr.Client(), logger)
		if res, err := det.Detect(ctx); err == nil && res.Active {
			label = RuntimeSwarm
		}
	}
	return rt, label, nil
}
