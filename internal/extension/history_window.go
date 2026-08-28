// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package extension

import (
	"strings"
	"time"
)

// HistoryWindow is one of the resource-history ranges the product knows how to
// serve. The name is what travels on the wire (the `range` and `period` query
// parameters); the duration is the only thing ever compared to an edition cap.
type HistoryWindow struct {
	Name     string
	Duration time.Duration
}

// HistoryWindowSpec is how one window is projected to the interface by
// GET /api/v1/edition. It carries the edition that opens the window so the
// frontend never has to hold an edition table of its own.
type HistoryWindowSpec struct {
	Window     string `json:"window"`
	Seconds    int64  `json:"seconds"`
	MinEdition string `json:"min_edition"`
}

// editionHistoryCap is how far back each edition may look. Three entries, and
// the only place a change of the commercial tiering has to touch. It is a
// duration and not a list of windows on purpose: adding a window to the product
// must not require editing this table.
var editionHistoryCap = map[Edition]time.Duration{
	Community: 7 * 24 * time.Hour,
	Personal:  30 * 24 * time.Hour,
	Pro:       90 * 24 * time.Hour,
}

// historyWindows are the windows the product serves, ordered by duration.
// Adding one here adds neither a capability nor an entry in editionHistoryCap:
// the edition that opens it is derived, never written down.
var historyWindows = []HistoryWindow{
	{Name: "1h", Duration: time.Hour},
	{Name: "6h", Duration: 6 * time.Hour},
	{Name: "24h", Duration: 24 * time.Hour},
	{Name: "7d", Duration: 7 * 24 * time.Hour},
	{Name: "30d", Duration: 30 * 24 * time.Hour},
	{Name: "90d", Duration: 90 * 24 * time.Hour},
}

// editionOrder is the ascending edition order the derivation walks.
var editionOrder = []Edition{Community, Personal, Pro}

// historyCap returns how far back e may look. An edition this binary does not
// know falls back to the Community cap rather than to zero: it keeps
// MaxHistoryWindow from returning an empty window, and it falls on the safe
// side, since an unreadable edition opens nothing that is paid for.
func historyCap(e Edition) time.Duration {
	if d, ok := editionHistoryCap[e]; ok {
		return d
	}
	return editionHistoryCap[Community]
}

// MinEditionForHistoryWindow returns the lowest edition whose cap covers w.
// Nothing writes this mapping by hand, so nothing can contradict it.
func MinEditionForHistoryWindow(w HistoryWindow) Edition {
	for _, e := range editionOrder {
		if historyCap(e) >= w.Duration {
			return e
		}
	}
	// A window past every cap is not one we hand out.
	return Pro
}

// MaxHistoryWindow returns the largest window the running edition opens. The
// catalogue always starts below the lowest cap, so this always resolves.
func MaxHistoryWindow() HistoryWindow {
	limit := historyCap(CurrentEdition())
	max := historyWindows[0]
	for _, w := range historyWindows {
		if w.Duration <= limit {
			max = w
		}
	}
	return max
}

// ResolveHistoryWindow looks a window up by name. The second result reports
// whether the product knows it at all: a window nobody declared is a bad
// request, which is a different refusal from one the edition does not open.
func ResolveHistoryWindow(name string) (HistoryWindow, bool) {
	for _, w := range historyWindows {
		if w.Name == name {
			return w, true
		}
	}
	return HistoryWindow{}, false
}

// AllowsHistoryWindow reports whether the running edition opens w, and names the
// edition that would open it when it does not.
func AllowsHistoryWindow(w HistoryWindow) (bool, Edition) {
	required := MinEditionForHistoryWindow(w)
	return w.Duration <= historyCap(CurrentEdition()), required
}

// HistoryWindowCatalog returns the whole catalogue, with the edition that opens
// each window. It is identical in every edition: it describes the product, not
// the running tier, which is what lets the interface show a closed window and
// name what would open it.
func HistoryWindowCatalog() []HistoryWindowSpec {
	out := make([]HistoryWindowSpec, 0, len(historyWindows))
	for _, w := range historyWindows {
		out = append(out, HistoryWindowSpec{
			Window:     w.Name,
			Seconds:    int64(w.Duration / time.Second),
			MinEdition: string(MinEditionForHistoryWindow(w)),
		})
	}
	return out
}

// HistoryWindowNames lists the catalogue for a refusal message, so the message
// cannot drift from what the resolver accepts.
func HistoryWindowNames() string {
	names := make([]string, 0, len(historyWindows))
	for _, w := range historyWindows {
		names = append(names, w.Name)
	}
	return strings.Join(names, ", ")
}
