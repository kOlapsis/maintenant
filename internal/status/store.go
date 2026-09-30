// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package status

import "context"

// ComponentStore defines the persistence interface for status components.
type ComponentStore interface {
	ListComponents(ctx context.Context) ([]Component, error)
	ListVisibleComponents(ctx context.Context) ([]Component, error)
	GetComponent(ctx context.Context, id string) (*Component, error)
	ListComponentsByMonitor(ctx context.Context, monitorType string, monitorID string) ([]Component, error)
	RemoveDanglingMonitorRefs(ctx context.Context, monitorType string, monitorID string) error
	CreateComponent(ctx context.Context, c *Component) (string, error)
	UpdateComponent(ctx context.Context, c *Component) error
	DeleteComponent(ctx context.Context, id string) error
}

// IncidentStore defines the persistence interface for incidents and updates.
type IncidentStore interface {
	ListIncidents(ctx context.Context, opts ListIncidentsOpts) ([]Incident, int, error)
	ListActiveIncidents(ctx context.Context) ([]Incident, error)
	ListRecentIncidents(ctx context.Context, days int) ([]Incident, error)
	GetIncident(ctx context.Context, id string) (*Incident, error)
	GetActiveIncidentByComponent(ctx context.Context, componentID string) (*Incident, error)
	CreateIncident(ctx context.Context, inc *Incident, componentIDs []string, initialMessage string) (string, error)
	UpdateIncident(ctx context.Context, inc *Incident, componentIDs []string) error
	DeleteIncident(ctx context.Context, id string) error

	// Incident updates
	ListUpdates(ctx context.Context, incidentID string) ([]IncidentUpdate, error)
	CreateUpdate(ctx context.Context, u *IncidentUpdate) (string, error)

	// Cleanup
	DeleteIncidentsOlderThan(ctx context.Context, days int) (int64, error)
}

// SubscriberStore defines the persistence interface for email subscribers.
type SubscriberStore interface {
	UpsertPendingSubscriber(ctx context.Context, s *StatusSubscriber) (issued bool, err error)
	GetSubscriberByToken(ctx context.Context, confirmToken string) (*StatusSubscriber, error)
	GetSubscriberByUnsubToken(ctx context.Context, unsubToken string) (*StatusSubscriber, error)
	ConfirmSubscriber(ctx context.Context, id string) error
	DeleteSubscriber(ctx context.Context, id string) error
	ListConfirmedSubscribers(ctx context.Context) ([]StatusSubscriber, error)
	ListSubscribers(ctx context.Context) ([]StatusSubscriber, error)
	GetSubscriberStats(ctx context.Context) (*SubscriberStats, error)
	CleanExpiredUnconfirmed(ctx context.Context) (int64, error)
}

// MaintenanceStore defines the persistence interface for maintenance windows.
type MaintenanceStore interface {
	ListMaintenance(ctx context.Context, statusFilter string, limit int) ([]MaintenanceWindow, error)
	GetMaintenance(ctx context.Context, id string) (*MaintenanceWindow, error)
	CreateMaintenance(ctx context.Context, mw *MaintenanceWindow, componentIDs []string) (string, error)
	UpdateMaintenance(ctx context.Context, mw *MaintenanceWindow, componentIDs []string) error
	DeleteMaintenance(ctx context.Context, id string) error

	// Scheduler queries
	GetPendingActivation(ctx context.Context, now int64) ([]MaintenanceWindow, error)
	GetPendingDeactivation(ctx context.Context, now int64) ([]MaintenanceWindow, error)
	SetActive(ctx context.Context, id string, active bool, incidentID *string) error
}
