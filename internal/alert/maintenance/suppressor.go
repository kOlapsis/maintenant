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

package maintenance

import (
	"context"
	"log/slog"
	"strconv"
	"time"
)

// Store is the minimal persistence interface needed by Suppressor.
// Implemented by *sqlite.MaintenanceStoreImpl.
type Store interface {
	IsEntitySuppressed(ctx context.Context, monitorType string, monitorID int64, now time.Time) (matched bool, windowID int64, endsAt time.Time, err error)
}

// Suppressor implements alert.MaintenanceSuppressor by consulting the maintenance
// store on every call. There is no in-process cache in v1; the SQL query is
// O(1) on indexed columns and well within the 5 ms p95 budget (research R7).
type Suppressor struct {
	store  Store
	logger *slog.Logger
	clock  func() time.Time
}

// NewSuppressor creates a new Suppressor. The clock defaults to time.Now.
func NewSuppressor(store Store, logger *slog.Logger) *Suppressor {
	return &Suppressor{
		store:  store,
		logger: logger,
		clock:  time.Now,
	}
}

// IsSuppressed implements alert.MaintenanceSuppressor. It returns (false, nil) on
// any error (fail-open per FR-007): a suppressor failure must never block alert
// creation.
func (s *Suppressor) IsSuppressed(ctx context.Context, source, entityType, entityID string) (bool, error) {
	if entityType == "" {
		return false, nil
	}
	id, err := strconv.ParseInt(entityID, 10, 64)
	if err != nil {
		return false, nil
	}
	matched, windowID, endsAt, err := s.store.IsEntitySuppressed(ctx, entityType, id, s.clock())
	if err != nil {
		s.logger.ErrorContext(ctx, "maintenance: suppressor store error",
			"error", err,
			"source", source,
			"entity_type", entityType,
			"entity_id", entityID,
		)
		return false, nil
	}
	if matched {
		s.logger.DebugContext(ctx, "alert suppressed by maintenance window",
			"source", source,
			"entity_type", entityType,
			"entity_id", entityID,
			"maintenance_id", windowID,
			"window_ends_at", endsAt,
		)
		return true, nil
	}
	return false, nil
}
