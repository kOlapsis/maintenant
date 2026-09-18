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

package container

import (
	"context"
	"time"

	"github.com/kolapsis/maintenant/internal/event"
)

const defaultRestartRecoveryInterval = time.Minute

func (s *Service) trackRestartAlert(id string) {
	s.restartMu.Lock()
	s.trackedRestartAlerts[id] = struct{}{}
	s.restartMu.Unlock()
}

func (s *Service) untrackRestartAlert(id string) {
	s.restartMu.Lock()
	delete(s.trackedRestartAlerts, id)
	s.restartMu.Unlock()
}

// TrackRestartAlerts adds containers that already have an open restart alert to the recovery loop.
func (s *Service) TrackRestartAlerts(ids []string) {
	s.restartMu.Lock()
	defer s.restartMu.Unlock()
	for _, id := range ids {
		s.trackedRestartAlerts[id] = struct{}{}
	}
}

// RunRestartRecoveryLoop re-evaluates the tracked containers until ctx is done.
func (s *Service) RunRestartRecoveryLoop(ctx context.Context) {
	if s.restartChecker == nil {
		return
	}
	ticker := time.NewTicker(s.restartRecoveryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.checkRestartRecoveries(ctx)
		}
	}
}

func (s *Service) checkRestartRecoveries(ctx context.Context) {
	s.restartMu.Lock()
	ids := make([]string, 0, len(s.trackedRestartAlerts))
	for id := range s.trackedRestartAlerts {
		ids = append(ids, id)
	}
	s.restartMu.Unlock()

	for _, id := range ids {
		s.recheckRestartRecovery(ctx, id)
	}
}

func (s *Service) recheckRestartRecovery(ctx context.Context, id string) {
	c, err := s.store.GetContainerByID(ctx, id)
	if err != nil {
		s.logger.Error("restart recovery recheck: get container", "container_id", id, "error", err)
		return
	}
	if c == nil || c.Archived {
		s.untrackRestartAlert(id)
		return
	}
	if c.State != StateRunning {
		return
	}

	result, err := s.restartChecker.Check(ctx, c)
	if err != nil {
		s.logger.Error("restart recovery recheck: check", "container_id", id, "error", err)
		return
	}
	if result != nil {
		return
	}

	s.untrackRestartAlert(id)
	s.emitEvent(event.ContainerRestartRecover, map[string]interface{}{
		"container_id":   c.ID,
		"container_name": c.Name,
		"timestamp":      time.Now(),
		"agent_id":       c.AgentID,
	})
}
