// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package anomaly

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	model "github.com/kolapsis/maintenant/internal/anomaly"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

func apiFixture(t *testing.T) (http.Handler, *store.AnomalyStore) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := store.NewAnomalyStore(storetest.Open(t, logger))
	return NewHandler(s, DefaultConfig(), logger), s
}

func seedReadySeries(t *testing.T, s model.Store) model.SeriesKey {
	t.Helper()
	now := time.Now()
	readyAt := now.Unix()
	key := model.SeriesKey{ScopeType: model.ScopeTypeContainer, ScopeID: "compose/app/web", Metric: model.MetricCPU}
	require.NoError(t, s.UpsertSeriesState(context.Background(), &model.SeriesState{
		SeriesKey: key, State: model.StateReady, Sensitivity: model.SensitivityMedium, Progress: 1,
		FirstSeenAt: now.Add(-28 * 24 * time.Hour).Unix(), ReadyAt: &readyAt, UpdatedAt: now.Unix(),
	}))
	require.NoError(t, s.UpsertBaseline(context.Background(), model.Baseline{
		SeriesKey: key, Bucket: TimeOfWeekBucket(now, time.UTC), Median: 50, MAD: 5, SampleCount: 4, UpdatedAt: now.Unix(),
	}))
	return key
}

func serve(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rec
}

func TestAPI_EveryRouteAnswers(t *testing.T) {
	h, s := apiFixture(t)
	seedReadySeries(t, s)

	for _, path := range []string{
		"/api/v1/anomaly/summary",
		"/api/v1/anomaly/series",
		"/api/v1/anomaly/events",
		"/api/v1/anomaly/settings",
		"/api/v1/anomaly/series/container/compose%2Fapp%2Fweb",
		"/api/v1/anomaly/baseline/container/compose%2Fapp%2Fweb?metric=cpu",
	} {
		assert.Equal(t, http.StatusOK, serve(t, h, http.MethodGet, path, "").Code, path)
	}
}

func TestAPI_SummaryShape(t *testing.T) {
	h, s := apiFixture(t)
	seedReadySeries(t, s)

	rec := serve(t, h, http.MethodGet, "/api/v1/anomaly/summary", "")
	require.Equal(t, http.StatusOK, rec.Code)

	var body map[string]struct {
		Learning        int     `json:"learning"`
		Ready           int     `json:"ready"`
		ActiveAnomalies int     `json:"active_anomalies"`
		Progress        float64 `json:"progress"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Contains(t, body, model.ScopeTypeContainer)
	require.Contains(t, body, model.ScopeTypeHost)
	assert.Equal(t, 1, body[model.ScopeTypeContainer].Ready)
}

func TestAPI_ListSeriesShape(t *testing.T) {
	h, s := apiFixture(t)
	seedReadySeries(t, s)

	rec := serve(t, h, http.MethodGet, "/api/v1/anomaly/series?scope_type=container", "")
	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Series []struct {
			ScopeID string   `json:"scope_id"`
			State   string   `json:"state"`
			Metrics []string `json:"metrics"`
		} `json:"series"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Series, 1)
	assert.Equal(t, "compose/app/web", body.Series[0].ScopeID)
	assert.Equal(t, model.StateReady, body.Series[0].State)
	assert.NotEmpty(t, body.Series[0].Metrics)
}

func TestAPI_SeriesDetailOfUnknownScope(t *testing.T) {
	h, _ := apiFixture(t)
	assert.Equal(t, http.StatusNotFound, serve(t, h, http.MethodGet, "/api/v1/anomaly/series/container/compose/app/none", "").Code)
}

func TestAPI_BaselineBandShape(t *testing.T) {
	h, s := apiFixture(t)
	seedReadySeries(t, s)

	rec := serve(t, h, http.MethodGet, "/api/v1/anomaly/baseline/container/compose/app/web?metric=cpu", "")
	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Metric string `json:"metric"`
		Points []struct {
			Median float64 `json:"median"`
			Lower  float64 `json:"lower"`
			Upper  float64 `json:"upper"`
		} `json:"points"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "cpu", body.Metric)
	require.NotEmpty(t, body.Points, "a ready series yields band points")
	for _, p := range body.Points {
		assert.LessOrEqual(t, p.Lower, p.Median)
		assert.LessOrEqual(t, p.Median, p.Upper)
	}
}

func TestAPI_BaselineEmptyWhileLearning(t *testing.T) {
	h, s := apiFixture(t)
	now := time.Now().Unix()
	require.NoError(t, s.UpsertSeriesState(context.Background(), &model.SeriesState{
		SeriesKey:   model.SeriesKey{ScopeType: model.ScopeTypeContainer, ScopeID: "compose/app/web", Metric: model.MetricCPU},
		State:       model.StateLearning,
		Sensitivity: model.SensitivityMedium,
		FirstSeenAt: now,
		UpdatedAt:   now,
	}))

	rec := serve(t, h, http.MethodGet, "/api/v1/anomaly/baseline/container/compose/app/web?metric=cpu", "")
	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Points []any `json:"points"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.NotNil(t, body.Points, "points is an empty list, not null")
	assert.Empty(t, body.Points)
}

func TestAPI_EventsShape(t *testing.T) {
	h, s := apiFixture(t)
	key := seedReadySeries(t, s)
	now := time.Now().Unix()
	require.NoError(t, s.InsertAnomalyEvent(context.Background(), &model.AnomalyEvent{
		SeriesKey: key, Detector: model.DetectorSpike, Tier: model.TierActive, StartedAt: now,
		PeakValue: 95, BaselineMedian: 50, PeakDeviation: 6, CreatedAt: now,
	}))

	rec := serve(t, h, http.MethodGet, "/api/v1/anomaly/events?active=true", "")
	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Events []struct {
			Tier string `json:"tier"`
		} `json:"events"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Events, 1)
	assert.Equal(t, model.TierActive, body.Events[0].Tier)
}

func TestAPI_SettingsRoundTrip(t *testing.T) {
	h, _ := apiFixture(t)

	read := func() map[string]any {
		rec := serve(t, h, http.MethodGet, "/api/v1/anomaly/settings", "")
		require.Equal(t, http.StatusOK, rec.Code)
		var body map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		return body
	}

	initial := read()
	assert.Equal(t, float64(DefaultBucketPull), initial["bucket_pull"])
	assert.Equal(t, float64(MaxBucketPull), initial["max_bucket_pull"])
	assert.Equal(t, float64(DefaultConfig().MinSamples), initial["min_samples"])
	assert.Equal(t, DefaultConfig().Severity, initial["alert_severity"])

	require.Equal(t, http.StatusOK, serve(t, h, http.MethodPut, "/api/v1/anomaly/settings", `{"bucket_pull":8}`).Code)
	assert.Equal(t, float64(8), read()["bucket_pull"])
}

func TestAPI_SettingsRejectsOutOfRange(t *testing.T) {
	h, _ := apiFixture(t)
	for _, body := range []string{`{"bucket_pull":-1}`, `{"bucket_pull":999}`, `{}`, `not json`} {
		assert.Equal(t, http.StatusBadRequest, serve(t, h, http.MethodPut, "/api/v1/anomaly/settings", body).Code, body)
	}
}
