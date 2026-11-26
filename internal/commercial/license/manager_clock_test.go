// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package license

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/extension"
)

func TestManager_RefusesALiveAnswerThatIsNotCurrent(t *testing.T) {
	cases := []struct {
		name   string
		offset time.Duration
		want   extension.Edition
	}{
		{"replayed over a day later", -25 * time.Hour, extension.Community},
		{"dated over a day ahead", 25 * time.Hour, extension.Community},
		{"an hour behind", -time.Hour, extension.Pro},
		{"an hour ahead", time.Hour, extension.Pro},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pub, priv := generateTestKeyPair(t)
			m := testManager(t, pub, func(w http.ResponseWriter, _ *http.Request) {
				payload := proPayload()
				payload.VerifiedAt = time.Now().Add(tc.offset)
				_ = json.NewEncoder(w).Encode(signPayload(t, priv, payload))
			})

			ok := m.check(context.Background())

			assert.Equal(t, tc.want, m.Edition())
			assert.Equal(t, tc.want == extension.Pro, ok, "a refused answer must be retried")
			if tc.want == extension.Community {
				assert.NoFileExists(t, cachePath(m.dataDir), "a refused answer must not be cached")
			}
		})
	}
}

func TestManager_ClockSetBackBeforeTheLastCheckDoesNotReviveALapsedCache(t *testing.T) {
	pub, priv := generateTestKeyPair(t)
	m := testManager(t, pub, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	})
	lapsed := proPayload()
	lapsed.VerifiedAt = time.Now().Add(30 * 24 * time.Hour)
	lapsed.ExpiresAt = time.Now().Add(10 * 24 * time.Hour)
	signed := signPayload(t, priv, lapsed)
	require.NoError(t, writeCache(m.dataDir, &signed, time.Now()))

	m.loadCache(context.Background())

	assert.Equal(t, extension.Community, m.Edition(), "the clock cannot read earlier than the last signed check")
	assert.Equal(t, "expired", m.State().Status)
}

func TestManager_ClockSetBackWhileRunningDoesNotExtendTheOfflineGrace(t *testing.T) {
	pub, priv := generateTestKeyPair(t)
	m := proManagerWithCache(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}, pub, priv)
	// 61 days ran on the monotonic clock while the wall clock was set back to the time of the check.
	m.floorSeen = m.floorSeen.Add(-61 * 24 * time.Hour)

	m.check(context.Background())

	assert.Equal(t, extension.Community, m.Edition())
	assert.Equal(t, "unreachable", m.State().Status)
}
