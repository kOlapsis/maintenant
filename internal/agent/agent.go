// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

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
	DataDir             string
	ServerURL           string
	EnrollmentToken     string
	RuntimeOverride     string
	Label               string
	NodeName            string
	AgentVersion        string
	InsecureSkipVerify  bool
	ProxyLabels         bool
	SpoolMaxMemoryBytes int64
	SpoolMaxDiskBytes   int64
	SpoolMaxAgeSeconds  int64
}

// Run is the main agent entry point (mode=agent).
// It detects the local runtime, loads or creates the agent identity, enrolls if needed,
// then enters the long-lived Push streaming loop (US2).
func Run(ctx context.Context, cfg AgentConfig, logger *slog.Logger) error {
	rt, rtLabel, err := resolveRuntime(ctx, cfg.RuntimeOverride, cfg.ProxyLabels, logger)
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

	enroll := func(ctx context.Context, id *Identity) error {
		return RunEnrollment(ctx, id, cfg.DataDir, cfg.EnrollmentToken, rtLabel, cfg.Label, cfg.AgentVersion, grpcClient)
	}
	serve := func(ctx context.Context, id *Identity) error {
		return runEnrolled(ctx, cfg, id, rt, rtLabel, grpcClient, logger)
	}
	return enrollAndServe(ctx, cfg, id, enroll, serve, logger)
}

// enrollAndServe enrolls id when needed and serves it, enrolling a fresh identity once with the configured token when the server refuses the stored one.
func enrollAndServe(
	ctx context.Context,
	cfg AgentConfig,
	id *Identity,
	enroll func(context.Context, *Identity) error,
	serve func(context.Context, *Identity) error,
	logger *slog.Logger,
) error {
	reenrolled := false
	for {
		if !id.Registered {
			if cfg.EnrollmentToken == "" {
				return fmt.Errorf("agent is not enrolled and --enrollment-token is empty")
			}
			if err := enroll(ctx, id); err != nil {
				if reenrolled {
					return fmt.Errorf("enrolling again after the server refused the previous identity: %w; "+
						"the stored identity is kept, create a new enrollment token and restart the agent with it", err)
				}
				return fmt.Errorf("enrollment: %w", err)
			}
			logger.Info("agent enrolled successfully", "agent_id", id.AgentID)
		} else {
			logger.Info("agent already enrolled", "agent_id", id.AgentID)
		}

		err := serve(ctx, id)
		if !errors.Is(err, ErrAgentRevokedServer) && !errors.Is(err, ErrAgentUnknownServer) {
			return err
		}
		if cfg.EnrollmentToken == "" || reenrolled {
			return fmt.Errorf("%w (agent %s): create an enrollment token on the server and restart the agent "+
				"with MAINTENANT_ENROLLMENT_TOKEN or --enrollment-token set to it", err, id.AgentID)
		}

		logger.Warn("agent: the server refused this identity, enrolling a new one with the configured token",
			"agent_id", id.AgentID, "reason", err.Error())
		fresh, gerr := newIdentity()
		if gerr != nil {
			return gerr
		}
		id = fresh
		reenrolled = true
	}
}

// runEnrolled streams as id until ctx ends or the server refuses the identity.
func runEnrolled(parent context.Context, cfg AgentConfig, id *Identity, rt runtime.Runtime, rtLabel string, grpcClient *Client, logger *slog.Logger) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	// From here the agent is enrolled and about to stream: report liveness so the
	// container healthcheck has something to read (the agent serves no HTTP).
	healthStopped := StartHealthReporter(ctx, cfg.DataDir, HealthInterval, logger)

	spool := NewSpool(cfg.DataDir, spoolConfig(cfg), logger)
	if perr := spool.PurgeExpired(ctx); perr != nil {
		logger.Warn("agent: cannot purge expired spooled events", "error", perr)
	}

	// The collector outlives any single stream: an agent that stops measuring
	// while the server is unreachable has nothing to replay afterwards.
	collectorDone := make(chan struct{})
	go func() {
		defer close(collectorDone)
		if cerr := RunCollector(ctx, id, rt, rtLabel, cfg.NodeName, spool, logger); cerr != nil && ctx.Err() == nil {
			logger.Error("agent: collector stopped", "error", cerr)
		}
	}()

	hooks := StreamHooks{Acked: spool.Acked, RateLimited: spool.RateLimited}
	err := RunWithReconnect(ctx, grpcClient, id, logger, hooks, func(ctx context.Context, stream *PushStream) error {
		logger.Info("agent: stream authenticated, draining spool", "agent_id", id.AgentID)
		spool.ResetDropped()
		spool.Attach(stream)
		defer spool.Detach()
		return spool.Drain(ctx)
	})

	// Waiting on the collector and the reporter keeps the data directory from
	// being written to after the agent has handed control back.
	cancel()
	<-collectorDone
	<-healthStopped

	if errors.Is(err, ErrAgentRevokedServer) || errors.Is(err, ErrAgentUnknownServer) {
		if derr := spool.Discard(); derr != nil {
			logger.Warn("agent: cannot discard spool after the server refused the identity", "error", derr)
		}
	}
	if cerr := spool.Close(); cerr != nil {
		logger.Warn("agent: spool did not close cleanly", "error", cerr)
	}
	return err
}

func spoolConfig(cfg AgentConfig) SpoolConfig {
	return SpoolConfig{
		MaxMemoryBytes: cfg.SpoolMaxMemoryBytes,
		MaxDiskBytes:   cfg.SpoolMaxDiskBytes,
		MaxAge:         time.Duration(cfg.SpoolMaxAgeSeconds) * time.Second,
	}
}

// resolveRuntime detects (or uses the override for) the local container runtime,
// connects to it, and returns it along with a label ("docker"/"swarm"/"kubernetes")
// suitable for the gRPC enrollment payload. Swarm is derived from the swarm.Detector
// applied to the docker runtime — there is no separate "swarm" factory.
func resolveRuntime(ctx context.Context, override string, proxyLabels bool, logger *slog.Logger) (runtime.Runtime, string, error) {
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
	if dr, ok := rt.(*docker.Runtime); ok {
		dr.SetProxyLabels(proxyLabels)
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
