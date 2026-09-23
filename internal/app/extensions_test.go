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
package app_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/app"
	"github.com/kolapsis/maintenant/internal/extpoint"
	"github.com/kolapsis/maintenant/internal/security"
	"github.com/kolapsis/maintenant/internal/update"
)

func TestNew_BuildsTheEnricherFromTheExtensionPoint(t *testing.T) {
	cfg, logger := modeGateCfg(t, "")

	var got extpoint.EnricherDeps
	calls := 0
	var postureDeps extpoint.PostureDeps
	var channelDeps extpoint.ChannelDeps
	var statusDeps extpoint.StatusPageDeps
	var suppressorDeps extpoint.SuppressorDeps
	var escalationDeps extpoint.EscalationDeps
	a, err := app.New(cfg, logger, app.WithExtensions(extpoint.Set{
		Enricher: func(d extpoint.EnricherDeps) update.Enricher {
			calls++
			got = d
			return nil
		},
		PostureScorer: func(d extpoint.PostureDeps) security.PostureScorer {
			postureDeps = d
			return nil
		},
		Channels: func(d extpoint.ChannelDeps) map[string]alert.ChannelSender {
			channelDeps = d
			return nil
		},
		StatusPage: func(d extpoint.StatusPageDeps) extpoint.StatusPage {
			statusDeps = d
			return extpoint.StatusPage{}
		},
		Suppressor: func(d extpoint.SuppressorDeps) alert.MaintenanceSuppressor {
			suppressorDeps = d
			return nil
		},
		Escalation: func(d extpoint.EscalationDeps) extpoint.Escalation {
			escalationDeps = d
			return extpoint.Escalation{}
		},
	}))
	require.NoError(t, err)
	require.NotNil(t, a)

	assert.Equal(t, 1, calls)
	assert.NotNil(t, got.Store)
	assert.NotNil(t, got.Registry)
	assert.NotNil(t, got.Logger)
	assert.NotNil(t, postureDeps.Acks)
	assert.NotNil(t, postureDeps.Insights)
	assert.NotNil(t, channelDeps.HTTPClient)
	assert.NotNil(t, channelDeps.Logger)
	assert.NotNil(t, statusDeps.Service)
	assert.NotNil(t, statusDeps.Incidents)
	assert.NotNil(t, statusDeps.Personalization)
	assert.NotNil(t, suppressorDeps.Windows)
	assert.NotNil(t, escalationDeps.Store)
	assert.NotNil(t, escalationDeps.Notifier)
	assert.Nil(t, escalationDeps.Suppressor, "a nil suppressor is passed through as is")
}

func TestNew_WithoutExtensionsStillBuilds(t *testing.T) {
	cfg, logger := modeGateCfg(t, "")
	a, err := app.New(cfg, logger)
	require.NoError(t, err)
	require.NotNil(t, a)
}
