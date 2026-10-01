// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert/escalation"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/extpoint"
)

type countingRetention struct {
	escalation.Service
	loops atomic.Int32
}

func (c *countingRetention) RunRetentionLoop(ctx context.Context) {
	c.loops.Add(1)
	<-ctx.Done()
}

func TestStart_RunsTheEscalationRetentionLoopOnce(t *testing.T) {
	for _, edition := range []extension.Edition{extension.Community, extension.Pro} {
		t.Run(string(edition), func(t *testing.T) {
			prev := extension.CurrentEdition
			extension.CurrentEdition = func() extension.Edition { return edition }
			t.Cleanup(func() { extension.CurrentEdition = prev })

			cfg, logs, logger := storageEnv(t)
			esc := &countingRetention{}
			a, err := New(cfg, logger, WithExtensions(extpoint.Set{
				Escalation: func(extpoint.EscalationDeps) extpoint.Escalation {
					return extpoint.Escalation{Service: esc}
				},
			}))
			require.NoError(t, err)

			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan error, 1)
			go func() { done <- a.Start(ctx) }()

			require.Eventually(t, func() bool { return strings.Contains(logs.String(), "starting HTTP server") },
				10*time.Second, 20*time.Millisecond)
			require.Eventually(t, func() bool { return esc.loops.Load() >= 1 }, 5*time.Second, 10*time.Millisecond)
			time.Sleep(100 * time.Millisecond)
			assert.EqualValues(t, 1, esc.loops.Load(), "the retention loop runs once")

			cancel()
			require.NoError(t, <-done)
			assert.NotContains(t, logs.String(), "retention loop exited unexpectedly",
				"a clean shutdown is not an error")
		})
	}
}
