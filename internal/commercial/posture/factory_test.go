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
