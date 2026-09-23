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
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testWindowEnd = time.Date(2027, time.August, 9, 12, 0, 0, 0, time.UTC)
	testNow       = time.Date(2027, time.September, 1, 8, 0, 0, 0, time.UTC)
)

func TestReconcileWindow(t *testing.T) {
	anchored := &updateWindowRecord{
		WindowEnd: testWindowEnd,
		OpenedAt:  testNow.Add(-10 * 24 * time.Hour),
		Version:   "1.2.3",
		BuildDate: "2027-08-20T00:00:00Z",
	}

	cases := []struct {
		name        string
		stored      *updateWindowRecord
		windowEnd   time.Time
		wantOpened  time.Time
		wantWindow  time.Time
		wantWritten bool
	}{
		{
			name:        "absent record opens a grace at now",
			stored:      nil,
			windowEnd:   testWindowEnd,
			wantOpened:  testNow,
			wantWindow:  testWindowEnd,
			wantWritten: true,
		},
		{
			// The assertion that protects the model: a newer build under the same
			// window inherits the running clock. Restarting it here would make
			// "update every month" a perpetual grace generator.
			name:        "same window, newer build, the clock does not restart",
			stored:      anchored,
			windowEnd:   testWindowEnd,
			wantOpened:  anchored.OpenedAt,
			wantWindow:  testWindowEnd,
			wantWritten: false,
		},
		{
			name:        "a later window is a renewal and re-anchors",
			stored:      anchored,
			windowEnd:   testWindowEnd.AddDate(1, 0, 0),
			wantOpened:  testNow,
			wantWindow:  testWindowEnd.AddDate(1, 0, 0),
			wantWritten: true,
		},
		{
			name:        "an earlier window is not a renewal",
			stored:      anchored,
			windowEnd:   testWindowEnd.AddDate(-1, 0, 0),
			wantOpened:  anchored.OpenedAt,
			wantWindow:  testWindowEnd,
			wantWritten: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, written := reconcileWindow(c.stored, c.windowEnd, testNow, "9.9.9", "2027-09-01T00:00:00Z")

			assert.Equal(t, c.wantWritten, written)
			assert.True(t, got.OpenedAt.Equal(c.wantOpened), "opened_at = %s, want %s", got.OpenedAt, c.wantOpened)
			assert.True(t, got.WindowEnd.Equal(c.wantWindow), "window_end = %s, want %s", got.WindowEnd, c.wantWindow)
		})
	}
}

// TestReconcileWindow_ClockTurnedBack: an anchor in the future can only come
// from a clock that moved backwards. Re-anchoring to now is the only reading
// that cannot hand out more grace than was granted.
func TestReconcileWindow_ClockTurnedBack(t *testing.T) {
	stored := &updateWindowRecord{
		WindowEnd: testWindowEnd,
		OpenedAt:  testNow.Add(48 * time.Hour),
	}

	got, written := reconcileWindow(stored, testWindowEnd, testNow, "1.0.0", "2027-09-01")

	assert.True(t, written)
	assert.True(t, got.OpenedAt.Equal(testNow))
}

// TestUpdateGraceEnd_Boundaries pins the month of grace at its edges.
func TestUpdateGraceEnd_Boundaries(t *testing.T) {
	opened := testNow
	rec := updateWindowRecord{WindowEnd: testWindowEnd, OpenedAt: opened}
	graceEnd := updateGraceEnd(rec)

	cases := []struct {
		name      string
		now       time.Time
		stillOpen bool
	}{
		{"day 0", opened, true},
		{"day 29", opened.Add(29 * 24 * time.Hour), true},
		{"day 30", opened.Add(30 * 24 * time.Hour), false},
		{"day 31", opened.Add(31 * 24 * time.Hour), false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.stillOpen, c.now.Before(graceEnd))
		})
	}
}

