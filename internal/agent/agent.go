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
// Phase 1 (US1): detect runtime, load/create identity, enroll if not yet registered, then exit.
// Phase 2 (US2) will add the long-lived gRPC Push streaming loop.
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

	if id.Registered {
		logger.Info("agent already enrolled, skipping register", "agent_id", id.AgentID)
		return nil
	}

	if cfg.EnrollmentToken == "" {
		return fmt.Errorf("agent is not enrolled and --enrollment-token is empty")
	}

	grpcClient, err := NewClient(ctx, cfg.ServerURL, cfg.InsecureSkipVerify, logger)
	if err != nil {
		return fmt.Errorf("connect to server: %w", err)
	}
	defer grpcClient.Close()

	if err := RunEnrollment(ctx, id, cfg.DataDir, cfg.EnrollmentToken, rt, cfg.Label, cfg.AgentVersion, grpcClient); err != nil {
		return fmt.Errorf("enrollment: %w", err)
	}

	logger.Info("agent enrolled successfully", "agent_id", id.AgentID, "runtime", rt)
	return nil
}
