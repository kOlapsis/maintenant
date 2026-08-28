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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// The two statuses a closed update window produces. A Personal license is
// bought once and never expires; what is bounded is the right to new versions.
// These overwrite State.Status rather than living in a parallel field: status
// is what every consumer already reads, and once the grace is spent "active"
// would be a lie.
const (
	StatusUpdateWindowGrace = "update_window_grace"
	StatusUpdateWindowEnded = "update_window_ended"
)

const (
	// updateWindowGracePeriod is how long a build released after the window
	// keeps its edition before falling back to Community.
	updateWindowGracePeriod = 30 * 24 * time.Hour

	windowFileName = ".maintenant-update-window"

	windowDateLayout = "2 January 2006"
)

// updateWindowRecord anchors the grace granted to a build released after the
// license's update window closed. It lives next to the signed cache rather than
// in SQLite: this package imports only internal/extension, and a store would
// need an interface, a migration and a nil path for agent mode. Neither file is
// a security boundary: the same person can delete both.
//
// WindowEnd is the whole anti-abuse design: the grace is opened against one
// window, so a newer build under that same window inherits the clock already
// running instead of starting a fresh one. Without that key, "update every
// month" would be a perpetual grace generator.
//
// Version and BuildDate are diagnostic only.
type updateWindowRecord struct {
	WindowEnd time.Time `json:"window_end"`
	OpenedAt  time.Time `json:"opened_at"`
	Version   string    `json:"version"`
	BuildDate string    `json:"build_date"`
}

func windowPath(dataDir string) string {
	return filepath.Join(dataDir, windowFileName)
}

// readWindowRecord returns nil when the record is missing, unreadable or
// corrupt. The three cases mean the same thing to the caller: no grace has been
// anchored yet.
func readWindowRecord(dataDir string) *updateWindowRecord {
	data, err := os.ReadFile(windowPath(dataDir))
	if err != nil {
		return nil
	}

	var rec updateWindowRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil
	}
	if rec.WindowEnd.IsZero() || rec.OpenedAt.IsZero() {
		return nil
	}

	return &rec
}

func writeWindowRecord(dataDir string, rec updateWindowRecord) error {
	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshaling update window record: %w", err)
	}

	if err := os.WriteFile(windowPath(dataDir), data, 0600); err != nil {
		return fmt.Errorf("writing update window record: %w", err)
	}

	return nil
}

// reconcileWindow decides which record to hold once the running build is known
// to sit outside the window. The second result says whether it must be
// persisted: the common case, a grace already anchored, writes nothing.
func reconcileWindow(stored *updateWindowRecord, windowEnd, now time.Time, version, buildDate string) (updateWindowRecord, bool) {
	fresh := updateWindowRecord{
		WindowEnd: windowEnd,
		OpenedAt:  now,
		Version:   version,
		BuildDate: buildDate,
	}

	switch {
	case stored == nil:
		return fresh, true

	// A renewal moves the window forward, and this is the only place the clock
	// restarts.
	case stored.WindowEnd.Before(windowEnd):
		return fresh, true

	// The clock was turned back past the anchor. Re-anchoring to now is the only
	// reading that cannot hand out more grace than was granted.
	case stored.OpenedAt.After(now):
		return fresh, true

	// Same window, whatever the build: the clock keeps running from where it
	// was. This branch is the model.
	default:
		return *stored, false
	}
}

// updateGraceEnd is when the bridled edition takes effect.
func updateGraceEnd(rec updateWindowRecord) time.Time {
	return rec.OpenedAt.Add(updateWindowGracePeriod)
}

// parseBuildDate reads the stamp ldflags inject into the binary. Two layouts are
// in service: RFC3339 from CI and scripts/build-local-release.sh, and the
// date-only form the compose files use. Anything else, "unknown" included,
// reports false and leaves the window unenforced.
func parseBuildDate(s string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339, time.DateOnly} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// The application composes these, not the license server: only it knows which
// version is running, how many days a local clock has left, and how to phrase
// the rollback. The server sends a date.
//
// Both must contain the bare verb "renew". DefaultLayout.vue builds the
// banner's action button from /\b(renew|resubscribe)\b/i, which matches neither
// "renewal" nor "renewing". Neither can name the version to fall back to: the
// application has no catalogue of release dates.

func updateWindowGraceMessage(windowEnd, graceEnd, now time.Time) string {
	return "This version was released after your update window closed on " +
		windowEnd.UTC().Format(windowDateLayout) +
		". Renew your Personal updates to keep it, or go back to the version you were using. " +
		"This instance falls back to the Community edition in " +
		formatDays(daysUntil(graceEnd, now)) + "."
}

func updateWindowEndedMessage(windowEnd time.Time) string {
	return "Your update window closed on " + windowEnd.UTC().Format(windowDateLayout) +
		" and this version is not covered. This instance has fallen back to the " +
		"Community edition. Renew your Personal updates, or go back to the version you were using."
}

// daysUntil rounds up and never returns less than 1: the message is only built
// while the grace is still open, and "in 0 days" would read as already over.
func daysUntil(deadline, now time.Time) int {
	remaining := deadline.Sub(now)
	if remaining <= 0 {
		return 1
	}
	days := int((remaining + 24*time.Hour - time.Nanosecond) / (24 * time.Hour))
	if days < 1 {
		return 1
	}
	return days
}
