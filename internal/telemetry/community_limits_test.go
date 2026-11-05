// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package telemetry

import (
	"context"
	"errors"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/extension"
)

// Acceptance tests of 051-community-label-limits, US5: four anonymous counters
// measure the effect of the cap (FR-018).

var communityLimitKeys = []string{
	"certificates_limited_total",
	"certificates_total",
	"containers_total",
	"edition",
	"endpoints_label_total",
	"endpoints_limited_total",
	"endpoints_standalone_total",
	"endpoints_total",
	"heartbeats_total",
	"status_components_total",
	"storage_engine",
	"webhooks_total",
}

func countOf(n int) CountFunc {
	return func(context.Context) (int, error) { return n, nil }
}

type limitCounts struct{ standalone, label, limitedEndpoints, limitedCerts int }

func limitDeps(c limitCounts) Deps {
	return Deps{
		Containers:          countOf(88),
		Endpoints:           countOf(c.standalone + c.label),
		Heartbeats:          countOf(3),
		Certificates:        countOf(9),
		Webhooks:            countOf(1),
		StatusComponents:    countOf(2),
		EndpointsStandalone: countOf(c.standalone),
		EndpointsLabel:      countOf(c.label),
		EndpointsLimited:    countOf(c.limitedEndpoints),
		CertificatesLimited: countOf(c.limitedCerts),
		Edition:             staticEdition{value: extension.Community},
	}
}

// CountFunc is the adapter the four counters are wired through.
func TestCountFunc_ReportsWhatTheFunctionCounts(t *testing.T) {
	for _, want := range []int{0, 7, 1234} {
		got, err := countOf(want).CountConfigured(context.Background())
		require.NoError(t, err)
		assert.Equal(t, want, got)
	}
	boom := errors.New("boom")
	_, err := CountFunc(func(context.Context) (int, error) { return 0, boom }).CountConfigured(context.Background())
	assert.ErrorIs(t, err, boom)
}

// US5 scenarios 1 and 2, FR-018, SC-007: the snapshot carries the manual, label,
// limited endpoints and limited certificates counts, integers only, and no key
// beyond the declared set.
func TestProvider_CarriesTheFourLimitCounters(t *testing.T) {
	for name, c := range map[string]limitCounts{
		"over the cap":  {standalone: 2, label: 12, limitedEndpoints: 4, limitedCerts: 1},
		"under the cap": {standalone: 3, label: 4},
		"many routers":  {standalone: 0, label: 40, limitedEndpoints: 30, limitedCerts: 6},
	} {
		t.Run(name, func(t *testing.T) {
			logger, _ := setupTestLogger()
			got := newMetricsProvider(limitDeps(c), logger)()

			keys := make([]string, 0, len(got))
			for k := range got {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			assert.Equal(t, communityLimitKeys, keys)

			assert.Equal(t, c.standalone, got["endpoints_standalone_total"])
			assert.Equal(t, c.label, got["endpoints_label_total"])
			assert.Equal(t, c.limitedEndpoints, got["endpoints_limited_total"])
			assert.Equal(t, c.limitedCerts, got["certificates_limited_total"])
			assert.Equal(t, c.standalone+c.label, got["endpoints_total"], "endpoints_total does not change")

			for k, v := range got {
				if k == "edition" || k == "storage_engine" {
					continue
				}
				_, isInt := v.(int)
				assert.True(t, isInt, "%s is a bare count, got %T", k, v)
			}
		})
	}
}

// US5 scenario 1: the counters reach the collector in the snapshot actually sent.
func TestTelemetryIntegration_SnapshotCarriesTheLimitCounters(t *testing.T) {
	collector := &fakeCollector{}
	server := httptest.NewServer(collector)
	t.Cleanup(server.Close)

	logger, buf := setupTestLogger()
	cfg := newTestConfig(t)
	cfg.Endpoint = server.URL
	svc := New(cfg, limitDeps(limitCounts{standalone: 2, label: 12, limitedEndpoints: 4, limitedCerts: 1}), logger)
	require.True(t, isActive(svc), buf.String())

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	svc.Start(ctx)
	require.Eventually(t, func() bool { return collector.snapshotCount() > 0 }, 3*time.Second, 20*time.Millisecond)

	metrics := collector.latestSnapshot()
	for key, want := range map[string]float64{
		"endpoints_standalone_total": 2,
		"endpoints_label_total":      12,
		"endpoints_limited_total":    4,
		"certificates_limited_total": 1,
	} {
		assert.Equal(t, want, metrics[key], key)
	}
}
