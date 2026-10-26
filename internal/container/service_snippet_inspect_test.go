package container

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type inspectLogFetcher struct {
	sinceLogFetcher
	startedAt time.Time
}

func (f *inspectLogFetcher) StartedAt(_ context.Context, _ string) (time.Time, error) {
	return f.startedAt, nil
}

func dieOfAlreadyRunning(t *testing.T, label string, fetcher *inspectLogFetcher, exitedAt time.Time) []*StateTransition {
	t.Helper()
	store := newSvcStore()
	c := makeTestContainer(extID(label), StateRunning)
	store.seed(c)
	c, err := store.GetContainerByExternalID(context.Background(), c.AgentID, c.ExternalID)
	require.NoError(t, err)

	svc := newTestService(store, func(d *Deps) { d.LogFetcher = fetcher })
	die := makeTestEvent("die", c.ExternalID)
	die.Timestamp = exitedAt
	die.ExitCode = "1"
	svc.ProcessEvent(context.Background(), die)

	transitions := store.transitionsFor(c.ID)
	require.Len(t, transitions, 1)
	return transitions
}

func TestService_ProcessEvent_DieSnippetFallsBackToTheRuntimeStart(t *testing.T) {
	exitedAt := time.Now()
	fetcher := &inspectLogFetcher{startedAt: exitedAt.Add(-time.Hour)}

	transitions := dieOfAlreadyRunning(t, "booted", fetcher, exitedAt)

	require.Len(t, fetcher.sinces, 1)
	assert.True(t, fetcher.sinces[0].Equal(fetcher.startedAt))
	assert.Equal(t, 0, fetcher.calls)
	assert.Equal(t, "since "+fetcher.startedAt.UTC().Format(time.RFC3339), transitions[0].LogSnippet)
}

func TestService_ProcessEvent_DieSnippetUnboundedWithoutRuntimeStart(t *testing.T) {
	exitedAt := time.Now()
	cases := map[string]time.Time{
		"never started":     {},
		"already restarted": exitedAt.Add(time.Second),
	}
	for name, startedAt := range cases {
		t.Run(name, func(t *testing.T) {
			fetcher := &inspectLogFetcher{sinceLogFetcher: sinceLogFetcher{mockLogFetcher: mockLogFetcher{snippet: "last 50 lines"}}, startedAt: startedAt}

			transitions := dieOfAlreadyRunning(t, "unbounded", fetcher, exitedAt)

			assert.Empty(t, fetcher.sinces)
			assert.Equal(t, 1, fetcher.calls)
			assert.Equal(t, "last 50 lines", transitions[0].LogSnippet)
		})
	}
}

func TestService_ProcessEvent_DieSnippetPrefersTheRuntimeStart(t *testing.T) {
	exitedAt := time.Now()
	fetcher := &inspectLogFetcher{startedAt: exitedAt.Add(-time.Minute)}
	store := newSvcStore()
	c := makeTestContainer(extID("stamped-late"), StateExited)
	store.seed(c)
	c, err := store.GetContainerByExternalID(context.Background(), c.AgentID, c.ExternalID)
	require.NoError(t, err)
	svc := newTestService(store, func(d *Deps) { d.LogFetcher = fetcher })

	start := makeTestEvent("start", c.ExternalID)
	start.Timestamp = fetcher.startedAt.Add(50 * time.Millisecond)
	svc.ProcessEvent(context.Background(), start)
	die := makeTestEvent("die", c.ExternalID)
	die.Timestamp = exitedAt
	die.ExitCode = "1"
	svc.ProcessEvent(context.Background(), die)

	require.Len(t, fetcher.sinces, 1)
	assert.True(t, fetcher.sinces[0].Equal(fetcher.startedAt), "since %s must be the runtime start %s, not the later event %s", fetcher.sinces[0], fetcher.startedAt, start.Timestamp)
	assert.Equal(t, 0, fetcher.calls)
	_, kept := svc.localStarts.Load(c.ExternalID)
	assert.False(t, kept, "the recorded start event is consumed by the exit")
}
