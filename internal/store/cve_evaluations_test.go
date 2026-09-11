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

package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/update"
)

func TestCVEEvaluation_UpsertThenGet(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	us := NewUpdateStore(db)

	evaluatedAt := time.Now().Truncate(time.Second)
	err := us.UpsertCVEEvaluation(ctx, &update.CVEEvaluation{
		ContainerID:    "c1",
		Status:         update.CVEEvaluated,
		EvaluatedAt:    evaluatedAt,
		Ecosystem:      "Debian",
		PackageName:    "openssl",
		PackageVersion: "1.1.1",
	})
	require.NoError(t, err)

	got, err := us.GetCVEEvaluation(ctx, "c1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "c1", got.ContainerID)
	assert.Equal(t, update.CVEEvaluated, got.Status)
	assert.True(t, evaluatedAt.Equal(got.EvaluatedAt))
	assert.Equal(t, "Debian", got.Ecosystem)
	assert.Equal(t, "openssl", got.PackageName)
	assert.Equal(t, "1.1.1", got.PackageVersion)
	assert.Empty(t, got.Error)
}

func TestCVEEvaluation_UpsertReplaces(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	us := NewUpdateStore(db)

	first := time.Now().Add(-time.Hour).Truncate(time.Second)
	require.NoError(t, us.UpsertCVEEvaluation(ctx, &update.CVEEvaluation{
		ContainerID: "c1",
		Status:      update.CVEEvaluated,
		EvaluatedAt: first,
		Ecosystem:   "Debian",
		PackageName: "openssl",
	}))

	second := time.Now().Truncate(time.Second)
	require.NoError(t, us.UpsertCVEEvaluation(ctx, &update.CVEEvaluation{
		ContainerID: "c1",
		Status:      update.CVEEvaluationError,
		EvaluatedAt: second,
		Error:       "osv.dev: connection refused",
	}))

	got, err := us.GetCVEEvaluation(ctx, "c1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, update.CVEEvaluationError, got.Status)
	assert.True(t, second.Equal(got.EvaluatedAt))
	assert.Equal(t, "osv.dev: connection refused", got.Error)

	rows, err := db.Reader().QueryContext(ctx, "SELECT COUNT(*) FROM cve_evaluations WHERE container_id = ?", "c1")
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	require.True(t, rows.Next())
	var count int
	require.NoError(t, rows.Scan(&count))
	assert.Equal(t, 1, count, "the second upsert must replace, not add a row")
}

func TestCVEEvaluation_GetMissingIsNil(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	us := NewUpdateStore(db)

	got, err := us.GetCVEEvaluation(ctx, "does-not-exist")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestCVEEvaluation_DeleteRemoves(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	us := NewUpdateStore(db)

	require.NoError(t, us.UpsertCVEEvaluation(ctx, &update.CVEEvaluation{
		ContainerID: "c1",
		Status:      update.CVEUnsupported,
		EvaluatedAt: time.Now(),
	}))

	require.NoError(t, us.DeleteCVEEvaluation(ctx, "c1"))

	got, err := us.GetCVEEvaluation(ctx, "c1")
	require.NoError(t, err)
	assert.Nil(t, got)
}
