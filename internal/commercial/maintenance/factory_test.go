// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package maintenance

import (
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kolapsis/maintenant/internal/commercial/tiers"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/extpoint"
)

func TestMain(m *testing.M) {
	extension.Register(tiers.Policy{}, nil)
	os.Exit(m.Run())
}

func TestNewMaintenanceSuppressor_OnlyInPro(t *testing.T) {
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

			got := NewMaintenanceSuppressor(extpoint.SuppressorDeps{Logger: logger})
			if wired {
				assert.IsType(t, &Suppressor{}, got)
			} else {
				assert.Nil(t, got)
			}
		})
	}
}
