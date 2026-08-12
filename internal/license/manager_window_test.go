// Copyright 2026 Benjamin Touchard (kOlapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package license

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/extension"
)

// The window closed a year ago, so "inside" and "outside" read unambiguously
// against a build stamped either side of it.
const (
	windowEndWire = "2027-08-09T12:00:00Z"
	buildInside   = "2027-06-01T00:00:00Z"
	buildOutside  = "2027-09-01T00:00:00Z"
	buildLater    = "2027-10-15T00:00:00Z"
)

// personalHandler serves an active Personal license. An empty updatesUntil
// stands for the Pro case, which never carries a window.
func personalHandler(t *testing.T, priv ed25519.PrivateKey, updatesUntil string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, _ *http.Request) {
		signed := signPayload(t, priv, map[string]any{
			"status":        "active",
			"edition":       "personal",
			"plan":          "personal",
			"features":      []string{"*"},
			"updates_until": updatesUntil,
			"verified_at":   time.Now().UTC().Format(time.RFC3339),
		})
		_ = json.NewEncoder(w).Encode(signed)
	}
}

func withBuildDate(t *testing.T, m *Manager, buildDate string) *Manager {
	t.Helper()
	parsed, ok := parseBuildDate(buildDate)
	require.True(t, ok, "the test build date must parse")
	m.buildDate, m.buildDateKnown = parsed, true
	return m
}

func assertNoWindowRecord(t *testing.T, m *Manager) {
	t.Helper()
	_, err := os.Stat(windowPath(m.dataDir))
	assert.True(t, os.IsNotExist(err), "no update window record should have been written")
}

func readRecord(t *testing.T, m *Manager) *updateWindowRecord {
	t.Helper()
	rec := readWindowRecord(m.dataDir)
	require.NotNil(t, rec, "the update window record should exist")
	return rec
}

// TestManager_BuildInsideTheWindowIsUntouched: a version released inside the
// window stays licensed for life. The zero-I/O property matters as much as the
// verdict: nothing is anchored, so nothing can later be misread as a grace
// already spent.
func TestManager_BuildInsideTheWindowIsUntouched(t *testing.T) {
	pub, priv := generateTestKeyPair(t)
	m := withBuildDate(t, testManager(t, pub, personalHandler(t, priv, windowEndWire)), buildInside)

	m.check(context.Background())

	state := m.State()
	assert.Equal(t, extension.Personal, state.Edition)
	assert.Equal(t, "active", state.Status)
	assert.Empty(t, state.Message)
	assert.False(t, state.UpdatesUntil.IsZero(), "the window is still reported to the UI")
	assert.True(t, state.UpdateGraceUntil.IsZero())
	assertNoWindowRecord(t, m)
}

func TestManager_BuildOutsideTheWindowOpensAGrace(t *testing.T) {
	pub, priv := generateTestKeyPair(t)
	m := withBuildDate(t, testManager(t, pub, personalHandler(t, priv, windowEndWire)), buildOutside)

	m.check(context.Background())

	state := m.State()
	assert.Equal(t, extension.Personal, state.Edition, "the grace keeps the edition")
	assert.Equal(t, StatusUpdateWindowGrace, state.Status)
	assert.Contains(t, state.Message, "Renew")
	assert.False(t, state.UpdateGraceUntil.IsZero())

	rec := readRecord(t, m)
	assert.True(t, rec.WindowEnd.Equal(mustTime(t, windowEndWire)))
}

func TestManager_SpentGraceFallsBackToCommunity(t *testing.T) {
	pub, priv := generateTestKeyPair(t)
	m := withBuildDate(t, testManager(t, pub, personalHandler(t, priv, windowEndWire)), buildOutside)

	require.NoError(t, writeWindowRecord(m.dataDir, updateWindowRecord{
		WindowEnd: mustTime(t, windowEndWire),
		OpenedAt:  time.Now().Add(-31 * 24 * time.Hour),
	}))

	m.check(context.Background())

	state := m.State()
	assert.Equal(t, extension.Community, state.Edition)
	assert.Equal(t, StatusUpdateWindowEnded, state.Status)
	assert.Contains(t, state.Message, "Renew")
	assert.False(t, extension.Personal.AtLeast(extension.Pro), "precondition: Personal is below Pro")
}

// TestManager_TheClockDoesNotRestartOnANewerBuild is the assertion that protects
// the model. window_end is the key the grace was opened against, so a second,
// newer build inherits the clock already running. Without it, "update every
// month" would be a perpetual grace generator.
func TestManager_TheClockDoesNotRestartOnANewerBuild(t *testing.T) {
	pub, priv := generateTestKeyPair(t)
	handler := personalHandler(t, priv, windowEndWire)

	first := withBuildDate(t, testManager(t, pub, handler), buildOutside)
	first.check(context.Background())
	opened := readRecord(t, first).OpenedAt

	second := withBuildDate(t, testManager(t, pub, handler), buildLater)
	second.dataDir = first.dataDir
	second.check(context.Background())

	assert.True(t, readRecord(t, second).OpenedAt.Equal(opened),
		"a newer build restarted the grace clock")
	assert.True(t, second.State().UpdateGraceUntil.Equal(first.State().UpdateGraceUntil))
}

