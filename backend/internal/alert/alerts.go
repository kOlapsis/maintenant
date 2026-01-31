package alert

import (
	"context"
	"log/slog"
	"time"

	"github.com/kolapsis/pulseboard/internal/container"
)

const defaultRestartWindow = 10 * time.Minute

// RestartDetector checks for crash-loop restart patterns.
type RestartDetector struct {
	store  container.ContainerStore
	logger *slog.Logger
}

// NewRestartDetector creates a new restart detector.
func NewRestartDetector(store container.ContainerStore, logger *slog.Logger) *RestartDetector {
	return &RestartDetector{
		store:  store,
		logger: logger,
	}
}

// RestartAlert represents a restart threshold alert.
type RestartAlert struct {
	ContainerID   int64
	ContainerName string
	RestartCount  int
	Threshold     int
	Severity      container.AlertSeverity
	Channels      string
	Timestamp     time.Time
}

// Check evaluates whether the container has exceeded its restart threshold.
// Returns an alert if threshold is exceeded, nil otherwise.
func (d *RestartDetector) Check(ctx context.Context, c *container.Container) (interface{}, error) {
	since := time.Now().Add(-defaultRestartWindow)
	count, err := d.store.CountRestartsSince(ctx, c.ID, since)
	if err != nil {
		return nil, err
	}

	if count < c.RestartThreshold {
		return nil, nil
	}

	d.logger.Warn("restart threshold exceeded",
		"container_id", c.ID,
		"name", c.Name,
		"restarts", count,
		"threshold", c.RestartThreshold,
	)

	return &RestartAlert{
		ContainerID:   c.ID,
		ContainerName: c.Name,
		RestartCount:  count,
		Threshold:     c.RestartThreshold,
		Severity:      c.AlertSeverity,
		Channels:      c.AlertChannels,
		Timestamp:     time.Now(),
	}, nil
}

// HealthAlert represents a health status change alert.
type HealthAlert struct {
	ContainerID    int64
	ContainerName  string
	PreviousHealth *container.HealthStatus
	NewHealth      container.HealthStatus
	Severity       container.AlertSeverity
	Channels       string
	Timestamp      time.Time
}

// CheckHealthTransition returns an alert if a container transitions from healthy to unhealthy.
func CheckHealthTransition(c *container.Container, previousHealth *container.HealthStatus, newHealth container.HealthStatus) *HealthAlert {
	// Only alert on healthy → unhealthy transition
	if previousHealth == nil || *previousHealth != container.HealthHealthy || newHealth != container.HealthUnhealthy {
		return nil
	}

	return &HealthAlert{
		ContainerID:    c.ID,
		ContainerName:  c.Name,
		PreviousHealth: previousHealth,
		NewHealth:      newHealth,
		Severity:       c.AlertSeverity,
		Channels:       c.AlertChannels,
		Timestamp:      time.Now(),
	}
}
