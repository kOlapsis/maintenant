// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/eol"
	"github.com/kolapsis/maintenant/internal/event"
)

func TestBaseRiskScore(t *testing.T) {
	for ut, want := range map[UpdateType]int{
		UpdateTypePatch:      45,
		UpdateTypeMinor:      40,
		UpdateTypeMajor:      20,
		UpdateTypeDigestOnly: 5,
		UpdateTypeUnknown:    0,
		"":                   0,
	} {
		assert.Equal(t, want, BaseRiskScore(ut), ut)
	}
}

type staticEOLTable eol.Table

func (s staticEOLTable) Current() eol.Table { return eol.Table(s) }

func cycleEndingIn(t *testing.T, name string, days int) eol.Cycle {
	t.Helper()
	d, err := eol.ParseDate(time.Now().UTC().AddDate(0, 0, days).Format(time.DateOnly))
	require.NoError(t, err)
	return eol.Cycle{Name: name, SecurityUntil: d}
}

func postgresLifecycle(t *testing.T) staticEOLTable {
	t.Helper()
	return staticEOLTable{Source: "test", Products: map[string][]eol.Cycle{"postgresql": {
		cycleEndingIn(t, "14", -10),
		cycleEndingIn(t, "15", 10),
		cycleEndingIn(t, "16", 400),
	}}}
}

func TestScanScore_RaisedByTheLifecycleOfTheRunningVersion(t *testing.T) {
	cases := []struct {
		name       string
		image      string
		updateType UpdateType
		noTable    bool
		want       int
	}{
		{"support ended", "postgres:14.12", UpdateTypePatch, false, 85},
		{"support ended, major", "postgres:14.12", UpdateTypeMajor, false, 85},
		{"support ending soon", "postgres:15.7", UpdateTypeMinor, false, 65},
		{"supported", "postgres:16.3", UpdateTypePatch, false, 45},
		{"not an official image", "acme/postgres:14.12", UpdateTypePatch, false, 45},
		{"unknown cycle", "postgres:9.1.2", UpdateTypeMajor, false, 20},
		{"no table provider", "postgres:14.12", UpdateTypePatch, true, 45},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := Deps{Store: &stubStore{}, Scanner: newTestScanner(&stubRegistry{}, &stubStore{}), Containers: stubLister{}, Logger: testLogger()}
			if !tc.noTable {
				deps.EOL = postgresLifecycle(t)
			}
			_, tag, _ := ParseImageRef(tc.image)
			got := NewService(deps).scanScore(UpdateResult{Image: tc.image, CurrentTag: tag, UpdateType: tc.updateType})
			assert.Equal(t, tc.want, got)
		})
	}
}

// scoringEnricher plays the Pro enricher: it records what it was given and sets a final score.
type scoringEnricher struct {
	seen  []UpdateResult
	order *[]string
}

func (e *scoringEnricher) Enrich(_ context.Context, results []UpdateResult) error {
	*e.order = append(*e.order, "enrich")
	e.seen = append(e.seen, results...)
	for i := range results {
		if results[i].HasUpdate {
			results[i].RiskScore = 92
		}
	}
	return nil
}

func TestRunScan_EmitsTheDetectionAfterEnrichment(t *testing.T) {
	store := &captureStore{stubStore: &stubStore{baseline: &DigestBaseline{ContainerID: "ctr1", Tag: "18.7", RemoteDigest: "sha256:new"}}}
	reg := &stubRegistry{tags: map[string][]string{"library/postgres": {"18.7", "18.8", "19.0"}}, digest: "sha256:new"}

	var detected []map[string]interface{}
	var order []string
	enricher := &scoringEnricher{order: &order}
	svc := NewService(Deps{
		Store:      store,
		Scanner:    newTestScanner(reg, store),
		Containers: stubLister{containers: []ContainerInfo{{UID: "uid1", ExternalID: "ctr1", Name: "db", Image: "postgres:18.7"}}},
		Logger:     testLogger(),
		Enricher:   enricher,
		EOL:        postgresLifecycle(t),
		EventCallback: func(eventType string, data interface{}) {
			if eventType == event.UpdateDetected {
				order = append(order, eventType)
				detected = append(detected, data.(map[string]interface{}))
			}
		},
	})

	svc.runScan(context.Background())

	require.Len(t, store.inserted, 1)
	assert.Equal(t, "18.8", store.inserted[0].LatestTag)
	assert.Equal(t, "19.0", store.inserted[0].MajorTag)
	assert.Equal(t, BaseRiskScore(UpdateTypeMinor), store.inserted[0].RiskScore, "the scan score is persisted first")

	require.Len(t, enricher.seen, 1)
	assert.Equal(t, BaseRiskScore(UpdateTypeMinor), enricher.seen[0].RiskScore, "the enricher receives the scan score")
	assert.Equal(t, []string{"enrich", event.UpdateDetected}, order, "nothing is announced before the enrichment")

	require.Len(t, detected, 1)
	assert.Equal(t, 92, detected[0]["risk_score"])
	assert.Equal(t, "19.0", detected[0]["major_tag"])
	assert.Equal(t, "18.8", detected[0]["latest_tag"])
}

func TestRunScan_DetectionWithoutMajorTagLeavesItOut(t *testing.T) {
	store := &captureStore{stubStore: &stubStore{}}
	reg := &stubRegistry{tags: map[string][]string{"library/postgres": {"14.12", "14.13"}}}

	var detected map[string]interface{}
	svc := NewService(Deps{
		Store:      store,
		Scanner:    newTestScanner(reg, store),
		Containers: stubLister{containers: []ContainerInfo{{ExternalID: "ctr1", Name: "db", Image: "postgres:14.12"}}},
		Logger:     testLogger(),
		EOL:        postgresLifecycle(t),
		EventCallback: func(eventType string, data interface{}) {
			if eventType == event.UpdateDetected {
				detected = data.(map[string]interface{})
			}
		},
	})

	svc.runScan(context.Background())

	require.NotNil(t, detected)
	assert.Equal(t, 85, detected["risk_score"], "without an enricher the scan score is final")
	assert.NotContains(t, detected, "major_tag")
}
