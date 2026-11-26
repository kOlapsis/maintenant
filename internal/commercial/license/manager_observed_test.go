// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package license

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/extension"
)

const setBackWarning = "clock reads earlier than"

func setWallClock(t *testing.T, at time.Time) {
	t.Helper()
	prev := wallClock
	offset := time.Until(at)
	wallClock = func() time.Time { return time.Now().Add(offset) }
	t.Cleanup(func() { wallClock = prev })
}

// restarted builds a manager over a cache last observed at observedAt and loads it, as NewManager does.
func restarted(t *testing.T, pub ed25519.PublicKey, priv ed25519.PrivateKey, handler http.HandlerFunc, payload LicensePayload, observedAt time.Time) (*Manager, *bytes.Buffer) {
	t.Helper()
	m := testManager(t, pub, handler)
	var logs bytes.Buffer
	m.logger = slog.New(slog.NewTextHandler(&logs, nil))
	signed := signPayload(t, priv, payload)
	require.NoError(t, writeCache(m.dataDir, &signed, observedAt))
	m.loadCache(context.Background())
	return m, &logs
}

func serverDown(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusBadGateway)
}

func TestManager_RestartWithTheClockSetBack(t *testing.T) {
	cases := []struct {
		name        string
		verifiedAgo time.Duration
		expiresIn   time.Duration
		wantEdition extension.Edition
		wantStatus  string
		wantMessage string
	}{
		{"inside the grace", 10 * 24 * time.Hour, 365 * 24 * time.Hour, extension.Pro, "active", "remains active from cache"},
		{"beyond the grace", 61 * 24 * time.Hour, 365 * 24 * time.Hour, extension.Community, "unreachable", "over 60 days"},
		{"past expires_at", 10 * 24 * time.Hour, -24 * time.Hour, extension.Community, "expired", "expired"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pub, priv := generateTestKeyPair(t)
			now := time.Now()
			payload := proPayload()
			payload.VerifiedAt = now.Add(-tc.verifiedAgo)
			payload.ExpiresAt = now.Add(tc.expiresIn)
			setWallClock(t, payload.VerifiedAt.Add(time.Hour))

			m, logs := restarted(t, pub, priv, serverDown, payload, now)
			m.check(context.Background())

			state := m.State()
			assert.Equal(t, tc.wantEdition, state.Edition)
			assert.Equal(t, tc.wantStatus, state.Status)
			assert.Contains(t, state.Message, tc.wantMessage)
			assert.Contains(t, logs.String(), setBackWarning)
		})
	}
}

func TestManager_RestartWithoutTheCacheFileHasNoOfflineGrace(t *testing.T) {
	pub, _ := generateTestKeyPair(t)
	setWallClock(t, time.Now().Add(-30*24*time.Hour))
	m := testManager(t, pub, serverDown)
	m.loadCache(context.Background())

	m.check(context.Background())

	assert.Equal(t, extension.Community, m.Edition())
	assert.Equal(t, "unreachable", m.State().Status)
}

func TestManager_ObservedTimeFromTheFutureIsRecoveredByTheServer(t *testing.T) {
	pub, priv := generateTestKeyPair(t)
	var online atomic.Bool
	handler := func(w http.ResponseWriter, r *http.Request) {
		if !online.Load() {
			serverDown(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(signPayload(t, priv, proPayload()))
	}
	payload := proPayload()
	payload.VerifiedAt = time.Now().Add(-time.Hour)
	m, logs := restarted(t, pub, priv, handler, payload, time.Now().Add(400*24*time.Hour))

	assert.Equal(t, extension.Community, m.Edition(), "offline, a time already observed cannot be proven wrong")
	assert.Contains(t, logs.String(), setBackWarning)

	m.check(context.Background())
	assert.Equal(t, extension.Community, m.Edition())

	online.Store(true)
	m.check(context.Background())
	assert.Equal(t, extension.Pro, m.Edition(), "a current signed answer that agrees with the clock corrects the observed time")
	_, observedAt, err := readCache(m.dataDir, pub)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now(), observedAt, time.Minute)
}

func TestManager_ClockCorrectedForwardAfterABadStartRecovers(t *testing.T) {
	pub, priv := generateTestKeyPair(t)
	handler := func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(signPayload(t, priv, proPayload()))
	}
	payload := proPayload()
	payload.VerifiedAt = time.Now().Add(-48 * time.Hour)
	setWallClock(t, time.Now().Add(-30*24*time.Hour))
	m, _ := restarted(t, pub, priv, handler, payload, time.Now().Add(-time.Hour))
	require.Equal(t, extension.Pro, m.Edition())

	assert.True(t, m.check(context.Background()), "an answer that agrees with the observed time is current")
	assert.Equal(t, extension.Pro, m.Edition())

	wallClock = time.Now
	assert.True(t, m.check(context.Background()))
	assert.Equal(t, "active", m.State().Status)
	assert.Empty(t, m.State().Message)
}

func TestManager_CorrectClockSeesNoDifference(t *testing.T) {
	pub, priv := generateTestKeyPair(t)
	payload := proPayload()
	payload.VerifiedAt = time.Now().Add(-time.Hour)
	m, logs := restarted(t, pub, priv, serverDown, payload, time.Now().Add(-time.Minute))

	m.check(context.Background())

	assert.Equal(t, extension.Pro, m.Edition())
	assert.Equal(t, "active", m.State().Status)
	assert.Empty(t, m.State().Message)
	assert.NotContains(t, logs.String(), setBackWarning)
}

func TestManager_ObservedTimeIsRecordedWhileRunningAndAtStop(t *testing.T) {
	cases := map[string]time.Duration{
		"while running": 10 * time.Millisecond,
		"at stop":       time.Hour,
	}
	for name, interval := range cases {
		t.Run(name, func(t *testing.T) {
			prev := observeInterval
			observeInterval = interval
			t.Cleanup(func() { observeInterval = prev })
			pub, priv := generateTestKeyPair(t)
			stale := time.Now().Add(-time.Hour)
			m, _ := restarted(t, pub, priv, serverDown, proPayload(), stale)
			observed := func() time.Time {
				_, at, _ := readCache(m.dataDir, pub)
				return at
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			m.Start(ctx)
			if interval < time.Second {
				require.Eventually(t, func() bool { return observed().After(stale.Add(30 * time.Minute)) },
					5*time.Second, 5*time.Millisecond, "the observed time must be recorded while the instance runs")
			}
			before := time.Now()
			m.Stop()

			assert.False(t, observed().Before(before.Add(-time.Second)), "the observed time must be recorded at stop")
		})
	}
}
