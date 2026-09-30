// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/commercial"
	"github.com/kolapsis/maintenant/internal/extension"
)

func TestNew_StatusPageMailsThroughTheEnvironmentSMTP(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Pro }
	t.Cleanup(func() { extension.CurrentEdition = original })

	t.Run("without an SMTP host", func(t *testing.T) {
		cfg, logger := downWiringConfig(t, 0)
		a, err := New(cfg, logger, WithExtensions(commercial.Extensions()))
		require.NoError(t, err)
		assert.Nil(t, a.statusMailer)
		assert.False(t, a.statusSvc.SubscriptionsEnabled())
	})

	t.Run("with an SMTP host", func(t *testing.T) {
		cfg, logger := downWiringConfig(t, 0)
		cfg.SMTP = SMTPConfig{Host: "smtp.example.com", Port: "587", From: "status@example.com"}
		a, err := New(cfg, logger, WithExtensions(commercial.Extensions()))
		require.NoError(t, err)
		assert.NotNil(t, a.statusMailer)
		assert.True(t, a.statusSvc.SubscriptionsEnabled())
	})
}
