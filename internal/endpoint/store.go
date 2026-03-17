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

package endpoint

import (
	"context"
	"time"
)

// EndpointStore defines the persistence interface for endpoint monitoring data.
type EndpointStore interface {
	// Endpoint CRUD (label-discovered)
	UpsertEndpoint(ctx context.Context, e *Endpoint) (int64, error)
	GetEndpointByIdentity(ctx context.Context, containerName, labelKey string) (*Endpoint, error)
	GetEndpointByID(ctx context.Context, id int64) (*Endpoint, error)
	ListEndpoints(ctx context.Context, opts ListEndpointsOpts) ([]*Endpoint, error)
	ListEndpointsByExternalID(ctx context.Context, externalID string) ([]*Endpoint, error)
	DeactivateEndpoint(ctx context.Context, id int64) error

	// Standalone endpoint CRUD
	InsertStandaloneEndpoint(ctx context.Context, e *Endpoint) (int64, error)
	UpdateStandaloneEndpoint(ctx context.Context, id int64, name, target string, endpointType EndpointType, configJSON string) error
	DeleteStandaloneEndpoint(ctx context.Context, id int64) error

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