// TestManager_RollingBackIsFree: going back to a covered version restores
// everything at once, and leaves the anchor alone so alternating rollback and
// update harvests no grace.
func TestManager_RollingBackIsFree(t *testing.T) {
	pub, priv := generateTestKeyPair(t)
	handler := personalHandler(t, priv, windowEndWire)

	bridled := withBuildDate(t, testManager(t, pub, handler), buildOutside)
	bridled.check(context.Background())
	require.Equal(t, StatusUpdateWindowGrace, bridled.State().Status)
	anchored := readRecord(t, bridled)

	rolledBack := withBuildDate(t, testManager(t, pub, handler), buildInside)
	rolledBack.dataDir = bridled.dataDir
	rolledBack.check(context.Background())

	state := rolledBack.State()
	assert.Equal(t, extension.Personal, state.Edition)
	assert.Equal(t, "active", state.Status)
	assert.Empty(t, state.Message)
	assert.True(t, readRecord(t, rolledBack).OpenedAt.Equal(anchored.OpenedAt),
		"the rollback must not touch the record")
}

func TestManager_RenewalGrantsThirtyFreshDays(t *testing.T) {
	pub, priv := generateTestKeyPair(t)
	renewed := mustTime(t, windowEndWire).AddDate(1, 0, 0)

	m := withBuildDate(t, testManager(t, pub, personalHandler(t, priv, windowEndWire)), buildOutside)
	require.NoError(t, writeWindowRecord(m.dataDir, updateWindowRecord{
		WindowEnd: mustTime(t, windowEndWire),
		OpenedAt:  time.Now().Add(-29 * 24 * time.Hour),
	}))
	m.check(context.Background())
	require.Equal(t, StatusUpdateWindowGrace, m.State().Status)

	// The renewal moves the window past the build, which covers it outright.
	renewedMgr := withBuildDate(t, testManager(t, pub,
		personalHandler(t, priv, renewed.Format(time.RFC3339))), buildOutside)
	renewedMgr.dataDir = m.dataDir
	renewedMgr.check(context.Background())

	assert.Equal(t, "active", renewedMgr.State().Status)
	assert.Equal(t, extension.Personal, renewedMgr.State().Edition)

	// And a build past the renewed window restarts the clock, which is the only
	// place it may restart.
	pastRenewal := withBuildDate(t, testManager(t, pub,
		personalHandler(t, priv, renewed.Format(time.RFC3339))), renewed.Add(24*time.Hour).Format(time.RFC3339))
	pastRenewal.dataDir = m.dataDir
	pastRenewal.check(context.Background())

	assert.Equal(t, StatusUpdateWindowGrace, pastRenewal.State().Status)
	assert.WithinDuration(t, time.Now().Add(30*24*time.Hour), pastRenewal.State().UpdateGraceUntil, time.Minute)
}

// TestManager_NoWindowToEnforce covers the two payloads that must leave the
// license alone and write nothing: a Pro subscription, whose expires_at already
// governs it, and a window the build cannot read.
func TestManager_NoWindowToEnforce(t *testing.T) {
	cases := map[string]string{
		"pro carries no window": "",
		"unreadable window":     "the-ninth-of-august",
	}

	for name, updatesUntil := range cases {
		t.Run(name, func(t *testing.T) {
			pub, priv := generateTestKeyPair(t)
			m := withBuildDate(t, testManager(t, pub, personalHandler(t, priv, updatesUntil)), buildOutside)

			m.check(context.Background())

			state := m.State()
			assert.Equal(t, extension.Personal, state.Edition)
			assert.Equal(t, "active", state.Status)
			assert.True(t, state.UpdatesUntil.IsZero())
			assertNoWindowRecord(t, m)
		})
	}
}

// TestManager_UnknownBuildDateEnforcesNothing: reaching this control with an
// unreadable date means building from source with the verification key, which
// means being the maintainer. Locking it down buys nothing and breaks the
// contribution loop.
func TestManager_UnknownBuildDateEnforcesNothing(t *testing.T) {
	pub, priv := generateTestKeyPair(t)
	m := testManager(t, pub, personalHandler(t, priv, windowEndWire))
	require.False(t, m.buildDateKnown, "precondition: the harness leaves the build date unset")

	m.check(context.Background())

	assert.Equal(t, extension.Personal, m.State().Edition)
	assert.Equal(t, "active", m.State().Status)
	assertNoWindowRecord(t, m)
}

