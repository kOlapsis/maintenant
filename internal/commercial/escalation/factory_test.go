// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package escalation

import (
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/commercial/tiers"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/extpoint"
)

func TestMain(m *testing.M) {
	extension.Register(tiers.Policy{}, nil)
	os.Exit(m.Run())
}

func TestNewEscalation_WiresRunnerInEveryEdition(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	setEdition(t, extension.Community)

	got := NewEscalation(extpoint.EscalationDeps{
		Store:    newMockStore(),
		Alerts:   newAlertStoreMock(),
		Channels: &mockChannelStore{},
		Notifier: alert.NewNotifier(nil, logger, true),
		Logger:   logger,
	})
	assert.NotNil(t, got.Service)
	assert.IsType(t, &Runner{}, got.Escalator, "the edition is checked per call, once the licence is known")
}
