package extension

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The catalogue is ordered by duration. The order is not decoration: it carries
// the window selector and the "largest open window" fallback.
func TestHistoryWindowCatalog_IsOrderedByDuration(t *testing.T) {
	catalog := HistoryWindowCatalog()
	require.Len(t, catalog, 6)
	for i := 1; i < len(catalog); i++ {
		assert.Greater(t, catalog[i].Seconds, catalog[i-1].Seconds,
			"catalogue is out of order at %q", catalog[i].Window)
	}
	assert.Equal(t, "1h", catalog[0].Window)
	assert.Equal(t, "90d", catalog[len(catalog)-1].Window)
}

// The minimum edition of every window is derived from the three caps. This is
// the table the specification states, and nothing writes it by hand.
func TestMinEditionForHistoryWindow_IsDerivedFromTheCaps(t *testing.T) {
	want := map[string]Edition{
		"1h": Community, "6h": Community,
		"24h": Personal, "7d": Personal, "30d": Personal,
		"90d": Pro,
	}
	for name, expected := range want {
		w, ok := ResolveHistoryWindow(name)
		require.True(t, ok, "window %q is not in the catalogue", name)
		assert.Equal(t, expected, MinEditionForHistoryWindow(w), "window %q", name)
	}
}

// The catalogue projection carries the same derivation, and it is identical in
// every edition: it describes the product, not the running tier.
func TestHistoryWindowCatalog_IsTheSameInEveryEdition(t *testing.T) {
	var first []HistoryWindowSpec
	for _, e := range []Edition{Community, Personal, Pro} {
		withEdition(t, e)
		catalog := HistoryWindowCatalog()
		if first == nil {
			first = catalog
			continue
		}
		assert.Equal(t, first, catalog, "catalogue changed under edition %q", e)
	}
}

func TestMaxHistoryWindow_PerEdition(t *testing.T) {
	for edition, want := range map[Edition]string{
		Community: "6h",
		Personal:  "30d",
		Pro:       "90d",
	} {
		withEdition(t, edition)
		assert.Equal(t, want, MaxHistoryWindow().Name, "edition %q", edition)
	}
}

func TestAllowsHistoryWindow_MatchesTheCaps(t *testing.T) {
	type want struct {
		allowed  bool
		required Edition
	}
	matrix := map[Edition]map[string]want{
		Community: {
			"1h": {true, Community}, "6h": {true, Community},
			"24h": {false, Personal}, "7d": {false, Personal}, "30d": {false, Personal},
			"90d": {false, Pro},
		},
		Personal: {
			"1h": {true, Community}, "6h": {true, Community},
			"24h": {true, Personal}, "7d": {true, Personal}, "30d": {true, Personal},
			"90d": {false, Pro},
		},
		Pro: {
			"1h": {true, Community}, "6h": {true, Community},
			"24h": {true, Personal}, "7d": {true, Personal}, "30d": {true, Personal},
			"90d": {true, Pro},
		},
	}
	for edition, windows := range matrix {
		withEdition(t, edition)
		for name, expected := range windows {
			w, ok := ResolveHistoryWindow(name)
			require.True(t, ok)
			allowed, required := AllowsHistoryWindow(w)
			assert.Equal(t, expected.allowed, allowed, "%s / %s", edition, name)
			assert.Equal(t, expected.required, required, "%s / %s required edition", edition, name)
		}
	}
}

// A window nobody declared is not resolved at all. The caller turns that into a
// bad request, which is a different refusal from one the edition does not open.
func TestResolveHistoryWindow_RejectsWhatTheProductDoesNotServe(t *testing.T) {
	for _, name := range []string{"12h", "2d", "", "1H", "365d"} {
		_, ok := ResolveHistoryWindow(name)
		assert.False(t, ok, "window %q should not resolve", name)
	}
}

// An edition absent from the cap table falls back to the Community floor, never
// to zero. Unreachable from the server, which reports its own edition, but it
// is what keeps max_window from being empty in the /api/v1/edition contract.
func TestMaxHistoryWindow_UnknownEditionFallsBackToTheFloor(t *testing.T) {
	withEdition(t, Edition("enterprise-2030"))

	assert.Equal(t, "6h", MaxHistoryWindow().Name)

	paid, ok := ResolveHistoryWindow("30d")
	require.True(t, ok)
	allowed, required := AllowsHistoryWindow(paid)
	assert.False(t, allowed, "an unreadable edition opens nothing that is paid for")
	assert.Equal(t, Personal, required)
}

func TestHistoryWindowNames_ListsTheWholeCatalogue(t *testing.T) {
	assert.Equal(t, "1h, 6h, 24h, 7d, 30d, 90d", HistoryWindowNames())
}

// The caps themselves, asserted once so a change of tiering is a deliberate act
// and not a silent edit.
func TestEditionHistoryCap_IsTheAnnouncedTiering(t *testing.T) {
	assert.Equal(t, 6*time.Hour, editionHistoryCap[Community])
	assert.Equal(t, 30*24*time.Hour, editionHistoryCap[Personal])
	assert.Equal(t, 90*24*time.Hour, editionHistoryCap[Pro])
}
