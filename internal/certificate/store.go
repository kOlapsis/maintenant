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

package certificate

import (
	"context"
	"time"
)

// CertificateStore defines the persistence interface for certificate monitoring data.
type CertificateStore interface {
	// Monitor CRUD
	CreateMonitor(ctx context.Context, m *CertMonitor) (string, error)
	GetMonitorByID(ctx context.Context, id string) (*CertMonitor, error)
	GetMonitorByHostPort(ctx context.Context, hostname string, port int) (*CertMonitor, error)
	GetMonitorByHostPortAgent(ctx context.Context, agentID *string, hostname string, port int) (*CertMonitor, error)
	GetMonitorByEndpointID(ctx context.Context, endpointID string) (*CertMonitor, error)
	ListMonitors(ctx context.Context, opts ListCertificatesOpts) ([]*CertMonitor, error)
	CountStandaloneMonitors(ctx context.Context) (int, error)
	UpdateMonitor(ctx context.Context, m *CertMonitor) error
	DeleteMonitor(ctx context.Context, id string) error

	// Check results
	InsertCheckResult(ctx context.Context, result *CertCheckResult) (string, error)
	GetLatestCheckResult(ctx context.Context, monitorID string) (*CertCheckResult, error)
	ListCheckResults(ctx context.Context, monitorID string, opts ListChecksOpts) ([]*CertCheckResult, int, error)

	// Chain entries
	InsertChainEntries(ctx context.Context, entries []*CertChainEntry) error
	GetChainEntries(ctx context.Context, checkResultID string) ([]*CertChainEntry, error)

	// Label-discovered monitors
	ListMonitorsByExternalID(ctx context.Context, externalID string) ([]*CertMonitor, error)

	// Scheduler
	ListDueScheduledMonitors(ctx context.Context, now time.Time) ([]*CertMonitor, error)

	// Retention
	DeleteCheckResultsBefore(ctx context.Context, before time.Time, batchSize int) (int64, error)
}
