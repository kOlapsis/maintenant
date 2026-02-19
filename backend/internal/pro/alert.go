package pro

import (
	"context"
	"time"
)

// Escalator evaluates whether an unacknowledged alert should escalate to a secondary channel.
// CE: no-op. Pro: timer-based escalation policies.
type Escalator interface {
	Evaluate(ctx context.Context, alertID string, elapsed time.Duration) (*EscalationAction, error)
}

// EscalationAction describes where and what to escalate.
type EscalationAction struct {
	ChannelID string
	Message   string
}

// EntityRouter provides per-entity alert routing (container X → channel A by severity).
// CE: no-op (falls through to default source+severity routing). Pro: entity-level rules.
type EntityRouter interface {
	Route(ctx context.Context, entityType string, entityID string, severity string) ([]string, error)
}

// MaintenanceSuppressor checks if an alert should be suppressed based on a maintenance calendar.
// CE: no-op. Pro: calendar-based scheduled suppression.
type MaintenanceSuppressor interface {
	IsSuppressed(ctx context.Context, source string, entityType string, entityID string) (bool, error)
}

// TemplateEngine renders notification messages using custom templates with variable substitution.
// CE: no-op (uses default JSON payload). Pro: user-defined templates per channel.
type TemplateEngine interface {
	Render(ctx context.Context, templateName string, vars map[string]any) (string, error)
}

// NoopEscalator is the CE default.
type NoopEscalator struct{}

func (NoopEscalator) Evaluate(_ context.Context, _ string, _ time.Duration) (*EscalationAction, error) {
	return nil, nil
}

// NoopEntityRouter is the CE default.
type NoopEntityRouter struct{}

func (NoopEntityRouter) Route(_ context.Context, _ string, _ string, _ string) ([]string, error) {
	return nil, nil
}

// NoopMaintenanceSuppressor is the CE default.
type NoopMaintenanceSuppressor struct{}

func (NoopMaintenanceSuppressor) IsSuppressed(_ context.Context, _ string, _ string, _ string) (bool, error) {
	return false, nil
}

// NoopTemplateEngine is the CE default.
type NoopTemplateEngine struct{}

func (NoopTemplateEngine) Render(_ context.Context, _ string, _ map[string]any) (string, error) {
	return "", ErrProFeature
}
