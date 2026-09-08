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

package app

import (
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func downWiringConfig(t *testing.T, after time.Duration) (Config, *slog.Logger) {
	t.Helper()
	t.Setenv("MAINTENANT_RUNTIME", "docker")
	t.Setenv("DOCKER_HOST", "unix:///nonexistent-test-socket-abc123.sock")
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBECONFIG", "")
	return Config{
		DBPath:             filepath.Join(t.TempDir(), "test.db"),
		Addr:               "127.0.0.1:0",
		ContainerDownAfter: after,
	}, slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestNew_ContainerDownDetectorFollowsTheThreshold(t *testing.T) {
	t.Run("off when unset", func(t *testing.T) {
		cfg, logger := downWiringConfig(t, 0)
		a, err := New(cfg, logger)
		require.NoError(t, err)
		assert.Nil(t, a.downDetector)
	})

	t.Run("on when set", func(t *testing.T) {
		cfg, logger := downWiringConfig(t, 5*time.Minute)
		a, err := New(cfg, logger)
		require.NoError(t, err)
		assert.NotNil(t, a.downDetector)
	})
}
