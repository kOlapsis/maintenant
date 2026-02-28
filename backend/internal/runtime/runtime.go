package runtime

import (
	"context"
	"io"

	"github.com/kolapsis/pulseboard/internal/container"
)

// Runtime abstracts the container orchestration platform.
// Implementations: docker.Runtime, kubernetes.Runtime.
type Runtime interface {
	// Lifecycle
	Connect(ctx context.Context) error
	IsConnected() bool
	SetDisconnected()
	Close() error

	// Identity
	Name() string // "docker" or "kubernetes"

	// Discovery
	DiscoverAll(ctx context.Context) ([]*container.Container, error)

	// Events
	StreamEvents(ctx context.Context) <-chan RuntimeEvent

	// Stats
	StatsSnapshot(ctx context.Context, externalID string) (*RawStats, error)

	// Logs
	FetchLogs(ctx context.Context, externalID string, lines int, timestamps bool) ([]string, error)
	StreamLogs(ctx context.Context, externalID string, lines int, timestamps bool) (io.ReadCloser, error)

	// Health
	GetHealthInfo(ctx context.Context, externalID string) (*HealthInfo, error)
}
