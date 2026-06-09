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

package extension

import (
	"context"
	"time"

	"github.com/kolapsis/maintenant/internal/alert"
)

// NoopEscalator is the CE default. Implements alert.Escalator.
type NoopEscalator struct{}

func (NoopEscalator) EvaluateCycle(_ context.Context) error                  { return nil }
func (NoopEscalator) OnAlertCreated(_ context.Context, _ *alert.Alert) error { return nil }
func (NoopEscalator) OnAlertAcknowledged(_ context.Context, _ string, _ alert.Acknowledgment) error {
	return nil
}
func (NoopEscalator) OnAlertResolved(_ context.Context, _ string, _ time.Time) error { return nil }
func (NoopEscalator) OnEditionDowngraded(_ context.Context) error                    { return nil }

// NoopEntityRouter is the CE default. Implements alert.EntityRouter.
type NoopEntityRouter struct{}

func (NoopEntityRouter) Route(_ context.Context, _ string, _ string, _ string) ([]string, error) {
	return nil, nil
}

// NoopMaintenanceSuppressor is the CE default. Implements alert.MaintenanceSuppressor.
type NoopMaintenanceSuppressor struct{}

func (NoopMaintenanceSuppressor) IsSuppressed(_ context.Context, _ string, _ string, _ string) (bool, error) {
	return false, nil
}
