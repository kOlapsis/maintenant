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

func TestNewEscalation_EscalatorOnlyInPro(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	for edition, wired := range map[extension.Edition]bool{
		extension.Community: false,
		extension.Personal:  false,
		extension.Pro:       true,
	} {
		t.Run(string(edition), func(t *testing.T) {
			prev := extension.CurrentEdition
			extension.CurrentEdition = func() extension.Edition { return edition }
			t.Cleanup(func() { extension.CurrentEdition = prev })

			got := NewEscalation(extpoint.EscalationDeps{
				Store:    newMockStore(),
				Alerts:   newAlertStoreMock(),
				Channels: &mockChannelStore{},
				Notifier: alert.NewNotifier(nil, logger, true),
				Logger:   logger,
			})
			assert.NotNil(t, got.Service, "the service is built in every edition")
			if wired {
				assert.IsType(t, &Runner{}, got.Escalator)
			} else {
				assert.Nil(t, got.Escalator)
			}
		})
	}
}
