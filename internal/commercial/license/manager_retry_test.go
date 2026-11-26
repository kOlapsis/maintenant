// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package license

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/extension"
)

func fastRetry(t *testing.T) {
	t.Helper()
	prevMin, prevMax := checkRetryMin, checkRetryMax
	checkRetryMin, checkRetryMax = 5*time.Millisecond, 20*time.Millisecond
	t.Cleanup(func() { checkRetryMin, checkRetryMax = prevMin, prevMax })
}

func TestManager_RetriesSoonAfterAFailedStartupCheck(t *testing.T) {
	fastRetry(t)
	pub, priv := generateTestKeyPair(t)
	var calls atomic.Int32
	m := testManager(t, pub, func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) <= 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_ = json.NewEncoder(w).Encode(signPayload(t, priv, proPayload()))
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Start(ctx)
	defer m.Stop()
	require.Equal(t, "unreachable", m.State().Status)

	require.Eventually(t, func() bool { return m.Edition() == extension.Pro }, 5*time.Second, 5*time.Millisecond,
		"the check must be retried soon once the network is back")

	settled := calls.Load()
	assert.Never(t, func() bool { return calls.Load() != settled }, 300*time.Millisecond, 10*time.Millisecond,
		"a successful check must bring back the normal interval")
}

func TestManager_SignedRefusalWaitsForTheNormalInterval(t *testing.T) {
	fastRetry(t)
	pub, priv := generateTestKeyPair(t)
	var calls atomic.Int32
	m := proManagerWithCache(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writeSignedRefusal(t, w, priv, http.StatusForbidden, LicensePayload{Status: "revoked", Message: "refunded"})
	}, pub, priv)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Start(ctx)
	defer m.Stop()

	assert.Equal(t, extension.Community, m.Edition())
	assert.NoFileExists(t, cachePath(m.dataDir), "a signed refusal deletes the cache")
	assert.Never(t, func() bool { return calls.Load() != 1 }, 300*time.Millisecond, 10*time.Millisecond,
		"a signed refusal is a verdict, not a failure to retry")
}

func TestManager_ExpiryPassedDuringAnOutageKeepsTheCache(t *testing.T) {
	cases := map[string]func(m *Manager, lapsed *LicensePayload){
		"at startup":    func(m *Manager, _ *LicensePayload) { m.loadCache(context.Background()) },
		"while running": func(m *Manager, lapsed *LicensePayload) { m.applyPayload(context.Background(), lapsed) },
	}
	for name, load := range cases {
		t.Run(name, func(t *testing.T) {
			pub, priv := generateTestKeyPair(t)
			var online atomic.Bool
			m := testManager(t, pub, func(w http.ResponseWriter, _ *http.Request) {
				if !online.Load() {
					w.WriteHeader(http.StatusBadGateway)
					return
				}
				_ = json.NewEncoder(w).Encode(signPayload(t, priv, proPayload()))
			})
			lapsed := proPayload()
			lapsed.VerifiedAt = time.Now().Add(-2 * 24 * time.Hour)
			lapsed.ExpiresAt = time.Now().Add(-time.Hour)
			signed := signPayload(t, priv, lapsed)
			require.NoError(t, writeCache(m.dataDir, &signed, time.Now()))
			load(m, &lapsed)

			m.check(context.Background())
			assert.Equal(t, extension.Community, m.Edition())
			assert.Equal(t, "expired", m.State().Status)
			assert.FileExists(t, cachePath(m.dataDir), "a network failure must not delete the cache")

			m.check(context.Background())
			assert.Equal(t, "expired", m.State().Status, "a second failed check must keep reporting the expiry")

			online.Store(true)
			m.check(context.Background())
			assert.Equal(t, extension.Pro, m.Edition(), "the renewal is read as soon as the server answers")
		})
	}
}

func TestManager_CacheOlderThanTheOfflineGraceGrantsNothingAtLoad(t *testing.T) {
	pub, priv := generateTestKeyPair(t)
	m := testManager(t, pub, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	})
	stale := proPayload()
	stale.VerifiedAt = time.Now().Add(-61 * 24 * time.Hour)
	signed := signPayload(t, priv, stale)
	require.NoError(t, writeCache(m.dataDir, &signed, time.Now()))

	m.loadCache(context.Background())
	assert.Equal(t, extension.Community, m.Edition())
	assert.Equal(t, "unreachable", m.State().Status)

	m.check(context.Background())
	assert.Equal(t, extension.Community, m.Edition())
	assert.FileExists(t, cachePath(m.dataDir), "a network failure must not delete the cache")
}
