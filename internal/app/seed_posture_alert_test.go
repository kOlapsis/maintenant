// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/security"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

type resumableScorer struct {
	security.PostureScorer
	resumed int
}

func (s *resumableScorer) ResumeBelowThreshold() { s.resumed++ }

func TestSeedRestartAlertTracking_ResumesAnActivePostureAlert(t *testing.T) {
	for name, tc := range map[string]struct {
		status string
		want   int
	}{
		"active":   {alert.StatusActive, 1},
		"resolved": {alert.StatusResolved, 0},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			db := storetest.Open(t, logger)
			alertStore := store.NewAlertStore(db)
			_, err := alertStore.InsertAlert(ctx, &alert.Alert{
				Source:     alert.SourceSecurity,
				AlertType:  alert.AlertTypePostureThreshold,
				Severity:   alert.SeverityWarning,
				Status:     tc.status,
				Message:    "score dropped",
				EntityType: "infrastructure",
				EntityName: "infrastructure",
				Details:    "{}",
				FiredAt:    time.Now(),
			})
			require.NoError(t, err)

			scorer := &resumableScorer{}
			a := &App{
				alertStore:   alertStore,
				containerSvc: container.NewService(container.Deps{Store: store.NewContainerStore(db), Logger: logger}),
				scorer:       scorer,
				logger:       logger,
			}
			a.seedRestartAlertTracking(ctx)

			assert.Equal(t, tc.want, scorer.resumed)
		})
	}
}
