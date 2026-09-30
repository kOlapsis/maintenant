// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/security"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

type stubPostureScorer struct{}

func (stubPostureScorer) ScoreContainer(_ context.Context, id, _, name string) (*security.SecurityScore, error) {
	return &security.SecurityScore{ContainerID: id, ContainerName: name}, nil
}

func (stubPostureScorer) ScoreContainers(_ context.Context, cs []security.ContainerInfo) ([]*security.SecurityScore, error) {
	scores := make([]*security.SecurityScore, len(cs))
	for i, c := range cs {
		scores[i] = &security.SecurityScore{ContainerID: c.ID, ContainerName: c.Name}
	}
	return scores, nil
}

func (stubPostureScorer) ScoreInfrastructure(_ context.Context, cs []security.ContainerInfo) (*security.InfrastructurePosture, error) {
	return &security.InfrastructurePosture{ContainerCount: len(cs)}, nil
}

func (stubPostureScorer) InvalidateCache(string)                                {}
func (stubPostureScorer) Threshold() int                                        { return 0 }
func (stubPostureScorer) SetPostureAlertCallback(security.PostureAlertCallback) {}
func (stubPostureScorer) SetPostureEventCallback(security.PostureEventCallback) {}

func TestPostureRoutes_PerEdition(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)
	r := NewRouter(HandlerDeps{
		Logger:     logger,
		Containers: container.NewService(container.Deps{Store: store.NewContainerStore(db), Logger: logger}),
		AckStore:   store.NewAcknowledgmentStore(db),
		Scorer:     stubPostureScorer{},
	})
	h := r.Handler()

	paths := []string{
		"/api/v1/security/posture",
		"/api/v1/security/posture/containers",
		"/api/v1/security/acknowledgments",
	}
	for edition, want := range map[extension.Edition]int{
		extension.Community: http.StatusForbidden,
		extension.Personal:  http.StatusOK,
		extension.Pro:       http.StatusOK,
	} {
		for _, path := range paths {
			t.Run(string(edition)+path, func(t *testing.T) {
				withEdition(t, edition)
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
				require.Equal(t, want, rec.Code, rec.Body.String())
				if want == http.StatusForbidden {
					detail := decodeRefusal(t, rec)
					assert.Equal(t, "EDITION_REQUIRED", detail.Code)
					assert.Equal(t, string(extension.CapSecurityPosture), detail.Feature)
				}
			})
		}
	}
}

func TestPostureRoutes_AbsentWithoutAScorer(t *testing.T) {
	withEdition(t, extension.Pro)
	r := NewRouter(HandlerDeps{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/security/posture", nil))
	assert.NotEqual(t, http.StatusOK, rec.Code)
}
