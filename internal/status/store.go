// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See LICENSE-COMMERCIAL.md
//
// Source: https://github.com/kolapsis/maintenant

package status

import "context"

// ComponentStore defines the persistence interface for component groups and status components.
type ComponentStore interface {
	// Groups
	ListGroups(ctx context.Context) ([]ComponentGroup, error)
	GetGroup(ctx context.Context, id int64) (*ComponentGroup, error)
	CreateGroup(ctx context.Context, g *ComponentGroup) (int64, error)
	UpdateGroup(ctx context.Context, g *ComponentGroup) error
	DeleteGroup(ctx context.Context, id int64) error

	// Components
	ListComponents(ctx context.Context) ([]Component, error)
	ListVisibleComponents(ctx context.Context) ([]Component, error)
	GetComponent(ctx context.Context, id int64) (*Component, error)
	GetComponentByMonitor(ctx context.Context, monitorType string, monitorID int64) (*Component, error)
	ListGlobalComponents(ctx context.Context, monitorType string) ([]Component, error)
	CreateComponent(ctx context.Context, c *Component) (int64, error)
	UpdateComponent(ctx context.Context, c *Component) error
	DeleteComponent(ctx context.Context, id int64) error
}

// IncidentStore defines the persistence interface for incidents and updates.
type IncidentStore interface {
	ListIncidents(ctx context.Context, opts ListIncidentsOpts) ([]Incident, int, error)
	ListActiveIncidents(ctx context.Context) ([]Incident, error)
	ListRecentIncidents(ctx context.Context, days int) ([]Incident, error)
	GetIncident(ctx context.Context, id int64) (*Incident, error)
	GetActiveIncidentByComponent(ctx context.Context, componentID int64) (*Incident, error)
	CreateIncident(ctx context.Context, inc *Incident, componentIDs []int64, initialMessage string) (int64, error)
	UpdateIncident(ctx context.Context, inc *Incident, componentIDs []int64) error
	DeleteIncident(ctx context.Context, id int64) error

	// Incident updates
	ListUpdates(ctx context.Context, incidentID int64) ([]IncidentUpdate, error)
	CreateUpdate(ctx context.Context, u *IncidentUpdate) (int64, error)

	// Cleanup
	DeleteIncidentsOlderThan(ctx context.Context, days int) (int64, error)
}

// SubscriberStore defines the persistence interface for email subscribers.
type SubscriberStore interface {
	CreateSubscriber(ctx context.Context, s *StatusSubscriber) (int64, error)
	GetSubscriberByToken(ctx context.Context, confirmToken string) (*StatusSubscriber, error)
	GetSubscriberByUnsubToken(ctx context.Context, unsubToken string) (*StatusSubscriber, error)
	ConfirmSubscriber(ctx context.Context, id int64) error
	DeleteSubscriber(ctx context.Context, id int64) error
	ListConfirmedSubscribers(ctx context.Context) ([]StatusSubscriber, error)
	ListSubscribers(ctx context.Context) ([]StatusSubscriber, error)
	GetSubscriberStats(ctx context.Context) (*SubscriberStats, error)
	CleanExpiredUnconfirmed(ctx context.Context) (int64, error)
}

// MaintenanceStore defines the persistence interface for maintenance windows.
type MaintenanceStore interface {
	ListMaintenance(ctx context.Context, statusFilter string, limit int) ([]MaintenanceWindow, error)
	GetMaintenance(ctx context.Context, id int64) (*MaintenanceWindow, error)
	CreateMaintenance(ctx context.Context, mw *MaintenanceWindow, componentIDs []int64) (int64, error)
	UpdateMaintenance(ctx context.Context, mw *MaintenanceWindow, componentIDs []int64) error
	DeleteMaintenance(ctx context.Context, id int64) error

	// Scheduler queries
	GetPendingActivation(ctx context.Context, now int64) ([]MaintenanceWindow, error)
	GetPendingDeactivation(ctx context.Context, now int64) ([]MaintenanceWindow, error)
	SetActive(ctx context.Context, id int64, active bool, incidentID *int64) error
}
