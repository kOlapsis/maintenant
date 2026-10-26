// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package container

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Acceptance tests of 047-audit-small-fixes, C13: a running → exited snippet covers the last run only.

type sinceLogFetcher struct {
	mockLogFetcher
	sinces []time.Time
}

func (f *sinceLogFetcher) FetchLogSnippetSince(_ context.Context, _ string, since time.Time) (string, error) {
	f.sinces = append(f.sinces, since)
	return "since " + since.UTC().Format(time.RFC3339), nil
}

// FR-011, US4 scenario 3: ran, stopped, restarted, stopped again: the snippet starts at the last start.
func TestService_ProcessEvent_DieSnippetStartsAtTheLastStart(t *testing.T) {
	store := newSvcStore()
	c := makeTestContainer(extID("runs"), StateExited)
	c.LastStateChangeAt = time.Now().Add(-time.Hour)
	store.seed(c)
	c, err := store.GetContainerByExternalID(context.Background(), c.AgentID, c.ExternalID)
	require.NoError(t, err)

	fetcher := &sinceLogFetcher{mockLogFetcher: mockLogFetcher{snippet: "lines of every run"}}
	svc := newTestService(store, func(d *Deps) { d.LogFetcher = fetcher })

	base := time.Now().Add(-10 * time.Minute)
	runs := []struct {
		start, die time.Time
	}{
		{base, base.Add(2 * time.Minute)},
		{base.Add(3 * time.Minute), base.Add(7 * time.Minute)},
	}
	for _, run := range runs {
		start := makeTestEvent("start", c.ExternalID)
		start.Timestamp = run.start
		svc.ProcessEvent(context.Background(), start)

		die := makeTestEvent("die", c.ExternalID)
		die.Timestamp = run.die
		die.ExitCode = "1"
		svc.ProcessEvent(context.Background(), die)
	}

	transitions := store.transitionsFor(c.ID)
	require.Len(t, transitions, 4)
	require.Len(t, fetcher.sinces, 2, "every exit of a container the runtime started is bounded to that start")
	assert.Equal(t, 0, fetcher.calls, "the unbounded snippet must not be used when the start is known")
	for i, run := range runs {
		exit := transitions[2*i+1]
		assert.Equal(t, StateRunning, exit.PreviousState)
		assert.Equal(t, StateExited, exit.NewState)
		assert.Equal(t, "since "+fetcher.sinces[i].UTC().Format(time.RFC3339), exit.LogSnippet)
		assert.False(t, fetcher.sinces[i].Before(run.start), "run %d: since %s is before the start %s", i, fetcher.sinces[i], run.start)
		assert.False(t, fetcher.sinces[i].After(run.die), "run %d: since %s is after the exit %s", i, fetcher.sinces[i], run.die)
	}
}

// FR-011, edge case "conteneur sans heure de démarrage connue": a runtime without start times keeps today's snippet.
func TestService_ProcessEvent_DieSnippetWithoutAStartTimeIsUnchanged(t *testing.T) {
	store := newSvcStore()
	c := makeTestContainer(extID("nostart"), StateRunning)
	store.seed(c)
	c, err := store.GetContainerByExternalID(context.Background(), c.AgentID, c.ExternalID)
	require.NoError(t, err)

	fetcher := &mockLogFetcher{snippet: "last 50 lines"}
	svc := newTestService(store, func(d *Deps) { d.LogFetcher = fetcher })

	die := makeTestEvent("die", c.ExternalID)
	die.ExitCode = "1"
	svc.ProcessEvent(context.Background(), die)

	transitions := store.transitionsFor(c.ID)
	require.Len(t, transitions, 1)
	assert.Equal(t, "last 50 lines", transitions[0].LogSnippet)
	assert.Equal(t, 1, fetcher.calls)
}
