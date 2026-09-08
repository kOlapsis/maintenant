package alert

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/kolapsis/maintenant/internal/container"
)

// AlertTypeContainerDown is raised for a container that has been stopped for
// longer than the configured threshold.
const AlertTypeContainerDown = "container_down"

// DownDetector raises an alert for every container stopped for longer than the
// threshold, and resolves it once the container runs again. It reads the state
// from the store on each pass instead of tracking it in memory, so a container
// that went down while the server was off is still caught, and it asks the
// alert store what is already firing instead of re-emitting every pass.
type DownDetector struct {
	containers container.ContainerStore
	alerts     AlertStore
	after      time.Duration
	logger     *slog.Logger
	emit       func(Event)
}

// NewDownDetector returns a detector for containers stopped longer than after.
func NewDownDetector(containers container.ContainerStore, alerts AlertStore, after time.Duration, logger *slog.Logger) *DownDetector {
	return &DownDetector{
		containers: containers,
		alerts:     alerts,
		after:      after,
		logger:     logger,
	}
}

// SetEmitter sets the sink receiving the fire and recovery events.
func (d *DownDetector) SetEmitter(emit func(Event)) {
	d.emit = emit
}

// isDown reports whether a state means the container stopped running on its
// own. "completed" is a job that ended as intended, "paused" and "created" were
// never running to begin with.
func isDown(state container.ContainerState) bool {
	return state == container.StateExited || state == container.StateDead
}

// Check runs one pass: fire for what crossed the threshold, resolve for what
// came back.
func (d *DownDetector) Check(ctx context.Context) error {
	containers, err := d.containers.ListContainers(ctx, container.ListContainersOpts{})
	if err != nil {
		return fmt.Errorf("container down check: list containers: %w", err)
	}
	active, err := d.alerts.ListActiveAlerts(ctx)
	if err != nil {
		return fmt.Errorf("container down check: list active alerts: %w", err)
	}

	now := time.Now()
	down := make(map[string]*container.Container, len(containers))
	known := make(map[string]*container.Container, len(containers))
	for _, c := range containers {
		known[c.ID] = c
		if isDown(c.State) && now.Sub(c.LastStateChangeAt) >= d.after {
			down[c.ID] = c
		}
	}

	firing := make(map[string]*Alert, len(active))
	for _, a := range active {
		if a.AlertType == AlertTypeContainerDown && a.EntityType == "container" {
			firing[a.EntityID] = a
		}
	}

	for id, c := range down {
		if _, already := firing[id]; already {
			continue
		}
		stoppedFor := now.Sub(c.LastStateChangeAt).Round(time.Second)
		d.logger.Warn("container down past threshold",
			"container_id", c.ID, "name", c.Name, "state", string(c.State),
			"stopped_for", stoppedFor, "threshold", d.after,
		)
		d.send(Event{
			Source:     SourceContainer,
			AlertType:  AlertTypeContainerDown,
			Severity:   downSeverity(c),
			Message:    fmt.Sprintf("Container %s has been %s for %s", c.Name, c.State, stoppedFor),
			EntityType: "container",
			EntityID:   c.ID,
			EntityName: c.Name,
			Details: map[string]any{
				"state":             string(c.State),
				"stopped_for":       int64(stoppedFor / time.Second),
				"threshold_seconds": int64(d.after / time.Second),
				"agent_id":          c.AgentID,
			},
			Timestamp: now,
		})
	}

	for id, a := range firing {
		if _, still := down[id]; still {
			continue
		}
		name := a.EntityName
		if c, ok := known[id]; ok {
			name = c.Name
		}
		d.logger.Info("container back up", "container_id", id, "name", name)
		d.send(Event{
			Source:     SourceContainer,
			AlertType:  AlertTypeContainerDown,
			Severity:   SeverityInfo,
			IsRecover:  true,
			Message:    fmt.Sprintf("Container %s is running again", name),
			EntityType: "container",
			EntityID:   id,
			EntityName: name,
			Timestamp:  now,
		})
	}

	return nil
}

// downSeverity honours the per-container alert severity the operator set.
func downSeverity(c *container.Container) string {
	switch c.AlertSeverity {
	case container.SeverityInfo:
		return SeverityInfo
	case container.SeverityWarning:
		return SeverityWarning
	default:
		return SeverityCritical
	}
}

func (d *DownDetector) send(evt Event) {
	if d.emit != nil {
		d.emit(evt)
	}
}
