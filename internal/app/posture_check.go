// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/security"
)

const postureCheckInterval = 5 * time.Minute

// startPostureCheck scores the infrastructure on a timer, so the MAINTENANT_SECURITY_SCORE_THRESHOLD alert does not wait for someone to open the posture.
func (a *App) startPostureCheck(ctx context.Context) {
	ticker := time.NewTicker(postureCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			checkPosture(ctx, a.scorer, a.containerSvc, a.logger)
		}
	}
}

// checkPosture scores the infrastructure the way the posture page does, which evaluates the threshold, while the edition opens the posture.
func checkPosture(ctx context.Context, scorer security.PostureScorer, containers containerLister, logger *slog.Logger) {
	if !extension.Allows(extension.CapSecurityPosture) {
		return
	}
	list, err := containers.ListContainers(ctx, container.ListContainersOpts{})
	if err != nil {
		logger.Warn("security posture check: list containers failed", "error", err)
		return
	}
	infos := make([]security.ContainerInfo, len(list))
	for i, c := range list {
		infos[i] = security.ContainerInfo{ID: c.ID, ExternalID: c.ExternalID, Name: c.Name}
	}
	if _, err := scorer.ScoreInfrastructure(ctx, infos); err != nil {
		logger.Warn("security posture check failed", "error", err)
	}
}
