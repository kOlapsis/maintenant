// Copyright 2026 Benjamin Touchard (kOlapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See LICENSE-COMMERCIAL.md
//
// Source: https://github.com/kolapsis/maintenant

package security

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// AlertCallback is called when security insights change and an alert should be fired.
type AlertCallback func(containerID int64, containerName string, insights []Insight, isRecover bool)

// EventCallback is called to broadcast SSE events.
type EventCallback func(eventType string, data any)

// Service manages in-memory security insight state and emits alerts/events on changes.
type Service struct {
	mu       sync.RWMutex
	store    map[int64][]Insight       // containerID → current insights
	previous map[int64]map[InsightType]bool // containerID → set of previously seen insight types
	logger   *slog.Logger

	onAlert AlertCallback
	onEvent EventCallback
}

// NewService creates a new security insight service.
func NewService(logger *slog.Logger) *Service {
	return &Service{
		store:    make(map[int64][]Insight),
		previous: make(map[int64]map[InsightType]bool),
		logger:   logger,
	}
}

// SetAlertCallback sets the callback for alert events.
func (s *Service) SetAlertCallback(cb AlertCallback) {
	s.onAlert = cb
}

// SetEventCallback sets the callback for SSE broadcasting.
func (s *Service) SetEventCallback(cb EventCallback) {
	s.onEvent = cb
}

// UpdateContainer processes new insights for a single container, computes diffs,
// and emits alerts/events as needed.
func (s *Service) UpdateContainer(containerID int64, containerName string, newInsights []Insight) {
	s.mu.Lock()
	defer s.mu.Unlock()

	oldTypes := s.previous[containerID]
	newTypes := make(map[InsightType]bool, len(newInsights))
	for _, i := range newInsights {
		newTypes[i.Type] = true
	}

	hadInsights := len(oldTypes) > 0
	hasInsights := len(newInsights) > 0

	// Determine if there's a material change
	changed := s.hasChange(oldTypes, newTypes)

	// Update stores
	if hasInsights {
		s.store[containerID] = newInsights
	} else {
		delete(s.store, containerID)
	}
	if hasInsights {
		s.previous[containerID] = newTypes
	} else {
		delete(s.previous, containerID)
	}

	if !changed {
		return
	}

	if hasInsights {
		s.logger.Info("security: insights detected",
			"container_id", containerID,
			"container_name", containerName,
			"count", len(newInsights),
			"highest_severity", HighestSeverity(newInsights),
		)

		if s.onAlert != nil {
			s.onAlert(containerID, containerName, newInsights, false)
		}
		if s.onEvent != nil {
			change := "updated"
			if !hadInsights {
				change = "new"
			}
			s.onEvent("security.insights_changed", map[string]any{
				"container_id":     containerID,
				"container_name":   containerName,
				"highest_severity": HighestSeverity(newInsights),
				"count":            len(newInsights),
				"change":           change,
			})
		}
	} else if hadInsights {
		// All insights cleared → resolution
		s.logger.Info("security: insights resolved",
			"container_id", containerID,
			"container_name", containerName,
		)

		if s.onAlert != nil {
			s.onAlert(containerID, containerName, nil, true)
		}
		if s.onEvent != nil {
			s.onEvent("security.insights_resolved", map[string]any{
				"container_id":   containerID,
				"container_name": containerName,
			})
		}
	}
}

// RemoveContainer cleans up state for a container that no longer exists.
func (s *Service) RemoveContainer(containerID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.store, containerID)
	delete(s.previous, containerID)
}

// GetContainerInsights returns the current insights for a container.
func (s *Service) GetContainerInsights(containerID int64) *ContainerInsights {
	s.mu.RLock()
	defer s.mu.RUnlock()

	insights, ok := s.store[containerID]
	if !ok || len(insights) == 0 {
		return &ContainerInsights{
			ContainerID: containerID,
			Count:       0,
			Insights:    []Insight{},
		}
	}

	hs := HighestSeverity(insights)
	return &ContainerInsights{
		ContainerID:     containerID,
		ContainerName:   insights[0].ContainerName,
		HighestSeverity: &hs,
		Count:           len(insights),
		Insights:        insights,
	}
}

// GetAllInsights returns insights grouped by container.
func (s *Service) GetAllInsights() []ContainerInsights {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]ContainerInsights, 0, len(s.store))
	for cid, insights := range s.store {
		if len(insights) == 0 {
			continue
		}
		hs := HighestSeverity(insights)
		result = append(result, ContainerInsights{
			ContainerID:     cid,
			ContainerName:   insights[0].ContainerName,
			HighestSeverity: &hs,
			Count:           len(insights),
			Insights:        insights,
		})
	}
	return result
}

// GetSummary returns aggregated counts across all containers.
func (s *Service) GetSummary(totalContainers int) Summary {
	s.mu.RLock()
	defer s.mu.RUnlock()

	bySeverity := make(map[string]int)
	byType := make(map[string]int)
	totalInsights := 0

	for _, insights := range s.store {
		for _, i := range insights {
			bySeverity[i.Severity]++
			byType[string(i.Type)]++
			totalInsights++
		}
	}

	return Summary{
		TotalContainersMonitored: totalContainers,
		TotalContainersAffected:  len(s.store),
		TotalInsights:            totalInsights,
		BySeverity:               bySeverity,
		ByType:                   byType,
	}
}

// InsightCount returns the count and highest severity for a container (used by container list API).
func (s *Service) InsightCount(containerID int64) (int, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	insights := s.store[containerID]
	if len(insights) == 0 {
		return 0, ""
	}
	return len(insights), HighestSeverity(insights)
}

// FormatAlertMessage builds a human-readable alert message for a container's insights.
func FormatAlertMessage(insights []Insight) string {
	if len(insights) == 0 {
		return ""
	}

	titles := make([]string, 0, len(insights))
	for _, i := range insights {
		detail := i.Title
		if port, ok := i.Details["port"]; ok {
			if proto, ok2 := i.Details["protocol"]; ok2 {
				detail = fmt.Sprintf("%s (%v/%v)", i.Title, port, proto)
			}
		}
		if dbType, ok := i.Details["database_type"]; ok {
			if port, ok2 := i.Details["port"]; ok2 {
				detail = fmt.Sprintf("%s (%v %v)", i.Title, dbType, port)
			}
		}
		titles = append(titles, detail)
	}

	return fmt.Sprintf("%d security issue(s) detected: %s", len(insights), strings.Join(titles, ", "))
}

func (s *Service) hasChange(old map[InsightType]bool, new map[InsightType]bool) bool {
	if len(old) != len(new) {
		return true
	}
	for t := range new {
		if !old[t] {
			return true
		}
	}
	for t := range old {
		if !new[t] {
			return true
		}
	}
	return false
}

// Now returns the current time (extracted for testing).
func Now() time.Time {
	return time.Now()
}
