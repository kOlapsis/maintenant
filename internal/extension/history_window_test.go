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

func TestResolveHistoryWindow_RejectsWhatTheProductDoesNotServe(t *testing.T) {
	for _, name := range []string{"12h", "2d", "", "1H", "365d"} {
		_, ok := ResolveHistoryWindow(name)
		assert.False(t, ok, "window %q should not resolve", name)
	}
}

// An edition absent from the cap table falls back to the Community floor, never
// to zero. Unreachable from the server, which reports its own edition, but it
// is what keeps max_window from being empty in the /api/v1/edition contract.
func TestHistoryWindowNames_ListsTheWholeCatalogue(t *testing.T) {
	assert.Equal(t, "1h, 6h, 24h, 7d, 30d, 90d", HistoryWindowNames())
}

// The caps themselves, asserted once so a change of tiering is a deliberate act
// and not a silent edit.

func TestHistoryWindows_DefaultPolicyOpensSevenDays(t *testing.T) {
	for _, e := range []Edition{Community, Personal, Pro} {
		withEdition(t, e)
		assert.Equal(t, "7d", MaxHistoryWindow().Name, "edition %q", e)
	}

	for name, want := range map[string]Edition{"7d": Community, "30d": Pro, "90d": Pro} {
		w, ok := ResolveHistoryWindow(name)
		require.True(t, ok)
		assert.Equal(t, want, MinEditionForHistoryWindow(w), "window %q", name)
	}
}

func TestHistoryWindows_DelegateToThePolicy(t *testing.T) {
	withPolicy(t, fakePolicy{history: map[Edition]time.Duration{
		Community: 24 * time.Hour,
		Personal:  7 * 24 * time.Hour,
		Pro:       90 * 24 * time.Hour,
	}})

	withEdition(t, Personal)
	assert.Equal(t, "7d", MaxHistoryWindow().Name)

	w, ok := ResolveHistoryWindow("30d")
	require.True(t, ok)
	allowed, required := AllowsHistoryWindow(w)
	assert.False(t, allowed)
	assert.Equal(t, Pro, required)

	w, ok = ResolveHistoryWindow("7d")
	require.True(t, ok)
	assert.Equal(t, Personal, MinEditionForHistoryWindow(w))
}
