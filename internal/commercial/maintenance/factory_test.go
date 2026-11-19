package maintenance

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/commercial/tiers"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/extpoint"
)

func TestMain(m *testing.M) {
	extension.Register(tiers.Policy{}, nil)
	os.Exit(m.Run())
}

func setEdition(t *testing.T, e extension.Edition) {
	t.Helper()
	prev := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return e }
	t.Cleanup(func() { extension.CurrentEdition = prev })
}

func buildSuppressor(t *testing.T, windows Store) func() bool {
	t.Helper()
	s := NewMaintenanceSuppressor(extpoint.SuppressorDeps{
		Windows: windows,
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	require.NotNil(t, s, "the suppressor is wired in every edition")
	return func() bool {
		got, err := s.IsSuppressed(context.Background(), "test", "container", "c1")
		require.NoError(t, err)
		return got
	}
}

func TestSuppressor_FollowsTheEditionAtEveryCall(t *testing.T) {
	for _, edition := range []extension.Edition{extension.Community, extension.Personal} {
		t.Run("built in "+string(edition)+" then Pro", func(t *testing.T) {
			setEdition(t, edition)
			isSuppressed := buildSuppressor(t, &mockStore{matched: true, windowID: "w1"})
			assert.False(t, isSuppressed(), "an edition without maintenance windows suppresses nothing")

			setEdition(t, extension.Pro)
			assert.True(t, isSuppressed(), "an edition that gains the capability suppresses without a rebuild")
		})
	}

	t.Run("built in Pro then Community", func(t *testing.T) {
		setEdition(t, extension.Pro)
		isSuppressed := buildSuppressor(t, &mockStore{matched: true, windowID: "w1"})
		assert.True(t, isSuppressed())

		setEdition(t, extension.Community)
		assert.False(t, isSuppressed(), "an edition that loses the capability stops suppressing at once")
	})
}

func TestSuppressor_NoStoreQueryWithoutTheCapability(t *testing.T) {
	setEdition(t, extension.Community)
	store := &mockStore{matched: true}
	buildSuppressor(t, store)()
	assert.Zero(t, store.calls)
}
