// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package posture

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/extpoint"
)

func TestNewPostureScorer_BuiltInEveryEdition(t *testing.T) {
	for _, edition := range []extension.Edition{extension.Community, extension.Personal, extension.Pro} {
		t.Run(string(edition), func(t *testing.T) {
			prev := extension.CurrentEdition
			extension.CurrentEdition = func() extension.Edition { return edition }
			t.Cleanup(func() { extension.CurrentEdition = prev })

			s := NewPostureScorer(extpoint.PostureDeps{Acks: &mockAckStore{}, Threshold: 70})
			assert.IsType(t, &Scorer{}, s)
			assert.Equal(t, 70, s.Threshold())
		})
	}
}
