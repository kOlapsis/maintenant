// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/extpoint"
)

func TestStart_FailureLeavesNoInstanceRow(t *testing.T) {
	t.Run("mode refused for the edition", func(t *testing.T) {
		pinEdition(t, extension.Community)
		cfg, _, logger := storageEnv(t)
		cfg.Mode = "server"
		a, err := New(cfg, logger)
		require.NoError(t, err)
		t.Cleanup(func() {
			a.background.Wait()
			_ = a.rt.Close()
			_ = a.db.Close()
		})

		require.ErrorContains(t, a.Start(context.Background()), "mode requires")
		assert.Zero(t, instanceRows(t, cfg.DBPath, a.instanceID))
	})

	t.Run("agent server cannot start", func(t *testing.T) {
		pinEdition(t, extension.Pro)
		cfg, _, logger := storageEnv(t)
		a, err := New(cfg, logger)
		require.NoError(t, err)
		refused := errors.New("address already in use")
		a.serveAgents = func(context.Context, extpoint.GRPCConfig) error { return refused }

		require.ErrorIs(t, a.Start(context.Background()), refused)
		assert.Zero(t, instanceRows(t, cfg.DBPath, a.instanceID))
	})
}
