package endpoint

import (
	"context"
	"time"
)

// EndpointStore defines the persistence interface for endpoint monitoring data.
type EndpointStore interface {
	// Endpoint CRUD
	UpsertEndpoint(ctx context.Context, e *Endpoint) (int64, error)
	GetEndpointByIdentity(ctx context.Context, containerName, labelKey string) (*Endpoint, error)
	GetEndpointByID(ctx context.Context, id int64) (*Endpoint, error)
	ListEndpoints(ctx context.Context, opts ListEndpointsOpts) ([]*Endpoint, error)
	ListEndpointsByExternalID(ctx context.Context, externalID string) ([]*Endpoint, error)
	DeactivateEndpoint(ctx context.Context, id int64) error

	// Check result updates on the endpoint record
	UpdateCheckResult(ctx context.Context, id int64, status EndpointStatus, alertState AlertState,
		consecutiveFailures, consecutiveSuccesses int,
		responseTimeMs int64, httpStatus *int, lastError string) error

	// Check result persistence
	InsertCheckResult(ctx context.Context, result *CheckResult) (int64, error)
	ListCheckResults(ctx context.Context, endpointID int64, opts ListChecksOpts) ([]*CheckResult, int, error)
	GetCheckResultsInWindow(ctx context.Context, endpointID int64, from, to time.Time) (int, int, error)

	// Retention
	DeleteCheckResultsBefore(ctx context.Context, before time.Time, batchSize int) (int64, error)
	DeleteInactiveEndpointsBefore(ctx context.Context, before time.Time) (int64, error)
}