// TestManager_CorruptRecordReopensTheGrace: an unusable record is not fatal, and
// must not be read as a grace already spent.
func TestManager_CorruptRecordReopensTheGrace(t *testing.T) {
	pub, priv := generateTestKeyPair(t)
	m := withBuildDate(t, testManager(t, pub, personalHandler(t, priv, windowEndWire)), buildOutside)
	require.NoError(t, os.WriteFile(windowPath(m.dataDir), []byte("{not json"), 0600))

	m.check(context.Background())

	assert.Equal(t, StatusUpdateWindowGrace, m.State().Status)
	assert.Equal(t, extension.Personal, m.State().Edition)
	assert.WithinDuration(t, time.Now(), readRecord(t, m).OpenedAt, time.Minute)
}

// TestManager_ColdStartArrivesBridled: loadCache runs inside NewManager, before
// extension.CurrentEdition is bound, so a restart past the grace comes up in
// Community with no transition to propagate.
func TestManager_ColdStartArrivesBridled(t *testing.T) {
	pub, priv := generateTestKeyPair(t)
	m := withBuildDate(t, testManager(t, pub, personalHandler(t, priv, windowEndWire)), buildOutside)
	m.check(context.Background())
	require.Equal(t, StatusUpdateWindowGrace, m.State().Status)

	require.NoError(t, writeWindowRecord(m.dataDir, updateWindowRecord{
		WindowEnd: mustTime(t, windowEndWire),
		OpenedAt:  time.Now().Add(-31 * 24 * time.Hour),
	}))

	restarted := withBuildDate(t, testManager(t, pub, personalHandler(t, priv, windowEndWire)), buildOutside)
	restarted.dataDir = m.dataDir
	restarted.state.Store(&State{Status: "unknown", Edition: extension.Community})
	restarted.loadCache(context.Background())

	assert.Equal(t, extension.Community, restarted.State().Edition)
	assert.Equal(t, StatusUpdateWindowEnded, restarted.State().Status)
}

// TestManager_TheClockKeepsRunningOffline: handleNetworkError never calls
// applyPayload, so without a replay an operator who updates then unplugs would
// freeze the verdict until the next restart. The window message must also
// survive the "server unreachable" rungs, which are the less urgent of the two.
func TestManager_TheClockKeepsRunningOffline(t *testing.T) {
	pub, priv := generateTestKeyPair(t)

	online := true
	handler := func(w http.ResponseWriter, r *http.Request) {
		if !online {
			http.Error(w, "gone", http.StatusBadGateway)
			return
		}
		personalHandler(t, priv, windowEndWire)(w, r)
	}

	m := withBuildDate(t, testManager(t, pub, handler), buildOutside)
	m.check(context.Background())
	require.Equal(t, StatusUpdateWindowGrace, m.State().Status)

	// The grace runs out while the server stays unreachable.
	require.NoError(t, writeWindowRecord(m.dataDir, updateWindowRecord{
		WindowEnd: mustTime(t, windowEndWire),
		OpenedAt:  time.Now().Add(-31 * 24 * time.Hour),
	}))
	online = false
	m.check(context.Background())

	state := m.State()
	assert.Equal(t, StatusUpdateWindowEnded, state.Status, "the verdict must advance from the cache")
	assert.Equal(t, extension.Community, state.Edition)
	assert.Contains(t, state.Message, "Renew")
	assert.NotContains(t, state.Message, "unreachable",
		"the window message must not be replaced by the degradation rung")
}

// TestManager_OfflineWithoutAWindowIsUnchanged: the replay must not disturb the
// three unreachable rungs for a license that carries no window.
func TestManager_OfflineWithoutAWindowIsUnchanged(t *testing.T) {
	pub, priv := generateTestKeyPair(t)

	online := true
	handler := func(w http.ResponseWriter, r *http.Request) {
		if !online {
			http.Error(w, "gone", http.StatusBadGateway)
			return
		}
		signed := signPayload(t, priv, map[string]any{
			"status":      "active",
			"edition":     "pro",
			"plan":        "pro",
			"expires_at":  time.Now().Add(365 * 24 * time.Hour).UTC().Format(time.RFC3339),
			"verified_at": time.Now().Add(-10 * 24 * time.Hour).UTC().Format(time.RFC3339),
		})
		_ = json.NewEncoder(w).Encode(signed)
	}

	m := withBuildDate(t, testManager(t, pub, handler), buildOutside)
	m.check(context.Background())
	require.Equal(t, extension.Pro, m.State().Edition)

	online = false
	m.check(context.Background())

	state := m.State()
	assert.Equal(t, extension.Pro, state.Edition)
	assert.Equal(t, "active", state.Status)
	assert.Contains(t, state.Message, "unreachable")
	assertNoWindowRecord(t, m)
}

func mustTime(t *testing.T, wire string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, wire)
	require.NoError(t, err)
	return parsed
}
