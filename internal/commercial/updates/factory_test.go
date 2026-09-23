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
package updates

import (
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kolapsis/maintenant/internal/commercial/tiers"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/extpoint"
	"github.com/kolapsis/maintenant/internal/update"
)

func TestMain(m *testing.M) {
	extension.Register(tiers.Policy{}, nil)
	os.Exit(m.Run())
}

func TestNewEnricher_PerEdition(t *testing.T) {
	deps := extpoint.EnricherDeps{
		Store:    &stubStore{},
		Registry: update.NewRegistryClient(),
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	for edition, active := range map[extension.Edition]bool{
		extension.Community: false,
		extension.Personal:  true,
		extension.Pro:       true,
	} {
		t.Run(string(edition), func(t *testing.T) {
			prev := extension.CurrentEdition
			extension.CurrentEdition = func() extension.Edition { return edition }
			t.Cleanup(func() { extension.CurrentEdition = prev })

			e := NewEnricher(deps)
			if !active {
				assert.True(t, e == nil, "Community must get no enricher")
				return
			}
			assert.IsType(t, &ProEnricher{}, e)
		})
	}
}
