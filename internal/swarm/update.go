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
	"time"

	"github.com/docker/docker/api/types/swarm"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/event"
)

// UpdateProgress represents the progress of a rolling update.
type UpdateProgress struct {
	ServiceID    string  `json:"service_id"`
	ServiceName  string  `json:"service_name"`
	State        string  `json:"state"`
	OldImage     string  `json:"old_image"`
	NewImage     string  `json:"new_image"`
	TasksUpdated int     `json:"tasks_updated"`
	TasksTotal   int     `json:"tasks_total"`
	Message      string  `json:"message"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
}

// trackedUpdate stores the last known state of an in-progress update.
type trackedUpdate struct {
	lastState   string
	oldImage    string
}

// UpdateTracker monitors rolling update progress for Swarm services.
type UpdateTracker struct {
	client   ServiceClient
	logger   *slog.Logger
	callback EventCallback
	alertCb  NodeAlertCallback

	mu       sync.Mutex
	tracked  map[string]*trackedUpdate // keyed by service ID
}

// NewUpdateTracker creates a new rolling update tracker.
func NewUpdateTracker(client ServiceClient, logger *slog.Logger) *UpdateTracker {
	return &UpdateTracker{
		client:  client,
		logger:  logger,
		tracked: make(map[string]*trackedUpdate),
	}
}

// SetEventCallback sets the SSE event callback.
func (ut *UpdateTracker) SetEventCallback(cb EventCallback) {
	ut.callback = cb
}

// SetAlertCallback sets the alert callback.
func (ut *UpdateTracker) SetAlertCallback(cb NodeAlertCallback) {
	ut.alertCb = cb
}

// CheckService checks the update status of a service after it was updated.
func (ut *UpdateTracker) CheckService(ctx context.Context, serviceID string) {
	svc, err := ut.client.ServiceInspect(ctx, serviceID)
	if err != nil {
		ut.logger.Warn("failed to inspect service for update check", "service_id", serviceID, "error", err)
		return
	}

	us := svc.UpdateStatus
	if us == nil || us.State == "" {
		ut.clearTracked(serviceID)
		return
	}

	state := string(us.State)
	serviceName := svc.Spec.Name
	newImage := ""
	if svc.Spec.TaskTemplate.ContainerSpec != nil {
		newImage = svc.Spec.TaskTemplate.ContainerSpec.Image
	}

	ut.mu.Lock()
	prev, hasPrev := ut.tracked[serviceID]
	if !hasPrev {
		prev = &trackedUpdate{oldImage: ""}
		ut.tracked[serviceID] = prev
	}

	prevState := prev.lastState
	prev.lastState = state
	ut.mu.Unlock()

	// Calculate progress by counting tasks on new vs old image.
	tasksUpdated, tasksTotal := ut.countProgress(ctx, serviceID, newImage)

	now := time.Now()

	switch {
	case state == string(swarm.UpdateStateUpdating):
		ut.emit(event.SwarmUpdateProgress, map[string]interface{}{
			"service_id":    serviceID,
			"service_name":  serviceName,
			"state":         state,
			"tasks_updated": tasksUpdated,
			"tasks_total":   tasksTotal,
			"old_image":     prev.oldImage,
			"new_image":     newImage,
			"message":       us.Message,
			"timestamp":     now.Format(time.RFC3339),
		})

	case state == string(swarm.UpdateStateCompleted) && prevState != string(swarm.UpdateStateCompleted):
		ut.emit(event.SwarmUpdateCompleted, map[string]interface{}{
			"service_id":   serviceID,
			"service_name": serviceName,
			"state":        "completed",
			"message":      us.Message,
			"started_at":   formatTimePtr(us.StartedAt),
			"completed_at": formatTimePtr(us.CompletedAt),
		})
		ut.clearTracked(serviceID)

	case state == string(swarm.UpdateStateRollbackCompleted) && prevState != string(swarm.UpdateStateRollbackCompleted):
		ut.emit(event.SwarmUpdateCompleted, map[string]interface{}{
			"service_id":   serviceID,
			"service_name": serviceName,
			"state":        "rollback_completed",
			"message":      us.Message,
			"started_at":   formatTimePtr(us.StartedAt),
			"completed_at": formatTimePtr(us.CompletedAt),
		})

		ut.sendAlert(alert.Event{
			Source:     "swarm",
			AlertType:  "update_rollback",
			Severity:   alert.SeverityWarning,
			Message:    fmt.Sprintf("Swarm service %s rolling update rolled back", serviceName),
			EntityType: "swarm_service",
			EntityName: serviceName,
			Details: map[string]any{
				"service_id": serviceID,
				"state":      state,
				"message":    us.Message,
			},
			Timestamp: now,
		})
		ut.clearTracked(serviceID)

	case state == string(swarm.UpdateStatePaused) && prevState != string(swarm.UpdateStatePaused):
		ut.sendAlert(alert.Event{
			Source:     "swarm",
			AlertType:  "update_stalled",
			Severity:   alert.SeverityWarning,
			Message:    fmt.Sprintf("Swarm service %s rolling update paused: %s", serviceName, us.Message),
			EntityType: "swarm_service",
			EntityName: serviceName,
			Details: map[string]any{
				"service_id": serviceID,
				"state":      state,
				"message":    us.Message,
			},
			Timestamp: now,
		})
	}
}

// GetUpdateStatus returns the current update status for a service.
func (ut *UpdateTracker) GetUpdateStatus(ctx context.Context, serviceID string) (*UpdateProgress, error) {
	svc, err := ut.client.ServiceInspect(ctx, serviceID)
	if err != nil {
		return nil, fmt.Errorf("inspect service %s: %w", serviceID, err)
	}

	if svc.UpdateStatus == nil || svc.UpdateStatus.State == "" {
		return nil, nil
	}

	newImage := ""
	if svc.Spec.TaskTemplate.ContainerSpec != nil {
		newImage = svc.Spec.TaskTemplate.ContainerSpec.Image
	}

	tasksUpdated, tasksTotal := ut.countProgress(ctx, serviceID, newImage)

	ut.mu.Lock()
	prev := ut.tracked[serviceID]
	ut.mu.Unlock()
	oldImage := ""
	if prev != nil {
		oldImage = prev.oldImage
	}

	return &UpdateProgress{
		ServiceID:    serviceID,
		ServiceName:  svc.Spec.Name,
		State:        string(svc.UpdateStatus.State),
		OldImage:     oldImage,
		NewImage:     newImage,
		TasksUpdated: tasksUpdated,
		TasksTotal:   tasksTotal,
		Message:      svc.UpdateStatus.Message,
		StartedAt:    svc.UpdateStatus.StartedAt,
		CompletedAt:  svc.UpdateStatus.CompletedAt,
	}, nil
}

func (ut *UpdateTracker) countProgress(ctx context.Context, serviceID, newImage string) (updated, total int) {
	tasks, err := ut.client.TaskList(ctx)
	if err != nil {
		return 0, 0
	}
	for _, t := range tasks {
		if t.ServiceID != serviceID {
			continue
		}
		if t.DesiredState != swarm.TaskStateRunning {
			continue
		}
		total++
		if t.Status.State == swarm.TaskStateRunning {
			taskImage := ""
			if t.Spec.ContainerSpec != nil {
				taskImage = t.Spec.ContainerSpec.Image
			}
			if taskImage == newImage {
				updated++
			}
		}
	}
	return updated, total
}

func (ut *UpdateTracker) clearTracked(serviceID string) {
	ut.mu.Lock()
	delete(ut.tracked, serviceID)
	ut.mu.Unlock()
}

func (ut *UpdateTracker) emit(eventType string, data interface{}) {
	if ut.callback != nil {
		ut.callback(eventType, data)
	}
}

func (ut *UpdateTracker) sendAlert(evt alert.Event) {
	if ut.alertCb != nil {
		ut.alertCb(evt)
	}
}

func formatTimePtr(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}