func TestParseBuildDate(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		ok    bool
		want  time.Time
		notes string
	}{
		{name: "RFC3339 as stamped by CI", in: "2026-08-10T14:32:07Z", ok: true,
			want: time.Date(2026, time.August, 10, 14, 32, 7, 0, time.UTC)},
		{name: "RFC3339 with an offset", in: "2026-08-10T16:32:07+02:00", ok: true,
			want: time.Date(2026, time.August, 10, 14, 32, 7, 0, time.UTC)},
		{name: "date only, as the compose files stamp it", in: "2026-03-01", ok: true,
			want: time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)},
		{name: "the unknown default", in: "unknown", ok: false},
		{name: "empty", in: "", ok: false},
		{name: "a locale format", in: "01/03/2026", ok: false},
		{name: "nonsense", in: "not-a-date", ok: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := parseBuildDate(c.in)
			require.Equal(t, c.ok, ok)
			if c.ok {
				assert.True(t, got.UTC().Equal(c.want), "got %s, want %s", got.UTC(), c.want)
			}
		})
	}
}

// TestUpdateWindowMessages_CarryTheActionVerb: DefaultLayout.vue builds the
// banner's action button from this exact expression. It matches neither
// "renewal" nor "renewing", so both messages must carry the bare verb.
func TestUpdateWindowMessages_CarryTheActionVerb(t *testing.T) {
	actionVerb := regexp.MustCompile(`(?i)\b(renew|resubscribe)\b`)
	graceEnd := testNow.Add(12 * 24 * time.Hour)

	messages := map[string]string{
		"grace": updateWindowGraceMessage(testWindowEnd, graceEnd, testNow),
		"ended": updateWindowEndedMessage(testWindowEnd),
	}

	for name, msg := range messages {
		t.Run(name, func(t *testing.T) {
			assert.Regexp(t, actionVerb, msg, "no action verb, the banner would show no button")
			assert.Contains(t, msg, "go back to the version you were using",
				"the rollback is the free way out and must be offered")
			assert.Contains(t, msg, "9 August 2027")
		})
	}

	assert.Contains(t, messages["grace"], "in 12 days")
	assert.Contains(t, messages["ended"], "has fallen back to the Community edition")
}

func TestDaysUntil(t *testing.T) {
	cases := []struct {
		name string
		in   time.Duration
		want int
	}{
		{"a full month", 30 * 24 * time.Hour, 30},
		{"twelve days", 12 * 24 * time.Hour, 12},
		{"a day and a half rounds up", 36 * time.Hour, 2},
		{"the last hour still reads as a day", time.Hour, 1},
		{"already elapsed never reads as zero", -time.Hour, 1},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, daysUntil(testNow.Add(c.in), testNow))
		})
	}
}

func TestWindowRecord_RoundTripAndPermissions(t *testing.T) {
	dir := t.TempDir()
	rec := updateWindowRecord{
		WindowEnd: testWindowEnd,
		OpenedAt:  testNow,
		Version:   "1.2.3",
		BuildDate: "2027-09-01T00:00:00Z",
	}

	require.NoError(t, writeWindowRecord(dir, rec))

	info, err := os.Stat(filepath.Join(dir, windowFileName))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())

	got := readWindowRecord(dir)
	require.NotNil(t, got)
	assert.True(t, got.OpenedAt.Equal(testNow))
	assert.True(t, got.WindowEnd.Equal(testWindowEnd))
	assert.Equal(t, "1.2.3", got.Version)
}

// TestReadWindowRecord_UnusableIsIndistinguishableFromAbsent: a corrupt or
// truncated record must reopen a grace, not crash and not bridle on the spot.
func TestReadWindowRecord_UnusableIsIndistinguishableFromAbsent(t *testing.T) {
	cases := map[string]string{
		"corrupt":         "{not json",
		"truncated":       `{"window_end":"2027-08-09T12:00:00`,
		"empty":           "",
		"missing anchors": `{"version":"1.2.3"}`,
	}

	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, windowFileName), []byte(content), 0600))

			assert.Nil(t, readWindowRecord(dir))
		})
	}

	t.Run("absent", func(t *testing.T) {
		assert.Nil(t, readWindowRecord(t.TempDir()))
	})
}
