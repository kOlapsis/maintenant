package updates

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
	"github.com/kolapsis/maintenant/internal/update"
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

// buildEnricher returns an enricher and a probe that runs it on one container and reports whether it recorded a CVE evaluation.
// The context is already cancelled so the ecosystem lookup never leaves the process.
func buildEnricher(t *testing.T) func() bool {
	t.Helper()
	store := newEnricherStubStore()
	e := NewEnricher(extpoint.EnricherDeps{
		Store:    store,
		Registry: update.NewRegistryClient(),
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	require.NotNil(t, e, "the enricher is wired in every edition")
	return func() bool {
		store.evaluations = map[string]*update.CVEEvaluation{}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		require.NoError(t, e.Enrich(ctx, []update.UpdateResult{{ContainerID: "c1", ContainerName: "c1", Image: "example.invalid/app"}}))
		return store.evaluations["c1"] != nil
	}
}

func TestEnricher_FollowsTheEditionAtEveryCall(t *testing.T) {
	t.Run("built in Community then Personal", func(t *testing.T) {
		setEdition(t, extension.Community)
		enriches := buildEnricher(t)
		assert.False(t, enriches(), "Community enriches nothing")

		setEdition(t, extension.Personal)
		assert.True(t, enriches(), "an edition that gains the capability enriches without a rebuild")
	})

	t.Run("built in Pro then Community", func(t *testing.T) {
		setEdition(t, extension.Pro)
		enriches := buildEnricher(t)
		assert.True(t, enriches())

		setEdition(t, extension.Community)
		assert.False(t, enriches(), "an edition that loses the capability stops enriching at once")
	})
}
