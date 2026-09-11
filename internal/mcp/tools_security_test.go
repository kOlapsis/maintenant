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

package mcp

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/security"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/update"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestSecurityService() *security.Service {
	return security.NewService(security.Deps{Logger: slog.Default()})
}

func TestGetSecurityInsights_All(t *testing.T) {
	svc := &Services{SecuritySvc: newTestSecurityService(), Logger: slog.Default(), Version: "test"}
	result, _, err := getSecurityInsightsHandler(svc)(context.Background(), nil, getSecurityInsightsInput{})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), "summary")
}

func TestGetSecurityInsights_ContainerEmpty(t *testing.T) {
	svc := &Services{SecuritySvc: newTestSecurityService(), Logger: slog.Default(), Version: "test"}
	result, _, err := getSecurityInsightsHandler(svc)(context.Background(), nil, getSecurityInsightsInput{ContainerID: "c-1"})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	text := textFromContent(t, result.Content)
	assert.Contains(t, text, "c-1")
	assert.Contains(t, text, `"count":0`)
}

func TestListCVE_CE_EditionRequired(t *testing.T) {
	withEdition(t, extension.Community)
	svc := &Services{Logger: slog.Default(), Version: "test"}
	result, _, err := listCVEHandler(svc)(context.Background(), nil, listCVEInput{})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), "edition_required")
}

func TestListRiskScores_CE_EditionRequired(t *testing.T) {
	withEdition(t, extension.Community)
	svc := &Services{Logger: slog.Default(), Version: "test"}
	result, _, err := listRiskScoresHandler(svc)(context.Background(), nil, listRiskScoresInput{})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), "edition_required")
}

func TestGetSecurityPosture_CE_EditionRequired(t *testing.T) {
	withEdition(t, extension.Community)
	svc := &Services{Logger: slog.Default(), Version: "test"}
	result, _, err := getSecurityPostureHandler(svc)(context.Background(), nil, getSecurityPostureInput{})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), "edition_required")
}

func newTestUpdateStore(t *testing.T) update.UpdateStore {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"), slog.Default())
	require.NoError(t, err)
	require.NoError(t, store.Migrate(context.Background(), db, slog.Default()))

	ctx, cancel := context.WithCancel(context.Background())
	db.StartWriter(ctx)
	t.Cleanup(func() {
		cancel()
		_ = db.Close()
	})
	return store.NewUpdateStore(db)
}

func TestListCVE_Container_NotEvaluated(t *testing.T) {
	withEdition(t, extension.Pro)
	svc := &Services{UpdateStore: newTestUpdateStore(t), Logger: slog.Default(), Version: "test"}

	result, _, err := listCVEHandler(svc)(context.Background(), nil, listCVEInput{ContainerID: "c-1"})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), `"evaluation":"not_evaluated"`)
}

func TestListCVE_Container_Evaluated(t *testing.T) {
	withEdition(t, extension.Pro)
	us := newTestUpdateStore(t)
	require.NoError(t, us.UpsertCVEEvaluation(context.Background(), &update.CVEEvaluation{
		ContainerID: "c-1",
		Status:      update.CVEEvaluated,
		EvaluatedAt: time.Now(),
	}))
	svc := &Services{UpdateStore: us, Logger: slog.Default(), Version: "test"}

	result, _, err := listCVEHandler(svc)(context.Background(), nil, listCVEInput{ContainerID: "c-1"})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), `"evaluation":"evaluated"`)
}
