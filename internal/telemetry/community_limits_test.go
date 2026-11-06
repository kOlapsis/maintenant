// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package telemetry

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kolapsis/maintenant/internal/extension"
)

// Acceptance guard of 051-community-label-limits: the telemetry does not change,
// no key is added to the snapshot (contracts/api.md, "Télémétrie").

var unchangedKeys = []string{
	"certificates_total",
	"containers_total",
	"edition",
	"endpoints_total",
	"heartbeats_total",
	"status_components_total",
	"storage_engine",
	"webhooks_total",
}

func TestProvider_SnapshotKeysDoNotChange(t *testing.T) {
	for name, c := range map[string]struct {
		edition                 extension.Edition
		endpoints, certificates int
	}{
		"community over the cap":  {extension.Community, 14, 7},
		"community under the cap": {extension.Community, 6, 4},
		"pro":                     {extension.Pro, 40, 12},
	} {
		t.Run(name, func(t *testing.T) {
			logger, _ := setupTestLogger()
			got := newMetricsProvider(Deps{
				Containers:       fakeCounter{value: 88},
				Endpoints:        fakeCounter{value: c.endpoints},
				Heartbeats:       fakeCounter{value: 3},
				Certificates:     fakeCounter{value: c.certificates},
				Webhooks:         fakeCounter{value: 1},
				StatusComponents: fakeCounter{value: 2},
				Edition:          staticEdition{value: c.edition},
			}, logger)()

			keys := make([]string, 0, len(got))
			for k := range got {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			assert.Equal(t, unchangedKeys, keys)
			assert.Equal(t, c.endpoints, got["endpoints_total"])
			assert.Equal(t, c.certificates, got["certificates_total"])
		})
	}
}
