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
	rt, err := Detect(ctx, cfg.RuntimeOverride)
	if err != nil {
		return fmt.Errorf("runtime detection: %w", err)
	}
	logger.Info("runtime detected", "runtime", rt)

	id, err := LoadOrCreate(cfg.DataDir)
	if err != nil {
		return fmt.Errorf("identity: %w", err)
	}

	grpcClient, err := NewClient(ctx, cfg.ServerURL, cfg.InsecureSkipVerify, logger)
	if err != nil {
		return fmt.Errorf("connect to server: %w", err)
	}
	defer grpcClient.Close()

	if !id.Registered {
		if cfg.EnrollmentToken == "" {
			return fmt.Errorf("agent is not enrolled and --enrollment-token is empty")
		}
		if err := RunEnrollment(ctx, id, cfg.DataDir, cfg.EnrollmentToken, rt, cfg.Label, cfg.AgentVersion, grpcClient); err != nil {
			return fmt.Errorf("enrollment: %w", err)
		}
		logger.Info("agent enrolled successfully", "agent_id", id.AgentID, "runtime", rt)
	} else {
		logger.Info("agent already enrolled", "agent_id", id.AgentID)
	}

	// Enter the long-lived streaming loop with reconnect.
	err = RunWithReconnect(ctx, grpcClient, id, logger, func(ctx context.Context, stream *PushStream) error {
		logger.Info("agent: stream authenticated, starting collector", "agent_id", id.AgentID)
		return RunCollector(ctx, id, rt, stream, logger)
	})
	if errors.Is(err, ErrAgentRevokedServer) {
		return fmt.Errorf("agent has been revoked by the server — re-enroll to reconnect")
	}
	return err
}
